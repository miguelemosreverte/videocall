#!/bin/bash

# Remote VPS Setup and Validation Script
# This script runs the setup on your VPS and validates it works

set -e

# VPS Configuration
VPS_IP="194.87.103.57"
VPS_USER="root"
VPS_PASS="DW3ZvctV7a"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "==========================================="
echo -e "${BLUE}VPS Video Streaming Server Setup${NC}"
echo "==========================================="
echo "Target VPS: ${VPS_IP}"
echo ""

# First, copy the setup script to VPS
echo -e "${YELLOW}Step 1: Copying setup script to VPS...${NC}"
sshpass -p "${VPS_PASS}" scp -o StrictHostKeyChecking=no \
    /Users/miguel_lemos/Desktop/videocall/vps-setup/setup-vps.sh \
    ${VPS_USER}@${VPS_IP}:/root/setup-vps.sh

# Create a simplified setup script that won't require interaction
echo -e "${YELLOW}Step 2: Creating simplified setup script...${NC}"
cat > /tmp/simple-setup.sh << 'SETUP_SCRIPT'
#!/bin/bash
set -e

echo "Starting VPS setup..."

# Update system
apt-get update -y
apt-get upgrade -y

# Install essential packages
DEBIAN_FRONTEND=noninteractive apt-get install -y \
    curl wget git build-essential \
    nginx ufw htop net-tools \
    docker.io docker-compose

# Install Go
GO_VERSION="1.21.5"
if ! command -v go &> /dev/null; then
    echo "Installing Go..."
    wget -q https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
    rm go${GO_VERSION}.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    export PATH=$PATH:/usr/local/go/bin
fi

# Create directories
mkdir -p /opt/videostream/{server,www,logs}

# Create simple WebSocket server
cat > /opt/videostream/server/main.go << 'EOF'
package main

import (
    "fmt"
    "log"
    "net/http"
    "time"
    "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func handleWS(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Print("upgrade failed: ", err)
        return
    }
    defer conn.Close()
    
    log.Println("Client connected from:", r.RemoteAddr)
    
    // Send welcome message
    conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"welcome","time":"`+time.Now().Format(time.RFC3339)+`"}`))
    
    // Echo messages
    for {
        mt, message, err := conn.ReadMessage()
        if err != nil {
            log.Println("read failed:", err)
            break
        }
        log.Printf("recv: %s", message)
        
        response := fmt.Sprintf(`{"type":"echo","data":%s,"time":"%s"}`, message, time.Now().Format(time.RFC3339))
        if err := conn.WriteMessage(mt, []byte(response)); err != nil {
            log.Println("write failed:", err)
            break
        }
    }
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"status":"healthy","time":"` + time.Now().Format(time.RFC3339) + `"}`))
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>VPS WebSocket Server</title></head>
<body>
<h1>WebSocket Server Running</h1>
<p>WebSocket endpoint: ws://` + r.Host + `/ws</p>
<p>Health check: <a href="/health">/health</a></p>
<p>Server time: ` + time.Now().Format(time.RFC3339) + `</p>
</body>
</html>`))
}

func main() {
    http.HandleFunc("/", handleRoot)
    http.HandleFunc("/ws", handleWS)
    http.HandleFunc("/health", handleHealth)
    
    port := "8080"
    log.Printf("Server starting on port %s", port)
    if err := http.ListenAndServe(":"+port, nil); err != nil {
        log.Fatal("ListenAndServe: ", err)
    }
}
EOF

# Create go.mod
cat > /opt/videostream/server/go.mod << 'EOF'
module videostream
go 1.21
require github.com/gorilla/websocket v1.5.1
EOF

# Build server
cd /opt/videostream/server
/usr/local/go/bin/go mod download
/usr/local/go/bin/go build -o videostream-server main.go

# Create systemd service
cat > /etc/systemd/system/videostream.service << 'EOF'
[Unit]
Description=Video Stream WebSocket Server
After=network.target

[Service]
Type=simple
ExecStart=/opt/videostream/server/videostream-server
Restart=always
WorkingDirectory=/opt/videostream/server
Environment=PORT=8080

[Install]
WantedBy=multi-user.target
EOF

# Start service
systemctl daemon-reload
systemctl enable videostream
systemctl restart videostream

# Configure firewall
ufw --force disable
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 8080/tcp
ufw --force enable

# Configure Nginx
cat > /etc/nginx/sites-available/default << 'EOF'
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /ws {
        proxy_pass http://localhost:8080/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }

    location /health {
        proxy_pass http://localhost:8080/health;
    }
}
EOF

# Restart nginx
systemctl restart nginx

echo "Setup complete!"
SETUP_SCRIPT

# Copy and run the simplified setup
echo -e "${YELLOW}Step 3: Copying simplified setup to VPS...${NC}"
sshpass -p "${VPS_PASS}" scp -o StrictHostKeyChecking=no \
    /tmp/simple-setup.sh ${VPS_USER}@${VPS_IP}:/root/simple-setup.sh

echo -e "${YELLOW}Step 4: Running setup on VPS (this will take a few minutes)...${NC}"
sshpass -p "${VPS_PASS}" ssh -o StrictHostKeyChecking=no ${VPS_USER}@${VPS_IP} << 'REMOTE_EXEC'
chmod +x /root/simple-setup.sh
/root/simple-setup.sh
REMOTE_EXEC

echo ""
echo -e "${GREEN}Step 5: Setup complete! Now validating...${NC}"
echo ""

# Wait for services to start
sleep 5

# Validation function
validate_endpoint() {
    local url=$1
    local name=$2
    
    echo -n "Testing ${name}... "
    
    if response=$(curl -s -f -m 5 "${url}" 2>/dev/null); then
        echo -e "${GREEN}✓ Working${NC}"
        echo "  Response preview: $(echo "$response" | head -c 100)..."
        return 0
    else
        echo -e "${RED}✗ Failed${NC}"
        return 1
    fi
}

echo -e "${BLUE}=== Validation Results ===${NC}"
echo ""

# Test HTTP root
validate_endpoint "http://${VPS_IP}/" "HTTP Root"

# Test health endpoint
validate_endpoint "http://${VPS_IP}/health" "Health Check"

# Test WebSocket endpoint with curl
echo -n "Testing WebSocket endpoint... "
if curl -s -f -m 5 \
    --include \
    --no-buffer \
    --header "Connection: Upgrade" \
    --header "Upgrade: websocket" \
    --header "Sec-WebSocket-Key: x3JJHMbDL1EzLkh9GBhXDw==" \
    --header "Sec-WebSocket-Version: 13" \
    "http://${VPS_IP}/ws" 2>/dev/null | grep -q "101 Switching Protocols"; then
    echo -e "${GREEN}✓ WebSocket upgrade working${NC}"
else
    echo -e "${RED}✗ WebSocket upgrade failed${NC}"
fi

# Check running services via SSH
echo ""
echo -e "${BLUE}=== Service Status ===${NC}"
sshpass -p "${VPS_PASS}" ssh -o StrictHostKeyChecking=no ${VPS_USER}@${VPS_IP} << 'CHECK_SERVICES'
echo -n "Nginx: "
systemctl is-active nginx || true
echo -n "WebSocket Server: "
systemctl is-active videostream || true
echo -n "Server ports: "
netstat -tlnp | grep -E ':(80|8080) ' | awk '{print $4}' | tr '\n' ' '
echo ""
CHECK_SERVICES

# Create test client script
echo ""
echo -e "${BLUE}=== Creating Local Test Client ===${NC}"
cat > /tmp/test-websocket.html << 'HTML'
<!DOCTYPE html>
<html>
<head>
    <title>VPS WebSocket Test</title>
    <style>
        body { font-family: monospace; padding: 20px; }
        #log { border: 1px solid #ccc; height: 300px; overflow-y: auto; padding: 10px; }
        .connected { color: green; }
        .disconnected { color: red; }
        .sent { color: blue; }
        .received { color: purple; }
    </style>
</head>
<body>
    <h1>VPS WebSocket Test Client</h1>
    <div>Status: <span id="status" class="disconnected">Disconnected</span></div>
    <div id="log"></div>
    <input type="text" id="message" placeholder="Type a message...">
    <button onclick="sendMessage()">Send</button>
    <button onclick="connect()">Connect</button>
    <button onclick="disconnect()">Disconnect</button>
    
    <script>
        let ws;
        const VPS_IP = '194.87.103.57';
        
        function log(message, className) {
            const logDiv = document.getElementById('log');
            const entry = document.createElement('div');
            entry.className = className;
            entry.textContent = new Date().toLocaleTimeString() + ' - ' + message;
            logDiv.appendChild(entry);
            logDiv.scrollTop = logDiv.scrollHeight;
        }
        
        function connect() {
            ws = new WebSocket(`ws://${VPS_IP}/ws`);
            
            ws.onopen = function() {
                document.getElementById('status').textContent = 'Connected';
                document.getElementById('status').className = 'connected';
                log('Connected to VPS WebSocket server', 'connected');
            };
            
            ws.onmessage = function(event) {
                log('Received: ' + event.data, 'received');
            };
            
            ws.onclose = function() {
                document.getElementById('status').textContent = 'Disconnected';
                document.getElementById('status').className = 'disconnected';
                log('Disconnected from server', 'disconnected');
            };
            
            ws.onerror = function(error) {
                log('Error: ' + error, 'disconnected');
            };
        }
        
        function disconnect() {
            if (ws) ws.close();
        }
        
        function sendMessage() {
            const input = document.getElementById('message');
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({message: input.value}));
                log('Sent: ' + input.value, 'sent');
                input.value = '';
            } else {
                alert('Not connected!');
            }
        }
        
        // Auto-connect on load
        window.onload = connect;
    </script>
</body>
</html>
HTML

echo "Test client created at: /tmp/test-websocket.html"
echo ""

# Final summary
echo "==========================================="
echo -e "${GREEN}✅ VPS Setup Complete!${NC}"
echo "==========================================="
echo ""
echo "Access Points:"
echo "  • HTTP: http://${VPS_IP}/"
echo "  • WebSocket: ws://${VPS_IP}/ws"
echo "  • Health: http://${VPS_IP}/health"
echo ""
echo "Test locally:"
echo "  • Open: /tmp/test-websocket.html"
echo "  • Or run: wscat -c ws://${VPS_IP}/ws"
echo ""
echo "SSH access:"
echo "  • ssh ${VPS_USER}@${VPS_IP}"
echo "  • Check logs: journalctl -u videostream -f"
echo ""
echo -e "${YELLOW}Note: The server is currently HTTP only.${NC}"
echo -e "${YELLOW}For production, set up SSL with a domain name.${NC}"
echo ""

# Clean up
rm -f /tmp/simple-setup.sh