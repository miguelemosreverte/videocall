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
    BuildTime   = "2025-09-03T22:50:00Z"  // Will be overridden by ldflags if provided
    BuildCommit = "latest"                 // Will be overridden by ldflags if provided
    BuildBy     = "GitHub Actions"         // Will be overridden by ldflags if provided
    BuildRef    = "refs/heads/main"        // Will be overridden by ldflags if provided
)

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
    mu         sync.RWMutex
}

var hub = Hub{
    clients:    make(map[*Client]bool),
    broadcast:  make(chan []byte),
    register:   make(chan *Client),
    unregister: make(chan *Client),
}

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
}

func (h *Hub) run() {
    for {
        select {
        case client := <-h.register:
            h.mu.Lock()
            h.clients[client] = true
            h.mu.Unlock()
            log.Printf("Client %s connected. Total clients: %d", client.id, len(h.clients))
            
            // Notify other clients about new connection
            notification := map[string]interface{}{
                "type": "user-joined",
                "id":   client.id,
                "time": time.Now().UTC().Format(time.RFC3339),
            }
            data, _ := json.Marshal(notification)
            h.broadcastToOthers(client, data)

        case client := <-h.unregister:
            h.mu.Lock()
            if _, ok := h.clients[client]; ok {
                delete(h.clients, client)
                close(client.send)
                h.mu.Unlock()
                log.Printf("Client %s disconnected. Total clients: %d", client.id, len(h.clients))
                
                // Notify other clients about disconnection
                notification := map[string]interface{}{
                    "type": "user-left",
                    "id":   client.id,
                    "time": time.Now().UTC().Format(time.RFC3339),
                }
                data, _ := json.Marshal(notification)
                h.broadcastToAll(data)
            } else {
                h.mu.Unlock()
            }

        case message := <-h.broadcast:
            h.broadcastToAll(message)
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
            // Client's send channel is full, close it
            delete(h.clients, client)
            close(client.send)
        }
    }
}

func (h *Hub) broadcastToOthers(sender *Client, message []byte) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    for client := range h.clients {
        if client != sender {
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
        
        // Parse the message to add sender info
        var msg map[string]interface{}
        if err := json.Unmarshal(message, &msg); err == nil {
            msg["from"] = c.id
            msg["timestamp"] = time.Now().UTC().Format(time.RFC3339)
            if modifiedMsg, err := json.Marshal(msg); err == nil {
                message = modifiedMsg
            }
        }
        
        // Broadcast to all OTHER clients (not back to sender)
        hub.mu.RLock()
        for client := range hub.clients {
            if client != c {
                select {
                case client.send <- message:
                default:
                    delete(hub.clients, client)
                    close(client.send)
                }
            }
        }
        hub.mu.RUnlock()
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
                // The hub closed the channel
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
    
    hub.mu.RLock()
    clientCount := len(hub.clients)
    hub.mu.RUnlock()
    
    health := map[string]interface{}{
        "status": "healthy",
        "deployment": map[string]string{
            "time":       BuildTime,
            "commit":     BuildCommit,
            "deployedBy": BuildBy,
            "ref":        BuildRef,
        },
        "server": map[string]interface{}{
            "type":     "simple-conference",
            "version":  "2.0.0",
            "features": []string{"websocket", "health-endpoint", "broadcasting", "multi-client"},
        },
        "stats": map[string]interface{}{
            "connected_clients": clientCount,
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
        clientID = time.Now().Format("150405.000")
    }
    
    client := &Client{
        conn: conn,
        send: make(chan []byte, 256),
        id:   clientID,
    }
    
    hub.register <- client
    
    // Start goroutines for reading and writing
    go client.writePump()
    go client.readPump()
}

func main() {
    // Start the hub
    go hub.run()
    
    http.HandleFunc("/health", handleHealth)
    http.HandleFunc("/info", handleHealth)
    http.HandleFunc("/ws", handleWebSocket)
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Conference Server v2.0 - Broadcasting Enabled"))
    })
    
    log.Printf("Conference Server v2.0 starting on :3001")
    log.Printf("Build: %s by %s", BuildCommit, BuildBy)
    log.Printf("Broadcasting between clients enabled")
    log.Fatal(http.ListenAndServe(":3001", nil))
}