# ==============================================================================
# Multi-stage Dockerfile for LeviaTech Shorts Studio (shorts-generator)
# Stage 1: Build Golang Server & WebUI binary
# Stage 2: Runtime image with Node.js 22, FFmpeg, Chromium & Fonts
# ==============================================================================

# ------------------------------------------------------------------------------
# Stage 1: Build Golang Server
# ------------------------------------------------------------------------------
FROM golang:1.23-bookworm AS builder

WORKDIR /build

# Copy Go module manifests
COPY server/go.mod ./server/

# Copy Go server source code & embedded WebUI
COPY server/ ./server/

# Build static Go binary with web assets embedded
RUN cd server && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/shorts-server main.go

# ------------------------------------------------------------------------------
# Stage 2: Production Runtime
# ------------------------------------------------------------------------------
FROM node:22-bookworm-slim AS runtime

# Set environment defaults
ENV NODE_ENV=production \
    PORT=2024 \
    RENDER_ENGINE=local \
    TTS_PROVIDER=edge-tts \
    EDGE_TTS_VOICE=vi-VN-NamMinhNeural \
    PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true \
    PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium \
    CHROME_BIN=/usr/bin/chromium

# Install system dependencies:
# - FFmpeg: Video encoding, audio mixing & optimization
# - Chromium & graphics libraries: Headless browser rendering for HyperFrames
# - Vietnamese & International fonts: Beautiful typography rendering
# - ca-certificates & curl: HTTPS requests & Docker healthcheck
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    chromium \
    fonts-inter \
    fonts-noto-cjk \
    fonts-liberation \
    fonts-freefont-ttf \
    ca-certificates \
    curl \
    libnss3 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libxcomposite1 \
    libxdamage1 \
    libxrandr2 \
    libgbm1 \
    libxkbcommon0 \
    libpango-1.0-0 \
    libasound2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install Node.js dependencies
COPY package.json package-lock.json* ./
RUN npm ci || npm install

# Copy application configuration and source files
COPY tsconfig.json hyperframes.json ./
COPY src/ ./src/
COPY assets/ ./assets/

# Copy server assets & channels database
COPY server/channels.json ./server/channels.json

# Copy compiled Go server binary from builder stage
COPY --from=builder /build/shorts-server /app/shorts-server

# Ensure binary is executable and persistent directories exist
RUN chmod +x /app/shorts-server && \
    mkdir -p /app/output /app/assets/channels

# Expose Studio WebUI & API port
EXPOSE 2024

# Declare video output volume for persistence
VOLUME ["/app/output"]

# Healthcheck to ensure Studio WebUI is responding
HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=15s \
    CMD curl -f http://localhost:${PORT}/health || exit 1

# Start LeviaTech Shorts Studio Server
CMD ["/app/shorts-server"]
