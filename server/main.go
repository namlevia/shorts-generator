package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config holds runtime configuration
type Config struct {
	Port              string
	TelegramToken     string
	TelegramAdminChat int64
	ProjectDir        string
	MaxConcurrent     int
}

// Job represents a video generation request
type Job struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Status    string    `json:"status"` // queued, processing, completed, failed
	Progress  string    `json:"progress,omitempty"`
	VideoPath string    `json:"video_path,omitempty"`
	OutputDir string    `json:"output_dir,omitempty"`
	Caption   string    `json:"caption,omitempty"`
	Error     string    `json:"error,omitempty"`
	ChatID    int64     `json:"chat_id,omitempty"` // for Telegram
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Server struct {
	cfg        Config
	jobs       map[string]*Job
	jobsMu     sync.RWMutex
	jobQueue   chan *Job
	httpServer *http.Server
}

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}
}

func main() {
	// Auto load .env from current and parent directory
	loadEnvFile(".env")
	loadEnvFile("../.env")
	loadEnvFile("../.env.local")

	// Determine project root directory (where package.json is located)
	cwd, _ := os.Getwd()
	projectDir := cwd
	if _, err := os.Stat(filepath.Join(cwd, "package.json")); err != nil {
		// check parent
		parent := filepath.Dir(cwd)
		if _, err := os.Stat(filepath.Join(parent, "package.json")); err == nil {
			projectDir = parent
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	adminChat, _ := strconv.ParseInt(os.Getenv("TELEGRAM_ADMIN_CHAT_ID"), 10, 64)

	cfg := Config{
		Port:              port,
		TelegramToken:     os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramAdminChat: adminChat,
		ProjectDir:        projectDir,
		MaxConcurrent:     1, // 1 concurrent job by default (ideal for Raspberry Pi 5)
	}

	s := &Server{
		cfg:      cfg,
		jobs:     make(map[string]*Job),
		jobQueue: make(chan *Job, 100),
	}

	log.Printf("==================================================")
	log.Printf("🚀 Shorts Generator Server (Golang)")
	log.Printf("📁 Project Root : %s", cfg.ProjectDir)
	log.Printf("🔌 HTTP Server  : http://localhost:%s", cfg.Port)
	if cfg.TelegramToken != "" {
		log.Printf("🤖 Telegram Bot : Enabled (polling active)")
	} else {
		log.Printf("🤖 Telegram Bot : Disabled (set TELEGRAM_BOT_TOKEN to enable)")
	}
	log.Printf("⚡ Max Concurrency: %d job(s)", cfg.MaxConcurrent)
	log.Printf("==================================================")

	// Start queue worker
	go s.worker()

	// Start Telegram poller if token provided
	if cfg.TelegramToken != "" {
		go s.startTelegramPoller()
	}

	// Setup HTTP router
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/jobs", s.handleJobs)
	mux.HandleFunc("/api/jobs/", s.handleJobDetail)

	s.httpServer = &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

// ── TASK QUEUE WORKER ───────────────────────────────────────────────────────

func (s *Server) worker() {
	for job := range s.jobQueue {
		s.processJob(job)
	}
}

func (s *Server) processJob(job *Job) {
	s.updateJob(job.ID, func(j *Job) {
		j.Status = "processing"
		j.Progress = "Bắt đầu cào nội dung và tạo kịch bản..."
		j.UpdatedAt = time.Now()
	})

	if job.ChatID != 0 {
		s.sendTelegramMessage(job.ChatID, fmt.Sprintf("🎬 [Job %s] Đang xử lý:\n🔗 %s\n⏳ Đang cào nội dung và sinh kịch bản...", job.ID, job.URL))
	}

	log.Printf("[Job %s] Executing pipeline for: %s", job.ID, job.URL)

	// Command: npx tsx src/make.ts <URL>
	var cmd *exec.Cmd
	if isWindows() {
		cmd = exec.Command("cmd", "/c", "npx", "tsx", "src/make.ts", job.URL)
	} else {
		cmd = exec.Command("npx", "tsx", "src/make.ts", job.URL)
	}
	cmd.Dir = s.cfg.ProjectDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.failJob(job, fmt.Sprintf("failed to get stdout pipe: %v", err))
		return
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		s.failJob(job, fmt.Sprintf("failed to start pipeline process: %v", err))
		return
	}

	// Parse progress line by line
	scanner := bufio.NewScanner(stdout)
	outputDir := ""
	videoPath := ""

	outputDirRegex := regexp.MustCompile(`\[Make\]\s+Output dir:\s+(.+)`)
	doneRegex := regexp.MustCompile(`\[Make\]\s+Done!\s+Video:\s+(.+)`)

	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("[Job %s] %s", job.ID, line)

		if matches := outputDirRegex.FindStringSubmatch(line); len(matches) > 1 {
			outputDir = strings.TrimSpace(matches[1])
		}
		if matches := doneRegex.FindStringSubmatch(line); len(matches) > 1 {
			videoPath = strings.TrimSpace(matches[1])
		}

		// Friendly status updates
		if strings.Contains(line, "Calling LLM") {
			s.updateJob(job.ID, func(j *Job) { j.Progress = "AI đang viết kịch bản video..."; j.UpdatedAt = time.Now() })
		} else if strings.Contains(line, "TTS audio") {
			s.updateJob(job.ID, func(j *Job) { j.Progress = "Đang lồng tiếng AI..."; j.UpdatedAt = time.Now() })
		} else if strings.Contains(line, "HyperFrames render") {
			s.updateJob(job.ID, func(j *Job) { j.Progress = "Đang render đồ họa chuyển động 1080x1920..."; j.UpdatedAt = time.Now() })
			if job.ChatID != 0 {
				s.sendTelegramMessage(job.ChatID, fmt.Sprintf("⚡ [Job %s] Đã xong kịch bản & giọng đọc! Đang tiến hành render video...", job.ID))
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		s.failJob(job, fmt.Sprintf("Pipeline process failed: %v", err))
		return
	}

	// Read caption if available
	caption := ""
	if outputDir != "" {
		captionBytes, err := os.ReadFile(filepath.Join(outputDir, "caption.txt"))
		if err == nil {
			caption = string(captionBytes)
		}
	}

	s.updateJob(job.ID, func(j *Job) {
		j.Status = "completed"
		j.Progress = "Hoàn thành 100%"
		j.OutputDir = outputDir
		j.VideoPath = videoPath
		j.Caption = caption
		j.UpdatedAt = time.Now()
	})

	log.Printf("[Job %s] COMPLETED: %s", job.ID, videoPath)

	if job.ChatID != 0 && videoPath != "" {
		s.sendTelegramVideo(job.ChatID, videoPath, fmt.Sprintf("🎉 Video đã tạo thành công!\n\n%s", caption))
	}
}

func (s *Server) failJob(job *Job, errMsg string) {
	s.updateJob(job.ID, func(j *Job) {
		j.Status = "failed"
		j.Error = errMsg
		j.UpdatedAt = time.Now()
	})
	log.Printf("[Job %s] FAILED: %s", job.ID, errMsg)
	if job.ChatID != 0 {
		s.sendTelegramMessage(job.ChatID, fmt.Sprintf("❌ [Job %s] Tạo video thất bại:\n%s", job.ID, errMsg))
	}
}

func (s *Server) updateJob(id string, fn func(j *Job)) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if job, ok := s.jobs[id]; ok {
		fn(job)
	}
}

// ── HTTP API HANDLERS ──────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.jobsMu.RLock()
	totalJobs := len(s.jobs)
	s.jobsMu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"version":     "1.0.0",
		"service":     "shorts-generator-server",
		"total_jobs":  totalJobs,
		"queue_len":   len(s.jobQueue),
		"project_dir": s.cfg.ProjectDir,
	})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		s.jobsMu.RLock()
		list := make([]*Job, 0, len(s.jobs))
		for _, j := range s.jobs {
			list = append(list, j)
		}
		s.jobsMu.RUnlock()
		json.NewEncoder(w).Encode(list)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
			http.Error(w, `{"error":"missing url"}`, http.StatusBadRequest)
			return
		}

		jobID := fmt.Sprintf("%d", time.Now().UnixMilli())
		job := &Job{
			ID:        jobID,
			URL:       req.URL,
			Status:    "queued",
			Progress:  "Đang chờ xếp hàng...",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		s.jobsMu.Lock()
		s.jobs[jobID] = job
		s.jobsMu.Unlock()

		s.jobQueue <- job

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(job)
		return
	}

	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

func (s *Server) handleJobDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if strings.HasSuffix(id, "/video") {
		jobID := strings.TrimSuffix(id, "/video")
		s.jobsMu.RLock()
		job, ok := s.jobs[jobID]
		s.jobsMu.RUnlock()
		if !ok || job.VideoPath == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, job.VideoPath)
		return
	}

	s.jobsMu.RLock()
	job, ok := s.jobs[id]
	s.jobsMu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// ── TELEGRAM BOT (POLLING) ─────────────────────────────────────────────────

type tgUpdate struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		MessageID int   `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

func (s *Server) startTelegramPoller() {
	offset := 0
	client := &http.Client{Timeout: 35 * time.Second}

	for {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", s.cfg.TelegramToken, offset)
		resp, err := client.Get(reqURL)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		var data struct {
			Ok     bool       `json:"ok"`
			Result []tgUpdate `json:"result"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && data.Ok {
			for _, u := range data.Result {
				offset = u.UpdateID + 1
				if u.Message == nil || u.Message.Text == "" {
					continue
				}

				chatID := u.Message.Chat.ID
				text := strings.TrimSpace(u.Message.Text)

				if text == "/start" || text == "/help" {
					welcome := "👋 Chào bạn! Tôi là **LeviaTech Shorts Generator Bot**.\n\n" +
						"⚡ Gửi cho tôi bất kỳ đường link nào (GitHub repository hoặc bài viết tin tức),\n" +
						"tôi sẽ tự động dùng AI viết kịch bản, lồng tiếng và render thành video ngắn 9:16 Full HD cho bạn!\n\n" +
						"Ví dụ: `https://github.com/dabeecao/telecloud-go`"
					s.sendTelegramMessage(chatID, welcome)
					continue
				}

				// Check if it looks like a URL
				parsed, err := url.ParseRequestURI(text)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
					s.sendTelegramMessage(chatID, "⚠️ Vui lòng gửi một đường link hợp lệ (bắt đầu bằng http:// hoặc https://).")
					continue
				}

				// Create & enqueue job
				jobID := fmt.Sprintf("%d", time.Now().UnixMilli())
				job := &Job{
					ID:        jobID,
					URL:       text,
					Status:    "queued",
					Progress:  "Đã tiếp nhận yêu cầu, đang xếp hàng xử lý...",
					ChatID:    chatID,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}

				s.jobsMu.Lock()
				s.jobs[jobID] = job
				s.jobsMu.Unlock()

				s.sendTelegramMessage(chatID, fmt.Sprintf("📥 Đã nhận yêu cầu tạo video [Mã: %s]!\nĐang xếp hàng thực thi...", jobID))
				s.jobQueue <- job
			}
		}
		resp.Body.Close()
	}
}

func (s *Server) sendTelegramMessage(chatID int64, text string) {
	if s.cfg.TelegramToken == "" {
		return
	}
	body, _ := json.Marshal(map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	})
	http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.cfg.TelegramToken),
		"application/json",
		bytes.NewReader(body),
	)
}

func (s *Server) sendTelegramVideo(chatID int64, videoPath, caption string) {
	if s.cfg.TelegramToken == "" {
		return
	}

	file, err := os.Open(videoPath)
	if err != nil {
		s.sendTelegramMessage(chatID, fmt.Sprintf("⚠️ Không thể mở video: %v", err))
		return
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return
	}

	// If file is > 48MB (Telegram Bot API limit is 50MB for upload)
	if fi.Size() > 48*1024*1024 {
		s.sendTelegramMessage(chatID, fmt.Sprintf("🎉 Video đã tạo thành công! (Dung lượng: %.1f MB vượt ngưỡng 50MB upload của Telegram Bot API)\n📁 Đường dẫn file trên server: `%s`\n\n%s", float64(fi.Size())/(1024*1024), videoPath, caption))
		return
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	_ = writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	_ = writer.WriteField("caption", caption)

	part, err := writer.CreateFormFile("video", filepath.Base(videoPath))
	if err != nil {
		return
	}
	_, _ = io.Copy(part, file)
	_ = writer.Close()

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		fmt.Sprintf("https://api.telegram.org/bot%s/sendVideo", s.cfg.TelegramToken),
		body,
	)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		s.sendTelegramMessage(chatID, fmt.Sprintf("⚠️ Gửi video qua Telegram thất bại: %v\n📁 File đã lưu tại: `%s`", err, videoPath))
		return
	}
	defer resp.Body.Close()
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}
