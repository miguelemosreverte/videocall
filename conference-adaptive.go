package main

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "image"
    "image/draw"
    _ "image/jpeg"
    _ "image/png"
    "log"
    "net/http"
    "sync"
    "sync/atomic"
    "time"

    "github.com/chai2010/webp"
    "github.com/gorilla/websocket"
    "github.com/nfnt/resize"
)

// Quality presets - from 144p to 4K
type QualityPreset struct {
    Name       string
    Width      uint
    Height     uint
    FPS        int
    Quality    float32
    Bitrate    int // Target bitrate in kbps
}

var QualityLevels = []QualityPreset{
    {"144p", 256, 144, 10, 0.3, 50},
    {"240p", 426, 240, 15, 0.4, 100},
    {"360p", 640, 360, 15, 0.5, 200},
    {"480p", 854, 480, 20, 0.6, 400},
    {"720p", 1280, 720, 30, 0.7, 800},
    {"1080p", 1920, 1080, 30, 0.8, 2000},
    {"1440p", 2560, 1440, 30, 0.85, 4000},
    {"4K", 3840, 2160, 30, 0.9, 8000},
    {"4K60", 3840, 2160, 60, 0.95, 12000},
}

// Client performance metrics
type ClientMetrics struct {
    Bandwidth          float64   // Measured in Mbps
    Latency           int64     // RTT in ms
    PacketLoss        float64   // Percentage
    JitterBuffer      []int64   // Latency samples
    FramesReceived    int64
    FramesDropped     int64
    LastUpdate        time.Time
    QualityScore      float64   // 0-100 quality score
    
    // Network feedback
    AckTimes          []int64   // Frame acknowledgment times
    BufferHealth      float64   // 0-1, how full is client buffer
    CPUUsage          float64   // Client CPU usage 0-100
    
    mu sync.RWMutex
}

// Enhanced Client with adaptive quality
type Client struct {
    ID            string
    Room          string
    Conn          *websocket.Conn
    Send          chan []byte
    Hub           *Hub
    
    // Quality management
    CurrentQuality    int // Index in QualityLevels
    TargetQuality     int
    QualityLocked     bool
    LastQualityChange time.Time
    
    // Performance tracking
    Metrics          *ClientMetrics
    LastFrameTime    time.Time
    FrameInterval    time.Duration
    
    // Feedback loop
    LastFeedbackTime time.Time
    FeedbackInterval time.Duration
    
    mu sync.RWMutex
}

// Message types
type Message struct {
    Type          string      `json:"type"`
    ID            string      `json:"id,omitempty"`
    Room          string      `json:"room,omitempty"`
    From          string      `json:"from,omitempty"`
    Data          string      `json:"data,omitempty"`
    Timestamp     int64       `json:"timestamp,omitempty"`
    Seq           int         `json:"seq,omitempty"`
    
    // Quality feedback
    Quality       string      `json:"quality,omitempty"`
    Width         int         `json:"width,omitempty"`
    Height        int         `json:"height,omitempty"`
    FPS           int         `json:"fps,omitempty"`
    
    // Client feedback
    Feedback      *ClientFeedback `json:"feedback,omitempty"`
}

type ClientFeedback struct {
    FramesReceived int64   `json:"framesReceived"`
    FramesDropped  int64   `json:"framesDropped"`
    BufferHealth   float64 `json:"bufferHealth"`   // 0-1
    CPUUsage       float64 `json:"cpuUsage"`       // 0-100
    Bandwidth      float64 `json:"bandwidth"`      // Mbps
    Latency        int64   `json:"latency"`        // ms
    RequestQuality string  `json:"requestQuality"` // Client requested quality
}

// Room with quality optimization
type Room struct {
    ID              string
    Clients         map[string]*Client
    
    // Aggregate metrics
    TotalBandwidth  float64
    AverageLatency  float64
    
    mu sync.RWMutex
}

// Hub manages everything
type Hub struct {
    Rooms      map[string]*Room
    Register   chan *Client
    Unregister chan *Client
    Broadcast  chan *BroadcastMessage
    
    // Global metrics
    TotalBandwidth   float64
    ActiveStreams    int64
    
    mu sync.RWMutex
}

type BroadcastMessage struct {
    Room    string
    Message []byte
    From    string
}

var (
    upgrader = websocket.Upgrader{
        ReadBufferSize:  1024 * 16, // Larger buffers for 4K
        WriteBufferSize: 1024 * 16,
        CheckOrigin: func(r *http.Request) bool { return true },
    }
    
    hub *Hub
)

// Adaptive quality algorithm
func (c *Client) calculateOptimalQuality() int {
    c.mu.RLock()
    metrics := c.Metrics
    current := c.CurrentQuality
    c.mu.RUnlock()
    
    if metrics == nil {
        return 0 // Start with lowest quality
    }
    
    metrics.mu.RLock()
    bandwidth := metrics.Bandwidth
    latency := metrics.Latency
    packetLoss := metrics.PacketLoss
    bufferHealth := metrics.BufferHealth
    cpuUsage := metrics.CPUUsage
    metrics.mu.RUnlock()
    
    // Calculate quality score (0-100)
    score := 100.0
    
    // Bandwidth factor (most important)
    if bandwidth > 0 {
        targetBitrate := float64(QualityLevels[current].Bitrate) / 1000.0 // Convert to Mbps
        bandwidthRatio := bandwidth / targetBitrate
        if bandwidthRatio < 1.0 {
            score *= bandwidthRatio
        }
    }
    
    // Latency factor
    if latency > 200 {
        score *= (200.0 / float64(latency))
    }
    
    // Packet loss factor
    if packetLoss > 0 {
        score *= (1.0 - packetLoss/100.0)
    }
    
    // Buffer health factor
    score *= bufferHealth
    
    // CPU usage factor
    if cpuUsage > 80 {
        score *= (100.0 - cpuUsage) / 20.0
    }
    
    // Determine target quality based on score
    targetQuality := current
    
    if score > 90 && bandwidth > float64(QualityLevels[current].Bitrate)/1000.0*1.5 {
        // Excellent conditions, try to increase quality
        if current < len(QualityLevels)-1 {
            targetQuality = current + 1
        }
    } else if score > 80 && bandwidth > float64(QualityLevels[current].Bitrate)/1000.0*1.2 {
        // Good conditions, maybe increase
        if current < len(QualityLevels)-2 {
            targetQuality = current + 1
        }
    } else if score < 50 {
        // Poor conditions, decrease quality
        if current > 0 {
            targetQuality = current - 1
        }
    } else if score < 30 {
        // Very poor conditions, decrease more
        if current > 1 {
            targetQuality = current - 2
        }
    }
    
    // Special handling for 4K - only if excellent bandwidth
    if targetQuality >= 6 { // 1440p or higher
        if bandwidth < 5.0 { // Need at least 5 Mbps for 1440p
            targetQuality = 5 // Cap at 1080p
        }
    }
    if targetQuality >= 7 { // 4K
        if bandwidth < 10.0 { // Need at least 10 Mbps for 4K
            targetQuality = 6 // Cap at 1440p
        }
    }
    if targetQuality == 8 { // 4K 60fps
        if bandwidth < 15.0 || latency > 50 { // Need excellent conditions
            targetQuality = 7 // Drop to 4K 30fps
        }
    }
    
    return targetQuality
}

// Process client feedback
func (c *Client) processFeedback(feedback *ClientFeedback) {
    if c.Metrics == nil {
        c.Metrics = &ClientMetrics{
            LastUpdate:   time.Now(),
            JitterBuffer: make([]int64, 0, 100),
            AckTimes:     make([]int64, 0, 100),
        }
    }
    
    c.Metrics.mu.Lock()
    defer c.Metrics.mu.Unlock()
    
    // Update metrics
    c.Metrics.Bandwidth = feedback.Bandwidth
    c.Metrics.Latency = feedback.Latency
    c.Metrics.FramesReceived = feedback.FramesReceived
    c.Metrics.FramesDropped = feedback.FramesDropped
    c.Metrics.BufferHealth = feedback.BufferHealth
    c.Metrics.CPUUsage = feedback.CPUUsage
    c.Metrics.LastUpdate = time.Now()
    
    // Calculate packet loss
    if c.Metrics.FramesReceived > 0 {
        c.Metrics.PacketLoss = float64(c.Metrics.FramesDropped) / float64(c.Metrics.FramesReceived) * 100
    }
    
    // Add to jitter buffer
    c.Metrics.JitterBuffer = append(c.Metrics.JitterBuffer, feedback.Latency)
    if len(c.Metrics.JitterBuffer) > 100 {
        c.Metrics.JitterBuffer = c.Metrics.JitterBuffer[1:]
    }
    
    // Calculate quality score
    c.Metrics.QualityScore = 100.0
    if c.Metrics.PacketLoss > 0 {
        c.Metrics.QualityScore -= c.Metrics.PacketLoss * 2
    }
    if c.Metrics.Latency > 100 {
        c.Metrics.QualityScore -= float64(c.Metrics.Latency-100) / 10
    }
    if c.Metrics.BufferHealth < 0.5 {
        c.Metrics.QualityScore *= c.Metrics.BufferHealth * 2
    }
}

// WebP compression with quality settings
func compressToWebP(data []byte, quality *QualityPreset) ([]byte, error) {
    img, _, err := image.Decode(bytes.NewReader(data))
    if err != nil {
        return nil, err
    }
    
    // Resize if needed
    if quality.Width > 0 && quality.Height > 0 {
        img = resize.Resize(quality.Width, quality.Height, img, resize.Lanczos3)
    }
    
    // Convert to RGBA
    bounds := img.Bounds()
    rgba := image.NewRGBA(bounds)
    draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
    
    // Compress to WebP with quality setting
    var buf bytes.Buffer
    options := &webp.Options{
        Lossless: false,
        Quality:  quality.Quality,
        Exact:    false,
    }
    
    if err := webp.Encode(&buf, rgba, options); err != nil {
        return nil, err
    }
    
    return buf.Bytes(), nil
}

// Handle client connection
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Upgrade error:", err)
        return
    }
    
    client := &Client{
        ID:               fmt.Sprintf("client-%d", time.Now().UnixNano()),
        Conn:             conn,
        Send:             make(chan []byte, 256),
        Hub:              hub,
        CurrentQuality:   0, // Start with lowest
        TargetQuality:    0,
        Metrics:          &ClientMetrics{},
        FeedbackInterval: time.Second,
        LastFrameTime:    time.Now(),
    }
    
    hub.Register <- client
    
    go client.writePump()
    go client.readPump()
    go client.qualityMonitor()
}

// Quality monitoring goroutine
func (c *Client) qualityMonitor() {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // Calculate optimal quality
            optimal := c.calculateOptimalQuality()
            
            c.mu.Lock()
            oldQuality := c.CurrentQuality
            
            // Smooth quality transitions
            if optimal > c.CurrentQuality && time.Since(c.LastQualityChange) > 2*time.Second {
                c.CurrentQuality++
                c.LastQualityChange = time.Now()
            } else if optimal < c.CurrentQuality && time.Since(c.LastQualityChange) > 500*time.Millisecond {
                c.CurrentQuality--
                c.LastQualityChange = time.Now()
            }
            
            newQuality := c.CurrentQuality
            c.mu.Unlock()
            
            // Notify client of quality change
            if newQuality != oldQuality {
                quality := QualityLevels[newQuality]
                msg := Message{
                    Type:    "quality-change",
                    Quality: quality.Name,
                    Width:   int(quality.Width),
                    Height:  int(quality.Height),
                    FPS:     quality.FPS,
                }
                
                data, _ := json.Marshal(msg)
                select {
                case c.Send <- data:
                default:
                }
                
                log.Printf("Client %s quality changed: %s -> %s (bandwidth: %.2f Mbps, latency: %dms)",
                    c.ID, QualityLevels[oldQuality].Name, quality.Name,
                    c.Metrics.Bandwidth, c.Metrics.Latency)
            }
        }
    }
}

func (c *Client) readPump() {
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
        _, data, err := c.Conn.ReadMessage()
        if err != nil {
            break
        }
        
        var msg Message
        if err := json.Unmarshal(data, &msg); err != nil {
            continue
        }
        
        switch msg.Type {
        case "join":
            c.Room = msg.Room
            hub.joinRoom(c, msg.Room)
            
        case "frame":
            // Process and compress frame based on current quality
            c.mu.RLock()
            quality := QualityLevels[c.CurrentQuality]
            c.mu.RUnlock()
            
            if decoded, err := base64.StdEncoding.DecodeString(msg.Data); err == nil {
                if compressed, err := compressToWebP(decoded, &quality); err == nil {
                    // Broadcast compressed frame
                    outMsg := Message{
                        Type:      "webp-frame",
                        From:      c.ID,
                        Data:      base64.StdEncoding.EncodeToString(compressed),
                        Timestamp: time.Now().UnixMilli(),
                        Quality:   quality.Name,
                        Width:     int(quality.Width),
                        Height:    int(quality.Height),
                        FPS:       quality.FPS,
                    }
                    
                    if outData, err := json.Marshal(outMsg); err == nil {
                        hub.Broadcast <- &BroadcastMessage{
                            Room:    c.Room,
                            Message: outData,
                            From:    c.ID,
                        }
                    }
                }
            }
            
        case "audio":
            // Forward audio with priority
            hub.Broadcast <- &BroadcastMessage{
                Room:    c.Room,
                Message: data,
                From:    c.ID,
            }
            
        case "feedback":
            // Process client feedback
            if msg.Feedback != nil {
                c.processFeedback(msg.Feedback)
                
                // Check if client requested specific quality
                if msg.Feedback.RequestQuality != "" {
                    for i, q := range QualityLevels {
                        if q.Name == msg.Feedback.RequestQuality {
                            c.mu.Lock()
                            c.TargetQuality = i
                            c.mu.Unlock()
                            break
                        }
                    }
                }
            }
            
        case "ping":
            // Respond with pong
            pong := Message{
                Type:      "pong",
                Timestamp: msg.Timestamp,
            }
            if data, err := json.Marshal(pong); err == nil {
                c.Send <- data
            }
        }
    }
}

func (c *Client) writePump() {
    ticker := time.NewTicker(54 * time.Second)
    defer func() {
        ticker.Stop()
        c.Conn.Close()
    }()
    
    for {
        select {
        case message, ok := <-c.Send:
            if !ok {
                c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }
            
            c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
            if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
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

// Hub methods
func (h *Hub) run() {
    for {
        select {
        case client := <-h.Register:
            h.mu.Lock()
            if room, ok := h.Rooms[client.Room]; ok {
                room.mu.Lock()
                room.Clients[client.ID] = client
                room.mu.Unlock()
            }
            atomic.AddInt64(&h.ActiveStreams, 1)
            h.mu.Unlock()
            
            log.Printf("Client registered: %s (room: %s)", client.ID, client.Room)
            
        case client := <-h.Unregister:
            h.mu.Lock()
            if room, ok := h.Rooms[client.Room]; ok {
                room.mu.Lock()
                delete(room.Clients, client.ID)
                room.mu.Unlock()
            }
            atomic.AddInt64(&h.ActiveStreams, -1)
            close(client.Send)
            h.mu.Unlock()
            
            log.Printf("Client unregistered: %s", client.ID)
            
        case broadcast := <-h.Broadcast:
            h.mu.RLock()
            room, ok := h.Rooms[broadcast.Room]
            h.mu.RUnlock()
            
            if ok {
                room.mu.RLock()
                clients := make([]*Client, 0, len(room.Clients))
                for _, client := range room.Clients {
                    if client.ID != broadcast.From {
                        clients = append(clients, client)
                    }
                }
                room.mu.RUnlock()
                
                // Send to all clients in parallel
                for _, client := range clients {
                    select {
                    case client.Send <- broadcast.Message:
                    default:
                        // Client buffer full, skip
                    }
                }
            }
        }
    }
}

func (h *Hub) joinRoom(client *Client, roomID string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    
    room, exists := h.Rooms[roomID]
    if !exists {
        room = &Room{
            ID:      roomID,
            Clients: make(map[string]*Client),
        }
        h.Rooms[roomID] = room
    }
    
    room.mu.Lock()
    room.Clients[client.ID] = client
    
    // Notify other clients
    users := make([]string, 0, len(room.Clients))
    for id := range room.Clients {
        if id != client.ID {
            users = append(users, id)
        }
    }
    room.mu.Unlock()
    
    // Send room info to client
    msg := Message{
        Type: "participants",
        Room: roomID,
    }
    if data, err := json.Marshal(msg); err == nil {
        client.Send <- data
    }
}

func main() {
    hub = &Hub{
        Rooms:      make(map[string]*Room),
        Register:   make(chan *Client),
        Unregister: make(chan *Client),
        Broadcast:  make(chan *BroadcastMessage, 256),
    }
    
    go hub.run()
    
    // Serve a simple status page
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        html := `<!DOCTYPE html>
<html>
<head>
    <title>Adaptive WebP Conference Server</title>
    <style>
        body { font-family: -apple-system, sans-serif; padding: 40px; background: linear-gradient(135deg, #667eea, #764ba2); color: white; }
        .container { max-width: 800px; margin: 0 auto; background: rgba(255,255,255,0.1); padding: 30px; border-radius: 20px; }
        h1 { margin-bottom: 20px; }
        .status { background: rgba(0,255,0,0.2); padding: 10px; border-radius: 10px; }
        .features { margin-top: 30px; }
        .features li { margin: 10px 0; }
        .quality-table { width: 100%; margin-top: 20px; border-collapse: collapse; }
        .quality-table th, .quality-table td { padding: 10px; text-align: left; border-bottom: 1px solid rgba(255,255,255,0.2); }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 Adaptive WebP Conference Server</h1>
        <div class="status">✅ Server is running on :3001</div>
        
        <div class="features">
            <h2>Adaptive Quality System</h2>
            <p>Real-time quality adjustment based on client feedback</p>
            
            <table class="quality-table">
                <tr>
                    <th>Quality</th>
                    <th>Resolution</th>
                    <th>FPS</th>
                    <th>Bitrate</th>
                    <th>Min Bandwidth</th>
                </tr>
                <tr><td>144p</td><td>256x144</td><td>10</td><td>50 kbps</td><td>0.1 Mbps</td></tr>
                <tr><td>240p</td><td>426x240</td><td>15</td><td>100 kbps</td><td>0.2 Mbps</td></tr>
                <tr><td>360p</td><td>640x360</td><td>15</td><td>200 kbps</td><td>0.4 Mbps</td></tr>
                <tr><td>480p</td><td>854x480</td><td>20</td><td>400 kbps</td><td>0.8 Mbps</td></tr>
                <tr><td>720p</td><td>1280x720</td><td>30</td><td>800 kbps</td><td>1.5 Mbps</td></tr>
                <tr><td>1080p</td><td>1920x1080</td><td>30</td><td>2 Mbps</td><td>3 Mbps</td></tr>
                <tr><td>1440p</td><td>2560x1440</td><td>30</td><td>4 Mbps</td><td>5 Mbps</td></tr>
                <tr><td>4K</td><td>3840x2160</td><td>30</td><td>8 Mbps</td><td>10 Mbps</td></tr>
                <tr><td>4K 60fps</td><td>3840x2160</td><td>60</td><td>12 Mbps</td><td>15 Mbps</td></tr>
            </table>
            
            <h3>Features:</h3>
            <ul>
                <li>✨ Automatic quality scaling from 144p to 4K 60fps</li>
                <li>📊 Real-time bandwidth measurement</li>
                <li>🎯 Client feedback loop for optimal quality</li>
                <li>🔄 Smooth quality transitions</li>
                <li>💾 WebP compression with dynamic quality</li>
                <li>⚡ Ultra-low latency prioritization</li>
                <li>🎵 Audio always prioritized</li>
                <li>📈 CPU and buffer health monitoring</li>
            </ul>
        </div>
        
        <div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid rgba(255,255,255,0.2);">
            <p>WebSocket endpoint: <code>ws://localhost:3001/ws</code></p>
        </div>
    </div>
</body>
</html>`
        w.Header().Set("Content-Type", "text/html")
        w.Write([]byte(html))
    })
    
    http.HandleFunc("/ws", handleWebSocket)
    
    log.Println("Starting Adaptive WebP Conference Server on :3001")
    log.Println("Quality range: 144p @ 50kbps to 4K 60fps @ 12Mbps")
    log.Fatal(http.ListenAndServe(":3001", nil))
}