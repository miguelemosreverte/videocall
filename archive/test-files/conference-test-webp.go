package main

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "image"
    "image/color"
    "image/jpeg"
    "log"
    "math/rand"
    "sync"
    "sync/atomic"
    "time"

    "github.com/gorilla/websocket"
)

// Test configuration adapted for WebP server
type TestConfig struct {
    Users      int
    Duration   time.Duration
    ServerURL  string
    Room       string
    Resolution string
}

// Message types matching server
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

// TestClient simulates a user WITHOUT pre-dropping frames
type TestClient struct {
    ID            string
    Room          string
    Conn          *websocket.Conn
    
    // Stats
    FramesSent    int64
    AudioSent     int64
    FramesRecv    int64
    AudioRecv     int64
    BytesSent     int64
    BytesRecv     int64
    
    // Test data
    VideoFrame    []byte
    AudioChunk    []byte
    
    mu sync.Mutex
}

// Metrics for analysis
type TestMetrics struct {
    VideoLatency  float64
    AudioLatency  float64
    VideoFPS      float64
    VideoLoss     float64
    AudioLoss     float64
    BandwidthKbps float64
}

func generateTestFrame(width, height int) []byte {
    // Create a test image
    img := image.NewRGBA(image.Rect(0, 0, width, height))
    
    // Draw something simple
    for y := 0; y < height; y++ {
        for x := 0; x < width; x++ {
            c := color.RGBA{
                R: uint8((x + rand.Intn(50)) % 256),
                G: uint8((y + rand.Intn(50)) % 256),
                B: uint8(rand.Intn(256)),
                A: 255,
            }
            img.Set(x, y, c)
        }
    }
    
    // Encode as JPEG (server will convert to WebP)
    var buf bytes.Buffer
    jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70})
    return buf.Bytes()
}

func generateAudioChunk() []byte {
    // Generate 20ms of audio at 8kHz (160 samples)
    chunk := make([]byte, 320)
    for i := range chunk {
        chunk[i] = byte(rand.Intn(256))
    }
    return chunk
}

func (c *TestClient) connect(serverURL string) error {
    conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
    if err != nil {
        return err
    }
    c.Conn = conn
    
    // Send join message
    join := Message{
        Type: "join",
        ID:   c.ID,
        Room: c.Room,
    }
    return conn.WriteJSON(join)
}

func (c *TestClient) sendLoop(duration time.Duration) {
    start := time.Now()
    frameSeq := 0
    audioSeq := 0
    
    // Send at natural rates - let server handle compression
    videoTicker := time.NewTicker(33 * time.Millisecond)  // 30 FPS
    audioTicker := time.NewTicker(20 * time.Millisecond)  // 50 packets/sec
    
    defer videoTicker.Stop()
    defer audioTicker.Stop()
    
    for time.Since(start) < duration {
        select {
        case <-videoTicker.C:
            // Send video frame
            msg := Message{
                Type:      "video-frame",
                Data:      base64.StdEncoding.EncodeToString(c.VideoFrame),
                Seq:       frameSeq,
                Timestamp: time.Now().UnixMilli(),
            }
            
            if data, err := json.Marshal(msg); err == nil {
                if err := c.Conn.WriteMessage(websocket.TextMessage, data); err == nil {
                    atomic.AddInt64(&c.FramesSent, 1)
                    atomic.AddInt64(&c.BytesSent, int64(len(data)))
                    frameSeq++
                }
            }
            
        case <-audioTicker.C:
            // Send audio chunk
            msg := Message{
                Type:      "audio-chunk",
                Data:      base64.StdEncoding.EncodeToString(c.AudioChunk),
                Seq:       audioSeq,
                Timestamp: time.Now().UnixMilli(),
            }
            
            if data, err := json.Marshal(msg); err == nil {
                if err := c.Conn.WriteMessage(websocket.TextMessage, data); err == nil {
                    atomic.AddInt64(&c.AudioSent, 1)
                    atomic.AddInt64(&c.BytesSent, int64(len(data)))
                    audioSeq++
                }
            }
        }
    }
}

func (c *TestClient) recvLoop(duration time.Duration) {
    start := time.Now()
    
    for time.Since(start) < duration {
        c.Conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
        
        _, data, err := c.Conn.ReadMessage()
        if err != nil {
            // Check if it's just a timeout (which is expected)
            if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
                continue
            }
            // Check if connection was closed
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                return
            }
            // For any other error, continue
            continue
        }
        
        var msg Message
        if err := json.Unmarshal(data, &msg); err == nil {
            atomic.AddInt64(&c.BytesRecv, int64(len(data)))
            
            switch msg.Type {
            case "video-frame":
                atomic.AddInt64(&c.FramesRecv, 1)
            case "audio-chunk":
                atomic.AddInt64(&c.AudioRecv, 1)
            }
        }
    }
}

func runTest(config TestConfig) TestMetrics {
    fmt.Printf("\n🧪 Testing %d users for %v\n", config.Users, config.Duration)
    
    // Create clients
    clients := make([]*TestClient, config.Users)
    
    // Parse resolution
    var width, height int
    fmt.Sscanf(config.Resolution, "%dx%d", &width, &height)
    
    // Generate test data
    videoFrame := generateTestFrame(width, height)
    audioChunk := generateAudioChunk()
    
    fmt.Printf("   Frame size: %d bytes (before compression)\n", len(videoFrame))
    
    // Initialize clients
    for i := 0; i < config.Users; i++ {
        clients[i] = &TestClient{
            ID:         fmt.Sprintf("test-user-%d", i+1),
            Room:       config.Room,
            VideoFrame: videoFrame,
            AudioChunk: audioChunk,
        }
    }
    
    // Connect all clients
    for _, client := range clients {
        if err := client.connect(config.ServerURL); err != nil {
            log.Printf("Failed to connect client %s: %v", client.ID, err)
            continue
        }
    }
    
    time.Sleep(100 * time.Millisecond) // Let connections stabilize
    
    // Start send/recv loops
    var wg sync.WaitGroup
    
    for _, client := range clients {
        wg.Add(1)
        
        go func(c *TestClient) {
            defer wg.Done()
            c.sendLoop(config.Duration)
        }(client)
        
        // Only start receive loop if there are other users to receive from
        if config.Users > 1 {
            wg.Add(1)
            go func(c *TestClient) {
                defer wg.Done()
                c.recvLoop(config.Duration)
            }(client)
        }
    }
    
    wg.Wait()
    
    // Calculate metrics
    var totalFramesSent, totalAudioSent int64
    var totalFramesRecv, totalAudioRecv int64
    var totalBytesSent, totalBytesRecv int64
    
    for _, client := range clients {
        sent := atomic.LoadInt64(&client.FramesSent)
        recv := atomic.LoadInt64(&client.FramesRecv)
        audioS := atomic.LoadInt64(&client.AudioSent)
        audioR := atomic.LoadInt64(&client.AudioRecv)
        bytesS := atomic.LoadInt64(&client.BytesSent)
        bytesR := atomic.LoadInt64(&client.BytesRecv)
        
        totalFramesSent += sent
        totalFramesRecv += recv
        totalAudioSent += audioS
        totalAudioRecv += audioR
        totalBytesSent += bytesS
        totalBytesRecv += bytesR
        
        client.Conn.Close()
    }
    
    // Expected receives (each client receives from all others)
    expectedFrames := totalFramesSent * int64(config.Users-1)
    expectedAudio := totalAudioSent * int64(config.Users-1)
    
    // Calculate loss percentages
    videoLoss := 0.0
    if expectedFrames > 0 {
        videoLoss = float64(expectedFrames-totalFramesRecv) / float64(expectedFrames) * 100
    }
    
    audioLoss := 0.0
    if expectedAudio > 0 {
        audioLoss = float64(expectedAudio-totalAudioRecv) / float64(expectedAudio) * 100
    }
    
    // Calculate FPS
    fps := float64(totalFramesRecv) / float64(config.Users) / config.Duration.Seconds()
    
    // Calculate bandwidth
    totalBytes := totalBytesSent + totalBytesRecv
    bandwidthKbps := float64(totalBytes*8) / config.Duration.Seconds() / 1000
    
    return TestMetrics{
        VideoFPS:      fps,
        VideoLoss:     videoLoss,
        AudioLoss:     audioLoss,
        BandwidthKbps: bandwidthKbps,
    }
}

func main() {
    fmt.Println("🎯 WebP Conference Server Test")
    fmt.Println("================================")
    fmt.Println("Testing WITHOUT client-side frame dropping")
    fmt.Println()
    
    serverURL := "ws://localhost:3001/ws"
    
    // Test scenarios
    scenarios := []struct {
        name       string
        users      int
        duration   time.Duration
        resolution string
    }{
        {"Single User", 1, 5 * time.Second, "320x240"},
        {"Two Users", 2, 5 * time.Second, "320x240"},
        {"Three Users", 3, 5 * time.Second, "160x120"},
        {"Four Users", 4, 5 * time.Second, "160x120"},
        {"Six Users", 6, 5 * time.Second, "160x120"},
    }
    
    fmt.Printf("Server: %s\n", serverURL)
    fmt.Println()
    
    results := make([]TestMetrics, 0)
    
    for i, scenario := range scenarios {
        fmt.Printf("📊 Test %d: %s\n", i+1, scenario.name)
        fmt.Printf("   Users: %d, Resolution: %s\n", scenario.users, scenario.resolution)
        
        config := TestConfig{
            Users:      scenario.users,
            Duration:   scenario.duration,
            ServerURL:  serverURL,
            Room:       fmt.Sprintf("test-room-%d", time.Now().Unix()),
            Resolution: scenario.resolution,
        }
        
        metrics := runTest(config)
        results = append(results, metrics)
        
        fmt.Printf("\n   ✅ Results:\n")
        fmt.Printf("   ├─ Video FPS: %.1f\n", metrics.VideoFPS)
        fmt.Printf("   ├─ Video Loss: %.1f%%\n", metrics.VideoLoss)
        fmt.Printf("   ├─ Audio Loss: %.1f%%\n", metrics.AudioLoss)
        fmt.Printf("   ├─ Bandwidth: %.0f kbps\n", metrics.BandwidthKbps)
        
        if metrics.BandwidthKbps <= 1200 {
            fmt.Printf("   └─ ✅ Within VPS limit (%.1f%% usage)\n", metrics.BandwidthKbps/1200*100)
        } else {
            fmt.Printf("   └─ ⚠️  Exceeds VPS limit by %.0f kbps\n", metrics.BandwidthKbps-1200)
        }
        
        fmt.Println()
        time.Sleep(2 * time.Second) // Pause between tests
    }
    
    // Summary
    fmt.Println("\n📈 Test Summary")
    fmt.Println("================")
    fmt.Printf("| Users | FPS  | Video Loss | Audio Loss | Bandwidth | Status |\n")
    fmt.Printf("|-------|------|------------|------------|-----------|--------|\n")
    
    for i, metrics := range results {
        status := "✅"
        if metrics.BandwidthKbps > 1200 {
            status = "⚠️"
        }
        if metrics.VideoLoss > 10 || metrics.AudioLoss > 5 {
            status = "❌"
        }
        
        fmt.Printf("| %d     | %.1f | %.1f%%      | %.1f%%      | %.0f kbps | %s     |\n",
            scenarios[i].users,
            metrics.VideoFPS,
            metrics.VideoLoss,
            metrics.AudioLoss,
            metrics.BandwidthKbps,
            status)
    }
    
    fmt.Println("\n✅ Test complete!")
}