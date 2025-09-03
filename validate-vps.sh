#!/bin/bash

# VPS WebSocket Server Validation Script
# This script validates that the VPS is properly configured and working

VPS_IP="194.87.103.57"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "==========================================="
echo -e "${BLUE}VPS WebSocket Server Validation${NC}"
echo "==========================================="
echo "VPS IP: ${VPS_IP}"
echo ""

# Test results counter
PASS=0
FAIL=0

# Function to test endpoint
test_endpoint() {
    local name=$1
    local url=$2
    local expected=$3
    
    echo -n "Testing ${name}... "
    
    if response=$(curl -s -f -m 5 "${url}" 2>/dev/null); then
        if echo "$response" | grep -q "$expected" 2>/dev/null; then
            echo -e "${GREEN}✓ PASS${NC}"
            ((PASS++))
            return 0
        else
            echo -e "${RED}✗ FAIL (unexpected response)${NC}"
            echo "  Expected to contain: $expected"
            echo "  Got: $(echo "$response" | head -c 100)..."
            ((FAIL++))
            return 1
        fi
    else
        echo -e "${RED}✗ FAIL (no response)${NC}"
        ((FAIL++))
        return 1
    fi
}

# Function to test WebSocket
test_websocket() {
    local name=$1
    local url=$2
    
    echo -n "Testing ${name}... "
    
    # Test WebSocket upgrade with wscat if available
    if command -v wscat &> /dev/null; then
        if timeout 2 wscat -c "${url}" 2>&1 | grep -q "welcome\|connected" 2>/dev/null; then
            echo -e "${GREEN}✓ PASS (wscat)${NC}"
            ((PASS++))
            return 0
        fi
    fi
    
    # Fallback to curl test
    response=$(curl -s -i -N -m 2 \
        -H "Connection: Upgrade" \
        -H "Upgrade: websocket" \
        -H "Sec-WebSocket-Key: SGVsbG8gV29ybGQh" \
        -H "Sec-WebSocket-Version: 13" \
        "${url}" 2>/dev/null | head -n 1)
    
    if echo "$response" | grep -q "101" 2>/dev/null; then
        echo -e "${GREEN}✓ PASS (upgrade successful)${NC}"
        ((PASS++))
        return 0
    else
        echo -e "${RED}✗ FAIL${NC}"
        echo "  Response: $response"
        ((FAIL++))
        return 1
    fi
}

echo -e "${YELLOW}=== Direct Port 8080 Tests ===${NC}"
test_endpoint "HTTP Root (8080)" "http://${VPS_IP}:8080/" "WebSocket Server Running"
test_endpoint "Health Check (8080)" "http://${VPS_IP}:8080/health" "healthy"
test_websocket "WebSocket (8080)" "ws://${VPS_IP}:8080/ws"

echo ""
echo -e "${YELLOW}=== Nginx Proxy Port 80 Tests ===${NC}"
test_endpoint "HTTP Root (80)" "http://${VPS_IP}/" "WebSocket Server Running"
test_endpoint "Health Check (80)" "http://${VPS_IP}/health" "healthy"
test_websocket "WebSocket (80)" "ws://${VPS_IP}/ws"

echo ""
echo "==========================================="
echo -e "${BLUE}Test Results${NC}"
echo "==========================================="
echo -e "Passed: ${GREEN}${PASS}${NC}"
echo -e "Failed: ${RED}${FAIL}${NC}"

if [ $FAIL -eq 0 ]; then
    echo ""
    echo -e "${GREEN}✅ All tests passed!${NC}"
    echo ""
    echo "Your VPS WebSocket server is ready at:"
    echo "  • HTTP: http://${VPS_IP}/"
    echo "  • WebSocket: ws://${VPS_IP}/ws"
    echo "  • Direct: ws://${VPS_IP}:8080/ws"
    echo ""
    echo "Test locally with:"
    echo "  • Browser: open test-vps-websocket.html"
    echo "  • CLI: wscat -c ws://${VPS_IP}/ws"
else
    echo ""
    echo -e "${RED}⚠️ Some tests failed${NC}"
    echo "Please check the server logs:"
    echo "  ssh root@${VPS_IP} 'journalctl -u videostream -n 50'"
fi

echo ""