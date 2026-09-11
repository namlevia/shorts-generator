# ⚡ LeviaTech Shorts Studio (shorts-generator)

> **⚡ Hệ thống tự động tạo video ngắn 9:16 (TikTok / Shorts / Reels) từ URL & GitHub bằng AI, giọng đọc TTS, đồ họa chuyển động lập trình & kiến trúc Đa Kênh (Content Matrix).**

[![Node.js](https://img.shields.io/badge/Node.js-v20+-green.svg)](https://nodejs.org/)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org/)
[![FFmpeg](https://img.shields.io/badge/FFmpeg-6.0+-007808.svg)](https://ffmpeg.org/)
[![HyperFrames](https://img.shields.io/badge/Render-HyperFrames-7c3aed.svg)](https://hyperframes.heygen.com/)
[![GitHub Actions](https://img.shields.io/badge/Cloud_Render-GitHub_Actions-2088FF.svg)](https://github.com/features/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## 🌟 Điểm nổi bật vượt trội

* 🤖 **1-Link tạo video hoàn chỉnh:** Chỉ cần nhập 1 URL GitHub hoặc bài báo công nghệ, AI (Gemini 3.8 Flash) tự động tóm tắt, viết kịch bản giật gân (hook-problem-solution-cta), lồng tiếng TTS và render video Full HD 1080x1920 9:16.
* 👥 **Hệ thống Quản Lý Đa Kênh (Multi-Channel Content Matrix):**
  * Hỗ trợ tạo và quản lý không giới hạn hồ sơ kênh (LeviaTech, Code Dạo Review, AI Explorer VN,...).
  * Mỗi kênh sở hữu thương hiệu riêng biệt: Tên hiển thị, TikTok Handle, Avatar riêng, Giọng đọc AI riêng và Phong cách Theme mặc định.
  * **Tạo video hàng loạt 1-Click:** Chọn 1 kênh, 2 kênh hoặc **🌟 Tất Cả Kênh (All Channels)** để từ 1 bài viết tự động sinh ra nhiều phiên bản video riêng cho từng kênh!
* 🎨 **6 Mẫu Giao Diện Đồ Họa Đỉnh Cao + Tùy Chọn Random:**
  * `dark-neon`: Cyberpunk Neon Cyan/Purple tương phản cao, hiện đại.
  * `cyberpunk-glitch`: Phong cách Glitch sắc cạnh, hiệu ứng scanline.
  * `liquid-aurora`: Gradient cực quang chuyển động mềm mại, sang trọng.
  * `bold-poster`: Typography khổ lớn, tương phản vàng - đen mạnh mẽ.
  * `pentagram-stat`: Phong cách phân tích số liệu, dashboard công nghệ.
  * `light-pro`: Tối giản, hiện đại, tone sáng thanh lịch.
  * `🎲 Random (Ngẫu Nhiên)`: Tự động bốc ngẫu nhiên theme cho từng video/kênh, tạo sự đa dạng cho nguồn cấp dữ liệu video.
* ☁️ **Cơ Chế Render Linh Hoạt (Local & Cloud GitHub Actions):**
  * **Local Render:** Tối ưu hóa render trực tiếp trên máy tính cá nhân hoặc Raspberry Pi 5.
  * **Cloud Render:** Tự động kích hoạt GitHub Actions workflow miễn phí trên đám mây, tải video hoàn thiện về máy mà không làm nóng máy hay chiếm RAM.
* 🐹 **Golang Server & WebUI Studio siêu tốc:**
  * Server viết bằng Go tiêu thụ cực ít tài nguyên (< 50MB RAM), tích hợp sẵn Task Queue thông minh chống nghẽn hệ thống.
  * Giao diện WebUI Studio Glassmorphism thời thượng, hỗ trợ song ngữ (Tiếng Việt / English).
  * Trình phát video 9:16 trực tiếp, quản lý lịch sử, nút xóa video với hộp thoại xác nhận thẩm mỹ cao, sao chép Caption & Hashtag trong 1 cú click.
* 💬 **Telegram Bot tích hợp 24/7:** Gửi link qua tin nhắn Telegram ➔ Bot tự động xếp hàng làm video và gửi ngược lại file `.mp4` hoàn chỉnh kèm caption.
* 🎙 **Giọng đọc AI tiếng Việt tự nhiên:** Hỗ trợ miễn phí 100% qua Edge-TTS (`vi-VN-NamMinhNeural`, `vi-VN-HoaiMyNeural`, `vi-VN-DaLyNeural`), cùng các tích hợp mở rộng ElevenLabs / LucyLab / Vbee.

---

## 📁 Cấu trúc dự án

```text
shorts-generator/
├── assets/                   # Tài nguyên thương hiệu
│   ├── avatar.jpg            # Avatar mặc định (LeviaTech)
│   └── channels/             # Avatar riêng của từng kênh (Code Dạo, AI Explorer...)
├── src/                      # Core Pipeline (TypeScript / Node.js)
│   ├── make.ts               # CLI 1 lệnh tạo video từ URL
│   ├── scraper/              # Thu thập dữ liệu từ GitHub Repo & URL bài viết
│   ├── llm/                  # AI Generator (Gemini 3.8 Flash) tạo kịch bản JSON
│   ├── tts/                  # Bộ chuyển đổi văn bản thành giọng nói (Edge-TTS, ElevenLabs)
│   ├── render/               # HyperFrames, GSAP Animation & Canvas Engine
│   └── pipeline.ts           # Trình điều phối và render video
├── server/                   # Golang Server & WebUI Studio (Tối ưu cho Pi 5 & VPS)
│   ├── main.go               # HTTP REST API, Queue Worker & Telegram Bot
│   ├── channels.json         # Cơ sở dữ liệu hồ sơ các kênh
│   ├── go.mod
│   └── web/
│       └── index.html        # Giao diện WebUI Studio tương tác cao
├── .github/
│   └── workflows/
│       └── render.yml        # Cloud Render Engine (GitHub Actions)
├── hyperframes.json          # Cấu hình render HyperFrames
├── package.json              # Khai báo thư viện Node.js
└── .env.example              # Mẫu cấu hình môi trường
```

---

## 🚀 Cài đặt & Cấu hình

### 1. Yêu cầu hệ thống
* **Node.js:** `>= 20.x`
* **Go:** `>= 1.22` *(dành cho WebUI Studio & Server)*
* **FFmpeg:** Đã cài đặt và có trong biến môi trường `PATH`.

### 2. Cài đặt thư viện
```bash
npm install
```

### 3. Cấu hình biến môi trường (`.env`)
Sao chép `.env.example` thành `.env`:
```bash
cp .env.example .env
```

Các biến môi trường chính:
```env
# 1. LLM API (Gemini 3.8 Flash hoặc OpenAI-compatible API)
LLM_BASE_URL=http://localhost:1998/v1
LLM_API_KEY=your_api_key_here
LLM_MODEL=gemini-3.8-flash

# 2. Giọng đọc AI Tiếng Việt (Mặc định miễn phí 100% qua Edge-TTS)
TTS_PROVIDER=edge-tts
EDGE_TTS_VOICE=vi-VN-NamMinhNeural

# 3. Kênh thương hiệu mặc định
CHANNEL_NAME=LeviaTech
TIKTOK_HANDLE=@leviatech
TIKTOK_DISPLAY_NAME=LeviaTech

# 4. Chế độ Render (local hoặc github)
RENDER_ENGINE=local
GITHUB_REPO=namlevia/shorts-generator
GITHUB_TOKEN=ghp_your_github_pat_token

# 5. Telegram Bot (Tùy chọn)
TELEGRAM_BOT_TOKEN=123456789:ABCdefGhIJKlmNoPQRstuVWXyz
PORT=2024
```

---

## 💻 Hướng dẫn sử dụng

### Cách 1: WebUI Studio (Khuyên dùng - Trực quan & Đầy đủ tính năng)

Khởi động server Golang:
```bash
cd server
go run main.go
```
Mở trình duyệt truy cập: **`http://localhost:2024`**

Các tính năng nổi bật trên WebUI:
1. **Tab Studio:**
   * Dán URL GitHub hoặc bài viết công nghệ.
   * Chọn kênh xuất bản: Chọn 1 kênh cụ thể hoặc bấm **🌟 Tất Cả Kênh** để tạo video ma trận đa kênh.
   * Lựa chọn Mẫu Giao Diện (Theme) từ dải thẻ trực quan hoặc chọn **🎲 Ngẫu Nhiên (Random)**.
   * Tùy chọn Render Engine: Render ngay trên máy (Local) hoặc chuyển giao cho GitHub Actions Cloud.
2. **Tab Hàng Đợi (Queue & History):**
   * Theo dõi tiến độ render từng giây theo thời gian thực (Real-time logs).
   * Xem trước video 9:16 tích hợp sẵn player chất lượng cao.
   * Nút tải video MP4 và 1-click Copy Caption TikTok kèm hashtag chuẩn SEO.
   * Nút Xóa video thành phẩm kèm Modal xác nhận bảo đảm an toàn.
3. **Tab Cài Đặt (Settings):**
   * **Quản Lý Hồ Sơ Kênh (Channel Profiles):** Thêm kênh mới, đổi tên, handle, đổi giọng đọc mặc định và tải avatar riêng cho từng kênh.
   * Cấu hình API LLM, Giọng đọc TTS, Cloud GitHub Actions và Telegram Bot trực tiếp trên giao diện không cần sửa code.

---

### Cách 2: Chạy trực tiếp từ CLI (1 lệnh duy nhất)

```bash
# Tạo video với theme mặc định
npm run make -- https://github.com/dabeecao/telecloud-go

# Chỉ định theme cụ thể
npm run make -- https://github.com/dabeecao/telecloud-go --theme cyberpunk-glitch
```

Video thành phẩm sẽ được lưu tại:
`output/<tên-dự-án>-<timestamp>/video.mp4`

---

### Cách 3: Điều khiển qua Telegram Bot (Tự động 24/7)

1. Cấu hình `TELEGRAM_BOT_TOKEN` trong tab Cài Đặt WebUI hoặc file `.env`.
2. Mở Telegram gửi link GitHub/bài báo cho Bot.
3. Bot tự động đưa vào hàng đợi xử lý và gửi trả lại file `.mp4` Full HD kèm Caption & Hashtag.

---

## 🌐 Hệ thống API REST

Server Golang cung cấp đầy đủ API chuẩn RESTful cho việc tích hợp vào các hệ thống tự động hóa:

| Phương thức | Endpoint | Mô tả |
| :--- | :--- | :--- |
| `GET` | `/health` | Kiểm tra trạng thái hoạt động của server |
| `GET` | `/api/config` | Lấy cấu hình hệ thống hiện tại |
| `POST` | `/api/config` | Cập nhật cấu hình runtime & lưu vào file `.env` |
| `GET` | `/api/channels` | Lấy danh sách toàn bộ hồ sơ kênh |
| `POST` | `/api/channels` | Thêm mới hoặc cập nhật thông tin hồ sơ kênh |
| `DELETE` | `/api/channels/{id}` | Xóa một hồ sơ kênh |
| `POST` | `/api/channels/{id}/avatar` | Tải lên file ảnh Avatar riêng cho kênh |
| `GET` | `/api/jobs` | Lấy danh sách công việc trong hàng đợi và lịch sử |
| `POST` | `/api/jobs` | Tạo job mới (hỗ trợ `url`, `theme`, `engine`, `channel_ids`) |
| `GET` | `/api/jobs/{id}` | Xem chi tiết trạng thái tiến độ và log của job |
| `DELETE` | `/api/jobs/{id}` | Xóa bỏ job / video khỏi danh sách |
| `GET` | `/api/jobs/{id}/video` | Xem hoặc tải trực tiếp file video `.mp4` |

---

## 🍓 Hướng dẫn triển khai trên Raspberry Pi 5 / VPS Linux

Chạy mượt mà 24/7 trên Raspberry Pi OS (Debian 64-bit):

```bash
# 1. Cài đặt FFmpeg & Node.js 20
sudo apt update
sudo apt install -y ffmpeg git
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo bash -
sudo apt install -y nodejs

# 2. Cài đặt font tiếng Việt hiển thị đẹp cho Chromium
sudo apt install -y fonts-inter fonts-noto-cjk

# 3. Cài đặt dependencies và build Golang Server nhúng
npm install
cd server
go build -o server main.go
cd ..

# 4. Khởi chạy server chạy nền (systemd service)
# Sử dụng /etc/systemd/system/shorts-generator.service để tự khởi động cùng hệ thống
```

---

## 📄 Bản quyền

Phát hành theo giấy phép [MIT License](LICENSE).  
Phát triển và duy trì bởi **LeviaTech Studio**.
