package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message types
type Message struct {
	Type        string          `json:"type"`
	ID          string          `json:"id,omitempty"`
	Room        string          `json:"room,omitempty"`
	From        string          `json:"from,omitempty"`
	Data        string          `json:"data,omitempty"`
	Timestamp   int64           `json:"timestamp,omitempty"`
	Samples     int             `json:"samples,omitempty"`
	SampleRate  int             `json:"sampleRate,omitempty"`
	Sequence    int             `json:"sequence,omitempty"`
	Resolution  json.RawMessage `json:"resolution,omitempty"`
	YourId      string          `json:"yourId,omitempty"`
	Participants []string       `json:"participants,omitempty"`
	ParticipantId string        `json:"participantId,omitempty"`
}

// Client represents a connected user
type Client struct {
	ID   string
	Room string
	Conn *websocket.Conn
	Send chan []byte
	Hub  *Hub
}

// Room represents a conference room
type Room struct {
	ID      string
	Clients map[string]*Client
	mu      sync.RWMutex
}

// Hub maintains active clients and broadcasts
type Hub struct {
	Rooms      map[string]*Room
	Register   chan *Client
	Unregister chan *Client
	mu         sync.RWMutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func NewHub() *Hub {
	return &Hub{
		Rooms:      make(map[string]*Room),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			room, exists := h.Rooms[client.Room]
			if !exists {
				room = &Room{
					ID:      client.Room,
					Clients: make(map[string]*Client),
				}
				h.Rooms[client.Room] = room
			}
			h.mu.Unlock()

			room.mu.Lock()
			room.Clients[client.ID] = client
			
			// Send welcome message with current participants
			participants := make([]string, 0)
			for id := range room.Clients {
				if id != client.ID {
					participants = append(participants, id)
				}
			}
			room.mu.Unlock()

			welcome := Message{
				Type:         "welcome",
				YourId:       client.ID,
				Participants: participants,
			}
			
			if welcomeData, err := json.Marshal(welcome); err == nil {
				select {
				case client.Send <- welcomeData:
				default:
				}
			}

			// Notify others about new participant
			room.mu.RLock()
			joinMsg := Message{
				Type:          "participant-joined",
				ParticipantId: client.ID,
			}
			if joinData, err := json.Marshal(joinMsg); err == nil {
				for _, c := range room.Clients {
					if c.ID != client.ID {
						select {
						case c.Send <- joinData:
						default:
						}
					}
				}
			}
			room.mu.RUnlock()

			log.Printf("Client %s joined room %s", client.ID, client.Room)

		case client := <-h.Unregister:
			h.mu.RLock()
			room := h.Rooms[client.Room]
			h.mu.RUnlock()
			
			if room != nil {
				room.mu.Lock()
				if _, ok := room.Clients[client.ID]; ok {
					delete(room.Clients, client.ID)
					close(client.Send)
					
					// Notify others about participant leaving
					leaveMsg := Message{
						Type:          "participant-left",
						ParticipantId: client.ID,
					}
					if leaveData, err := json.Marshal(leaveMsg); err == nil {
						for _, c := range room.Clients {
							select {
							case c.Send <- leaveData:
							default:
							}
						}
					}
				}
				room.mu.Unlock()
				
				log.Printf("Client %s left room %s", client.ID, client.Room)
			}
		}
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()
	
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}
		
		// Handle different message types
		switch msg.Type {
		case "join":
			// Already handled in connection
			
		case "video-frame", "audio-chunk":
			// Relay to others in room
			msg.From = c.ID
			c.Hub.mu.RLock()
			room := c.Hub.Rooms[c.Room]
			c.Hub.mu.RUnlock()
			
			if room != nil {
				if relayData, err := json.Marshal(msg); err == nil {
					room.mu.RLock()
					for id, client := range room.Clients {
						if id != c.ID {
							select {
							case client.Send <- relayData:
							default:
								// Client's send channel is full, skip
							}
						}
					}
					room.mu.RUnlock()
				}
			}
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			
			// Add queued messages to current message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				<-c.Send
			}
			
			if err := w.Close(); err != nil {
				return
			}
			
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func handleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	
	// Read first message to get client info
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Println(err)
		conn.Close()
		return
	}
	
	var msg Message
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Println(err)
		conn.Close()
		return
	}
	
	if msg.Type != "join" {
		log.Println("First message must be join")
		conn.Close()
		return
	}
	
	client := &Client{
		ID:   msg.ID,
		Room: msg.Room,
		Conn: conn,
		Send: make(chan []byte, 256),
		Hub:  hub,
	}
	
	client.Hub.Register <- client
	
	go client.WritePump()
	go client.ReadPump()
}

func main() {
	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}
	
	// Get TLS mode from environment
	useTLS := os.Getenv("USE_TLS") == "true"
	certFile := os.Getenv("CERT_FILE")
	keyFile := os.Getenv("KEY_FILE")
	
	if certFile == "" {
		certFile = "/etc/letsencrypt/live/conference.example.com/fullchain.pem"
	}
	if keyFile == "" {
		keyFile = "/etc/letsencrypt/live/conference.example.com/privkey.pem"
	}
	
	hub := NewHub()
	go hub.Run()
	
	// Serve a simple status page at root
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Conference Server</title>
    <style>
        body { font-family: Arial; padding: 40px; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; }
        .status { background: rgba(255,255,255,0.2); padding: 20px; border-radius: 10px; max-width: 600px; margin: 0 auto; }
        code { background: rgba(0,0,0,0.3); padding: 2px 8px; border-radius: 4px; }
    </style>
</head>
<body>
    <div class="status">
        <h1>🎥 Conference Server Active</h1>
        <p>WebSocket endpoint: <code>%s://%s/ws</code></p>
        <p>TLS/WSS: <code>%v</code></p>
        <p>Ready to accept connections</p>
    </div>
</body>
</html>`, 
			func() string {
				if useTLS {
					return "wss"
				}
				return "ws"
			}(),
			r.Host,
			useTLS)
	})
	
	// WebSocket endpoint
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})
	
	if useTLS {
		// Create TLS configuration
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		
		server := &http.Server{
			Addr:      ":" + port,
			TLSConfig: tlsConfig,
		}
		
		log.Printf("Starting HTTPS/WSS server on port %s with TLS", port)
		log.Fatal(server.ListenAndServeTLS(certFile, keyFile))
	} else {
		log.Printf("Starting HTTP/WS server on port %s (no TLS)", port)
		log.Fatal(http.ListenAndServe(":"+port, nil))
	}
}