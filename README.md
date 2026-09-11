# ⚡ shorts-generator

> **⚡ Hệ thống tự động tạo video ngắn 9:16 (TikTok / Shorts / Reels) từ URL & GitHub bằng AI, giọng đọc TTS và đồ họa chuyển động lập trình.**

[![Node.js](https://img.shields.io/badge/Node.js-v20+-green.svg)](https://nodejs.org/)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org/)
[![FFmpeg](https://img.shields.io/badge/FFmpeg-6.0+-007808.svg)](https://ffmpeg.org/)
[![HyperFrames](https://img.shields.io/badge/Render-HyperFrames-7c3aed.svg)](https://hyperframes.heygen.com/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## 🌟 Điểm nổi bật

* 🤖 **1-Lệnh sinh video hoàn chỉnh:** Chỉ cần đưa vào 1 đường link GitHub (hoặc bài báo công nghệ), hệ thống tự động cào nội dung, dùng AI viết kịch bản, lồng tiếng và render thành video Full HD 1080x1920 9:16.
* 🐹 **Golang Server siêu nhẹ:** Tích hợp sẵn server Golang tối ưu cho **Raspberry Pi 5** hoặc VPS nhỏ (tiêu thụ < 50MB RAM, quản lý hàng đợi 1 task để tránh nghẽn CPU/RAM).
* 💬 **Telegram Bot tích hợp:** Gửi link qua tin nhắn Telegram ➔ Bot tự động làm video và gửi ngược lại file `.mp4` kèm Caption & Hashtag.
* 🎙 **Giọng đọc AI tiếng Việt tự nhiên:** Hỗ trợ miễn phí 100% qua Edge-TTS (`vi-VN-NamMinhNeural` / `vi-VN-HoaiMyNeural`), hoặc ElevenLabs / LucyLab / Vbee.
* 🎨 **Đồ họa chuyển động cao cấp:** Hiệu ứng Glassmorphism, Neon Glow, Kinetic Typography (GSAP), tự động căn chỉnh khoảng cách chữ tiếng Việt có dấu, không bao giờ bị đè chữ.
* 🏷 **Brand Shell nhận diện thương hiệu:** Tự động gắn Logo (Avatar), tên kênh `@leviatech`, nhãn trích dẫn nguồn repository cụ thể.

---

## 📁 Cấu trúc thư mục

```text
shorts-generator/
├── src/                      # Core Pipeline (TypeScript / Node.js)
│   ├── make.ts               # CLI 1 lệnh tạo video từ URL
│   ├── scraper/              # Cào README GitHub & bài viết web
│   ├── llm/                  # Gọi AI (Gemini 3.8 Flash) sinh kịch bản JSON
│   ├── tts/                  # Sinh giọng đọc tiếng Việt (Edge-TTS / ElevenLabs)
│   ├── render/               # HyperFrames, GSAP animation & CSS layout
│   └── pipeline.ts           # Trình điều phối render video
├── assets/                   # Avatar, Logo thương hiệu LeviaTech, SFX
├── server/                   # Golang Server & Telegram Bot (tối ưu cho Pi 5)
│   ├── go.mod
│   └── main.go               # HTTP REST API + Task Queue + Telegram Bot
├── hyperframes.json          # Cấu hình HyperFrames video engine
├── package.json              # Khai báo dependency Node.js
└── .env.example              # Mẫu cấu hình môi trường
```

---

## 🚀 Cài đặt & Cấu hình

### 1. Yêu cầu môi trường
* **Node.js:** `>= 20.x`
* **Go:** `>= 1.22` *(nếu chạy server Golang)*
* **FFmpeg:** Đã cài đặt và có trong biến môi trường `PATH`.

### 2. Cài đặt dependencies
```bash
npm install
```

### 3. Cấu hình biến môi trường
Sao chép `.env.example` thành `.env`:
```bash
cp .env.example .env
```

Cấu hình các thông số quan trọng trong `.env`:
```env
# 1. LLM API (Dùng Gemini 3.8 Flash hoặc OpenAI-compatible)
LLM_BASE_URL=http://localhost:1998/v1
LLM_API_KEY=your_api_key_here
LLM_MODEL=gemini-3.8-flash

# 2. TTS Voice tiếng Việt (Mặc định miễn phí 100% qua Edge-TTS)
TTS_PROVIDER=edge-tts
EDGE_TTS_VOICE=vi-VN-NamMinhNeural

# 3. Kênh thương hiệu
CHANNEL_NAME=LeviaTech
TIKTOK_HANDLE=@leviatech
TIKTOK_DISPLAY_NAME=LeviaTech

# 4. Telegram Bot (Tùy chọn: Dành cho Golang Server trên Pi 5)
TELEGRAM_BOT_TOKEN=123456789:ABCdefGhIJKlmNoPQRstuVWXyz
PORT=8080
```

---

## 💻 Hướng dẫn sử dụng

### Cách 1: Chạy trực tiếp từ CLI (1 lệnh duy nhất)

```bash
npm run make -- https://github.com/dabeecao/telecloud-go
```

Toàn bộ video hoàn chỉnh sẽ được lưu tại:
`output/<tên-dự-án>-<timestamp>/video.mp4`

---

### Cách 2: Chạy Golang Server & Telegram Bot (Dành cho Raspberry Pi 5 / Server 24/7)

1. **Vào thư mục server và khởi chạy:**
```bash
cd server
go run main.go
```

2. **Các tính năng server tự động kích hoạt:**
* **REST API:**
  * `POST http://localhost:8080/api/jobs` kèm body `{"url": "https://github.com/..."}` để đưa vào hàng đợi.
  * `GET http://localhost:8080/api/jobs/{id}` để theo dõi tiến độ.
  * `GET http://localhost:8080/api/jobs/{id}/video` để tải video MP4.
* **Telegram Bot:**
  * Chỉ cần mở Telegram nhắn tin đường link GitHub cho Bot.
  * Bot sẽ tự xếp hàng, xử lý và gửi ngược lại video Full HD kèm Caption hoàn chỉnh!

---

## 🍓 Hướng dẫn triển khai trên Raspberry Pi 5

Trên Raspberry Pi OS (Debian 64-bit):

```bash
# 1. Cài đặt FFmpeg & Node.js 20
sudo apt update
sudo apt install -y ffmpeg git
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo bash -
sudo apt install -y nodejs

# 2. Cài đặt font tiếng Việt cho Chromium
sudo apt install -y fonts-inter fonts-noto-cjk

# 3. Cài đặt dependencies và build server Golang
npm install
cd server && go build -o server main.go && cd ..

# 4. Chạy server ngầm (systemd service)
# Tạo file /etc/systemd/system/shorts-generator.service để tự khởi động cùng Pi 5
```

---

## 📄 Bản quyền
Phát hành theo giấy phép [MIT License](LICENSE).
Thương hiệu & Đồ họa: **LeviaTech**.
