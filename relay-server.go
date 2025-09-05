package main

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	username string
	hub      *Hub
}

type Hub struct {
	clients    map[string]*Client
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

type Message struct {
	From string `json:"from"`
	Data []byte `json:"data"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for simplicity
	},
	ReadBufferSize:  1024 * 1024, // 1MB
	WriteBufferSize: 1024 * 1024, // 1MB
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		broadcast:  make(chan Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.username] = client
			h.mu.Unlock()
			log.Printf("User '%s' connected. Total users: %d", client.username, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.username]; ok {
				delete(h.clients, client.username)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("User '%s' disconnected. Total users: %d", client.username, len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			// Send to all clients except the sender
			for username, client := range h.clients {
				if username != message.From {
					select {
					case client.send <- message.Data:
					default:
						close(client.send)
						delete(h.clients, username)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (c *Client) ReadPump() {
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
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Broadcast the raw message to all other clients
		c.hub.broadcast <- Message{
			From: c.username,
			Data: data,
		}
	}
}

func (c *Client) WritePump() {
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
			c.conn.WriteMessage(websocket.BinaryMessage, message)

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func HandleWebSocket(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract username from URL path
		vars := mux.Vars(r)
		username := vars["username"]
		
		if username == "" {
			http.Error(w, "Username required in URL", http.StatusBadRequest)
			return
		}

		// Check if username already exists
		hub.mu.RLock()
		if _, exists := hub.clients[username]; exists {
			hub.mu.RUnlock()
			http.Error(w, "Username already connected", http.StatusConflict)
			return
		}
		hub.mu.RUnlock()

		// Upgrade to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}

		client := &Client{
			conn:     conn,
			send:     make(chan []byte, 256),
			username: username,
			hub:      hub,
		}

		hub.register <- client

		go client.WritePump()
		go client.ReadPump()
	}
}

func HandleHealth(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hub.mu.RLock()
		users := make([]string, 0, len(hub.clients))
		for username := range hub.clients {
			users = append(users, username)
		}
		hub.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write([]byte(`{"status":"healthy","users":["`))
		w.Write([]byte(strings.Join(users, `","`)))
		w.Write([]byte(`"],"count":`))
		w.Write([]byte(string(rune(len(users)+'0'))))
		w.Write([]byte(`}`))
	}
}

func main() {
	hub := NewHub()
	go hub.Run()

	router := mux.NewRouter()
	
	// WebSocket endpoint with username in URL
	router.HandleFunc("/ws/{username}", HandleWebSocket(hub))
	
	// Health check endpoint
	router.HandleFunc("/health", HandleHealth(hub))
	
	// CORS middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	})

	port := ":8080"
	log.Printf("WebSocket Relay Server starting on %s", port)
	log.Printf("Connect via: ws://localhost%s/ws/{username}", port)
	log.Fatal(http.ListenAndServe(port, router))
}