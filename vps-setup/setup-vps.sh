#!/bin/bash

# Video Streaming VPS Setup Script
# For Ubuntu 22.04/24.04 VPS
# IP: 194.87.103.57

set -e

echo "==========================================="
echo "Video Streaming VPS Setup"
echo "==========================================="

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

echo -e "${GREEN}Step 1: Updating system packages...${NC}"
apt update && apt upgrade -y

echo -e "${GREEN}Step 2: Installing essential packages...${NC}"
apt install -y \
    curl \
    wget \
    git \
    build-essential \
    nginx \
    certbot \
    python3-certbot-nginx \
    ufw \
    htop \
    net-tools \
    docker.io \
    docker-compose

# Enable Docker
systemctl enable docker
systemctl start docker

echo -e "${GREEN}Step 3: Installing Go (for high-performance WebSocket server)...${NC}"
GO_VERSION="1.21.5"
wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
rm go${GO_VERSION}.linux-amd64.tar.gz

# Add Go to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
export PATH=$PATH:/usr/local/go/bin

echo -e "${GREEN}Step 4: Installing Node.js (for signaling server)...${NC}"
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt install -y nodejs

echo -e "${GREEN}Step 5: Setting up firewall...${NC}"
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP
ufw allow 443/tcp   # HTTPS
ufw allow 8080/tcp  # WebSocket HTTP
ufw allow 8443/tcp  # WebSocket HTTPS
ufw allow 3478/tcp  # STUN
ufw allow 3478/udp  # STUN
ufw allow 49152:65535/udp  # WebRTC media ports
echo "y" | ufw enable

echo -e "${GREEN}Step 6: Creating application user...${NC}"
# Create non-root user for running services
if ! id "videostream" &>/dev/null; then
    useradd -m -s /bin/bash videostream
    usermod -aG docker videostream
    echo -e "${YELLOW}Created user 'videostream' for running services${NC}"
fi

echo -e "${GREEN}Step 7: Creating directory structure...${NC}"
mkdir -p /opt/videostream/{server,config,logs,data}
chown -R videostream:videostream /opt/videostream

echo -e "${GREEN}Step 8: Installing Coturn (TURN/STUN server)...${NC}"
apt install -y coturn

# Configure Coturn
cat > /etc/turnserver.conf << 'EOF'
# TURN server configuration
listening-port=3478
fingerprint
lt-cred-mech
realm=videocall.yourdomain.com
# Generate these with: openssl rand -hex 32
static-auth-secret=CHANGE_THIS_TO_RANDOM_SECRET
server-name=videocall.yourdomain.com
total-quota=100
stale-nonce=600
no-multicast-peers
no-stdout-log
log-file=/var/log/turnserver.log
EOF

# Enable Coturn
sed -i 's/#TURNSERVER_ENABLED/TURNSERVER_ENABLED/' /etc/default/coturn
systemctl enable coturn
systemctl restart coturn

echo -e "${GREEN}Step 9: Creating WebSocket server...${NC}"
cat > /opt/videostream/server/main.go << 'EOF'
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "sync"
    "time"

    "github.com/gorilla/websocket"
)

type Hub struct {
    clients    map[string]*Client
    rooms      map[string]map[string]*Client
    register   chan *Client
    unregister chan *Client
    broadcast  chan Message
    mu         sync.RWMutex
}

type Client struct {
    id     string
    room   string
    conn   *websocket.Conn
    send   chan []byte
    hub    *Hub
}

type Message struct {
    Type      string          `json:"type"`
    Room      string          `json:"room"`
    From      string          `json:"from"`
    To        string          `json:"to,omitempty"`
    Data      json.RawMessage `json:"data"`
    Timestamp int64           `json:"timestamp"`
}

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024 * 64,  // 64KB for video data
    WriteBufferSize: 1024 * 64,
    CheckOrigin: func(r *http.Request) bool {
        return true // Configure properly in production
    },
}

func NewHub() *Hub {
    return &Hub{
        clients:    make(map[string]*Client),
        rooms:      make(map[string]map[string]*Client),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        broadcast:  make(chan Message, 256),
    }
}

func (h *Hub) run() {
    for {
        select {
        case client := <-h.register:
            h.mu.Lock()
            h.clients[client.id] = client
            if h.rooms[client.room] == nil {
                h.rooms[client.room] = make(map[string]*Client)
            }
            h.rooms[client.room][client.id] = client
            h.mu.Unlock()
            
            log.Printf("Client %s joined room %s", client.id, client.room)
            h.notifyRoomMembers(client.room, client.id, "joined")

        case client := <-h.unregister:
            h.mu.Lock()
            if _, ok := h.clients[client.id]; ok {
                delete(h.clients, client.id)
                if room, ok := h.rooms[client.room]; ok {
                    delete(room, client.id)
                    if len(room) == 0 {
                        delete(h.rooms, client.room)
                    }
                }
                close(client.send)
                h.mu.Unlock()
                log.Printf("Client %s left room %s", client.id, client.room)
                h.notifyRoomMembers(client.room, client.id, "left")
            } else {
                h.mu.Unlock()
            }

        case message := <-h.broadcast:
            h.handleMessage(message)
        }
    }
}

func (h *Hub) handleMessage(msg Message) {
    h.mu.RLock()
    defer h.mu.RUnlock()

    data, _ := json.Marshal(msg)
    
    if msg.To != "" {
        // Direct message
        if client, ok := h.clients[msg.To]; ok {
            select {
            case client.send <- data:
            default:
                close(client.send)
                delete(h.clients, msg.To)
            }
        }
    } else if msg.Room != "" {
        // Room broadcast
        if room, ok := h.rooms[msg.Room]; ok {
            for id, client := range room {
                if id != msg.From {
                    select {
                    case client.send <- data:
                    default:
                        close(client.send)
                        delete(h.clients, id)
                        delete(room, id)
                    }
                }
            }
        }
    }
}

func (h *Hub) notifyRoomMembers(room, clientID, action string) {
    h.mu.RLock()
    members := make([]string, 0)
    if r, ok := h.rooms[room]; ok {
        for id := range r {
            members = append(members, id)
        }
    }
    h.mu.RUnlock()

    msg := Message{
        Type:      action,
        Room:      room,
        From:      "server",
        Data:      json.RawMessage(fmt.Sprintf(`{"clientId":"%s","members":%s}`, clientID, toJSON(members))),
        Timestamp: time.Now().Unix(),
    }
    
    h.broadcast <- msg
}

func toJSON(v interface{}) string {
    b, _ := json.Marshal(v)
    return string(b)
}

func (c *Client) readPump() {
    defer func() {
        c.hub.unregister <- c
        c.conn.Close()
    }()

    c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    c.conn.SetPongHandler(func(string) error {
        c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
        return nil
    })

    for {
        _, data, err := c.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("websocket error: %v", err)
            }
            break
        }

        var msg Message
        if err := json.Unmarshal(data, &msg); err != nil {
            log.Printf("json unmarshal error: %v", err)
            continue
        }

        msg.From = c.id
        msg.Room = c.room
        msg.Timestamp = time.Now().Unix()

        c.hub.broadcast <- msg
    }
}

func (c *Client) writePump() {
    ticker := time.NewTicker(54 * time.Second)
    defer func() {
        ticker.Stop()
        c.conn.Close()
    }()

    for {
        select {
        case message, ok := <-c.send:
            c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if !ok {
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            c.conn.WriteMessage(websocket.TextMessage, message)

        case <-ticker.C:
            c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}

func handleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
    room := r.URL.Query().Get("room")
    if room == "" {
        room = "default"
    }

    clientID := r.URL.Query().Get("id")
    if clientID == "" {
        clientID = fmt.Sprintf("client_%d", time.Now().UnixNano())
    }

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println(err)
        return
    }

    client := &Client{
        id:   clientID,
        room: room,
        hub:  hub,
        conn: conn,
        send: make(chan []byte, 256),
    }

    client.hub.register <- client

    go client.writePump()
    go client.readPump()
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    hub := NewHub()
    go hub.run()

    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        handleWebSocket(hub, w, r)
    })
    http.HandleFunc("/health", handleHealth)

    log.Printf("WebSocket server starting on port %s", port)
    if err := http.ListenAndServe(":"+port, nil); err != nil {
        log.Fatal("ListenAndServe: ", err)
    }
}
EOF

# Create go.mod
cat > /opt/videostream/server/go.mod << 'EOF'
module videostream-server

go 1.21

require github.com/gorilla/websocket v1.5.1
EOF

# Build the server
cd /opt/videostream/server
go mod download
go build -o videostream-server main.go
chown -R videostream:videostream /opt/videostream

echo -e "${GREEN}Step 10: Creating systemd service...${NC}"
cat > /etc/systemd/system/videostream.service << 'EOF'
[Unit]
Description=Video Stream WebSocket Server
After=network.target

[Service]
Type=simple
User=videostream
WorkingDirectory=/opt/videostream/server
ExecStart=/opt/videostream/server/videostream-server
Restart=always
RestartSec=10
Environment=PORT=8080

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable videostream
systemctl start videostream

echo -e "${GREEN}Step 11: Configuring Nginx...${NC}"
cat > /etc/nginx/sites-available/videostream << 'EOF'
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}

server {
    listen 80;
    server_name 194.87.103.57;

    location / {
        return 301 https://$server_name$request_uri;
    }
}

server {
    listen 443 ssl http2;
    server_name 194.87.103.57;

    # SSL will be configured by certbot
    
    # WebSocket configuration
    location /ws {
        proxy_pass http://localhost:8080/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 7d;
        proxy_send_timeout 7d;
        proxy_read_timeout 7d;
    }

    location /health {
        proxy_pass http://localhost:8080/health;
    }

    # Static files
    location / {
        root /opt/videostream/www;
        index index.html;
        try_files $uri $uri/ =404;
    }
}
EOF

ln -sf /etc/nginx/sites-available/videostream /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx

echo -e "${GREEN}Step 12: Creating test webpage...${NC}"
mkdir -p /opt/videostream/www
cat > /opt/videostream/www/index.html << 'EOF'
<!DOCTYPE html>
<html>
<head>
    <title>Video Streaming Server</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #1a1a1a;
            color: white;
        }
        .status {
            padding: 10px;
            border-radius: 5px;
            margin: 10px 0;
        }
        .connected { background: #2d5016; }
        .disconnected { background: #5c1616; }
        button {
            background: #4CAF50;
            color: white;
            border: none;
            padding: 10px 20px;
            margin: 5px;
            border-radius: 5px;
            cursor: pointer;
        }
        button:hover { background: #45a049; }
        #messages {
            background: #2a2a2a;
            padding: 10px;
            height: 300px;
            overflow-y: auto;
            border-radius: 5px;
            margin: 20px 0;
        }
        input[type="text"] {
            padding: 10px;
            width: 300px;
            margin: 5px;
            border-radius: 5px;
            border: 1px solid #444;
            background: #2a2a2a;
            color: white;
        }
    </style>
</head>
<body>
    <h1>Video Streaming Server - Test Page</h1>
    
    <div id="status" class="status disconnected">Disconnected</div>
    
    <div>
        <input type="text" id="roomInput" placeholder="Room name" value="test-room">
        <input type="text" id="clientIdInput" placeholder="Your ID" value="">
        <button onclick="connect()">Connect</button>
        <button onclick="disconnect()">Disconnect</button>
    </div>
    
    <div id="messages"></div>
    
    <div>
        <input type="text" id="messageInput" placeholder="Type a message...">
        <button onclick="sendMessage()">Send</button>
    </div>

    <script>
        let ws = null;
        let clientId = 'client_' + Math.random().toString(36).substr(2, 9);
        document.getElementById('clientIdInput').value = clientId;

        function connect() {
            if (ws) {
                ws.close();
            }

            const room = document.getElementById('roomInput').value || 'test-room';
            const id = document.getElementById('clientIdInput').value || clientId;
            
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = `${protocol}//${window.location.host}/ws?room=${room}&id=${id}`;
            
            ws = new WebSocket(wsUrl);
            
            ws.onopen = function() {
                document.getElementById('status').className = 'status connected';
                document.getElementById('status').textContent = `Connected to room: ${room}`;
                addMessage('System', 'Connected to server');
            };
            
            ws.onmessage = function(event) {
                const msg = JSON.parse(event.data);
                if (msg.type === 'joined') {
                    addMessage('System', `User joined: ${JSON.parse(msg.data).clientId}`);
                } else if (msg.type === 'left') {
                    addMessage('System', `User left: ${JSON.parse(msg.data).clientId}`);
                } else {
                    addMessage(msg.from, JSON.stringify(msg.data));
                }
            };
            
            ws.onclose = function() {
                document.getElementById('status').className = 'status disconnected';
                document.getElementById('status').textContent = 'Disconnected';
                addMessage('System', 'Disconnected from server');
            };
            
            ws.onerror = function(error) {
                addMessage('System', 'Error: ' + error);
            };
        }

        function disconnect() {
            if (ws) {
                ws.close();
                ws = null;
            }
        }

        function sendMessage() {
            const input = document.getElementById('messageInput');
            if (ws && ws.readyState === WebSocket.OPEN && input.value) {
                const msg = {
                    type: 'message',
                    data: input.value
                };
                ws.send(JSON.stringify(msg));
                addMessage('You', input.value);
                input.value = '';
            }
        }

        function addMessage(from, text) {
            const messages = document.getElementById('messages');
            const msgDiv = document.createElement('div');
            msgDiv.innerHTML = `<strong>${from}:</strong> ${text}`;
            messages.appendChild(msgDiv);
            messages.scrollTop = messages.scrollHeight;
        }

        document.getElementById('messageInput').addEventListener('keypress', function(e) {
            if (e.key === 'Enter') sendMessage();
        });
    </script>
</body>
</html>
EOF

chown -R videostream:videostream /opt/videostream/www

echo -e "${GREEN}Step 13: Setting up monitoring...${NC}"
cat > /opt/videostream/check-services.sh << 'EOF'
#!/bin/bash
# Health check script

check_service() {
    if systemctl is-active --quiet $1; then
        echo "✓ $1 is running"
    else
        echo "✗ $1 is not running"
        systemctl start $1
    fi
}

echo "Service Status:"
check_service nginx
check_service videostream
check_service coturn

# Check WebSocket server
if curl -s http://localhost:8080/health > /dev/null; then
    echo "✓ WebSocket server is responding"
else
    echo "✗ WebSocket server is not responding"
fi

# Check ports
echo -e "\nOpen Ports:"
netstat -tlnp | grep -E ':(22|80|443|8080|3478) '
EOF

chmod +x /opt/videostream/check-services.sh

echo "==========================================="
echo -e "${GREEN}Installation Complete!${NC}"
echo "==========================================="
echo ""
echo "Your video streaming server is ready!"
echo ""
echo "Services running:"
echo "  - WebSocket Server: http://194.87.103.57:8080/ws"
echo "  - Web Interface: http://194.87.103.57"
echo "  - TURN Server: turn:194.87.103.57:3478"
echo ""
echo "Next steps:"
echo "  1. Set up a domain name pointing to 194.87.103.57"
echo "  2. Run: certbot --nginx -d yourdomain.com"
echo "  3. Update TURN server configuration with your domain"
echo ""
echo "Check services: /opt/videostream/check-services.sh"
echo "View logs: journalctl -u videostream -f"
echo ""
echo -e "${YELLOW}IMPORTANT: Change the TURN server secret in /etc/turnserver.conf${NC}"
echo ""