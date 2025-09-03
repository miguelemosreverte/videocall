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

type MessageType struct {
	Type        string          `json:"type"`
	ID          string          `json:"id,omitempty"`
	Room        string          `json:"room,omitempty"`
	From        string          `json:"from,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
	Timestamp   int64           `json:"timestamp,omitempty"`
	FrameType   string          `json:"frameType,omitempty"` // "video" or "audio"
	Participants []string        `json:"participants,omitempty"`
}

type Client struct {
	ID   string
	Room string
	Conn *websocket.Conn
	Send chan []byte
	mu   sync.Mutex
}

type Room struct {
	Name    string
	Clients map[string]*Client
	mu      sync.RWMutex
}

type Hub struct {
	Rooms      map[string]*Room
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan *MessageType
	mu         sync.RWMutex
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins
		},
		ReadBufferSize:  1024 * 64,  // 64KB for video
		WriteBufferSize: 1024 * 64,
	}
	
	hub = &Hub{
		Rooms:      make(map[string]*Room),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan *MessageType, 256),
	}
)

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.addClient(client)
			
		case client := <-h.Unregister:
			h.removeClient(client)
			
		case msg := <-h.Broadcast:
			h.broadcastToRoom(msg)
		}
	}
}

func (h *Hub) addClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	// Get or create room
	room, exists := h.Rooms[client.Room]
	if !exists {
		room = &Room{
			Name:    client.Room,
			Clients: make(map[string]*Client),
		}
		h.Rooms[client.Room] = room
	}
	
	// Add client to room
	room.mu.Lock()
	room.Clients[client.ID] = client
	roomSize := len(room.Clients)
	
	// Get list of other participants
	participants := make([]string, 0, roomSize-1)
	for id := range room.Clients {
		if id != client.ID {
			participants = append(participants, id)
		}
	}
	room.mu.Unlock()
	
	log.Printf("Client %s joined room %s. Room size: %d", client.ID, client.Room, roomSize)
	
	// Send welcome message with participants list
	welcome := &MessageType{
		Type:         "welcome",
		ID:           client.ID,
		Room:         client.Room,
		Participants: participants,
	}
	
	if data, err := json.Marshal(welcome); err == nil {
		select {
		case client.Send <- data:
		default:
			log.Printf("Failed to send welcome to %s", client.ID)
		}
	}
	
	// Notify others about new participant
	notification := &MessageType{
		Type: "participant-joined",
		ID:   client.ID,
		Room: client.Room,
	}
	h.broadcastToRoom(notification)
}

func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	room, exists := h.Rooms[client.Room]
	if !exists {
		return
	}
	
	room.mu.Lock()
	delete(room.Clients, client.ID)
	roomSize := len(room.Clients)
	room.mu.Unlock()
	
	close(client.Send)
	
	log.Printf("Client %s left room %s. Room size: %d", client.ID, client.Room, roomSize)
	
	// Remove empty rooms
	if roomSize == 0 {
		delete(h.Rooms, client.Room)
	} else {
		// Notify others about participant leaving
		notification := &MessageType{
			Type: "participant-left",
			ID:   client.ID,
			Room: client.Room,
		}
		h.broadcastToRoom(notification)
	}
}

func (h *Hub) broadcastToRoom(msg *MessageType) {
	h.mu.RLock()
	room, exists := h.Rooms[msg.Room]
	h.mu.RUnlock()
	
	if !exists {
		return
	}
	
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	
	room.mu.RLock()
	defer room.mu.RUnlock()
	
	for id, client := range room.Clients {
		// Don't send back to sender (unless it's a system message)
		if msg.From != "" && id == msg.From {
			continue
		}
		
		select {
		case client.Send <- data:
		default:
			// Client's send channel is full, skip
			log.Printf("Client %s send buffer full", id)
		}
	}
}

func (c *Client) ReadPump() {
	defer func() {
		hub.Unregister <- c
		c.Conn.Close()
	}()
	
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	
	for {
		messageType, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}
		
		// Handle different message types
		if messageType == websocket.TextMessage {
			var msg MessageType
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Error unmarshaling message: %v", err)
				continue
			}
			
			// Process based on message type
			switch msg.Type {
			case "join":
				// Already handled in connection setup
				continue
				
			case "video-frame", "audio-frame":
				// Relay frame to others in room
				msg.From = c.ID
				msg.Room = c.Room
				msg.Timestamp = time.Now().UnixMilli()
				log.Printf("Broadcasting %s from %s to room %s", msg.Type, c.ID, c.Room)
				hub.Broadcast <- &msg
				
			default:
				log.Printf("Unknown message type: %s", msg.Type)
			}
		} else if messageType == websocket.BinaryMessage {
			// Handle binary frames (future optimization)
			log.Printf("Received binary message of size %d from %s", len(message), c.ID)
		}
		
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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
			
			c.Conn.WriteMessage(websocket.TextMessage, message)
			
			// Drain queued messages
			n := len(c.Send)
			for i := 0; i < n; i++ {
				c.Conn.WriteMessage(websocket.TextMessage, <-c.Send)
			}
			
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	
	log.Println("New WebSocket connection")
	
	// Wait for join message
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Error reading join message: %v", err)
		conn.Close()
		return
	}
	
	log.Printf("Received message: %s", string(message))
	
	var joinMsg MessageType
	if err := json.Unmarshal(message, &joinMsg); err != nil {
		log.Printf("Error unmarshaling join message: %v", err)
		conn.Close()
		return
	}
	
	if joinMsg.Type != "join" {
		log.Printf("Expected join message, got: %s", joinMsg.Type)
		conn.Close()
		return
	}
	
	// Create client
	clientID := joinMsg.ID
	if clientID == "" {
		clientID = fmt.Sprintf("user-%d", time.Now().UnixNano())
	}
	
	roomName := joinMsg.Room
	if roomName == "" {
		roomName = "global"
	}
	
	client := &Client{
		ID:   clientID,
		Room: roomName,
		Conn: conn,
		Send: make(chan []byte, 256),
	}
	
	hub.Register <- client
	
	// Start goroutines
	go client.WritePump()
	go client.ReadPump()
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	
	status := make(map[string]interface{})
	status["rooms"] = len(hub.Rooms)
	
	roomInfo := make([]map[string]interface{}, 0)
	for name, room := range hub.Rooms {
		room.mu.RLock()
		info := map[string]interface{}{
			"name":         name,
			"participants": len(room.Clients),
		}
		room.mu.RUnlock()
		roomInfo = append(roomInfo, info)
	}
	status["roomDetails"] = roomInfo
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head><title>Video Relay Server</title></head>
<body>
	<h1>Go Video Relay Server</h1>
	<p>WebSocket endpoint: ws://` + r.Host + `/ws</p>
	<p><a href="/status">Server Status</a></p>
	<script>
		fetch('/status').then(r => r.json()).then(console.log);
	</script>
</body>
</html>`
	
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	go hub.Run()
	
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/status", handleStatus)
	
	port := "3000"
	log.Printf("Go Video Relay Server starting on port %s", port)
	if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}