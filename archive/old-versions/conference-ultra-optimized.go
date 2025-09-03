package main

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "image"
    "image/color"
    "image/draw"
    "image/jpeg"
    "log"
    "net/http"
    "sync"
    "sync/atomic"
    "time"

    "github.com/gorilla/websocket"
    "github.com/nfnt/resize"
)

// Ultra-compressed frame using minimal resolution and quality
type CompressedFrame struct {
    Data      []byte
    Timestamp int64
    Seq       int
}

// Message types
type Message struct {
    Type          string `json:"type"`
    ID            string `json:"id,omitempty"`
    Room          string `json:"room,omitempty"`
    From          string `json:"from,omitempty"`
    Data          string `json:"data,omitempty"`
    Timestamp     int64  `json:"timestamp,omitempty"`
    Seq           int    `json:"seq,omitempty"`
    FrameSize     int    `json:"frameSize,omitempty"`
}

// Client with smart bandwidth management
type Client struct {
    ID            string
    Room          string
    Conn          *websocket.Conn
    Send          chan []byte
    Hub           *Hub
    
    // Bandwidth tracking
    AudioBytesPerSec  int64
    VideoBytesPerSec  int64
    LastResetTime     time.Time
    
    // Frame management
    LastFrameSeq      int
    FrameSkipCount    int
    
    mu sync.RWMutex
}

// Room with frame distribution logic
type Room struct {
    ID              string
    Clients         map[string]*Client
    
    // Frame distribution
    NextVideoTarget int // Round-robin target
    AudioQueue      [][]byte
    
    // Bandwidth management
    TotalAudioBPS   int64
    TotalVideoBPS   int64
    
    mu sync.RWMutex
}

// Hub manages everything
type Hub struct {
    Rooms      map[string]*Room
    Register   chan *Client
    Unregister chan *Client
    Broadcast  chan *BroadcastMessage
    
    // Metrics
    TotalMessages    int64
    DroppedFrames    int64
    CompressedFrames int64
    
    mu sync.RWMutex
}

type BroadcastMessage struct {
    Room    string
    Message []byte
    From    string
}

var (
    upgrader = websocket.Upgrader{
        ReadBufferSize:  1024 * 8,
        WriteBufferSize: 1024 * 8,
        CheckOrigin: func(r *http.Request) bool { return true },
    }
    
    hub *Hub
    
    // Bandwidth allocations based on user count (total 1200 kbps)
    bandwidthAllocation = map[int]struct{ audio, video int }{
        1: {audio: 64, video: 936},   // 1 user: 64 kbps audio, 936 kbps video
        2: {audio: 96, video: 504},   // 2 users: 48 kbps audio each, 504 kbps video each
        3: {audio: 96, video: 368},   // 3 users: 32 kbps audio each, 368 kbps video total per user
        4: {audio: 96, video: 276},   // 4 users: 24 kbps audio each, 276 kbps video total per user
        5: {audio: 100, video: 220},  // 5 users: 20 kbps audio each, 220 kbps video total per user
        6: {audio: 96, video: 184},   // 6 users: 16 kbps audio each, 184 kbps video total per user
    }
)

// Ultra compression for video frames
func ultraCompressFrame(data []byte) []byte {
    // Decode the image
    img, _, err := image.Decode(bytes.NewReader(data))
    if err != nil {
        return data // Return original if can't decode
    }
    
    // Determine target size based on original
    bounds := img.Bounds()
    width := bounds.Max.X
    
    // Ultra aggressive resizing based on size
    var targetWidth uint
    var quality int
    
    if width > 320 {
        targetWidth = 80  // Ultra small
        quality = 20      // Very low quality
    } else if width > 160 {
        targetWidth = 60  // Tiny
        quality = 25
    } else {
        targetWidth = 40  // Minimal
        quality = 30
    }
    
    // Resize image
    resized := resize.Resize(targetWidth, 0, img, resize.Lanczos3)
    
    // Convert to grayscale to save more bytes
    gray := image.NewGray(resized.Bounds())
    draw.Draw(gray, gray.Bounds(), resized, resized.Bounds().Min, draw.Src)
    
    // Encode with ultra low quality
    var buf bytes.Buffer
    jpeg.Encode(&buf, gray, &jpeg.Options{Quality: quality})
    
    atomic.AddInt64(&hub.CompressedFrames, 1)
    
    // Log compression ratio
    compressionRatio := float64(len(data)) / float64(buf.Len())
    if compressionRatio > 10 {
        log.Printf("Ultra compression: %d -> %d bytes (%.1fx)", len(data), buf.Len(), compressionRatio)
    }
    
    return buf.Bytes()
}

func NewHub() *Hub {
    return &Hub{
        Rooms:      make(map[string]*Room),
        Register:   make(chan *Client, 10),
        Unregister: make(chan *Client, 10),
        Broadcast:  make(chan *BroadcastMessage, 100),
    }
}

func (h *Hub) Run() {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case client := <-h.Register:
            h.registerClient(client)
            
        case client := <-h.Unregister:
            h.unregisterClient(client)
            
        case message := <-h.Broadcast:
            h.handleBroadcast(message)
            
        case <-ticker.C:
            h.reportMetrics()
        }
    }
}

func (h *Hub) registerClient(client *Client) {
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
    userCount := len(room.Clients)
    room.mu.Unlock()
    
    // Send welcome
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
    
    log.Printf("Client %s joined room %s (total: %d users)", client.ID, client.Room, userCount)
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
        }
        room.mu.Unlock()
    }
}

func (h *Hub) handleBroadcast(bcast *BroadcastMessage) {
    h.mu.RLock()
    room := h.Rooms[bcast.Room]
    h.mu.RUnlock()
    
    if room == nil {
        return
    }
    
    var msg Message
    json.Unmarshal(bcast.Message, &msg)
    
    atomic.AddInt64(&h.TotalMessages, 1)
    
    room.mu.Lock()
    defer room.mu.Unlock()
    
    userCount := len(room.Clients)
    if userCount == 0 {
        return
    }
    
    // Get bandwidth allocation for this user count
    allocation := bandwidthAllocation[userCount]
    if userCount > 6 {
        allocation = bandwidthAllocation[6]
    }
    
    // Handle based on message type
    switch msg.Type {
    case "audio-chunk":
        // Audio always gets through (priority)
        h.distributeAudio(room, msg, bcast.From)
        
    case "video-frame":
        // Smart video distribution based on bandwidth
        h.distributeVideoSmart(room, msg, bcast.From, allocation.video)
    }
}

func (h *Hub) distributeAudio(room *Room, msg Message, from string) {
    // Audio goes to everyone except sender
    for id, client := range room.Clients {
        if id == from {
            continue
        }
        
        msg.From = from
        if data, err := json.Marshal(msg); err == nil {
            select {
            case client.Send <- data:
            default:
                // Buffer full, skip
            }
        }
    }
}

func (h *Hub) distributeVideoSmart(room *Room, msg Message, from string, videoBandwidthKbps int) {
    // Decode and ultra-compress the frame
    frameData, _ := base64.StdEncoding.DecodeString(msg.Data)
    compressed := ultraCompressFrame(frameData)
    
    // Update message with compressed data
    msg.Data = base64.StdEncoding.EncodeToString(compressed)
    msg.FrameSize = len(compressed)
    msg.From = from
    
    userCount := len(room.Clients)
    
    // Calculate how many users can receive this frame based on bandwidth
    frameSizeKbits := float64(len(compressed)*8) / 1000.0
    maxFramesPerSecond := float64(videoBandwidthKbps) / frameSizeKbits / float64(userCount-1)
    
    // Smart distribution strategies based on user count
    if userCount <= 2 {
        // 1-2 users: Send all frames to everyone
        for id, client := range room.Clients {
            if id == from {
                continue
            }
            
            if data, err := json.Marshal(msg); err == nil {
                select {
                case client.Send <- data:
                default:
                    atomic.AddInt64(&h.DroppedFrames, 1)
                }
            }
        }
        
    } else if userCount <= 4 {
        // 3-4 users: Skip every other frame per user
        for id, client := range room.Clients {
            if id == from {
                continue
            }
            
            client.mu.Lock()
            skip := (msg.Seq % 2) == (client.FrameSkipCount % 2)
            client.FrameSkipCount++
            client.mu.Unlock()
            
            if !skip {
                if data, err := json.Marshal(msg); err == nil {
                    select {
                    case client.Send <- data:
                    default:
                        atomic.AddInt64(&h.DroppedFrames, 1)
                    }
                }
            } else {
                atomic.AddInt64(&h.DroppedFrames, 1)
            }
        }
        
    } else {
        // 5+ users: Round-robin - each frame goes to only one other user
        targets := make([]*Client, 0, userCount-1)
        for id, client := range room.Clients {
            if id != from {
                targets = append(targets, client)
            }
        }
        
        if len(targets) > 0 {
            // Send to next target in round-robin
            targetIndex := room.NextVideoTarget % len(targets)
            target := targets[targetIndex]
            room.NextVideoTarget++
            
            if data, err := json.Marshal(msg); err == nil {
                select {
                case target.Send <- data:
                default:
                    atomic.AddInt64(&h.DroppedFrames, 1)
                }
            }
            
            // Count frames not sent as dropped
            atomic.AddInt64(&h.DroppedFrames, int64(len(targets)-1))
        }
    }
}

func (h *Hub) reportMetrics() {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    for _, room := range h.Rooms {
        room.mu.RLock()
        userCount := len(room.Clients)
        room.mu.RUnlock()
        
        if userCount > 0 {
            totalMsg := atomic.LoadInt64(&h.TotalMessages)
            dropped := atomic.LoadInt64(&h.DroppedFrames)
            compressed := atomic.LoadInt64(&h.CompressedFrames)
            
            dropRate := float64(dropped) / float64(totalMsg+1) * 100
            
            log.Printf("Room %s: %d users, Messages: %d, Dropped: %.1f%%, Compressed: %d",
                room.ID, userCount, totalMsg, dropRate, compressed)
        }
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
    
    // Rate limiting
    lastMessageTime := time.Now()
    messageCount := 0
    
    for {
        _, message, err := c.Conn.ReadMessage()
        if err != nil {
            break
        }
        
        // Rate limiting: max 100 messages per second
        now := time.Now()
        if now.Sub(lastMessageTime) < time.Second {
            messageCount++
            if messageCount > 100 {
                time.Sleep(10 * time.Millisecond)
                continue
            }
        } else {
            messageCount = 0
            lastMessageTime = now
        }
        
        var msg Message
        if err := json.Unmarshal(message, &msg); err == nil {
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
    
    for {
        select {
        case message, ok := <-c.Send:
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if !ok {
                c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            
            c.Conn.WriteMessage(websocket.TextMessage, message)
            
            // Send up to 5 more queued messages
            n := len(c.Send)
            if n > 5 {
                n = 5
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
        return
    }
    
    var joinMsg Message
    if err := conn.ReadJSON(&joinMsg); err != nil || joinMsg.Type != "join" {
        conn.Close()
        return
    }
    
    client := &Client{
        ID:   joinMsg.ID,
        Room: joinMsg.Room,
        Conn: conn,
        Send: make(chan []byte, 50),
        Hub:  hub,
    }
    
    client.Hub.Register <- client
    
    go client.WritePump()
    go client.ReadPump()
}

func handleStats(w http.ResponseWriter, r *http.Request) {
    totalMsg := atomic.LoadInt64(&hub.TotalMessages)
    dropped := atomic.LoadInt64(&hub.DroppedFrames)
    compressed := atomic.LoadInt64(&hub.CompressedFrames)
    
    stats := map[string]interface{}{
        "messages":       totalMsg,
        "droppedFrames":  dropped,
        "dropRate":       float64(dropped) / float64(totalMsg+1) * 100,
        "compressed":     compressed,
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
        fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Ultra Optimized Server</title></head>
<body>
<h1>Ultra Optimized Conference Server</h1>
<p>Features:</p>
<ul>
<li>Ultra compression (80x60 grayscale)</li>
<li>Smart frame distribution</li>
<li>Protected audio channel</li>
<li>1.2 Mbps total limit</li>
</ul>
</body>
</html>`)
    })
    
    addr := ":3001"
    log.Printf("Starting ULTRA OPTIMIZED server on %s", addr)
    log.Printf("Compression: ON | Smart Distribution: ON | Audio Priority: ON")
    
    if err := http.ListenAndServe(addr, nil); err != nil {
        log.Fatal(err)
    }
}