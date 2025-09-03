package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
	ReadBufferSize:  1024 * 64,  // 64KB for video data
	WriteBufferSize: 1024 * 64,
}

type MessageType struct {
	Type   string          `json:"type"`
	From   string          `json:"from,omitempty"`
	To     string          `json:"to,omitempty"`
	Room   string          `json:"room,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	Events []interface{}   `json:"events,omitempty"`
}

type Client struct {
	ID     string
	Room   string
	conn   *websocket.Conn
	send   chan []byte
	hub    *Hub
}

type Hub struct {
	rooms      map[string]map[*Client]bool
	clients    map[string]*Client
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

var hub = &Hub{
	rooms:      make(map[string]map[*Client]bool),
	clients:    make(map[string]*Client),
	broadcast:  make(chan []byte, 256),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			
			if h.rooms[client.Room] == nil {
				h.rooms[client.Room] = make(map[*Client]bool)
			}
			h.rooms[client.Room][client] = true
			h.mu.Unlock()
			
			log.Printf("Client %s joined room %s", client.ID, client.Room)
			
			// Notify others in room
			h.broadcastToRoom(client.Room, client.ID, &MessageType{
				Type: "user-joined",
				From: client.ID,
				Room: client.Room,
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.rooms[client.Room][client]; ok {
				delete(h.rooms[client.Room], client)
				delete(h.clients, client.ID)
				close(client.send)
				
				if len(h.rooms[client.Room]) == 0 {
					delete(h.rooms, client.Room)
				}
			}
			h.mu.Unlock()
			
			log.Printf("Client %s left room %s", client.ID, client.Room)
			
			// Notify others in room
			h.broadcastToRoom(client.Room, client.ID, &MessageType{
				Type: "user-left",
				From: client.ID,
				Room: client.Room,
			})
		}
	}
}

func (h *Hub) broadcastToRoom(room string, sender string, msg *MessageType) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	for client := range h.rooms[room] {
		if client.ID != sender {
			select {
			case client.send <- data:
			default:
				// Client's send channel is full
				log.Printf("Client %s send buffer full", client.ID)
			}
		}
	}
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
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		
		var msg MessageType
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}
		
		// Handle different message types
		switch msg.Type {
		case "motion-events":
			// Relay motion events to other clients in room
			msg.From = c.ID
			msg.Room = c.Room
			c.hub.broadcastToRoom(c.Room, c.ID, &msg)
			
		case "offer", "answer", "ice-candidate":
			// WebRTC signaling - relay to specific peer
			if msg.To != "" {
				c.hub.mu.RLock()
				if targetClient, ok := c.hub.clients[msg.To]; ok {
					msg.From = c.ID
					if data, err := json.Marshal(msg); err == nil {
						select {
						case targetClient.send <- data:
						default:
							log.Printf("Failed to send to client %s", msg.To)
						}
					}
				}
				c.hub.mu.RUnlock()
			}
			
		case "get-room-users":
			// Send list of users in room
			c.hub.mu.RLock()
			users := []string{}
			for client := range c.hub.rooms[c.Room] {
				if client.ID != c.ID {
					users = append(users, client.ID)
				}
			}
			c.hub.mu.RUnlock()
			
			response := MessageType{
				Type: "room-users",
				Room: c.Room,
				Data: json.RawMessage(fmt.Sprintf(`{"users":%v}`, users)),
			}
			if data, err := json.Marshal(response); err == nil {
				c.send <- data
			}
			
		default:
			// Echo or broadcast unknown message types
			msg.From = c.ID
			msg.Room = c.Room
			c.hub.broadcastToRoom(c.Room, c.ID, &msg)
		}
		
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("Upgrade failed: ", err)
		return
	}
	
	// Get room and user ID from query params
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "default"
	}
	
	userID := r.URL.Query().Get("id")
	if userID == "" {
		userID = fmt.Sprintf("user-%d", time.Now().Unix())
	}
	
	client := &Client{
		ID:   userID,
		Room: room,
		conn: conn,
		send: make(chan []byte, 256),
		hub:  hub,
	}
	
	hub.register <- client
	
	// Send welcome message
	welcome := MessageType{
		Type: "welcome",
		Room: room,
		Data: json.RawMessage(fmt.Sprintf(`{"id":"%s","room":"%s"}`, userID, room)),
	}
	if data, err := json.Marshal(welcome); err == nil {
		client.send <- data
	}
	
	go client.writePump()
	go client.readPump()
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	hub.mu.RLock()
	numClients := len(hub.clients)
	numRooms := len(hub.rooms)
	hub.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"clients": numClients,
		"rooms":   numRooms,
		"time":    time.Now().Format(time.RFC3339),
	})
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Video Stream Server</title></head>
<body>
<h1>Video Streaming WebSocket Server</h1>
<p>Server is running and ready for video calls!</p>
<p>WebSocket endpoint: ws://%s/ws</p>
<p>Query parameters:</p>
<ul>
  <li>room - Room ID to join (default: "default")</li>
  <li>id - User ID (auto-generated if not provided)</li>
</ul>
<p>Example: ws://%s/ws?room=myroom&id=user123</p>
<p>Current status: <a href="/health">/health</a></p>
</body>
</html>`, r.Host, r.Host)
}

func main() {
	go hub.run()
	
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/ws", handleWS)
	http.HandleFunc("/health", handleHealth)
	
	port := "8080"
	log.Printf("Video streaming server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}