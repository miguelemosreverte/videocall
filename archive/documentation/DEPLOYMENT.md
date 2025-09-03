# 🚀 WebP Conference Server - Docker Deployment Guide

## Overview

This guide will help you deploy the WebP-optimized video conference server on any server using Docker. The server features:

- **6-14x WebP compression** for video frames
- **Adaptive quality** based on user count
- **Protected audio channel**
- **Automatic SSL/TLS** with Let's Encrypt
- **Bandwidth optimization** for limited connections

## 📋 Prerequisites

1. **Server Requirements**:
   - Linux server (Ubuntu, Debian, CentOS, etc.)
   - Minimum 2GB RAM
   - Docker and Docker Compose installed
   - Ports 80 and 443 available (for HTTPS)
   - Domain name pointing to your server (for SSL)

2. **Install Docker** (if not already installed):
```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Install Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose
```

## 🎯 Quick Start

### 1. Clone the Repository
```bash
git clone https://github.com/miguelemosreverte/videocall.git
cd videocall
```

### 2. Configure Environment
```bash
# Copy environment template
cp .env.example .env

# Edit configuration
nano .env
```

Update these values in `.env`:
```env
DOMAIN=your-domain.com      # Your server's domain
USE_SSL=true                # Enable HTTPS/WSS
CERT_EMAIL=you@email.com    # Email for Let's Encrypt
BANDWIDTH_LIMIT=0           # 0 for unlimited, or set in kbps
```

### 3. Deploy with One Command
```bash
# Build and start the server
./deploy.sh build
./deploy.sh start
```

That's it! Your server is now running at `https://your-domain.com`

## 📦 Deployment Commands

The `deploy.sh` script provides all deployment commands:

```bash
./deploy.sh build    # Build Docker image
./deploy.sh start    # Start the server
./deploy.sh stop     # Stop the server
./deploy.sh restart  # Restart the server
./deploy.sh logs     # View real-time logs
./deploy.sh status   # Check server status
./deploy.sh clean    # Remove all Docker resources
```

## 🔧 Configuration Options

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN` | localhost | Your server's domain name |
| `USE_SSL` | false | Enable HTTPS/WSS with Let's Encrypt |
| `CERT_EMAIL` | admin@example.com | Email for SSL certificates |
| `MAX_USERS_PER_ROOM` | 10 | Maximum users per conference room |
| `VIDEO_QUALITY` | auto | Video quality: auto, high, medium, low |
| `AUDIO_BITRATE` | 32000 | Audio bitrate in bits per second |
| `BANDWIDTH_LIMIT` | 0 | Total bandwidth limit (0 = unlimited) |
| `PER_USER_BANDWIDTH` | 1000 | Bandwidth per user in kbps |

### Compression Levels by User Count

The server automatically adjusts compression based on participants:

| Users | Resolution | Quality | Compression Ratio |
|-------|------------|---------|-------------------|
| 1 | 320x240 | 75% | ~7x |
| 2 | 240x180 | 65% | ~10x |
| 3 | 180x135 | 55% | ~7x |
| 4 | 120x90 | 45% | ~8-9x |
| 6+ | 80x60 | 25% gray | ~14x |

## 🌐 Client Configuration

Update your HTML client to connect to your server:

```javascript
// In index.html, update the WebSocket URL:
const WS_URL = 'wss://your-domain.com/ws';
```

Or use environment-based configuration:

```javascript
const WS_URL = window.location.protocol === 'https:' 
  ? `wss://${window.location.host}/ws`
  : `ws://${window.location.host}/ws`;
```

## 🔒 SSL/TLS Setup

### Automatic SSL with Let's Encrypt

When `USE_SSL=true` and you have a valid domain, the server automatically:
1. Obtains SSL certificates from Let's Encrypt
2. Sets up HTTPS on port 443
3. Configures WSS for WebSocket connections
4. Auto-renews certificates

### Manual SSL (Optional)

If you have your own certificates:

1. Place certificates in `./certs/` directory:
   - `cert.pem` - Certificate file
   - `key.pem` - Private key file

2. Mount in docker-compose.yml:
```yaml
volumes:
  - ./certs:/app/certs
```

## 📊 Monitoring

### View Logs
```bash
# All logs
./deploy.sh logs

# Only errors
docker-compose logs --tail=100 | grep ERROR

# Follow specific container
docker logs -f webp-conference
```

### Check Statistics
```bash
# Server stats endpoint
curl http://your-domain.com:3001/stats

# Docker resource usage
docker stats webp-conference
```

## 🚨 Troubleshooting

### Common Issues

1. **Port Already in Use**
```bash
# Check what's using port 443
sudo lsof -i :443

# Kill the process or change ports in docker-compose.yml
```

2. **SSL Certificate Issues**
```bash
# Check certificate status
docker exec webp-conference ls -la /app/certs

# Force certificate renewal
docker exec webp-conference rm -rf /app/certs/*
./deploy.sh restart
```

3. **WebSocket Connection Failed**
```bash
# Check if server is running
./deploy.sh status

# Test WebSocket endpoint
curl -i -N -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: SGVsbG8sIHdvcmxkIQ==" \
  https://your-domain.com/ws
```

4. **High CPU/Memory Usage**
```bash
# Limit resources in docker-compose.yml
services:
  conference-server:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
```

## 🔄 Updates and Maintenance

### Update to Latest Version
```bash
git pull origin main
./deploy.sh build
./deploy.sh restart
```

### Backup Configuration
```bash
# Backup certificates and config
tar -czf backup-$(date +%Y%m%d).tar.gz .env certs/ logs/

# Restore from backup
tar -xzf backup-20250903.tar.gz
```

### Clean Up Old Logs
```bash
# Remove logs older than 7 days
find ./logs -name "*.log" -mtime +7 -delete

# Or use Docker's log rotation
docker-compose down
docker-compose up -d
```

## 🎯 Performance Optimization

### For Limited Bandwidth (like 1.2 Mbps VPS)
```env
BANDWIDTH_LIMIT=1200
PER_USER_BANDWIDTH=300
VIDEO_QUALITY=low
```

### For High-Performance Servers
```env
BANDWIDTH_LIMIT=0
PER_USER_BANDWIDTH=2000
VIDEO_QUALITY=high
MAX_USERS_PER_ROOM=20
```

### For Many Users with Low Quality
```env
BANDWIDTH_LIMIT=10000
PER_USER_BANDWIDTH=150
VIDEO_QUALITY=low
MAX_USERS_PER_ROOM=50
```

## 📝 Advanced Configuration

### Custom Nginx Proxy (Optional)

To use Nginx as a reverse proxy:

```bash
# Start with nginx profile
docker-compose --profile with-nginx up -d

# Configure nginx.conf for your needs
nano nginx.conf
```

### Horizontal Scaling

For multiple servers:

1. Use a load balancer (HAProxy, Nginx)
2. Share session state with Redis
3. Use consistent hashing for room assignment

## 🆘 Support

- **Issues**: [GitHub Issues](https://github.com/miguelemosreverte/videocall/issues)
- **Logs**: Check `./logs/` directory
- **Stats**: `http://your-domain.com:3001/stats`

## 📄 License

MIT License - See LICENSE file for details

---

**Ready to deploy?** Run `./deploy.sh start` and your WebP conference server will be live in seconds! 🚀