package main

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "image"
    "image/draw"
    _ "image/jpeg" // Register JPEG decoder
    _ "image/png"  // Register PNG decoder
    "log"
    "net/http"
    "sync"
    "sync/atomic"
    "time"

    "github.com/chai2010/webp"
    "github.com/gorilla/websocket"
    "github.com/nfnt/resize"
)

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
    CompressionType string `json:"compressionType,omitempty"`
}

// Client with smart bandwidth management
type Client struct {
    ID            string
    Room          string
    Conn          *websocket.Conn
    Send          chan []byte
    Hub           *Hub
    
    // Frame management
    LastFrameSeq      int
    FrameSkipCount    int
    LastAudioTime     time.Time
    
    mu sync.RWMutex
}

// Room with optimized distribution
type Room struct {
    ID              string
    Clients         map[string]*Client
    
    // Frame distribution
    NextVideoTarget int
    LastFrameTime   time.Time
    
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
    BytesSaved       int64
    
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
    
    // Bandwidth allocations for 1.2 Mbps total
    // Prioritize audio, use WebP for video
    bandwidthAllocation = map[int]struct{ audioPct, videoPct int }{
        1: {audioPct: 10, videoPct: 90},  // 1 user: 120kb audio, 1080kb video
        2: {audioPct: 15, videoPct: 85},  // 2 users: 90kb audio each, 510kb video each
        3: {audioPct: 20, videoPct: 80},  // 3 users: 80kb audio each, 320kb video each
        4: {audioPct: 25, videoPct: 75},  // 4 users: 75kb audio each, 225kb video each
        5: {audioPct: 30, videoPct: 70},  // 5 users: 72kb audio each, 168kb video each
        6: {audioPct: 35, videoPct: 65},  // 6 users: 70kb audio each, 130kb video each
    }
)

// WebP compression with adaptive quality
func webpCompressFrame(data []byte, userCount int) []byte {
    // Decode the image
    img, format, err := image.Decode(bytes.NewReader(data))
    if err != nil {
        log.Printf("Failed to decode image: %v (format: %s, size: %d bytes)", err, format, len(data))
        return data
    }
    
    bounds := img.Bounds()
    originalSize := bounds.Max.X
    
    // Adaptive sizing based on user count
    var targetWidth uint
    var quality float32
    
    switch userCount {
    case 1:
        targetWidth = 320  // Good quality for single user
        quality = 75
    case 2:
        targetWidth = 240  // Medium quality
        quality = 65
    case 3:
        targetWidth = 180  // Lower resolution
        quality = 55
    case 4:
        targetWidth = 120  // Small
        quality = 45
    case 5:
        targetWidth = 100  // Tiny
        quality = 35
    default:
        targetWidth = 80   // Ultra tiny for 6+
        quality = 25
    }
    
    // Resize if needed
    var finalImg image.Image
    if uint(originalSize) > targetWidth {
        finalImg = resize.Resize(targetWidth, 0, img, resize.Lanczos3)
    } else {
        finalImg = img
    }
    
    // For 5+ users, use grayscale to save more
    if userCount >= 5 {
        gray := image.NewGray(finalImg.Bounds())
        draw.Draw(gray, gray.Bounds(), finalImg, finalImg.Bounds().Min, draw.Src)
        finalImg = gray
    }
    
    // Encode to WebP
    var buf bytes.Buffer
    options := &webp.Options{
        Lossless: false,
        Quality:  quality,
        Exact:    false,
    }
    
    if err := webp.Encode(&buf, finalImg, options); err != nil {
        // Fallback to original if WebP fails
        return data
    }
    
    compressedSize := buf.Len()
    atomic.AddInt64(&hub.CompressedFrames, 1)
    atomic.AddInt64(&hub.BytesSaved, int64(len(data)-compressedSize))
    
    // Log significant compressions
    ratio := float64(len(data)) / float64(compressedSize)
    if ratio > 5 {
        log.Printf("WebP compression: %d -> %d bytes (%.1fx) for %d users", 
            len(data), compressedSize, ratio, userCount)
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
    ticker := time.NewTicker(5 * time.Second)
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
    
    // Send welcome with compression info
    welcome := Message{
        Type: "welcome",
        ID:   client.ID,
        CompressionType: "webp",
    }
    
    if data, err := json.Marshal(welcome); err == nil {
        select {
        case client.Send <- data:
        default:
        }
    }
    
    log.Printf("Client %s joined room %s (total: %d users, using WebP)", 
        client.ID, client.Room, userCount)
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
            log.Printf("Client %s left room %s", client.ID, client.Room)
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
    
    room.mu.RLock()
    userCount := len(room.Clients)
    room.mu.RUnlock()
    
    if userCount == 0 {
        return
    }
    
    switch msg.Type {
    case "audio-chunk":
        // Audio always gets through
        h.distributeAudio(room, msg, bcast.From)
        
    case "video-frame":
        // Compress with WebP and distribute smartly
        h.distributeVideoWebP(room, msg, bcast.From, userCount)
    }
}

func (h *Hub) distributeAudio(room *Room, msg Message, from string) {
    room.mu.RLock()
    defer room.mu.RUnlock()
    
    // Audio goes to everyone except sender
    for id, client := range room.Clients {
        if id == from {
            continue
        }
        
        // Mark audio priority
        client.mu.Lock()
        client.LastAudioTime = time.Now()
        client.mu.Unlock()
        
        msg.From = from
        if data, err := json.Marshal(msg); err == nil {
            select {
            case client.Send <- data:
            default:
                // Only drop if buffer truly full
            }
        }
    }
}

func (h *Hub) distributeVideoWebP(room *Room, msg Message, from string, userCount int) {
    // Decode and compress with WebP
    frameData, _ := base64.StdEncoding.DecodeString(msg.Data)
    compressed := webpCompressFrame(frameData, userCount)
    
    // Update message with compressed data
    msg.Data = base64.StdEncoding.EncodeToString(compressed)
    msg.FrameSize = len(compressed)
    msg.From = from
    msg.CompressionType = "webp"
    
    room.mu.RLock()
    defer room.mu.RUnlock()
    
    // Calculate frame distribution strategy
    targetFPS := 30.0 / float64(userCount) // Distribute FPS among users
    
    if userCount <= 2 {
        // 1-2 users: Send all frames
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
        // 3-4 users: Adaptive frame skipping
        now := time.Now()
        minFrameInterval := time.Duration(1000/targetFPS) * time.Millisecond
        
        if now.Sub(room.LastFrameTime) < minFrameInterval {
            atomic.AddInt64(&h.DroppedFrames, int64(userCount-1))
            return
        }
        room.LastFrameTime = now
        
        for id, client := range room.Clients {
            if id == from {
                continue
            }
            
            // Skip if client is receiving audio
            client.mu.RLock()
            receivingAudio := time.Since(client.LastAudioTime) < 100*time.Millisecond
            client.mu.RUnlock()
            
            if !receivingAudio {
                if data, err := json.Marshal(msg); err == nil {
                    select {
                    case client.Send <- data:
                    default:
                        atomic.AddInt64(&h.DroppedFrames, 1)
                    }
                }
            }
        }
        
    } else {
        // 5+ users: Round-robin with priority
        targets := make([]*Client, 0, userCount-1)
        for id, client := range room.Clients {
            if id != from {
                targets = append(targets, client)
            }
        }
        
        if len(targets) > 0 {
            // Send to 2 targets per frame for better coverage
            sendCount := 2
            if userCount > 8 {
                sendCount = 1
            }
            
            for i := 0; i < sendCount && i < len(targets); i++ {
                targetIdx := (room.NextVideoTarget + i) % len(targets)
                target := targets[targetIdx]
                
                if data, err := json.Marshal(msg); err == nil {
                    select {
                    case target.Send <- data:
                    default:
                        atomic.AddInt64(&h.DroppedFrames, 1)
                    }
                }
            }
            room.NextVideoTarget = (room.NextVideoTarget + sendCount) % len(targets)
            
            // Count unsent as dropped
            atomic.AddInt64(&h.DroppedFrames, int64(len(targets)-sendCount))
        }
    }
}

func (h *Hub) reportMetrics() {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    totalMsg := atomic.LoadInt64(&h.TotalMessages)
    dropped := atomic.LoadInt64(&h.DroppedFrames)
    compressed := atomic.LoadInt64(&h.CompressedFrames)
    saved := atomic.LoadInt64(&h.BytesSaved)
    
    if totalMsg > 0 {
        dropRate := float64(dropped) / float64(totalMsg) * 100
        savedMB := float64(saved) / (1024 * 1024)
        
        log.Printf("Stats - Messages: %d, Dropped: %.1f%%, WebP frames: %d, Saved: %.1f MB",
            totalMsg, dropRate, compressed, savedMB)
    }
    
    for _, room := range h.Rooms {
        room.mu.RLock()
        userCount := len(room.Clients)
        room.mu.RUnlock()
        
        if userCount > 0 {
            log.Printf("Room %s: %d users active", room.ID, userCount)
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
    limiter := time.NewTicker(10 * time.Millisecond) // 100 msg/sec max
    defer limiter.Stop()
    
    for {
        _, message, err := c.Conn.ReadMessage()
        if err != nil {
            break
        }
        
        <-limiter.C // Rate limit
        
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
            
            // Batch send queued messages
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
        Send: make(chan []byte, 100), // Larger buffer for WebP frames
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
    saved := atomic.LoadInt64(&hub.BytesSaved)
    
    stats := map[string]interface{}{
        "messages":       totalMsg,
        "droppedFrames":  dropped,
        "dropRate":       float64(dropped) / float64(totalMsg+1) * 100,
        "webpFrames":     compressed,
        "bytesSaved":     saved,
        "mbSaved":        float64(saved) / (1024 * 1024),
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
<head><title>WebP Conference Server</title></head>
<body>
<h1>WebP-Optimized Conference Server</h1>
<p>Features:</p>
<ul>
<li>WebP compression (25-35% better than JPEG)</li>
<li>Adaptive quality based on user count</li>
<li>Protected audio channel</li>
<li>Smart frame distribution</li>
<li>1.2 Mbps bandwidth compliance</li>
</ul>
<p>Compression rates:</p>
<ul>
<li>1 user: 320px @ 75% quality</li>
<li>2 users: 240px @ 65% quality</li>
<li>3 users: 180px @ 55% quality</li>
<li>4 users: 120px @ 45% quality</li>
<li>5+ users: 80-100px @ 25-35% quality + grayscale</li>
</ul>
</body>
</html>`)
    })
    
    addr := ":3001"
    log.Printf("Starting WebP-optimized server on %s", addr)
    log.Printf("Features: WebP compression | Smart distribution | Audio priority")
    
    if err := http.ListenAndServe(addr, nil); err != nil {
        log.Fatal(err)
    }
}