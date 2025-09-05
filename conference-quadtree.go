package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	BuildTime   = "2025-09-05T00:00:00Z"
	BuildCommit = "quadtree"
	BuildBy     = "Quad-Tree Codec"
	BuildRef    = "refs/heads/main"
)

type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	id     string
	userId string
	room   string
	hub    *Hub
}

type Hub struct {
	clients           map[*Client]bool
	rooms             map[string]map[*Client]bool
	broadcast         chan []byte
	register          chan *Client
	unregister        chan *Client
	mu                sync.RWMutex
	quadTreeProcessor *QuadTreeProcessor
}

var hub = &Hub{
	clients:           make(map[*Client]bool),
	rooms:             make(map[string]map[*Client]bool),
	broadcast:         make(chan []byte, 256),
	register:          make(chan *Client),
	unregister:        make(chan *Client),
	quadTreeProcessor: NewQuadTreeProcessor(),
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func (h *Hub) run() {
	// Bandwidth optimizer ticker
	bandwidthTicker := time.NewTicker(5 * time.Second)
	defer bandwidthTicker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			
			// Add to room
			if client.room != "" {
				if h.rooms[client.room] == nil {
					h.rooms[client.room] = make(map[*Client]bool)
				}
				h.rooms[client.room][client] = true
			}
			h.mu.Unlock()
			
			log.Printf("Client %s connected to room %s. Total clients: %d", 
				client.userId, client.room, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if client.room != "" && h.rooms[client.room] != nil {
					delete(h.rooms[client.room], client)
					if len(h.rooms[client.room]) == 0 {
						delete(h.rooms, client.room)
					}
				}
				close(client.send)
			}
			h.mu.Unlock()
			
			log.Printf("Client %s disconnected. Total clients: %d", 
				client.userId, len(h.clients))

		case message := <-h.broadcast:
			h.broadcastToAll(message)

		case <-bandwidthTicker.C:
			// Optimize bandwidth periodically
			h.quadTreeProcessor.OptimizeForBandwidth(50.0) // Target 50 Mbps
		}
	}
}

func (h *Hub) broadcastToAll(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			delete(h.clients, client)
			close(client.send)
		}
	}
}

func (h *Hub) broadcastToRoom(room string, message []byte, sender *Client) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	if roomClients, ok := h.rooms[room]; ok {
		for client := range roomClients {
			if client != sender {
				select {
				case client.send <- message:
				default:
					delete(h.clients, client)
					delete(roomClients, client)
					close(client.send)
				}
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	
	c.conn.SetReadLimit(10 * 1024 * 1024) // 10MB max message
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
		
		// Try to parse as quad-tree packet first
		var packet VideoPacket
		if err := json.Unmarshal(message, &packet); err == nil && packet.Type != "" {
			// Process with quad-tree codec
			optimizedPacket, err := c.hub.quadTreeProcessor.ProcessPacket(c.userId, message)
			if err == nil {
				optimizedPacket.UserID = c.userId
				optimizedPacket.Room = c.room
				
				if data, err := json.Marshal(optimizedPacket); err == nil {
					c.hub.broadcastToRoom(c.room, data, c)
					continue
				}
			}
		}
		
		// Fallback to regular message handling
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			// Handle join message
			if msg["type"] == "join" {
				if room, ok := msg["room"].(string); ok {
					c.room = room
					c.userId = msg["userId"].(string)
					
					c.hub.mu.Lock()
					if c.hub.rooms[c.room] == nil {
						c.hub.rooms[c.room] = make(map[*Client]bool)
					}
					c.hub.rooms[c.room][c] = true
					c.hub.mu.Unlock()
					
					log.Printf("Client %s joined room %s", c.userId, c.room)
					
					// Send confirmation
					response := map[string]interface{}{
						"type":   "joined",
						"room":   c.room,
						"userId": c.userId,
					}
					if data, err := json.Marshal(response); err == nil {
						c.send <- data
					}
				}
			} else {
				// Add sender info
				msg["from"] = c.userId
				msg["timestamp"] = time.Now().UTC().Format(time.RFC3339)
				
				if modifiedMsg, err := json.Marshal(msg); err == nil {
					c.hub.broadcastToRoom(c.room, modifiedMsg, c)
				}
			}
		}
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

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	hub.mu.RLock()
	clientCount := len(hub.clients)
	roomCount := len(hub.rooms)
	hub.mu.RUnlock()
	
	stats := hub.quadTreeProcessor.GetStats()
	
	health := map[string]interface{}{
		"status": "healthy",
		"deployment": map[string]string{
			"time":       BuildTime,
			"commit":     BuildCommit,
			"deployedBy": BuildBy,
			"ref":        BuildRef,
		},
		"server": map[string]interface{}{
			"type":     "conference-quadtree",
			"version":  "3.0.0",
			"features": []string{"websocket", "quad-tree-codec", "audio-priority", "delta-frames"},
		},
		"stats": map[string]interface{}{
			"connected_clients": clientCount,
			"active_rooms":      roomCount,
			"total_packets":     stats.TotalPackets,
			"average_fps":       stats.AverageFPS,
			"audio_integrity":   stats.AudioIntegrity,
			"bandwidth_mbps":    stats.BandwidthMbps,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	
	json.NewEncoder(w).Encode(health)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("Upgrade failed: ", err)
		return
	}
	
	clientID := r.Header.Get("X-Client-Id")
	if clientID == "" {
		clientID = "client-" + time.Now().Format("150405.000")
	}
	
	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
		id:   clientID,
		hub:  hub,
	}
	
	hub.register <- client
	
	go client.writePump()
	go client.readPump()
}

func main() {
	go hub.run()
	
	// Serve static files
	fs := http.FileServer(http.Dir("."))
	http.Handle("/", fs)
	
	// API endpoints
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/ws", handleWebSocket)
	
	log.Printf("Quad-Tree Conference Server v3.0 starting on :3001")
	log.Printf("Features: Audio Priority, Delta Frames, Adaptive Quality")
	log.Printf("Target: 4K@60fps with guaranteed audio")
	log.Fatal(http.ListenAndServe(":3001", nil))
}