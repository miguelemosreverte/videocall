# WebP Conference Server - Hetzner Deployment Instructions

## Overview
This package contains all necessary files to deploy the WebP Conference Server on your Hetzner server at 91.99.159.21.

## Prerequisites
- Docker and Docker Compose installed on the server
- Root access to the server
- Ports 80, 443, and 3001 available

## Quick Deployment Steps

### Method 1: If you have SSH access
```bash
# 1. Copy this entire folder to the server
scp -r videocall-deployment-* root@91.99.159.21:/root/videocall

# 2. SSH into the server
ssh root@91.99.159.21

# 3. Navigate to the directory
cd /root/videocall

# 4. Run the quick deployment
./quick-deploy.sh
```

### Method 2: Manual deployment (if SSH has issues)
```bash
# 1. Copy files using alternative methods (SFTP, control panel file manager, etc.)
# 2. Once files are on the server, execute these commands:

cd /root/videocall
chmod +x *.sh
./quick-deploy.sh
```

### Method 3: Step-by-step manual deployment
```bash
# Navigate to project directory
cd /root/videocall

# Stop any existing containers
docker-compose down

# Build the images
docker-compose build --no-cache

# Start the services
docker-compose up -d

# Verify deployment
./verify-server.sh
```

## Verification

After deployment, the server should be accessible at:
- **HTTP**: http://91.99.159.21
- **HTTPS**: https://91.99.159.21.nip.io  
- **WebSocket**: wss://91.99.159.21.nip.io/ws

## Troubleshooting

### If containers fail to start:
```bash
# Check logs
docker-compose logs

# Check system resources
df -h
free -m

# Restart Docker service
systemctl restart docker
```

### If ports are not accessible:
```bash
# Check if ports are listening
netstat -tlnp | grep -E "(80|443|3001)"

# Check firewall (if applicable)
ufw status
iptables -L
```

### If SSL certificates fail:
```bash
# Check certificate directory
ls -la /root/videocall/certs/

# Manually restart with HTTP only
docker-compose down
# Edit .env file and set USE_SSL=false
docker-compose up -d
```

## Alternative SSH Connection Methods

If you're having trouble with SSH authentication, try:

### Method 1: Reset root password via Hetzner console
1. Log into Hetzner Cloud Console
2. Go to your server
3. Click "Reset root password"
4. Use the new password for SSH

### Method 2: Use Hetzner Web Console
1. Access the web console from Hetzner panel
2. Upload files directly through the web interface
3. Execute commands through the web terminal

### Method 3: Create new SSH key
```bash
# Generate new SSH key pair
ssh-keygen -t ed25519 -f ~/.ssh/hetzner_key

# Add public key to server via Hetzner console
# Then connect with:
ssh -i ~/.ssh/hetzner_key root@91.99.159.21
```

## Files Included
- `conference-webp.go` - Main WebP conference server
- `conference-webp-ssl.go` - SSL wrapper
- `Dockerfile` - Container definition  
- `docker-compose.yml` - Service orchestration
- `.env` - Environment configuration
- `quick-deploy.sh` - One-command deployment
- `verify-server.sh` - Health check script
- `deploy.sh` - Full deployment script with options
- `remote-deploy.sh` - Original remote deployment script

## Support Commands
```bash
# View real-time logs
docker-compose logs -f

# Check container status
docker-compose ps

# Restart services
docker-compose restart

# Stop services
docker-compose down

# Rebuild and restart
docker-compose down && docker-compose build --no-cache && docker-compose up -d
```
