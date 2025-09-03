package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now (configure properly in production)
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	id     string
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

var hub = Hub{
	clients:    make(map[*Client]bool),
	broadcast:  make(chan []byte),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Printf("Client connected: %s (Total: %d)", client.id, len(h.clients))
			
			// Send welcome message
			welcome := fmt.Sprintf(`{"type":"welcome","message":"Hello from WebSocket server!","time":"%s"}`, 
				time.Now().Format(time.RFC3339))
			client.send <- []byte(welcome)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Printf("Client disconnected: %s (Total: %d)", client.id, len(h.clients))
			}

		case message := <-h.broadcast:
			// Broadcast message to all clients
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client's send channel is full, close it
					delete(h.clients, client)
					close(client.send)
				}
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		// Echo the message back with timestamp
		response := fmt.Sprintf(`{"type":"echo","original":%s,"server_time":"%s"}`, 
			string(message), time.Now().Format(time.RFC3339))
		
		// Broadcast to all clients
		hub.broadcast <- []byte(response)
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

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	clientID := fmt.Sprintf("client_%d", time.Now().UnixNano())
	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
		id:   clientID,
	}

	hub.register <- client

	go client.writePump()
	go client.readPump()
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>WebSocket Test</title>
    <style>
        body { font-family: monospace; padding: 20px; }
        #messages { border: 1px solid #ccc; height: 300px; overflow-y: auto; padding: 10px; margin: 10px 0; }
        input { width: 300px; padding: 5px; }
        button { padding: 5px 10px; }
        .sent { color: blue; }
        .received { color: green; }
        .system { color: red; }
    </style>
</head>
<body>
    <h1>WebSocket Hello World Test</h1>
    <div id="status">Connecting...</div>
    <div id="messages"></div>
    <input type="text" id="messageInput" placeholder="Type a message..." />
    <button onclick="sendMessage()">Send</button>
    <button onclick="connect()">Reconnect</button>
    
    <script>
        let ws;
        const messages = document.getElementById('messages');
        const status = document.getElementById('status');
        const input = document.getElementById('messageInput');
        
        function addMessage(msg, className) {
            const div = document.createElement('div');
            div.className = className;
            div.textContent = new Date().toLocaleTimeString() + ' - ' + msg;
            messages.appendChild(div);
            messages.scrollTop = messages.scrollHeight;
        }
        
        function connect() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.host + '/ws';
            
            ws = new WebSocket(wsUrl);
            
            ws.onopen = function() {
                status.textContent = 'Connected!';
                status.style.color = 'green';
                addMessage('Connected to server', 'system');
            };
            
            ws.onmessage = function(event) {
                addMessage('Received: ' + event.data, 'received');
            };
            
            ws.onclose = function() {
                status.textContent = 'Disconnected';
                status.style.color = 'red';
                addMessage('Disconnected from server', 'system');
            };
            
            ws.onerror = function(error) {
                addMessage('Error: ' + error, 'system');
            };
        }
        
        function sendMessage() {
            if (ws && ws.readyState === WebSocket.OPEN) {
                const msg = input.value;
                if (msg) {
                    ws.send(JSON.stringify({message: msg}));
                    addMessage('Sent: ' + msg, 'sent');
                    input.value = '';
                }
            } else {
                alert('Not connected!');
            }
        }
        
        input.addEventListener('keypress', function(e) {
            if (e.key === 'Enter') sendMessage();
        });
        
        // Connect on load
        connect();
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start hub
	go hub.run()

	// Routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/health", handleHealth)

	log.Printf("WebSocket server starting on port %s", port)
	log.Printf("Visit http://localhost:%s to test", port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}