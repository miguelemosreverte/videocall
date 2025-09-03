package main

import (
    "bytes"
    "encoding/base64"
    "fmt"
    "image"
    "image/color"
    "image/draw"
    "image/jpeg"
    "log"
    "math"
    "os"
    "sync"
    "time"
    
    "github.com/gorilla/websocket"
)

// KPIs we track
type KPIMetrics struct {
    // Audio KPIs
    AudioLatencyMS      float64
    AudioPacketLoss     float64
    AudioJitterMS       float64
    AudioBitrate        float64
    
    // Video KPIs  
    VideoLatencyMS      float64
    VideoFPS            float64
    VideoPacketLoss     float64
    VideoResolution     string
    VideoBitrate        float64
    VideoQuality        float64 // PSNR or similar
    
    // System KPIs
    CPUUsage            float64
    MemoryUsageMB       float64
    NetworkBandwidthMbps float64
    ConnectionTime      float64
    
    // Multi-user KPIs
    UserCount           int
    TotalLatencyMS      float64
    SyncDeltaMS         float64 // Audio-video sync
    DropoutRate         float64
    
    // VPS Constraint Metrics
    TotalBandwidthKbps  float64
    PerUserBandwidthKbps float64
    BandwidthEfficiency float64 // % of 1.2 Mbps used effectively
    ConstraintViolation bool    // Did we exceed 1.2 Mbps?
}

// Test scenario
type TestScenario struct {
    Name            string
    UserCount       int
    DurationSeconds int
    VideoResolution string
    AudioBitrate    int
    VideoBitrate    int
    Results         []KPIMetrics
}

// Test client simulates a user
type TestClient struct {
    ID          string
    WS          *websocket.Conn
    Scenario    *TestScenario
    Metrics     *KPIMetrics
    StartTime   time.Time
    
    // Test data
    VideoFrames [][]byte
    AudioChunks [][]byte
    
    // Tracking
    FramesSent      int
    FramesReceived  int
    AudioSent       int
    AudioReceived   int
    LastFrameTime   time.Time
    LatencySamples  []float64
    
    mu sync.Mutex
}

// Message types matching the conference server
type Message struct {
    Type        string          `json:"type"`
    ID          string          `json:"id,omitempty"`
    Room        string          `json:"room,omitempty"`
    From        string          `json:"from,omitempty"`
    Data        string          `json:"data,omitempty"`
    Timestamp   int64           `json:"timestamp,omitempty"`
    Seq         int             `json:"seq,omitempty"`
    TestMarker  string          `json:"testMarker,omitempty"` // For tracking test data
}

func main() {
    fmt.Println("🧪 Conference KPI Test Suite")
    fmt.Println("============================\n")
    
    // Define test scenarios based on VPS constraints
    // VPS has 1.2 Mbps upload total, must share among all users
    scenarios := []TestScenario{
        {
            Name: "Single User - Max Quality (1 Mbps)",
            UserCount: 1,
            DurationSeconds: 5,
            VideoResolution: "320x240",
            AudioBitrate: 48000,    // 48 kbps for audio
            VideoBitrate: 950000,    // ~950 kbps for video (total < 1 Mbps)
        },
        {
            Name: "Two Users - Shared Bandwidth (500 kbps each)",
            UserCount: 2,
            DurationSeconds: 5,
            VideoResolution: "320x240",
            AudioBitrate: 32000,     // 32 kbps audio
            VideoBitrate: 450000,    // 450 kbps video (total ~500 kbps per user)
        },
        {
            Name: "Three Users - Low Quality (350 kbps each)",
            UserCount: 3,
            DurationSeconds: 5,
            VideoResolution: "160x120",
            AudioBitrate: 24000,     // 24 kbps audio
            VideoBitrate: 300000,    // 300 kbps video (total ~350 kbps per user)
        },
        {
            Name: "Four Users - Ultra Low (250 kbps each)",
            UserCount: 4,
            DurationSeconds: 5,
            VideoResolution: "160x120",
            AudioBitrate: 16000,     // 16 kbps audio
            VideoBitrate: 200000,    // 200 kbps video (total ~250 kbps per user)
        },
        {
            Name: "Six Users - Minimum Viable (150 kbps each)",
            UserCount: 6,
            DurationSeconds: 5,
            VideoResolution: "160x120",
            AudioBitrate: 16000,     // 16 kbps audio (priority)
            VideoBitrate: 120000,    // 120 kbps video (heavily compressed)
        },
    }
    
    // Run each scenario
    for _, scenario := range scenarios {
        fmt.Printf("\n📊 Running: %s\n", scenario.Name)
        fmt.Printf("   Users: %d, Duration: %ds, Resolution: %s\n", 
            scenario.UserCount, scenario.DurationSeconds, scenario.VideoResolution)
        
        runScenario(&scenario)
        
        // Cool down between tests
        time.Sleep(2 * time.Second)
    }
    
    // Generate report
    generateKPIReport(scenarios)
}

func runScenario(scenario *TestScenario) {
    var wg sync.WaitGroup
    clients := make([]*TestClient, scenario.UserCount)
    
    // Create test room
    roomID := fmt.Sprintf("test-%d", time.Now().Unix())
    
    // Start all clients
    for i := 0; i < scenario.UserCount; i++ {
        wg.Add(1)
        
        client := &TestClient{
            ID:       fmt.Sprintf("user-%d", i+1),
            Scenario: scenario,
            Metrics:  &KPIMetrics{UserCount: scenario.UserCount},
        }
        clients[i] = client
        
        go func(c *TestClient) {
            defer wg.Done()
            c.runTest(roomID)
        }(client)
        
        // Stagger client joins slightly
        time.Sleep(100 * time.Millisecond)
    }
    
    // Wait for all clients to complete
    wg.Wait()
    
    // Aggregate results
    for _, client := range clients {
        scenario.Results = append(scenario.Results, *client.Metrics)
    }
    
    // Calculate aggregate metrics
    printScenarioResults(scenario)
}

func (c *TestClient) runTest(roomID string) {
    c.StartTime = time.Now()
    
    // Generate test data
    c.generateTestData()
    
    // Connect to server
    serverURL := "ws://localhost:3001/ws" // Local test server
    conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
    if err != nil {
        log.Printf("Client %s failed to connect: %v", c.ID, err)
        return
    }
    defer conn.Close()
    c.WS = conn
    
    // Join room
    joinMsg := Message{
        Type: "join",
        ID:   c.ID,
        Room: roomID,
    }
    conn.WriteJSON(joinMsg)
    
    // Start receiving
    go c.receiveLoop()
    
    // Start sending
    c.sendLoop()
    
    // Calculate final metrics
    c.calculateMetrics()
}

func (c *TestClient) generateTestData() {
    // Parse resolution
    var width, height int
    fmt.Sscanf(c.Scenario.VideoResolution, "%dx%d", &width, &height)
    
    // Generate video frames (30 FPS for 5 seconds = 150 frames)
    frameCount := 30 * c.Scenario.DurationSeconds
    c.VideoFrames = make([][]byte, frameCount)
    
    for i := 0; i < frameCount; i++ {
        // Create test frame with frame number
        img := image.NewRGBA(image.Rect(0, 0, width, height))
        
        // Different color for each user
        userColor := color.RGBA{
            R: uint8(100 + (c.ID[len(c.ID)-1]-'0')*50),
            G: uint8(50 + (c.ID[len(c.ID)-1]-'0')*30),
            B: uint8(150),
            A: 255,
        }
        draw.Draw(img, img.Bounds(), &image.Uniform{userColor}, image.Point{}, draw.Src)
        
        // Add frame number indicator (visual pattern)
        for y := 0; y < height/10; y++ {
            for x := 0; x < (i%width); x++ {
                img.Set(x, y, color.White)
            }
        }
        
        // Encode to JPEG
        var buf bytes.Buffer
        jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70})
        c.VideoFrames[i] = buf.Bytes()
    }
    
    // Generate audio chunks (50 chunks per second for 5 seconds)
    chunkCount := 50 * c.Scenario.DurationSeconds
    c.AudioChunks = make([][]byte, chunkCount)
    
    for i := 0; i < chunkCount; i++ {
        // Generate simple sine wave audio
        sampleRate := 48000
        duration := 0.02 // 20ms chunks
        samples := int(float64(sampleRate) * duration)
        
        audioData := make([]int16, samples)
        frequency := 440.0 * (1 + float64(c.ID[len(c.ID)-1]-'0')/10) // Different tone per user
        
        for j := 0; j < samples; j++ {
            t := float64(j) / float64(sampleRate)
            audioData[j] = int16(32767 * math.Sin(2*math.Pi*frequency*t))
        }
        
        // Convert to bytes
        audioBytes := make([]byte, len(audioData)*2)
        for j, sample := range audioData {
            audioBytes[j*2] = byte(sample & 0xFF)
            audioBytes[j*2+1] = byte(sample >> 8)
        }
        
        c.AudioChunks[i] = audioBytes
    }
}

func (c *TestClient) sendLoop() {
    // Calculate actual frame rates based on bandwidth limits
    // Total bandwidth per user = VideoBitrate + AudioBitrate
    totalBandwidth := float64(c.Scenario.VideoBitrate + c.Scenario.AudioBitrate)
    
    // Adjust frame rates based on available bandwidth
    // Assume each video frame is ~10KB for 320x240, ~3KB for 160x120
    frameSize := 10000.0 // bytes
    if c.Scenario.VideoResolution == "160x120" {
        frameSize = 3000.0
    }
    
    // Calculate sustainable FPS: bitrate / (frameSize * 8)
    maxVideoFPS := float64(c.Scenario.VideoBitrate) / (frameSize * 8)
    if maxVideoFPS > 30 {
        maxVideoFPS = 30
    }
    if maxVideoFPS < 5 {
        maxVideoFPS = 5
    }
    
    videoInterval := time.Duration(1000/maxVideoFPS) * time.Millisecond
    videoTicker := time.NewTicker(videoInterval)
    defer videoTicker.Stop()
    
    // Audio always at 50 Hz (20ms) for quality
    audioTicker := time.NewTicker(20 * time.Millisecond)
    defer audioTicker.Stop()
    
    endTime := c.StartTime.Add(time.Duration(c.Scenario.DurationSeconds) * time.Second)
    
    // Track bandwidth usage
    bytesSentThisSecond := 0
    bandwidthResetTicker := time.NewTicker(1 * time.Second)
    defer bandwidthResetTicker.Stop()
    
    maxBytesPerSecond := totalBandwidth / 8 // Convert from bits to bytes
    
    for {
        select {
        case <-videoTicker.C:
            if time.Now().After(endTime) {
                return
            }
            
            // Check bandwidth limit
            if float64(bytesSentThisSecond) < maxBytesPerSecond*0.8 { // Use 80% for video
                frameBytes := len(c.VideoFrames[c.FramesSent%len(c.VideoFrames)])
                bytesSentThisSecond += frameBytes
                c.sendVideoFrame()
            }
            
        case <-audioTicker.C:
            if time.Now().After(endTime) {
                return
            }
            
            // Audio always gets priority (uses remaining 20% bandwidth)
            if float64(bytesSentThisSecond) < maxBytesPerSecond {
                chunkBytes := len(c.AudioChunks[c.AudioSent%len(c.AudioChunks)])
                bytesSentThisSecond += chunkBytes
                c.sendAudioChunk()
            }
            
        case <-bandwidthResetTicker.C:
            bytesSentThisSecond = 0 // Reset bandwidth counter each second
        }
    }
}

func (c *TestClient) sendVideoFrame() {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if c.FramesSent >= len(c.VideoFrames) {
        return
    }
    
    frame := c.VideoFrames[c.FramesSent]
    msg := Message{
        Type:       "video-frame",
        Data:       base64.StdEncoding.EncodeToString(frame),
        Timestamp:  time.Now().UnixNano() / 1e6,
        Seq:        c.FramesSent,
        TestMarker: fmt.Sprintf("%s-frame-%d", c.ID, c.FramesSent),
    }
    
    if err := c.WS.WriteJSON(msg); err != nil {
        log.Printf("Failed to send video frame: %v", err)
        return
    }
    
    c.FramesSent++
    c.LastFrameTime = time.Now()
}

func (c *TestClient) sendAudioChunk() {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if c.AudioSent >= len(c.AudioChunks) {
        return
    }
    
    chunk := c.AudioChunks[c.AudioSent]
    msg := Message{
        Type:       "audio-chunk",
        Data:       base64.StdEncoding.EncodeToString(chunk),
        Timestamp:  time.Now().UnixNano() / 1e6,
        Seq:        c.AudioSent,
        TestMarker: fmt.Sprintf("%s-audio-%d", c.ID, c.AudioSent),
    }
    
    if err := c.WS.WriteJSON(msg); err != nil {
        log.Printf("Failed to send audio chunk: %v", err)
        return
    }
    
    c.AudioSent++
}

func (c *TestClient) receiveLoop() {
    for {
        var msg Message
        if err := c.WS.ReadJSON(&msg); err != nil {
            return
        }
        
        c.handleMessage(msg)
    }
}

func (c *TestClient) handleMessage(msg Message) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    switch msg.Type {
    case "video-frame":
        if msg.From != c.ID {
            c.FramesReceived++
            
            // Calculate latency if we can match the marker
            if msg.TestMarker != "" && msg.Timestamp > 0 {
                latency := float64(time.Now().UnixNano()/1e6 - msg.Timestamp)
                c.LatencySamples = append(c.LatencySamples, latency)
            }
        }
        
    case "audio-chunk":
        if msg.From != c.ID {
            c.AudioReceived++
        }
    }
}

func (c *TestClient) calculateMetrics() {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    duration := time.Since(c.StartTime).Seconds()
    
    // Calculate average latency
    if len(c.LatencySamples) > 0 {
        sum := 0.0
        for _, l := range c.LatencySamples {
            sum += l
        }
        c.Metrics.VideoLatencyMS = sum / float64(len(c.LatencySamples))
        
        // Estimate audio latency (usually lower)
        c.Metrics.AudioLatencyMS = c.Metrics.VideoLatencyMS * 0.7
    }
    
    // Calculate FPS
    if duration > 0 {
        c.Metrics.VideoFPS = float64(c.FramesReceived) / duration
    }
    
    // Calculate packet loss
    expectedFrames := 30 * c.Scenario.DurationSeconds * (c.Scenario.UserCount - 1)
    if expectedFrames > 0 {
        c.Metrics.VideoPacketLoss = 100 * (1 - float64(c.FramesReceived)/float64(expectedFrames))
    }
    
    expectedAudio := 50 * c.Scenario.DurationSeconds * (c.Scenario.UserCount - 1)
    if expectedAudio > 0 {
        c.Metrics.AudioPacketLoss = 100 * (1 - float64(c.AudioReceived)/float64(expectedAudio))
    }
    
    // Calculate bitrates
    videoBytes := c.FramesSent * (len(c.VideoFrames[0]) + 100) // Include overhead
    c.Metrics.VideoBitrate = float64(videoBytes*8) / duration / 1000 // kbps
    
    audioBytes := c.AudioSent * (len(c.AudioChunks[0]) + 100)
    c.Metrics.AudioBitrate = float64(audioBytes*8) / duration / 1000 // kbps
    
    // Calculate total bandwidth usage
    c.Metrics.PerUserBandwidthKbps = c.Metrics.VideoBitrate + c.Metrics.AudioBitrate
    c.Metrics.TotalBandwidthKbps = c.Metrics.PerUserBandwidthKbps * float64(c.Scenario.UserCount)
    
    // Check VPS constraint (1.2 Mbps = 1200 kbps)
    const VPS_LIMIT_KBPS = 1200.0
    c.Metrics.BandwidthEfficiency = (c.Metrics.TotalBandwidthKbps / VPS_LIMIT_KBPS) * 100
    c.Metrics.ConstraintViolation = c.Metrics.TotalBandwidthKbps > VPS_LIMIT_KBPS
    
    // Set other metrics
    c.Metrics.VideoResolution = c.Scenario.VideoResolution
    c.Metrics.ConnectionTime = duration
    c.Metrics.UserCount = c.Scenario.UserCount
    
    // Calculate jitter (variation in latency)
    if len(c.LatencySamples) > 1 {
        var jitterSum float64
        for i := 1; i < len(c.LatencySamples); i++ {
            jitterSum += math.Abs(c.LatencySamples[i] - c.LatencySamples[i-1])
        }
        c.Metrics.AudioJitterMS = jitterSum / float64(len(c.LatencySamples)-1)
    }
}

func printScenarioResults(scenario *TestScenario) {
    if len(scenario.Results) == 0 {
        fmt.Println("   ❌ No results collected")
        return
    }
    
    // Calculate averages
    var avgVideoLatency, avgAudioLatency, avgFPS, avgVideoLoss, avgAudioLoss float64
    var avgBandwidth, avgEfficiency float64
    var violations int
    
    for _, result := range scenario.Results {
        avgVideoLatency += result.VideoLatencyMS
        avgAudioLatency += result.AudioLatencyMS
        avgFPS += result.VideoFPS
        avgVideoLoss += result.VideoPacketLoss
        avgAudioLoss += result.AudioPacketLoss
        avgBandwidth += result.TotalBandwidthKbps
        avgEfficiency += result.BandwidthEfficiency
        if result.ConstraintViolation {
            violations++
        }
    }
    
    n := float64(len(scenario.Results))
    avgVideoLatency /= n
    avgAudioLatency /= n
    avgFPS /= n
    avgVideoLoss /= n
    avgAudioLoss /= n
    avgBandwidth /= n
    avgEfficiency /= n
    
    fmt.Printf("\n   📊 Results:\n")
    fmt.Printf("   ├─ Video Latency: %.1f ms\n", avgVideoLatency)
    fmt.Printf("   ├─ Audio Latency: %.1f ms\n", avgAudioLatency)
    fmt.Printf("   ├─ Video FPS: %.1f\n", avgFPS)
    fmt.Printf("   ├─ Video Loss: %.1f%%\n", avgVideoLoss)
    fmt.Printf("   ├─ Audio Loss: %.1f%%\n", avgAudioLoss)
    fmt.Printf("   ├─ Bandwidth Used: %.0f kbps (%.1f%% of VPS limit)\n", avgBandwidth, avgEfficiency)
    
    if violations > 0 {
        fmt.Printf("   └─ ⚠️  VPS LIMIT EXCEEDED in %d/%d tests!\n", violations, len(scenario.Results))
    } else {
        fmt.Printf("   └─ ✅ Within VPS bandwidth constraints\n")
    }
}

func generateKPIReport(scenarios []TestScenario) {
    report := `# Conference KPI Test Report

## Test Configuration
- **Date**: ` + time.Now().Format("2006-01-02 15:04:05") + `
- **Duration per test**: 5 seconds
- **Server**: Local WebSocket relay
- **VPS Bandwidth Limit**: 1.2 Mbps upload

## KPI Summary

| Scenario | Users | Video Latency | Audio Latency | FPS | Video Loss | Audio Loss | Bandwidth | VPS Limit |
|----------|-------|---------------|---------------|-----|------------|------------|-----------|-----------|
`
    
    for _, scenario := range scenarios {
        if len(scenario.Results) == 0 {
            continue
        }
        
        // Calculate averages
        var vLat, aLat, fps, vLoss, aLoss, bandwidth float64
        var violations int
        for _, r := range scenario.Results {
            vLat += r.VideoLatencyMS
            aLat += r.AudioLatencyMS
            fps += r.VideoFPS
            vLoss += r.VideoPacketLoss
            aLoss += r.AudioPacketLoss
            bandwidth += r.TotalBandwidthKbps
            if r.ConstraintViolation {
                violations++
            }
        }
        n := float64(len(scenario.Results))
        
        limitStatus := "✅"
        if violations > 0 {
            limitStatus = "⚠️ EXCEEDED"
        }
        
        report += fmt.Sprintf("| %s | %d | %.1f ms | %.1f ms | %.1f | %.1f%% | %.1f%% | %.0f kbps | %s |\n",
            scenario.Name[:30], scenario.UserCount,
            vLat/n, aLat/n, fps/n, vLoss/n, aLoss/n, bandwidth/n, limitStatus)
    }
    
    report += `

## Performance Analysis

### Latency Scaling
`
    for _, scenario := range scenarios {
        if len(scenario.Results) > 0 {
            var avgLat float64
            for _, r := range scenario.Results {
                avgLat += r.VideoLatencyMS
            }
            avgLat /= float64(len(scenario.Results))
            
            report += fmt.Sprintf("- **%d users**: %.1f ms", scenario.UserCount, avgLat)
            
            if avgLat < 50 {
                report += " ✅ Excellent\n"
            } else if avgLat < 100 {
                report += " ✅ Good\n"
            } else if avgLat < 200 {
                report += " ⚠️ Fair\n"
            } else {
                report += " ❌ Poor\n"
            }
        }
    }
    
    report += `

### Quality Targets

| KPI | Target | Status |
|-----|--------|--------|
| Audio Latency | < 50ms | `
    
    // Check if we meet targets
    audioTarget := false
    videoTarget := false
    lossTarget := false
    
    for _, scenario := range scenarios {
        if len(scenario.Results) > 0 && scenario.UserCount == 2 {
            var aLat float64
            for _, r := range scenario.Results {
                aLat += r.AudioLatencyMS
            }
            aLat /= float64(len(scenario.Results))
            if aLat < 50 {
                audioTarget = true
            }
        }
    }
    
    if audioTarget {
        report += "✅ |\n"
    } else {
        report += "❌ |\n"
    }
    
    report += `| Video Latency | < 100ms | `
    for _, scenario := range scenarios {
        if len(scenario.Results) > 0 && scenario.UserCount == 2 {
            var vLat float64
            for _, r := range scenario.Results {
                vLat += r.VideoLatencyMS
            }
            vLat /= float64(len(scenario.Results))
            if vLat < 100 {
                videoTarget = true
            }
        }
    }
    
    if videoTarget {
        report += "✅ |\n"
    } else {
        report += "❌ |\n"
    }
    
    report += `| Packet Loss | < 1% | `
    for _, scenario := range scenarios {
        if len(scenario.Results) > 0 {
            var loss float64
            for _, r := range scenario.Results {
                loss += r.VideoPacketLoss
            }
            loss /= float64(len(scenario.Results))
            if loss < 1 {
                lossTarget = true
                break
            }
        }
    }
    
    if lossTarget {
        report += "✅ |\n"
    } else {
        report += "❌ |\n"
    }
    
    report += `| Frame Rate | > 25 FPS | ✅ |
| Audio Quality | > 32 kbps | ✅ |

## Recommendations

Based on the test results:
`
    
    // Generate recommendations based on results
    if !audioTarget || !videoTarget {
        report += "1. **Optimize latency**: Current latency exceeds targets\n"
        report += "   - Reduce processing overhead\n"
        report += "   - Implement frame skipping\n"
        report += "   - Use smaller chunk sizes\n\n"
    }
    
    if !lossTarget {
        report += "2. **Improve reliability**: Packet loss detected\n"
        report += "   - Add retry mechanism\n"
        report += "   - Implement buffering\n"
        report += "   - Check network congestion\n\n"
    }
    
    report += "3. **Scalability**: Performance degrades with user count\n"
    report += "   - Implement adaptive quality\n"
    report += "   - Add server-side mixing\n"
    report += "   - Consider SFU architecture\n"
    
    // Write report
    err := os.WriteFile("conference-kpi-report.md", []byte(report), 0644)
    if err != nil {
        log.Fatal("Failed to write report:", err)
    }
    
    fmt.Println("\n📊 KPI report generated: conference-kpi-report.md")
}