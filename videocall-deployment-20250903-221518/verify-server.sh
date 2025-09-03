#!/bin/bash

# Server verification script
echo "Verifying WebP Conference Server..."

# Check if containers are running
echo "Container Status:"
docker-compose ps

echo ""
echo "Service Health Checks:"

# Test HTTP
echo -n "HTTP (port 80): "
if curl -s -o /dev/null -w "%{http_code}" http://localhost:80 | grep -q "200\|301\|302"; then
    echo -e "\033[0;32mOK\033[0m"
else
    echo -e "\033[0;31mFAILED\033[0m"
fi

# Test HTTPS
echo -n "HTTPS (port 443): "
if curl -k -s -o /dev/null -w "%{http_code}" https://localhost:443 | grep -q "200\|301\|302"; then
    echo -e "\033[0;32mOK\033[0m"
else
    echo -e "\033[0;31mFAILED\033[0m"
fi

# Test WebSocket
echo -n "WebSocket (port 3001): "
if curl -s -o /dev/null -w "%{http_code}" http://localhost:3001 | grep -q "200\|400\|426"; then
    echo -e "\033[0;32mOK\033[0m"
else
    echo -e "\033[0;31mFAILED\033[0m"
fi

echo ""
echo "To view logs: docker-compose logs -f"
