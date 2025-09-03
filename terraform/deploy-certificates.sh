#!/bin/bash

# Deploy certificates from Terraform to VPS and restart conference server
# Run this after terraform apply

set -e

echo "📦 Extracting certificates from Terraform..."

# Extract certificates from Terraform output
terraform output -raw certificate_pem > cert.pem
terraform output -raw private_key_pem > key.pem
terraform output -raw full_chain_pem > fullchain.pem

DOMAIN=$(terraform output -raw domain_url | sed 's|https://||')
SERVER_IP="194.87.103.57"
SSH_PASS="DW3ZvctV7a"

echo "🚀 Deploying certificates to VPS..."

# Copy certificates to VPS
sshpass -p "$SSH_PASS" scp -o StrictHostKeyChecking=no cert.pem root@$SERVER_IP:/root/letsencrypt-cert.pem
sshpass -p "$SSH_PASS" scp -o StrictHostKeyChecking=no key.pem root@$SERVER_IP:/root/letsencrypt-key.pem
sshpass -p "$SSH_PASS" scp -o StrictHostKeyChecking=no fullchain.pem root@$SERVER_IP:/root/letsencrypt-fullchain.pem

echo "🔄 Restarting conference server with new certificates..."

# Stop current server and start with Let's Encrypt certificates
sshpass -p "$SSH_PASS" ssh root@$SERVER_IP << 'EOF'
# Kill existing conference server
pkill -f conference-https || true
sleep 2

# Start with Let's Encrypt certificates
cd /root
export USE_TLS=true
export CERT_FILE=/root/letsencrypt-fullchain.pem
export KEY_FILE=/root/letsencrypt-key.pem
export PORT=3001
nohup ./conference-https > /var/log/conference-letsencrypt.log 2>&1 &

sleep 3
# Check if running
ps aux | grep conference-https | grep -v grep
echo "✅ Server restarted with Let's Encrypt certificates"
EOF

# Clean up local certificate files
rm -f cert.pem key.pem fullchain.pem

echo ""
echo "🎉 Deployment complete!"
echo "📍 Domain: $DOMAIN"
echo "🔌 WebSocket URL: wss://$DOMAIN:3001/ws"
echo ""
echo "Update your HTML to use: wss://$DOMAIN:3001/ws"