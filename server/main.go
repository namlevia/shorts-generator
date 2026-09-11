package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var webFS embed.FS

// Config holds runtime configuration
type Config struct {
	Port              string `json:"port"`
	TelegramToken     string `json:"telegram_token"`
	TelegramAdminChat int64  `json:"telegram_admin_chat"`
	ProjectDir        string `json:"project_dir"`
	MaxConcurrent     int    `json:"max_concurrent"`
	LLMBaseURL        string `json:"llm_base_url"`
	LLMAPIKey         string `json:"llm_api_key"`
	LLMModel          string `json:"llm_model"`
	ChannelName       string `json:"channel_name"`
	TiktokHandle      string `json:"tiktok_handle"`
	TTSProvider       string `json:"tts_provider"`
	RenderEngine      string `json:"render_engine"`   // "local" or "github"
	GitHubRepo        string `json:"github_repo"`     // "namlevia/shorts-generator"
	GitHubToken       string `json:"github_token"`    // Personal Access Token
	GitHubWorkflow    string `json:"github_workflow"` // "render.yml"
}

// ChannelProfile defines brand identity and presets for a specific channel
type ChannelProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Handle    string `json:"handle"`
	Avatar    string `json:"avatar,omitempty"` // URL or /assets/ path
	Voice     string `json:"voice,omitempty"`
	Theme     string `json:"theme,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

// Job represents a video generation request
type Job struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Theme     string    `json:"theme,omitempty"`
	Voice     string    `json:"voice,omitempty"`
	Channel   string    `json:"channel,omitempty"`
	Handle    string    `json:"handle,omitempty"`
	Avatar    string    `json:"avatar,omitempty"`
	Engine    string    `json:"engine,omitempty"` // "local" or "github"
	Status    string    `json:"status"`           // queued, processing, completed, failed
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
	cfgMu      sync.RWMutex
	jobs       map[string]*Job
	jobsMu     sync.RWMutex
	channels   map[string]*ChannelProfile
	channelsMu sync.RWMutex
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
	// Auto load .env from current, project, and parent directory
	loadEnvFile(".env")
	loadEnvFile("../.env")
	loadEnvFile("../.env.local")

	// Determine project root directory (where package.json is located)
	cwd, _ := os.Getwd()
	projectDir := cwd
	if _, err := os.Stat(filepath.Join(cwd, "package.json")); err != nil {
		parent := filepath.Dir(cwd)
		if _, err := os.Stat(filepath.Join(parent, "package.json")); err == nil {
			projectDir = parent
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "2024" // Default port 2024 requested by user
	}

	adminChat, _ := strconv.ParseInt(os.Getenv("TELEGRAM_ADMIN_CHAT_ID"), 10, 64)

	cfg := Config{
		Port:              port,
		TelegramToken:     os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramAdminChat: adminChat,
		ProjectDir:        projectDir,
		MaxConcurrent:     1, // 1 concurrent job by default (ideal for Pi 5)
		LLMBaseURL:        os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:         os.Getenv("LLM_API_KEY"),
		LLMModel:          os.Getenv("LLM_MODEL"),
		ChannelName:       os.Getenv("CHANNEL_NAME"),
		TiktokHandle:      os.Getenv("TIKTOK_HANDLE"),
		TTSProvider:       os.Getenv("TTS_PROVIDER"),
		RenderEngine:      os.Getenv("RENDER_ENGINE"),
		GitHubRepo:        os.Getenv("GITHUB_REPO"),
		GitHubToken:       os.Getenv("GITHUB_TOKEN"),
		GitHubWorkflow:    os.Getenv("GITHUB_WORKFLOW"),
	}
	if cfg.LLMModel == "" {
		cfg.LLMModel = "gemini-3.8-flash"
	}
	if cfg.ChannelName == "" {
		cfg.ChannelName = "LeviaTech"
	}
	if cfg.TiktokHandle == "" {
		cfg.TiktokHandle = "@leviatech"
	}
	if cfg.TTSProvider == "" {
		cfg.TTSProvider = "edge-tts"
	}
	if cfg.RenderEngine == "" {
		cfg.RenderEngine = "local"
	}
	if cfg.GitHubRepo == "" {
		cfg.GitHubRepo = "namlevia/shorts-generator"
	}
	if cfg.GitHubWorkflow == "" {
		cfg.GitHubWorkflow = "render.yml"
	}

	s := &Server{
		cfg:        cfg,
		jobs:       make(map[string]*Job),
		channels:   make(map[string]*ChannelProfile),
		jobQueue:   make(chan *Job, 100),
	}

	// Load channel profiles from server/channels.json
	s.loadChannels()

	// Scan historical completed videos from output/
	s.scanExistingOutputs()

	log.Printf("==================================================")
	log.Printf("🚀 Shorts Generator Studio (Golang Server)")
	log.Printf("📁 Project Root : %s", cfg.ProjectDir)
	log.Printf("🔌 WebUI & API  : http://localhost:%s", cfg.Port)
	log.Printf("⚙️ Render Engine : %s (default)", cfg.RenderEngine)
	if cfg.GitHubToken != "" {
		log.Printf("☁️ GitHub Actions: Configured (%s)", cfg.GitHubRepo)
	} else {
		log.Printf("☁️ GitHub Actions: Token not set (configure in Settings)")
	}
	if cfg.TelegramToken != "" {
		log.Printf("🤖 Telegram Bot : Enabled (polling active)")
	} else {
		log.Printf("🤖 Telegram Bot : Disabled (configure token in WebUI or .env)")
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

	// WebUI root & static
	mux.HandleFunc("/", s.handleWebUI)

	// Serve assets (avatars, logos)
	assetsDir := filepath.Join(projectDir, "assets")
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))

	// REST API
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/channels", s.handleChannels)
	mux.HandleFunc("/api/channels/", s.handleChannelDetail)
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

// ── HISTORICAL JOBS SCANNER ────────────────────────────────────────────────

func (s *Server) scanExistingOutputs() {
	outDir := filepath.Join(s.cfg.ProjectDir, "output")
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		itemDir := filepath.Join(outDir, entry.Name())
		videoPath := filepath.Join(itemDir, "video.mp4")
		scriptPath := filepath.Join(itemDir, "script.json")

		if _, err := os.Stat(videoPath); err == nil {
			info, _ := entry.Info()
			modTime := time.Now()
			if info != nil {
				modTime = info.ModTime()
			}

			// Read title & url from script.json
			jobURL := entry.Name()
			jobChannel := ""
			caption := ""

			if scriptBytes, err := os.ReadFile(scriptPath); err == nil {
				var parsed struct {
					Metadata struct {
						Title   string `json:"title"`
						Channel string `json:"channel"`
						Source  struct {
							URL string `json:"url"`
						} `json:"source"`
					} `json:"metadata"`
				}
				if json.Unmarshal(scriptBytes, &parsed) == nil {
					if parsed.Metadata.Source.URL != "" {
						jobURL = parsed.Metadata.Source.URL
					} else if parsed.Metadata.Title != "" {
						jobURL = parsed.Metadata.Title
					}
					if parsed.Metadata.Channel != "" {
						jobChannel = parsed.Metadata.Channel
					}
				}
			}

			if capBytes, err := os.ReadFile(filepath.Join(itemDir, "caption.txt")); err == nil {
				caption = string(capBytes)
			}

			jobID := entry.Name()
			s.jobs[jobID] = &Job{
				ID:        jobID,
				URL:       jobURL,
				Channel:   jobChannel,
				Status:    "completed",
				Progress:  "Đã hoàn thành",
				VideoPath: videoPath,
				OutputDir: itemDir,
				Caption:   caption,
				CreatedAt: modTime,
				UpdatedAt: modTime,
			}
		}
	}
}

// ── WEBUI HANDLER ──────────────────────────────────────────────────────────

func (s *Server) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var indexHTML []byte
	var err error

	// Try reading from disk first for instant hot-reload during development
	diskPath := filepath.Join(s.cfg.ProjectDir, "server", "web", "index.html")
	if bytes, diskErr := os.ReadFile(diskPath); diskErr == nil {
		indexHTML = bytes
	} else {
		indexHTML, err = webFS.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "WebUI not found", http.StatusInternalServerError)
			return
		}
	}

	// Dynamic replacement of port
	rendered := strings.ReplaceAll(string(indexHTML), ":2024", ":"+s.cfg.Port)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(rendered))
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
		j.Progress = "Bắt đầu xử lý..."
		j.UpdatedAt = time.Now()
	})

	s.cfgMu.RLock()
	defaultEngine := s.cfg.RenderEngine
	s.cfgMu.RUnlock()

	engine := job.Engine
	if engine == "" || engine == "auto" {
		engine = defaultEngine
	}
	if engine == "" {
		engine = "local"
	}

	if engine == "github" {
		s.processGitHubJob(job)
	} else {
		s.processLocalJob(job)
	}
}

func (s *Server) processLocalJob(job *Job) {
	s.updateJob(job.ID, func(j *Job) {
		j.Progress = "Bắt đầu cào nội dung và tạo kịch bản..."
		j.UpdatedAt = time.Now()
	})

	if job.ChatID != 0 {
		s.sendTelegramMessage(job.ChatID, fmt.Sprintf("🎬 [Job %s] Đang xử lý (Local):\n🔗 %s\n⏳ Đang cào nội dung và sinh kịch bản...", job.ID, job.URL))
	}

	log.Printf("[Job %s] Executing local pipeline for: %s", job.ID, job.URL)

	// Command: npx tsx src/make.ts <URL>
	var cmd *exec.Cmd
	if isWindows() {
		cmd = exec.Command("cmd", "/c", "npx", "tsx", "src/make.ts", job.URL)
	} else {
		cmd = exec.Command("npx", "tsx", "src/make.ts", job.URL)
	}
	cmd.Dir = s.cfg.ProjectDir

	// Forward custom job options via Environment Variables
	cmd.Env = os.Environ()
	if job.Theme != "" {
		cmd.Env = append(cmd.Env, "VIDEO_THEME="+job.Theme)
	}
	if job.Voice != "" {
		cmd.Env = append(cmd.Env, "EDGE_TTS_VOICE="+job.Voice)
	}
	if job.Channel != "" {
		cmd.Env = append(cmd.Env, "CHANNEL_NAME="+job.Channel)
	}
	if job.Handle != "" {
		cmd.Env = append(cmd.Env, "TIKTOK_HANDLE="+job.Handle)
	}
	if job.Avatar != "" {
		if strings.HasPrefix(job.Avatar, "/") {
			avatarRel := strings.TrimPrefix(job.Avatar, "/")
			avatarAbs := filepath.Join(s.cfg.ProjectDir, filepath.FromSlash(avatarRel))
			cmd.Env = append(cmd.Env, "TIKTOK_AVATAR_PATH="+avatarAbs)
		} else if strings.HasPrefix(job.Avatar, "http://") || strings.HasPrefix(job.Avatar, "https://") {
			cmd.Env = append(cmd.Env, "TIKTOK_AVATAR_URL="+job.Avatar)
		} else {
			cmd.Env = append(cmd.Env, "TIKTOK_AVATAR_PATH="+job.Avatar)
		}
	}

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

	outputDirRegex := regexp.MustCompile(`(?:\[Make\]\s+Output dir:|Thư mục output:)\s*(.+)`)
	doneRegex := regexp.MustCompile(`(?:\[Make\]\s+Done!\s+Video:|Video:)\s*(.+)`)

	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("[Job %s] %s", job.ID, line)

		if matches := outputDirRegex.FindStringSubmatch(line); len(matches) > 1 {
			rawDir := strings.TrimSpace(matches[1])
			if !filepath.IsAbs(rawDir) {
				outputDir = filepath.Join(s.cfg.ProjectDir, rawDir)
			} else {
				outputDir = rawDir
			}
		}
		if matches := doneRegex.FindStringSubmatch(line); len(matches) > 1 {
			rawVid := strings.TrimSpace(matches[1])
			if !filepath.IsAbs(rawVid) {
				videoPath = filepath.Join(s.cfg.ProjectDir, rawVid)
			} else {
				videoPath = rawVid
			}
		}

		// Friendly status updates for WebUI
		if strings.Contains(line, "Calling LLM") || strings.Contains(line, "biên kịch") {
			s.updateJob(job.ID, func(j *Job) { j.Progress = "AI đang viết kịch bản video..."; j.UpdatedAt = time.Now() })
		} else if strings.Contains(line, "TTS audio") || strings.Contains(line, "TTS scene") || strings.Contains(line, "lồng tiếng") {
			s.updateJob(job.ID, func(j *Job) { j.Progress = "Đang lồng tiếng AI tiếng Việt..."; j.UpdatedAt = time.Now() })
		} else if strings.Contains(line, "Render with hyperframes") || strings.Contains(line, "Rendering") || strings.Contains(line, "render video") {
			s.updateJob(job.ID, func(j *Job) { j.Progress = "Đang render đồ họa chuyển động 1080x1920..."; j.UpdatedAt = time.Now() })
			if job.ChatID != 0 {
				s.sendTelegramMessage(job.ChatID, fmt.Sprintf("⚡ [Job %s] Đã xong kịch bản & giọng đọc! Đang render video...", job.ID))
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		s.failJob(job, fmt.Sprintf("Pipeline process failed: %v", err))
		return
	}

	// Fallback detection if regex missed outputDir or videoPath
	if videoPath == "" || outputDir == "" {
		allOutDir := filepath.Join(s.cfg.ProjectDir, "output")
		if entries, err := os.ReadDir(allOutDir); err == nil {
			var newestDir string
			var newestTime time.Time
			for _, entry := range entries {
				if entry.IsDir() {
					fullDir := filepath.Join(allOutDir, entry.Name())
					vPath := filepath.Join(fullDir, "video.mp4")
					if fi, err := os.Stat(vPath); err == nil {
						if fi.ModTime().After(newestTime) {
							newestTime = fi.ModTime()
							newestDir = fullDir
						}
					}
				}
			}
			if newestDir != "" {
				outputDir = newestDir
				videoPath = filepath.Join(newestDir, "video.mp4")
			}
		}
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

func (s *Server) processGitHubJob(job *Job) {
	s.cfgMu.RLock()
	token := s.cfg.GitHubToken
	repo := s.cfg.GitHubRepo
	workflow := s.cfg.GitHubWorkflow
	s.cfgMu.RUnlock()

	if repo == "" {
		repo = "namlevia/shorts-generator"
	}
	if workflow == "" {
		workflow = "render.yml"
	}

	if token == "" {
		s.failJob(job, "Chưa cấu hình GitHub Personal Access Token (PAT). Vui lòng vào Cài Đặt để cập nhật token.")
		return
	}

	s.updateJob(job.ID, func(j *Job) {
		j.Progress = "Đang gửi lệnh kích hoạt sang GitHub Actions..."
		j.UpdatedAt = time.Now()
	})

	theme := job.Theme
	if theme == "" {
		theme = "dark-neon"
	}
	voice := job.Voice
	if voice == "" {
		voice = "vi-VN-NamMinhNeural"
	}

	// 1. Sinh kịch bản & caption tại Local (Raspberry Pi / PC) bằng local LLM
	s.updateJob(job.ID, func(j *Job) {
		j.Progress = "AI đang bóc tách nội dung & tạo kịch bản tại Local..."
		j.UpdatedAt = time.Now()
	})

	if job.ChatID != 0 {
		s.sendTelegramMessage(job.ChatID, fmt.Sprintf("🎬 [Job %s] Đang tạo kịch bản tại máy local...\n🔗 %s", job.ID, job.URL))
	}

	log.Printf("[Job %s] Generating script.json locally for: %s", job.ID, job.URL)

	var dryCmd *exec.Cmd
	if isWindows() {
		dryCmd = exec.Command("cmd", "/c", "npx", "tsx", "src/make.ts", job.URL, "--dry-run")
	} else {
		dryCmd = exec.Command("npx", "tsx", "src/make.ts", job.URL, "--dry-run")
	}
	dryCmd.Dir = s.cfg.ProjectDir
	dryCmd.Env = os.Environ()
	if job.Theme != "" {
		dryCmd.Env = append(dryCmd.Env, "VIDEO_THEME="+job.Theme)
	}
	if job.Voice != "" {
		dryCmd.Env = append(dryCmd.Env, "EDGE_TTS_VOICE="+job.Voice)
	}
	if job.Channel != "" {
		dryCmd.Env = append(dryCmd.Env, "CHANNEL_NAME="+job.Channel)
	}
	if job.Handle != "" {
		dryCmd.Env = append(dryCmd.Env, "TIKTOK_HANDLE="+job.Handle)
	}
	if job.Avatar != "" {
		if strings.HasPrefix(job.Avatar, "/") {
			avatarRel := strings.TrimPrefix(job.Avatar, "/")
			avatarAbs := filepath.Join(s.cfg.ProjectDir, filepath.FromSlash(avatarRel))
			dryCmd.Env = append(dryCmd.Env, "TIKTOK_AVATAR_PATH="+avatarAbs)
		} else if strings.HasPrefix(job.Avatar, "http://") || strings.HasPrefix(job.Avatar, "https://") {
			dryCmd.Env = append(dryCmd.Env, "TIKTOK_AVATAR_URL="+job.Avatar)
		} else {
			dryCmd.Env = append(dryCmd.Env, "TIKTOK_AVATAR_PATH="+job.Avatar)
		}
	}

	stdout, err := dryCmd.StdoutPipe()
	if err != nil {
		s.failJob(job, fmt.Sprintf("Lỗi stdout pipe khi tạo kịch bản local: %v", err))
		return
	}
	dryCmd.Stderr = dryCmd.Stdout

	if err := dryCmd.Start(); err != nil {
		s.failJob(job, fmt.Sprintf("Lỗi khởi chạy tạo kịch bản local: %v", err))
		return
	}

	scriptPath := ""
	outputDir := ""
	scriptRegex := regexp.MustCompile(`\[Make\]\s+Script:\s*(.+)`)
	outDirRegex := regexp.MustCompile(`\[Make\]\s+Output dir:\s*(.+)`)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("[Job %s DryRun] %s", job.ID, line)
		if m := scriptRegex.FindStringSubmatch(line); len(m) > 1 {
			scriptPath = strings.TrimSpace(m[1])
		}
		if m := outDirRegex.FindStringSubmatch(line); len(m) > 1 {
			outputDir = strings.TrimSpace(m[1])
		}
	}

	if err := dryCmd.Wait(); err != nil {
		s.failJob(job, fmt.Sprintf("Quá trình tạo kịch bản local thất bại: %v", err))
		return
	}

	if scriptPath == "" {
		s.failJob(job, "Không tìm thấy file kịch bản script.json được tạo ra")
		return
	}

	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		s.failJob(job, fmt.Sprintf("Không thể đọc file kịch bản: %v", err))
		return
	}

	captionBytes, _ := os.ReadFile(filepath.Join(outputDir, "caption.txt"))
	localCaption := string(captionBytes)

	// 2. Gửi kịch bản sang GitHub Actions để render thuần đồ họa + âm thanh
	s.updateJob(job.ID, func(j *Job) {
		j.Progress = "Đã có kịch bản! Đang gửi sang GitHub Actions để render video..."
		j.UpdatedAt = time.Now()
	})

	dispatchURL := fmt.Sprintf("https://api.github.com/repos/%s/actions/workflows/%s/dispatches", repo, workflow)
	inputs := map[string]string{
		"url":         job.URL,
		"theme":       theme,
		"voice":       voice,
		"script_json": string(scriptBytes),
		"caption":     localCaption,
	}

	dispatchPayload := map[string]interface{}{
		"ref":    "main",
		"inputs": inputs,
	}
	bodyBytes, _ := json.Marshal(dispatchPayload)

	req, err := http.NewRequest(http.MethodPost, dispatchURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		s.failJob(job, fmt.Sprintf("Lỗi tạo HTTP request GitHub: %v", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.failJob(job, fmt.Sprintf("Gọi GitHub API thất bại: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var ghErr struct {
			Message string `json:"message"`
		}
		errMsg := string(respBody)
		if json.Unmarshal(respBody, &ghErr) == nil && ghErr.Message != "" {
			errMsg = ghErr.Message
		}
		s.failJob(job, fmt.Sprintf("GitHub lỗi (HTTP %d): %s", resp.StatusCode, errMsg))
		return
	}

	log.Printf("[Job %s] Dispatched GitHub Actions workflow successfully: %s", job.ID, dispatchURL)

	s.updateJob(job.ID, func(j *Job) {
		j.Progress = "Đã kích hoạt GitHub Actions! Đang đợi Runner nhận việc..."
		j.UpdatedAt = time.Now()
	})

	if job.ChatID != 0 {
		s.sendTelegramMessage(job.ChatID, fmt.Sprintf("☁️ [Job %s] Đã gửi lệnh render sang GitHub Actions!\nĐang đợi máy chủ đám mây xử lý...", job.ID))
	}

	// Wait 5s before finding the newly started run
	time.Sleep(5 * time.Second)

	runsURL := fmt.Sprintf("https://api.github.com/repos/%s/actions/workflows/%s/runs?per_page=3", repo, workflow)
	var activeRunID int64
	var runHTMLURL string

	for attempt := 0; attempt < 8; attempt++ {
		rReq, _ := http.NewRequest(http.MethodGet, runsURL, nil)
		rReq.Header.Set("Authorization", "Bearer "+token)
		rReq.Header.Set("Accept", "application/vnd.github+json")
		rReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		rResp, err := client.Do(rReq)
		if err == nil && rResp.StatusCode == http.StatusOK {
			var runsData struct {
				WorkflowRuns []struct {
					ID        int64     `json:"id"`
					HTMLURL   string    `json:"html_url"`
					CreatedAt time.Time `json:"created_at"`
					Status    string    `json:"status"`
				} `json:"workflow_runs"`
			}
			json.NewDecoder(rResp.Body).Decode(&runsData)
			rResp.Body.Close()

			if len(runsData.WorkflowRuns) > 0 {
				latest := runsData.WorkflowRuns[0]
				if time.Since(latest.CreatedAt) < 3*time.Minute {
					activeRunID = latest.ID
					runHTMLURL = latest.HTMLURL
					break
				}
			}
		} else if rResp != nil {
			rResp.Body.Close()
		}
		time.Sleep(3 * time.Second)
	}

	if activeRunID == 0 {
		s.updateJob(job.ID, func(j *Job) {
			j.Progress = "Đã kích hoạt GitHub Actions. Xem tab Actions trên GitHub."
			j.UpdatedAt = time.Now()
		})
		log.Printf("[Job %s] Could not locate run ID within timeout, but dispatch was successful.", job.ID)
		return
	}

	log.Printf("[Job %s] Tracking GitHub Actions Run #%d: %s", job.ID, activeRunID, runHTMLURL)

	// Poll until completed (max 20 minutes)
	pollURL := fmt.Sprintf("https://api.github.com/repos/%s/actions/runs/%d", repo, activeRunID)
	maxWait := 20 * time.Minute
	startTime := time.Now()

	for time.Since(startTime) < maxWait {
		time.Sleep(7 * time.Second)

		pReq, _ := http.NewRequest(http.MethodGet, pollURL, nil)
		pReq.Header.Set("Authorization", "Bearer "+token)
		pReq.Header.Set("Accept", "application/vnd.github+json")
		pReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		pResp, err := client.Do(pReq)
		if err != nil {
			continue
		}

		var runDetail struct {
			Status     string `json:"status"`     // queued, in_progress, completed
			Conclusion string `json:"conclusion"` // success, failure, cancelled
			HTMLURL    string `json:"html_url"`
		}
		json.NewDecoder(pResp.Body).Decode(&runDetail)
		pResp.Body.Close()

		if runDetail.Status == "in_progress" || runDetail.Status == "queued" {
			s.updateJob(job.ID, func(j *Job) {
				j.Progress = fmt.Sprintf("Cloud Runner đang render... (<a href='%s' target='_blank' style='color:#22d3ee;'>Xem log</a>)", runHTMLURL)
				j.UpdatedAt = time.Now()
			})
		} else if runDetail.Status == "completed" {
			if runDetail.Conclusion == "success" {
				s.updateJob(job.ID, func(j *Job) {
					j.Progress = "Render cloud thành công! Đang tải video hoàn chỉnh về máy..."
					j.UpdatedAt = time.Now()
				})

				// Download Artifact
				outputDir, videoPath, caption := s.downloadGitHubArtifact(repo, activeRunID, token, job.ID)
				if caption == "" {
					caption = localCaption
				}

				s.updateJob(job.ID, func(j *Job) {
					j.Status = "completed"
					j.Progress = "Hoàn thành 100% qua GitHub Actions"
					j.OutputDir = outputDir
					j.VideoPath = videoPath
					j.Caption = caption
					j.UpdatedAt = time.Now()
				})

				log.Printf("[Job %s] COMPLETED via GitHub Actions (Run %d)", job.ID, activeRunID)

				if job.ChatID != 0 {
					if videoPath != "" {
						s.sendTelegramVideo(job.ChatID, videoPath, fmt.Sprintf("🎉 Video đã tạo thành công qua GitHub Actions!\n\n%s", caption))
					} else {
						s.sendTelegramMessage(job.ChatID, fmt.Sprintf("🎉 Video đã tạo thành công qua GitHub Actions!\n🔗 Xem run: %s", runHTMLURL))
					}
				}
				return
			} else {
				s.failJob(job, fmt.Sprintf("GitHub Actions kết thúc với trạng thái: %s (<a href='%s' target='_blank' style='color:#ef4444;'>Xem chi tiết</a>)", runDetail.Conclusion, runHTMLURL))
				return
			}
		}
	}

	s.failJob(job, "Hết thời gian chờ GitHub Actions (timeout 20 phút)")
}

func (s *Server) downloadGitHubArtifact(repo string, runID int64, token string, jobID string) (outputDir, videoPath, caption string) {
	artifactsURL := fmt.Sprintf("https://api.github.com/repos/%s/actions/runs/%d/artifacts", repo, runID)
	req, _ := http.NewRequest(http.MethodGet, artifactsURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[Job %s] Failed to fetch artifacts list: %v", jobID, err)
		return
	}
	defer resp.Body.Close()

	var artList struct {
		Artifacts []struct {
			ID                  int64  `json:"id"`
			Name                string `json:"name"`
			ArchiveDownloadURL string `json:"archive_download_url"`
		} `json:"artifacts"`
	}
	json.NewDecoder(resp.Body).Decode(&artList)

	var downloadURL string
	for _, a := range artList.Artifacts {
		if a.Name == "generated-shorts-video" {
			downloadURL = a.ArchiveDownloadURL
			break
		}
	}

	if downloadURL == "" && len(artList.Artifacts) > 0 {
		downloadURL = artList.Artifacts[0].ArchiveDownloadURL
	}

	if downloadURL == "" {
		log.Printf("[Job %s] No artifact found in run %d", jobID, runID)
		return
	}

	// Download zip
	dlReq, _ := http.NewRequest(http.MethodGet, downloadURL, nil)
	dlReq.Header.Set("Authorization", "Bearer "+token)

	// Download zip with streaming to temp file (low RAM for Pi 5, 10 min timeout)
	dlClient := &http.Client{Timeout: 600 * time.Second}
	dlResp, err := dlClient.Do(dlReq)
	if err != nil || dlResp.StatusCode != http.StatusOK {
		log.Printf("[Job %s] Failed to download artifact zip: %v", jobID, err)
		return
	}
	defer dlResp.Body.Close()

	tmpZip, err := os.CreateTemp("", "gh-art-*.zip")
	if err != nil {
		log.Printf("[Job %s] Failed to create temp zip file: %v", jobID, err)
		return
	}
	tmpZipPath := tmpZip.Name()
	defer os.Remove(tmpZipPath)

	written, err := io.Copy(tmpZip, dlResp.Body)
	tmpZip.Close()
	if err != nil {
		log.Printf("[Job %s] Failed to stream artifact zip to file: %v", jobID, err)
		return
	}
	log.Printf("[Job %s] Downloaded artifact zip: %d bytes (%.2f MB)", jobID, written, float64(written)/(1024*1024))

	zipReader, err := zip.OpenReader(tmpZipPath)
	if err != nil {
		log.Printf("[Job %s] Failed to parse artifact zip: %v", jobID, err)
		return
	}
	defer zipReader.Close()

	targetDir := filepath.Join(s.cfg.ProjectDir, "output", jobID)
	os.MkdirAll(targetDir, 0755)
	outputDir = targetDir

	for _, file := range zipReader.File {
		base := filepath.Base(file.Name)
		destFile := filepath.Join(targetDir, base)

		rc, err := file.Open()
		if err != nil {
			continue
		}
		outFile, err := os.Create(destFile)
		if err == nil {
			io.Copy(outFile, rc)
			outFile.Close()
		}
		rc.Close()

		if base == "video.mp4" {
			videoPath = destFile
		}
		if base == "caption.txt" {
			if capBytes, err := os.ReadFile(destFile); err == nil {
				caption = string(capBytes)
			}
		}
	}

	return
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
		"service":     "shorts-generator-studio",
		"port":        s.cfg.Port,
		"total_jobs":  totalJobs,
		"queue_len":   len(s.jobQueue),
		"project_dir": s.cfg.ProjectDir,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		s.cfgMu.RLock()
		defer s.cfgMu.RUnlock()
		json.NewEncoder(w).Encode(s.cfg)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			LLMBaseURL     string `json:"llm_base_url"`
			LLMAPIKey      string `json:"llm_api_key"`
			LLMModel       string `json:"llm_model"`
			ChannelName    string `json:"channel_name"`
			TiktokHandle   string `json:"tiktok_handle"`
			TTSProvider    string `json:"tts_provider"`
			TelegramToken  string `json:"telegram_token"`
			RenderEngine   string `json:"render_engine"`
			GitHubRepo     string `json:"github_repo"`
			GitHubToken    string `json:"github_token"`
			GitHubWorkflow string `json:"github_workflow"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
			return
		}

		s.cfgMu.Lock()
		if req.LLMBaseURL != "" {
			s.cfg.LLMBaseURL = req.LLMBaseURL
			os.Setenv("LLM_BASE_URL", req.LLMBaseURL)
		}
		if req.LLMAPIKey != "" {
			s.cfg.LLMAPIKey = req.LLMAPIKey
			os.Setenv("LLM_API_KEY", req.LLMAPIKey)
		}
		if req.LLMModel != "" {
			s.cfg.LLMModel = req.LLMModel
			os.Setenv("LLM_MODEL", req.LLMModel)
		}
		if req.ChannelName != "" {
			s.cfg.ChannelName = req.ChannelName
			os.Setenv("CHANNEL_NAME", req.ChannelName)
		}
		if req.TiktokHandle != "" {
			s.cfg.TiktokHandle = req.TiktokHandle
			os.Setenv("TIKTOK_HANDLE", req.TiktokHandle)
		}
		if req.TTSProvider != "" {
			s.cfg.TTSProvider = req.TTSProvider
			os.Setenv("TTS_PROVIDER", req.TTSProvider)
		}
		if req.RenderEngine != "" {
			s.cfg.RenderEngine = req.RenderEngine
			os.Setenv("RENDER_ENGINE", req.RenderEngine)
		}
		if req.GitHubRepo != "" {
			s.cfg.GitHubRepo = req.GitHubRepo
			os.Setenv("GITHUB_REPO", req.GitHubRepo)
		}
		if req.GitHubToken != "" {
			s.cfg.GitHubToken = req.GitHubToken
			os.Setenv("GITHUB_TOKEN", req.GitHubToken)
		}
		if req.GitHubWorkflow != "" {
			s.cfg.GitHubWorkflow = req.GitHubWorkflow
			os.Setenv("GITHUB_WORKFLOW", req.GitHubWorkflow)
		}
		if req.TelegramToken != "" && s.cfg.TelegramToken != req.TelegramToken {
			s.cfg.TelegramToken = req.TelegramToken
			os.Setenv("TELEGRAM_BOT_TOKEN", req.TelegramToken)
			go s.startTelegramPoller()
		}
		s.cfgMu.Unlock()

		// Persist updates to .env file in ProjectDir
		s.saveConfigToEnv()

		json.NewEncoder(w).Encode(map[string]interface{}{"status": "updated"})
		return
	}

	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

func (s *Server) saveConfigToEnv() {
	envPath := filepath.Join(s.cfg.ProjectDir, ".env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	updates := map[string]string{
		"LLM_BASE_URL":       s.cfg.LLMBaseURL,
		"LLM_API_KEY":        s.cfg.LLMAPIKey,
		"LLM_MODEL":          s.cfg.LLMModel,
		"CHANNEL_NAME":       s.cfg.ChannelName,
		"TIKTOK_HANDLE":      s.cfg.TiktokHandle,
		"TTS_PROVIDER":       s.cfg.TTSProvider,
		"TELEGRAM_BOT_TOKEN": s.cfg.TelegramToken,
		"RENDER_ENGINE":      s.cfg.RenderEngine,
		"GITHUB_REPO":        s.cfg.GitHubRepo,
		"GITHUB_TOKEN":       s.cfg.GitHubToken,
		"GITHUB_WORKFLOW":    s.cfg.GitHubWorkflow,
	}

	var newLines []string
	seen := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		replaced := false
		for k, v := range updates {
			if strings.HasPrefix(trimmed, k+"=") {
				newLines = append(newLines, fmt.Sprintf("%s=%s", k, v))
				seen[k] = true
				replaced = true
				break
			}
		}
		if !replaced {
			newLines = append(newLines, line)
		}
	}

	for k, v := range updates {
		if !seen[k] && v != "" {
			newLines = append(newLines, fmt.Sprintf("%s=%s", k, v))
		}
	}

	_ = os.WriteFile(envPath, []byte(strings.Join(newLines, "\n")), 0644)
}

var concreteThemes = []string{
	"dark-neon",
	"cyberpunk-glitch",
	"liquid-aurora",
	"bold-poster",
	"pentagram-stat",
	"light-pro",
}

func randomConcreteTheme() string {
	return concreteThemes[rand.Intn(len(concreteThemes))]
}

// ── CHANNEL PROFILES MANAGEMENT ───────────────────────────────────────────

func (s *Server) loadChannels() {
	s.channelsMu.Lock()
	defer s.channelsMu.Unlock()

	channelsFile := filepath.Join(s.cfg.ProjectDir, "server", "channels.json")
	data, err := os.ReadFile(channelsFile)
	if err == nil {
		var list []*ChannelProfile
		if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
			s.channels = make(map[string]*ChannelProfile)
			for _, cp := range list {
				s.channels[cp.ID] = cp
			}
			log.Printf("Loaded %d channel profile(s) from channels.json", len(s.channels))
			return
		}
	}

	// Default fallback profiles
	defaults := []*ChannelProfile{
		{
			ID:        "leviatech",
			Name:      "LeviaTech",
			Handle:    "@leviatech",
			Avatar:    "/assets/avatar.jpg",
			Voice:     "vi-VN-NamMinhNeural",
			Theme:     "dark-neon",
			IsDefault: true,
		},
		{
			ID:        "codedao",
			Name:      "Code Dạo Review",
			Handle:    "@codedao.vn",
			Avatar:    "/assets/avatar.jpg",
			Voice:     "vi-VN-HoaiMyNeural",
			Theme:     "bold-poster",
			IsDefault: false,
		},
		{
			ID:        "aitrend",
			Name:      "AI Explorer VN",
			Handle:    "@aiexplorer.vn",
			Avatar:    "/assets/avatar.jpg",
			Voice:     "vi-VN-DaLyNeural",
			Theme:     "liquid-aurora",
			IsDefault: false,
		},
	}
	s.channels = make(map[string]*ChannelProfile)
	for _, cp := range defaults {
		s.channels[cp.ID] = cp
	}
	s.saveChannelsLocked()
}

func (s *Server) saveChannelsLocked() {
	list := make([]*ChannelProfile, 0, len(s.channels))
	for _, cp := range s.channels {
		list = append(list, cp)
	}
	channelsFile := filepath.Join(s.cfg.ProjectDir, "server", "channels.json")
	bytes, err := json.MarshalIndent(list, "", "  ")
	if err == nil {
		_ = os.WriteFile(channelsFile, bytes, 0644)
	}
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		s.channelsMu.RLock()
		list := make([]*ChannelProfile, 0, len(s.channels))
		for _, cp := range s.channels {
			list = append(list, cp)
		}
		s.channelsMu.RUnlock()

		sort.Slice(list, func(i, j int) bool {
			if list[i].IsDefault != list[j].IsDefault {
				return list[i].IsDefault
			}
			return list[i].Name < list[j].Name
		})
		json.NewEncoder(w).Encode(list)
		return
	}

	if r.Method == http.MethodPost {
		var req ChannelProfile
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, `{"error":"invalid channel data, name is required"}`, http.StatusBadRequest)
			return
		}

		s.channelsMu.Lock()
		defer s.channelsMu.Unlock()

		if req.ID == "" {
			slug := strings.ToLower(strings.TrimSpace(req.Name))
			slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
			slug = strings.Trim(slug, "-")
			if slug == "" {
				slug = fmt.Sprintf("ch-%d", time.Now().UnixMilli()%10000)
			}
			baseSlug := slug
			counter := 1
			for s.channels[slug] != nil {
				slug = fmt.Sprintf("%s-%d", baseSlug, counter)
				counter++
			}
			req.ID = slug
		}

		if req.Avatar == "" {
			req.Avatar = "/assets/avatar.jpg"
		}
		if req.Voice == "" {
			req.Voice = "vi-VN-NamMinhNeural"
		}
		if req.Theme == "" {
			req.Theme = "random"
		}

		if req.IsDefault {
			for _, cp := range s.channels {
				cp.IsDefault = false
			}
			s.cfgMu.Lock()
			s.cfg.ChannelName = req.Name
			s.cfg.TiktokHandle = req.Handle
			os.Setenv("CHANNEL_NAME", req.Name)
			os.Setenv("TIKTOK_HANDLE", req.Handle)
			s.cfgMu.Unlock()
			s.saveConfigToEnv()
		}

		s.channels[req.ID] = &req
		s.saveChannelsLocked()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(req)
		return
	}

	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

func (s *Server) handleChannelDetail(w http.ResponseWriter, r *http.Request) {
	subPath := strings.TrimPrefix(r.URL.Path, "/api/channels/")
	parts := strings.Split(subPath, "/")
	channelID := parts[0]

	if channelID == "" {
		http.NotFound(w, r)
		return
	}

	// Avatar upload: POST /api/channels/{id}/avatar
	if len(parts) > 1 && parts[1] == "avatar" && r.Method == http.MethodPost {
		err := r.ParseMultipartForm(10 << 20) // 10MB max
		if err != nil {
			http.Error(w, `{"error":"file too large or invalid multipart"}`, http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("avatar")
		if err != nil {
			http.Error(w, `{"error":"missing avatar file in form-data"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
			ext = ".jpg"
		}

		avatarDir := filepath.Join(s.cfg.ProjectDir, "assets", "channels")
		_ = os.MkdirAll(avatarDir, 0755)

		fileName := fmt.Sprintf("%s%s", channelID, ext)
		destPath := filepath.Join(avatarDir, fileName)

		destFile, err := os.Create(destPath)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to save file: %v"}`, err), http.StatusInternalServerError)
			return
		}
		defer destFile.Close()

		if _, err := io.Copy(destFile, file); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to copy file: %v"}`, err), http.StatusInternalServerError)
			return
		}

		avatarURL := fmt.Sprintf("/assets/channels/%s", fileName)

		s.channelsMu.Lock()
		if cp, ok := s.channels[channelID]; ok {
			cp.Avatar = avatarURL
			s.saveChannelsLocked()
		}
		s.channelsMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"avatar": avatarURL})
		return
	}

	if r.Method == http.MethodDelete {
		s.channelsMu.Lock()
		defer s.channelsMu.Unlock()

		if len(s.channels) <= 1 {
			http.Error(w, `{"error":"cannot delete the only channel profile"}`, http.StatusBadRequest)
			return
		}

		cp, ok := s.channels[channelID]
		if !ok {
			http.NotFound(w, r)
			return
		}

		delete(s.channels, channelID)
		if cp.IsDefault {
			for _, remaining := range s.channels {
				remaining.IsDefault = true
				break
			}
		}
		s.saveChannelsLocked()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "deleted": channelID})
		return
	}

	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
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
			URL      string   `json:"url"`
			Theme    string   `json:"theme,omitempty"`
			Voice    string   `json:"voice,omitempty"`
			Channel  string   `json:"channel,omitempty"`
			Channels []string `json:"channels,omitempty"`
			Engine   string   `json:"engine,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
			http.Error(w, `{"error":"missing url"}`, http.StatusBadRequest)
			return
		}

		engine := req.Engine
		if engine == "" || engine == "auto" {
			s.cfgMu.RLock()
			engine = s.cfg.RenderEngine
			s.cfgMu.RUnlock()
		}
		if engine == "" {
			engine = "local"
		}

		// Determine target channels
		var targetChannels []*ChannelProfile
		s.channelsMu.RLock()
		if len(req.Channels) > 0 {
			for _, chID := range req.Channels {
				if cp, ok := s.channels[chID]; ok {
					targetChannels = append(targetChannels, cp)
				}
			}
		}
		if len(targetChannels) == 0 {
			if req.Channel != "" {
				if cp, ok := s.channels[req.Channel]; ok {
					targetChannels = append(targetChannels, cp)
				} else {
					targetChannels = append(targetChannels, &ChannelProfile{
						ID:     "custom",
						Name:   req.Channel,
						Handle: s.cfg.TiktokHandle,
						Avatar: "/assets/avatar.jpg",
						Voice:  req.Voice,
						Theme:  req.Theme,
					})
				}
			} else {
				var defaultCP *ChannelProfile
				for _, cp := range s.channels {
					if cp.IsDefault {
						defaultCP = cp
						break
					}
				}
				if defaultCP != nil {
					targetChannels = append(targetChannels, defaultCP)
				} else {
					targetChannels = append(targetChannels, &ChannelProfile{
						ID:     "default",
						Name:   s.cfg.ChannelName,
						Handle: s.cfg.TiktokHandle,
						Avatar: "/assets/avatar.jpg",
						Voice:  req.Voice,
						Theme:  req.Theme,
					})
				}
			}
		}
		s.channelsMu.RUnlock()

		createdJobs := make([]*Job, 0, len(targetChannels))

		s.jobsMu.Lock()
		for i, cp := range targetChannels {
			// Resolve theme: if specific theme requested in form, use it.
			// If theme is "random" or empty, pick random concrete theme
			jobTheme := req.Theme
			if jobTheme == "" || jobTheme == "default" {
				jobTheme = cp.Theme
			}
			if jobTheme == "random" || jobTheme == "" {
				jobTheme = randomConcreteTheme()
			}

			// Resolve voice
			jobVoice := req.Voice
			if jobVoice == "" || jobVoice == "default" {
				jobVoice = cp.Voice
			}
			if jobVoice == "" {
				jobVoice = "vi-VN-NamMinhNeural"
			}

			jobID := fmt.Sprintf("%d", time.Now().UnixMilli())
			if len(targetChannels) > 1 {
				jobID = fmt.Sprintf("%d-%d", time.Now().UnixMilli(), i+1)
			}

			job := &Job{
				ID:        jobID,
				URL:       req.URL,
				Theme:     jobTheme,
				Voice:     jobVoice,
				Channel:   cp.Name,
				Handle:    cp.Handle,
				Avatar:    cp.Avatar,
				Engine:    engine,
				Status:    "queued",
				Progress:  "Đang chờ xếp hàng...",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			s.jobs[jobID] = job
			createdJobs = append(createdJobs, job)
		}
		s.jobsMu.Unlock()

		for _, job := range createdJobs {
			s.jobQueue <- job
		}

		w.WriteHeader(http.StatusAccepted)
		if len(createdJobs) == 1 {
			json.NewEncoder(w).Encode(createdJobs[0])
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "queued",
				"count":  len(createdJobs),
				"jobs":   createdJobs,
			})
		}
		return
	}

	if r.Method == http.MethodDelete {
		filter := r.URL.Query().Get("filter") // "all" or "done" (default)
		s.jobsMu.Lock()
		deleted := 0
		for id, j := range s.jobs {
			shouldDel := false
			if filter == "all" {
				shouldDel = true
			} else if j.Status == "completed" || j.Status == "failed" {
				shouldDel = true
			}

			if shouldDel {
				if filter == "all" && j.OutputDir != "" && strings.HasPrefix(j.OutputDir, s.cfg.ProjectDir) {
					_ = os.RemoveAll(j.OutputDir)
				}
				delete(s.jobs, id)
				deleted++
			}
		}
		s.jobsMu.Unlock()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "deleted": deleted})
		return
	}

	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

func (s *Server) handleJobDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")

	// Stream video: /api/jobs/{id}/video
	if strings.HasSuffix(id, "/video") {
		jobID := strings.TrimSuffix(id, "/video")
		s.jobsMu.RLock()
		job, ok := s.jobs[jobID]
		s.jobsMu.RUnlock()
		if !ok || job.VideoPath == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeFile(w, r, job.VideoPath)
		return
	}

	if r.Method == http.MethodDelete {
		s.jobsMu.Lock()
		job, ok := s.jobs[id]
		if ok {
			delete(s.jobs, id)
		}
		s.jobsMu.Unlock()

		if !ok {
			http.NotFound(w, r)
			return
		}

		// Clean up output files if within ProjectDir
		if job.OutputDir != "" && strings.HasPrefix(job.OutputDir, s.cfg.ProjectDir) {
			_ = os.RemoveAll(job.OutputDir)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
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
		s.cfgMu.RLock()
		token := s.cfg.TelegramToken
		s.cfgMu.RUnlock()

		if token == "" {
			time.Sleep(10 * time.Second)
			continue
		}

		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", token, offset)
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
	s.cfgMu.RLock()
	token := s.cfg.TelegramToken
	s.cfgMu.RUnlock()
	if token == "" {
		return
	}

	body, _ := json.Marshal(map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	})
	http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token),
		"application/json",
		bytes.NewReader(body),
	)
}

func (s *Server) sendTelegramVideo(chatID int64, videoPath, caption string) {
	s.cfgMu.RLock()
	token := s.cfg.TelegramToken
	s.cfgMu.RUnlock()
	if token == "" {
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
		fmt.Sprintf("https://api.telegram.org/bot%s/sendVideo", token),
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
