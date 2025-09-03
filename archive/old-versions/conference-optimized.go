package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "sync"
    "sync/atomic"
    "time"

    "github.com/gorilla/websocket"
)

// Message types for the conference
type Message struct {
    Type          string   `json:"type"`
    ID            string   `json:"id,omitempty"`
    Room          string   `json:"room,omitempty"`
    From          string   `json:"from,omitempty"`
    To            string   `json:"to,omitempty"`
    Data          string   `json:"data,omitempty"`
    Timestamp     int64    `json:"timestamp,omitempty"`
    Seq           int      `json:"seq,omitempty"`
    TestMarker    string   `json:"testMarker,omitempty"`
    Priority      int      `json:"priority,omitempty"` // Audio = 10, Video = 5
}

// Client represents a connected user
type Client struct {
    ID            string
    Room          string
    Conn          *websocket.Conn
    Send          chan []byte
    Hub           *Hub
    
    // Bandwidth management
    BandwidthKbps float64
    LastSendTime  time.Time
    BytesSent     int64
    
    // Quality control
    DropFrames    bool
    FrameInterval int // Send every Nth frame
    
    mu sync.RWMutex
}

// Room manages clients in a conference
type Room struct {
    ID              string
    Clients         map[string]*Client
    TotalBandwidth  float64
    MaxBandwidth    float64 // 1200 kbps for VPS
    
    // Adaptive quality
    CurrentQuality  string
    FrameDropRatio  int
    
    mu sync.RWMutex
}

// Hub manages all rooms
type Hub struct {
    Rooms      map[string]*Room
    Register   chan *Client
    Unregister chan *Client
    Broadcast  chan *BroadcastMessage
    
    // Performance metrics
    MessageCount  int64
    DroppedFrames int64
    
    mu sync.RWMutex
}

type BroadcastMessage struct {
    Room    string
    Message []byte
    From    string
}

var (
    upgrader = websocket.Upgrader{
        ReadBufferSize:  1024 * 16,  // 16KB read buffer
        WriteBufferSize: 1024 * 16,  // 16KB write buffer
        CheckOrigin: func(r *http.Request) bool {
            return true
        },
    }
    
    hub *Hub
)

// Bandwidth limits based on user count
var bandwidthLimits = map[int]float64{
    1: 1000,  // 1 user: 1000 kbps
    2: 500,   // 2 users: 500 kbps each
    3: 350,   // 3 users: 350 kbps each
    4: 250,   // 4 users: 250 kbps each
    5: 200,   // 5 users: 200 kbps each
    6: 150,   // 6+ users: 150 kbps each
}

func NewHub() *Hub {
    return &Hub{
        Rooms:      make(map[string]*Room),
        Register:   make(chan *Client, 100),
        Unregister: make(chan *Client, 100),
        Broadcast:  make(chan *BroadcastMessage, 1000),
    }
}

func (h *Hub) Run() {
    // Periodic quality adjustment
    qualityTicker := time.NewTicker(1 * time.Second)
    defer qualityTicker.Stop()
    
    for {
        select {
        case client := <-h.Register:
            h.registerClient(client)
            
        case client := <-h.Unregister:
            h.unregisterClient(client)
            
        case message := <-h.Broadcast:
            h.broadcastMessage(message)
            
        case <-qualityTicker.C:
            h.adjustQuality()
        }
    }
}

func (h *Hub) registerClient(client *Client) {
    h.mu.Lock()
    room, exists := h.Rooms[client.Room]
    if !exists {
        room = &Room{
            ID:           client.Room,
            Clients:      make(map[string]*Client),
            MaxBandwidth: 1200, // VPS limit in kbps
        }
        h.Rooms[client.Room] = room
    }
    h.mu.Unlock()
    
    room.mu.Lock()
    room.Clients[client.ID] = client
    userCount := len(room.Clients)
    
    // Set bandwidth limit per user
    maxBandwidthPerUser := room.MaxBandwidth / float64(userCount)
    if limit, ok := bandwidthLimits[userCount]; ok {
        if limit < maxBandwidthPerUser {
            maxBandwidthPerUser = limit
        }
    }
    
    // Update all clients' bandwidth limits
    for _, c := range room.Clients {
        c.mu.Lock()
        c.BandwidthKbps = maxBandwidthPerUser
        
        // Set frame dropping based on user count
        if userCount > 3 {
            c.DropFrames = true
            c.FrameInterval = userCount - 2 // Drop more frames with more users
        }
        c.mu.Unlock()
    }
    room.mu.Unlock()
    
    // Send welcome message
    welcome := Message{
        Type: "welcome",
        ID:   client.ID,
    }
    
    if data, err := json.Marshal(welcome); err == nil {
        select {
        case client.Send <- data:
        default:
        }
    }
    
    // Notify others
    h.notifyParticipantJoined(client)
    
    log.Printf("Client %s joined room %s (total: %d, bandwidth: %.0f kbps each)",
        client.ID, client.Room, userCount, maxBandwidthPerUser)
}

func (h *Hub) unregisterClient(client *Client) {
    h.mu.RLock()
    room := h.Rooms[client.Room]
    h.mu.RUnlock()
    
    if room != nil {
        room.mu.Lock()
        if _, ok := room.Clients[client.ID]; ok {
            delete(room.Clients, client.ID)
            close(client.Send)
            
            // Recalculate bandwidth for remaining clients
            if len(room.Clients) > 0 {
                userCount := len(room.Clients)
                maxBandwidthPerUser := room.MaxBandwidth / float64(userCount)
                
                for _, c := range room.Clients {
                    c.mu.Lock()
                    c.BandwidthKbps = maxBandwidthPerUser
                    
                    if userCount <= 3 {
                        c.DropFrames = false
                        c.FrameInterval = 1
                    } else {
                        c.DropFrames = true
                        c.FrameInterval = userCount - 2
                    }
                    c.mu.Unlock()
                }
            }
        }
        room.mu.Unlock()
        
        // Notify others
        h.notifyParticipantLeft(client)
    }
}

func (h *Hub) broadcastMessage(bcast *BroadcastMessage) {
    h.mu.RLock()
    room := h.Rooms[bcast.Room]
    h.mu.RUnlock()
    
    if room == nil {
        return
    }
    
    var msg Message
    json.Unmarshal(bcast.Message, &msg)
    
    // Add sender info
    msg.From = bcast.From
    
    // Track message count
    atomic.AddInt64(&h.MessageCount, 1)
    
    room.mu.RLock()
    defer room.mu.RUnlock()
    
    for id, client := range room.Clients {
        if id == bcast.From {
            continue // Don't send back to sender
        }
        
        // Check if we should drop frames for this client
        if msg.Type == "video-frame" && client.DropFrames {
            frameSeq := msg.Seq
            if frameSeq%client.FrameInterval != 0 {
                atomic.AddInt64(&h.DroppedFrames, 1)
                continue // Drop this frame
            }
        }
        
        // Re-encode with recipient info
        if data, err := json.Marshal(msg); err == nil {
            select {
            case client.Send <- data:
            default:
                // Buffer full, drop the message
                if msg.Type == "video-frame" {
                    atomic.AddInt64(&h.DroppedFrames, 1)
                }
            }
        }
    }
}

func (h *Hub) notifyParticipantJoined(client *Client) {
    notification := Message{
        Type: "participant-joined",
        ID:   client.ID,
    }
    
    if data, err := json.Marshal(notification); err == nil {
        h.Broadcast <- &BroadcastMessage{
            Room:    client.Room,
            Message: data,
            From:    client.ID,
        }
    }
}

func (h *Hub) notifyParticipantLeft(client *Client) {
    notification := Message{
        Type: "participant-left",
        ID:   client.ID,
    }
    
    if data, err := json.Marshal(notification); err == nil {
        h.Broadcast <- &BroadcastMessage{
            Room:    client.Room,
            Message: data,
            From:    client.ID,
        }
    }
}

func (h *Hub) adjustQuality() {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    for _, room := range h.Rooms {
        room.mu.RLock()
        
        // Calculate total bandwidth usage
        totalBandwidth := 0.0
        for _, client := range room.Clients {
            client.mu.RLock()
            if time.Since(client.LastSendTime) < 2*time.Second {
                // Calculate actual bandwidth used in last second
                bandwidth := float64(atomic.LoadInt64(&client.BytesSent)) * 8 / 1000 // kbps
                totalBandwidth += bandwidth
                
                // Reset counter
                atomic.StoreInt64(&client.BytesSent, 0)
            }
            client.mu.RUnlock()
        }
        
        // Adjust quality if exceeding limits
        if totalBandwidth > room.MaxBandwidth {
            log.Printf("Room %s exceeding bandwidth: %.0f/%.0f kbps",
                room.ID, totalBandwidth, room.MaxBandwidth)
            
            // Increase frame dropping
            for _, client := range room.Clients {
                client.mu.Lock()
                if client.FrameInterval < 10 {
                    client.FrameInterval++
                }
                client.mu.Unlock()
            }
        }
        room.mu.RUnlock()
    }
}

// Client handlers
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
            break
        }
        
        // Track bandwidth usage
        atomic.AddInt64(&c.BytesSent, int64(len(message)))
        c.mu.Lock()
        c.LastSendTime = time.Now()
        c.mu.Unlock()
        
        // Parse message type for prioritization
        var msg Message
        if err := json.Unmarshal(message, &msg); err == nil {
            // Handle join specially
            if msg.Type == "join" {
                continue
            }
            
            // Broadcast to room
            c.Hub.Broadcast <- &BroadcastMessage{
                Room:    c.Room,
                Message: message,
                From:    c.ID,
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
    
    // Rate limiting
    rateLimiter := time.NewTicker(10 * time.Millisecond) // 100 messages/sec max
    defer rateLimiter.Stop()
    
    for {
        select {
        case message, ok := <-c.Send:
            <-rateLimiter.C // Rate limit
            
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if !ok {
                c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            
            if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
                return
            }
            
            // Send up to 10 queued messages at once
            n := len(c.Send)
            if n > 10 {
                n = 10
            }
            for i := 0; i < n; i++ {
                if msg, ok := <-c.Send; ok {
                    c.Conn.WriteMessage(websocket.TextMessage, msg)
                }
            }
            
        case <-ticker.C:
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}

// HTTP handlers
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("Failed to upgrade: %v", err)
        return
    }
    
    // Read first message for join
    var joinMsg Message
    if err := conn.ReadJSON(&joinMsg); err != nil || joinMsg.Type != "join" {
        conn.Close()
        return
    }
    
    client := &Client{
        ID:            joinMsg.ID,
        Room:          joinMsg.Room,
        Conn:          conn,
        Send:          make(chan []byte, 256),
        Hub:           hub,
        BandwidthKbps: 1000, // Default, will be adjusted
        FrameInterval: 1,
    }
    
    client.Hub.Register <- client
    
    // Start goroutines
    go client.WritePump()
    go client.ReadPump()
}

func handleStats(w http.ResponseWriter, r *http.Request) {
    hub.mu.RLock()
    roomCount := len(hub.Rooms)
    clientCount := 0
    for _, room := range hub.Rooms {
        room.mu.RLock()
        clientCount += len(room.Clients)
        room.mu.RUnlock()
    }
    hub.mu.RUnlock()
    
    stats := map[string]interface{}{
        "rooms":          roomCount,
        "clients":        clientCount,
        "messages":       atomic.LoadInt64(&hub.MessageCount),
        "droppedFrames":  atomic.LoadInt64(&hub.DroppedFrames),
        "dropRate":       float64(atomic.LoadInt64(&hub.DroppedFrames)) / float64(atomic.LoadInt64(&hub.MessageCount)+1) * 100,
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(stats)
}

func main() {
    hub = NewHub()
    go hub.Run()
    
    http.HandleFunc("/ws", handleWebSocket)
    http.HandleFunc("/stats", handleStats)
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html")
        fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Optimized Conference Server</title>
</head>
<body>
    <h1>Conference Server (Optimized for 1.2 Mbps)</h1>
    <p>WebSocket endpoint: ws://localhost:3001/ws</p>
    <p>Stats: <a href="/stats">/stats</a></p>
    <div id="stats"></div>
    <script>
        setInterval(() => {
            fetch('/stats')
                .then(r => r.json())
                .then(data => {
                    document.getElementById('stats').innerHTML = 
                        '<pre>' + JSON.stringify(data, null, 2) + '</pre>';
                });
        }, 1000);
    </script>
</body>
</html>`)
    })
    
    addr := ":3001"
    log.Printf("Starting optimized conference server on %s", addr)
    log.Printf("Bandwidth limit: 1.2 Mbps total")
    log.Printf("Adaptive quality enabled")
    
    if err := http.ListenAndServe(addr, nil); err != nil {
        log.Fatal(err)
    }
}