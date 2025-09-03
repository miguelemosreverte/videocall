#!/bin/bash

# Remote deployment script for WebP Conference Server
# Run this on the Hetzner server after copying files

set -e

echo "======================================"
echo "WebP Conference Server Deployment"
echo "======================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo -e "${RED}Please run as root${NC}"
    exit 1
fi

# Navigate to project directory
cd /root/videocall || {
    echo -e "${RED}Project directory not found!${NC}"
    echo "Please ensure the repository is cloned to /root/videocall"
    exit 1
}

echo -e "${GREEN}Step 1: Fixing Dockerfile Go version...${NC}"
sed -i 's/golang:1.21-alpine/golang:1.23.4-alpine/' Dockerfile

echo -e "${GREEN}Step 2: Building Docker images...${NC}"
docker-compose build

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Docker images built successfully${NC}"
else
    echo -e "${RED}✗ Docker build failed${NC}"
    exit 1
fi

echo -e "${GREEN}Step 3: Stopping any existing containers...${NC}"
docker-compose down

echo -e "${GREEN}Step 4: Starting WebP Conference Server...${NC}"
docker-compose up -d

echo -e "${GREEN}Step 5: Checking container status...${NC}"
sleep 3
docker-compose ps

echo -e "${GREEN}Step 6: Checking logs...${NC}"
docker-compose logs --tail=20

echo ""
echo -e "${GREEN}======================================"
echo "Deployment Complete!"
echo "======================================"
echo ""
echo "Server should be accessible at:"
echo "  - HTTP:  http://91.99.159.21"
echo "  - HTTPS: https://91.99.159.21.nip.io"
echo "  - WSS:   wss://91.99.159.21.nip.io/ws"
echo ""
echo "Useful commands:"
echo "  - View logs:    docker-compose logs -f"
echo "  - Stop server:  docker-compose down"
echo "  - Restart:      docker-compose restart"
echo "  - Status:       docker-compose ps"
echo ""