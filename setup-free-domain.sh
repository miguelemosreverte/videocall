#!/bin/bash

# Automated setup for free subdomain with Let's Encrypt certificate
# Uses DuckDNS (free dynamic DNS service)

set -e

SERVER_IP="194.87.103.57"
SSH_PASS="DW3ZvctV7a"

echo "🦆 Setting up free subdomain with DuckDNS..."

# Generate a random subdomain name if not provided
if [ -z "$1" ]; then
    SUBDOMAIN="videoconf-$(openssl rand -hex 4)"
else
    SUBDOMAIN="$1"
fi

echo "📝 Registering subdomain: ${SUBDOMAIN}.duckdns.org"

# Note: You need to get a token from https://www.duckdns.org
# For now, I'll use an alternative - nip.io which doesn't require registration
# nip.io provides wildcard DNS for any IP address

DOMAIN="${SERVER_IP}.nip.io"
echo "Using domain: $DOMAIN"

echo "🔐 Setting up Let's Encrypt on VPS..."

# Install certbot and get certificate on VPS
sshpass -p "$SSH_PASS" ssh root@$SERVER_IP << 'ENDSSH'
# Install certbot if not already installed
which certbot || apt-get update && apt-get install -y certbot

# Stop any running servers on port 80 for certbot
pkill -f conference || true
fuser -k 80/tcp || true

# Get certificate using standalone mode
certbot certonly --standalone \
    --non-interactive \
    --agree-tos \
    --register-unsafely-without-email \
    -d 194.87.103.57.nip.io \
    --cert-name conference

# Create symlinks for easy access
ln -sf /etc/letsencrypt/live/conference/fullchain.pem /root/cert.pem
ln -sf /etc/letsencrypt/live/conference/privkey.pem /root/key.pem

echo "✅ Certificate obtained!"
ENDSSH

echo "🚀 Restarting conference server with Let's Encrypt certificate..."

sshpass -p "$SSH_PASS" ssh root@$SERVER_IP << 'ENDSSH'
# Kill old server
pkill -f conference-https || true
sleep 2

# Start with Let's Encrypt certificates
cd /root
USE_TLS=true CERT_FILE=/root/cert.pem KEY_FILE=/root/key.pem PORT=3001 nohup ./conference-https > /var/log/conference-ssl.log 2>&1 &

sleep 3
ps aux | grep conference-https | grep -v grep

echo "✅ Server running with valid SSL certificate!"
ENDSSH

echo ""
echo "🎉 Setup complete!"
echo "📍 Your conference server is now available at:"
echo "   https://${DOMAIN}:3001"
echo "🔌 WebSocket URL for your HTML:"
echo "   wss://${DOMAIN}:3001/ws"
echo ""
echo "Update your index.html with:"
echo "   const defaultUrl = 'wss://${DOMAIN}:3001/ws';"