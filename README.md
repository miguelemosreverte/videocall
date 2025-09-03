# WebP Video Conference

Real-time video conferencing using WebSocket and WebP compression. Built to bypass WebRTC blocking with server-mediated streaming.

## 🚀 Live Demo

https://[your-github-username].github.io/videocall/

## ✨ Features

- **WebP Compression**: 6-14x bandwidth reduction
- **Adaptive Quality**: Automatic resolution adjustment based on participants
- **Low Latency**: < 200ms typical latency
- **Audio Support**: Real-time PCM audio streaming
- **Beautiful UI**: Responsive design with animal avatars
- **Auto-SSL**: Automatic HTTPS certificates via Caddy
- **Docker Deployment**: One-command deployment

## 📦 Quick Deploy

### Server Setup (Docker)

1. Clone the repository:
```bash
git clone https://github.com/[your-username]/videocall.git
cd videocall
```

2. Deploy with Docker Compose:
```bash
docker-compose up -d
```

The server will be available at:
- HTTPS: `https://your-domain.com`
- WebSocket: `wss://your-domain.com/ws`

### Client Setup (GitHub Pages)

1. Fork this repository
2. Enable GitHub Pages in Settings
3. Update `index.html` with your server URL:
```javascript
const WS_URL = 'wss://your-server.com/ws';
```
4. Push changes

## 🛠️ Configuration

### Server Environment Variables

Create a `.env` file:

```env
DOMAIN=your-domain.com
MAX_USERS_PER_ROOM=10
VIDEO_QUALITY=auto
AUDIO_BITRATE=32000
```

### Docker Compose

The stack includes:
- **conference-server**: WebP video conference server (Go)
- **caddy**: Reverse proxy with automatic SSL

## 📊 Performance

| Metric | Value |
|--------|-------|
| Compression | 6-14x |
| Latency | < 200ms |
| Bandwidth | ~100-200 KB/s per user |
| FPS | 10 |
| Audio | 16kHz PCM |

## 🏗️ Architecture

```
Client (Browser)
    ↓ WSS
Caddy (Auto-SSL)
    ↓ WS
Conference Server (Go)
    ↓ WebP Compression
Clients (Browsers)
```

## 📁 Project Structure

```
videocall/
├── index.html           # Client (GitHub Pages)
├── conference-webp.go   # Server (Go)
├── Dockerfile          # Container definition
├── docker-compose.yml  # Stack orchestration
├── Caddyfile          # SSL configuration
└── README.md          # This file
```

## 🔧 Development

### Local Development

1. Run server locally:
```bash
go run conference-webp.go
```

2. Open `index.html` in browser
3. Update WebSocket URL to `ws://localhost:3001/ws`

### Building from Source

```bash
# Build server
go build -o conference-webp conference-webp.go

# Build Docker image
docker build -t webp-conference .
```

## 🚀 Deployment Options

### Option 1: VPS with Docker (Recommended)

Requirements:
- VPS with Docker installed
- Domain name (or use nip.io)
- Ports 80, 443 open

```bash
# Deploy
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

### Option 2: Manual Deployment

1. Build the binary:
```bash
go build conference-webp.go
```

2. Run with systemd or supervisor
3. Configure reverse proxy (nginx/caddy)
4. Setup SSL certificates

## 🎯 Optimizations

The server automatically adjusts based on participants:

| Users | Resolution | Quality | Bandwidth |
|-------|------------|---------|-----------|
| 1 | 640x480 | 80% | ~200 KB/s |
| 2-3 | 320x240 | 60% | ~150 KB/s |
| 4+ | 160x120 | 40% | ~100 KB/s |

## 🔒 Security

- All connections use WSS (WebSocket Secure)
- Automatic SSL via Let's Encrypt
- No peer-to-peer connections
- Server-mediated streaming only

## 📝 License

MIT

## 🤝 Contributing

Pull requests are welcome! Please open an issue first to discuss changes.

## 🙏 Acknowledgments

Built with:
- [Gorilla WebSocket](https://github.com/gorilla/websocket)
- [WebP Library](https://github.com/chai2010/webp)
- [Caddy Server](https://caddyserver.com)
- [Docker](https://docker.com)