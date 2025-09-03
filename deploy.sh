#!/bin/bash

echo "🚀 Building and deploying conference server with echo cancellation..."

# Build Docker image
echo "📦 Building Docker image..."
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_BY="Manual Deploy"
BUILD_REF=$(git branch --show-current 2>/dev/null || echo "unknown")

docker build -t webp-conference:latest \
  --build-arg BUILD_TIME="$BUILD_TIME" \
  --build-arg BUILD_COMMIT="$BUILD_COMMIT" \
  --build-arg BUILD_BY="$BUILD_BY" \
  --build-arg BUILD_REF="$BUILD_REF" \
  .

# Save image
echo "💾 Saving Docker image..."
docker save webp-conference:latest | gzip > conference.tar.gz

# Copy files to server
echo "📤 Copying files to Hetzner server..."
scp conference.tar.gz docker-compose.yml Caddyfile root@91.99.159.21:/root/conference/

# Deploy on server
echo "🔄 Deploying on server..."
ssh root@91.99.159.21 << 'EOF'
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
echo "🌐 Server: wss://91.99.159.21.nip.io/ws"
echo "📄 GitHub Pages: https://miguelemosreverte.github.io/videocall/"

# Clean up local file
rm conference.tar.gz