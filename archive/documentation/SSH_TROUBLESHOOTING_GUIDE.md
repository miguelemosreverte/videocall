# SSH Connection Troubleshooting Guide for Hetzner Server 91.99.159.21

## Current Status
- ❌ SSH password authentication failing
- ❌ SSH key authentication failing  
- ❌ SCP file transfer failing
- ✅ Deployment package created locally

## Possible Causes
1. **Password Changed**: The root password was changed but new password is unknown
2. **SSH Configuration**: SSH server may have disabled password authentication
3. **Key Authentication**: SSH keys not properly configured on server
4. **Account Lockout**: Too many failed attempts may have triggered temporary lockout
5. **Firewall**: Network-level blocking (less likely as connection reaches SSH)

## Solution Methods (In Order of Preference)

### Method 1: Hetzner Cloud Console Password Reset
```bash
# 1. Log into Hetzner Cloud Console: https://console.hetzner.cloud/
# 2. Navigate to your server (91.99.159.21)
# 3. Click "Reset root password" 
# 4. Use the new password provided
# 5. Connect: ssh root@91.99.159.21
```

### Method 2: Hetzner Web Console
```bash
# 1. In Hetzner Cloud Console, click "Console" for your server
# 2. This opens a web-based terminal directly to the server
# 3. Upload deployment files through the web interface
# 4. Execute deployment commands directly in web terminal
```

### Method 3: SSH Key Recovery via Console
```bash
# 1. Access server via Hetzner Web Console
# 2. Add your SSH public key to authorized_keys:
echo "$(cat ~/.ssh/id_ed25519.pub)" >> /root/.ssh/authorized_keys
# 3. Set proper permissions:
chmod 600 /root/.ssh/authorized_keys
chmod 700 /root/.ssh
# 4. Try SSH key connection again
```

### Method 4: Temporary New SSH Key
```bash
# Generate a new key pair specifically for this server
ssh-keygen -t ed25519 -f ~/.ssh/hetzner_91_key -C "hetzner-91.99.159.21"

# Add the public key via Hetzner console or web terminal:
# Copy content of ~/.ssh/hetzner_91_key.pub to server's /root/.ssh/authorized_keys

# Connect with new key:
ssh -i ~/.ssh/hetzner_91_key root@91.99.159.21
```

### Method 5: Alternative File Transfer Methods

#### Using Hetzner's File Manager (if available)
1. Access Hetzner Cloud Console
2. Look for file management features
3. Upload `videocall-deployment-20250903-221518.tar.gz` directly

#### Using SFTP with GUI Client
- **Windows**: WinSCP, FileZilla
- **Mac**: Cyberduck, FileZilla  
- **Linux**: Nautilus (sftp://), FileZilla

#### Using rsync (if SSH keys work)
```bash
rsync -avz -e "ssh -i ~/.ssh/hetzner_key" videocall-deployment-20250903-221518.tar.gz root@91.99.159.21:/root/
```

## Manual Deployment (If File Transfer Impossible)

If you can access the server via web console but can't transfer files, manually create the necessary files:

### Step 1: Create Project Directory
```bash
mkdir -p /root/videocall
cd /root/videocall
```

### Step 2: Create Essential Files

#### docker-compose.yml
```bash
cat > docker-compose.yml << 'EOF'
version: '3.8'

services:
  conference-server:
    build: .
    image: webp-conference:latest
    container_name: webp-conference
    ports:
      - "80:80"
      - "443:443"
      - "3001:3001"
    environment:
      - DOMAIN=91.99.159.21.nip.io
      - USE_SSL=true
      - CERT_EMAIL=admin@91.99.159.21.nip.io
      - MAX_USERS_PER_ROOM=10
      - VIDEO_QUALITY=auto
      - AUDIO_BITRATE=32000
      - BANDWIDTH_LIMIT=0
      - PER_USER_BANDWIDTH=1000
    volumes:
      - ./certs:/app/certs
      - ./logs:/app/logs
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3001/"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
EOF
```

#### Dockerfile
```bash
cat > Dockerfile << 'EOF'
FROM golang:1.23.4-alpine AS builder

RUN apk add --no-cache git gcc musl-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY conference-webp.go .
COPY conference-webp-ssl.go .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o conference-webp conference-webp.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o conference-webp-ssl conference-webp-ssl.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/conference-webp .
COPY --from=builder /build/conference-webp-ssl .
RUN mkdir -p /app/certs
EXPOSE 3001 443 80
CMD ["./conference-webp-ssl"]
EOF
```

### Step 3: Get Source Code
You'll need to manually copy the Go source files. The key files are:
- `conference-webp.go` (main WebP server)
- `conference-webp-ssl.go` (SSL wrapper)
- `go.mod` and `go.sum` (dependencies)

## Recovery Commands

### If Server is Accessible but Deployment Fails
```bash
# Check Docker installation
docker --version
docker-compose --version

# Install Docker if missing
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Install Docker Compose if missing
curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose
```

### Clear SSH Connection Issues
```bash
# On your local machine, clear known hosts
ssh-keygen -R 91.99.159.21

# Restart SSH service on server (via web console)
systemctl restart sshd

# Check SSH configuration
cat /etc/ssh/sshd_config | grep -E "(PasswordAuthentication|PubkeyAuthentication|PermitRootLogin)"
```

## Testing After Resolution

Once you regain access:
```bash
# Test SSH connection
ssh -v root@91.99.159.21

# Transfer deployment package
scp videocall-deployment-20250903-221518.tar.gz root@91.99.159.21:/root/

# Extract and deploy
ssh root@91.99.159.21 << 'EOF'
cd /root
tar -xzf videocall-deployment-20250903-221518.tar.gz
cd videocall-deployment-20250903-221518
chmod +x *.sh
./quick-deploy.sh
EOF
```

## Expected Results After Successful Deployment

The server should respond on:
- **HTTP**: http://91.99.159.21 (port 80)
- **HTTPS**: https://91.99.159.21.nip.io (port 443) 
- **WebSocket**: wss://91.99.159.21.nip.io/ws (port 443)

## Contact Information

If you need additional assistance, the deployment package includes:
- Complete troubleshooting documentation
- Multiple deployment scripts
- Health verification tools
- Log analysis commands

All files are ready in: `videocall-deployment-20250903-221518/`