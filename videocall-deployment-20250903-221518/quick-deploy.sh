#!/bin/bash

# Quick deployment script for Hetzner server
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "====================================="
echo "WebP Conference Quick Deploy"
echo "====================================="

# Stop existing containers
echo -e "${YELLOW}Stopping existing containers...${NC}"
docker-compose down 2>/dev/null || true

# Build new images
echo -e "${YELLOW}Building Docker images...${NC}"
docker-compose build --no-cache

# Start services
echo -e "${YELLOW}Starting services...${NC}"
docker-compose up -d

# Wait a moment for services to start
sleep 5

# Check status
echo -e "${GREEN}Checking service status...${NC}"
docker-compose ps

echo ""
echo -e "${GREEN}======================================"
echo "Deployment Complete!"
echo "======================================"
echo ""
echo "Services should be running on:"
echo "  - HTTP:  http://91.99.159.21"
echo "  - HTTPS: https://91.99.159.21.nip.io"
echo "  - WSS:   wss://91.99.159.21.nip.io/ws"
echo ""
echo "To check logs: docker-compose logs -f"
echo ""
