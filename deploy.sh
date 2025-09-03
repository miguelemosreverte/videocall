#!/bin/bash

echo "🚀 Building and deploying conference server with echo cancellation..."

# Build Docker image
echo "📦 Building Docker image..."
docker build -t webp-conference:latest .

# Save image
echo "💾 Saving Docker image..."
docker save webp-conference:latest | gzip > conference.tar.gz

# Copy files to server
echo "📤 Copying files to Hetzner server..."
scp conference.tar.gz docker-compose.yml Caddyfile root@91.107.208.116:/root/conference/

# Deploy on server
echo "🔄 Deploying on server..."
ssh root@91.107.208.116 << 'EOF'
cd /root/conference

# Stop existing containers
docker-compose down

# Load new image
docker load < conference.tar.gz

# Start services
docker-compose up -d

# Check status
sleep 3
docker-compose ps
docker-compose logs --tail=20
EOF

echo "✅ Deployment complete!"
echo "🌐 Server: wss://91.107.208.116.nip.io/ws"
echo "📄 GitHub Pages: https://miguelemosreverte.github.io/videocall/"

# Clean up local file
rm conference.tar.gz