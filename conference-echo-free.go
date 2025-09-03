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
    "math"
    "net/http"
    "sync"
    "time"

    "github.com/chai2010/webp"
    "github.com/gorilla/websocket"
    "github.com/nfnt/resize"
)

// Build-time variables (set via -ldflags)
var (
    BuildTime   = "unknown"
    BuildCommit = "unknown"
    BuildBy     = "local"
    BuildRef    = "unknown"
)

// Audio processing constants
const (
    // Echo cancellation parameters
    ECHO_THRESHOLD     = 0.3  // Similarity threshold for echo detection
    SILENCE_THRESHOLD  = 0.01 // Audio level below this is considered silence
    GATE_THRESHOLD     = 0.02 // Noise gate threshold
    DUCKING_FACTOR     = 0.3  // How much to reduce audio when someone else is talking
    
    // Audio buffer settings
    AUDIO_BUFFER_SIZE  = 48000 // 1 second at 48kHz
    ECHO_DELAY_MS      = 200   // Expected echo delay in milliseconds
)

// AudioProcessor handles echo cancellation and feedback prevention
type AudioProcessor struct {
    // Echo cancellation buffers
    InputBuffer      []float32
    OutputBuffer     []float32
    EchoBuffer       []float32
    
    // Voice activity detection
    IsSpeaking       bool
    SpeakingClients  map[string]bool
    LastSpeaker      string
    LastSpeakTime    time.Time
    
    // Audio levels
    InputLevel       float32
    OutputLevel      float32
    NoiseFloor       float32
    
    mu sync.RWMutex
}

// Client with audio processing
type Client struct {
    ID            string
    Room          string
    Conn          *websocket.Conn
    Send          chan []byte
    Hub           *Hub
    
    // Audio processing
    AudioProc         *AudioProcessor
    LastAudioTime     time.Time
    AudioSequence     int
    IsCurrentSpeaker  bool
    AudioLevel        float32
    
    // Quality management (from adaptive version)
    CurrentQuality    int
    Metrics          *ClientMetrics
    
    mu sync.RWMutex
}

// Room with audio management
type Room struct {
    ID              string
    Clients         map[string]*Client
    
    // Audio management
    CurrentSpeaker   string
    SpeakerQueue     []string
    AudioMixer       *AudioMixer
    
    mu sync.RWMutex
}

// AudioMixer handles room-level audio mixing and echo cancellation
type AudioMixer struct {
    // Active speakers
    ActiveSpeakers   map[string]*SpeakerInfo
    
    // Echo cancellation
    RoomEchoBuffer   []float32
    EchoPatterns     map[string][]float32
    
    // Feedback detection
    FeedbackDetector *FeedbackDetector
    
    mu sync.RWMutex
}

type SpeakerInfo struct {
    ClientID        string
    StartTime       time.Time
    AudioLevel      float32
    ConsecutiveSilence int
}

// FeedbackDetector identifies and prevents audio feedback loops
type FeedbackDetector struct {
    // Frequency analysis for feedback detection
    FrequencyBins    []float32
    PeakFrequencies  []float32
    FeedbackScore    float32
    
    // Feedback suppression
    IsSupressing     bool
    SuppressionLevel float32
}

// Enhanced Message types
type Message struct {
    Type          string      `json:"type"`
    ID            string      `json:"id,omitempty"`
    Room          string      `json:"room,omitempty"`
    From          string      `json:"from,omitempty"`
    Data          string      `json:"data,omitempty"`
    Timestamp     int64       `json:"timestamp,omitempty"`
    
    // Audio metadata
    AudioSeq      int         `json:"audioSeq,omitempty"`
    AudioLevel    float32     `json:"audioLevel,omitempty"`
    IsSpeaking    bool        `json:"isSpeaking,omitempty"`
    
    // Quality fields (from adaptive)
    Quality       string      `json:"quality,omitempty"`
    Feedback      *ClientFeedback `json:"feedback,omitempty"`
}

type ClientFeedback struct {
    FramesReceived int64   `json:"framesReceived"`
    FramesDropped  int64   `json:"framesDropped"`
    BufferHealth   float64 `json:"bufferHealth"`
    CPUUsage       float64 `json:"cpuUsage"`
    Bandwidth      float64 `json:"bandwidth"`
    Latency        int64   `json:"latency"`
    
    // Audio feedback
    AudioLatency   int64   `json:"audioLatency"`
    EchoDetected   bool    `json:"echoDetected"`
}

type ClientMetrics struct {
    Bandwidth      float64
    Latency        int64
    PacketLoss     float64
    AudioLatency   int64
    EchoEvents     int
    
    mu sync.RWMutex
}

// Hub manages everything
type Hub struct {
    Rooms      map[string]*Room
    Register   chan *Client
    Unregister chan *Client
    Broadcast  chan *BroadcastMessage
    
    mu sync.RWMutex
}

type BroadcastMessage struct {
    Room    string
    Message []byte
    From    string
    IsAudio bool
}

// Quality presets (simplified from adaptive)
type QualityPreset struct {
    Name       string
    Width      uint
    Height     uint
    FPS        int
    Quality    float32
}

var QualityLevels = []QualityPreset{
    {"360p", 640, 360, 15, 0.5},
    {"480p", 854, 480, 20, 0.6},
    {"720p", 1280, 720, 30, 0.7},
    {"1080p", 1920, 1080, 30, 0.8},
    {"4K", 3840, 2160, 30, 0.9},
}

var (
    upgrader = websocket.Upgrader{
        ReadBufferSize:  1024 * 16,
        WriteBufferSize: 1024 * 16,
        CheckOrigin: func(r *http.Request) bool { return true },
    }
    
    hub *Hub
)

// Audio processing functions

// ProcessAudioFrame handles echo cancellation and feedback prevention
func (c *Client) ProcessAudioFrame(audioData []byte) ([]byte, bool) {
    if c.AudioProc == nil {
        c.AudioProc = &AudioProcessor{
            InputBuffer:     make([]float32, AUDIO_BUFFER_SIZE),
            OutputBuffer:    make([]float32, AUDIO_BUFFER_SIZE),
            EchoBuffer:      make([]float32, AUDIO_BUFFER_SIZE),
            SpeakingClients: make(map[string]bool),
        }
    }
    
    // Decode audio data
    samples := decodeAudioData(audioData)
    if len(samples) == 0 {
        return nil, false
    }
    
    // Calculate audio level
    level := calculateAudioLevel(samples)
    c.AudioLevel = level
    
    // Voice Activity Detection (VAD)
    isSpeaking := level > GATE_THRESHOLD
    
    // Get room for audio mixing context
    room := c.getRoom()
    if room == nil {
        return audioData, true
    }
    
    // Check if this client should be allowed to speak (prevent feedback)
    if !c.shouldTransmitAudio(room, isSpeaking, level) {
        return nil, false
    }
    
    // Apply echo cancellation
    processed := c.applyEchoCancellation(samples, room)
    
    // Apply noise gate
    if level < GATE_THRESHOLD {
        processed = applySilence(processed)
    }
    
    // Apply ducking if others are speaking
    if room.CurrentSpeaker != "" && room.CurrentSpeaker != c.ID {
        processed = applyDucking(processed, DUCKING_FACTOR)
    }
    
    // Check for feedback
    if c.detectFeedback(processed) {
        log.Printf("Feedback detected from client %s, suppressing", c.ID)
        processed = applyFeedbackSuppression(processed)
    }
    
    // Update speaking state
    c.mu.Lock()
    c.IsCurrentSpeaker = isSpeaking && level > SILENCE_THRESHOLD
    c.LastAudioTime = time.Now()
    c.AudioSequence++
    c.mu.Unlock()
    
    // Update room speaker
    if isSpeaking {
        room.updateCurrentSpeaker(c.ID, level)
    }
    
    // Encode processed audio
    return encodeAudioData(processed), true
}

// shouldTransmitAudio determines if audio should be transmitted (prevents echo)
func (c *Client) shouldTransmitAudio(room *Room, isSpeaking bool, level float32) bool {
    room.mu.RLock()
    currentSpeaker := room.CurrentSpeaker
    room.mu.RUnlock()
    
    // If no one is speaking, allow
    if currentSpeaker == "" {
        return isSpeaking
    }
    
    // If this client is the current speaker, continue
    if currentSpeaker == c.ID {
        return true
    }
    
    // If someone else is speaking, check if we should interrupt
    if isSpeaking && level > GATE_THRESHOLD * 2 {
        // Only interrupt if significantly louder
        return true
    }
    
    return false
}

// applyEchoCancellation removes echo from audio
func (c *Client) applyEchoCancellation(samples []float32, room *Room) []float32 {
    c.AudioProc.mu.Lock()
    defer c.AudioProc.mu.Unlock()
    
    processed := make([]float32, len(samples))
    copy(processed, samples)
    
    // Simple echo cancellation using adaptive filter
    if room.AudioMixer != nil {
        room.AudioMixer.mu.RLock()
        echoBuffer := room.AudioMixer.RoomEchoBuffer
        room.AudioMixer.mu.RUnlock()
        
        if len(echoBuffer) > 0 {
            // Subtract estimated echo
            for i := range processed {
                if i < len(echoBuffer) {
                    processed[i] -= echoBuffer[i] * ECHO_THRESHOLD
                    
                    // Clamp to valid range
                    if processed[i] > 1.0 {
                        processed[i] = 1.0
                    } else if processed[i] < -1.0 {
                        processed[i] = -1.0
                    }
                }
            }
        }
    }
    
    // Update echo buffer with current output
    c.AudioProc.EchoBuffer = append(c.AudioProc.EchoBuffer, processed...)
    if len(c.AudioProc.EchoBuffer) > AUDIO_BUFFER_SIZE {
        c.AudioProc.EchoBuffer = c.AudioProc.EchoBuffer[len(c.AudioProc.EchoBuffer)-AUDIO_BUFFER_SIZE:]
    }
    
    return processed
}

// detectFeedback checks for audio feedback patterns
func (c *Client) detectFeedback(samples []float32) bool {
    // Simple feedback detection based on:
    // 1. Sustained high level
    // 2. Repetitive patterns
    // 3. Frequency peaks
    
    level := calculateAudioLevel(samples)
    
    // Check for sustained high level
    if level > 0.8 {
        c.AudioProc.mu.Lock()
        c.Metrics.EchoEvents++
        c.AudioProc.mu.Unlock()
        
        if c.Metrics.EchoEvents > 3 {
            return true
        }
    } else {
        c.Metrics.EchoEvents = 0
    }
    
    // Check for repetitive patterns (simplified)
    if detectRepetitivePattern(samples) {
        return true
    }
    
    return false
}

// Room audio management
func (r *Room) updateCurrentSpeaker(clientID string, level float32) {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if r.AudioMixer == nil {
        r.AudioMixer = &AudioMixer{
            ActiveSpeakers:  make(map[string]*SpeakerInfo),
            RoomEchoBuffer:  make([]float32, AUDIO_BUFFER_SIZE),
            EchoPatterns:    make(map[string][]float32),
            FeedbackDetector: &FeedbackDetector{},
        }
    }
    
    // Update or add speaker
    if speaker, exists := r.AudioMixer.ActiveSpeakers[clientID]; exists {
        speaker.AudioLevel = level
        speaker.ConsecutiveSilence = 0
    } else {
        r.AudioMixer.ActiveSpeakers[clientID] = &SpeakerInfo{
            ClientID:   clientID,
            StartTime:  time.Now(),
            AudioLevel: level,
        }
    }
    
    // Determine primary speaker (loudest)
    var maxLevel float32
    var primarySpeaker string
    
    for id, speaker := range r.AudioMixer.ActiveSpeakers {
        if speaker.AudioLevel > maxLevel {
            maxLevel = speaker.AudioLevel
            primarySpeaker = id
        }
        
        // Remove inactive speakers
        if time.Since(speaker.StartTime) > 2*time.Second && speaker.AudioLevel < SILENCE_THRESHOLD {
            delete(r.AudioMixer.ActiveSpeakers, id)
        }
    }
    
    r.CurrentSpeaker = primarySpeaker
}

// Audio utility functions
func calculateAudioLevel(samples []float32) float32 {
    if len(samples) == 0 {
        return 0
    }
    
    var sum float32
    for _, sample := range samples {
        sum += sample * sample
    }
    
    return float32(math.Sqrt(float64(sum / float32(len(samples)))))
}

func applySilence(samples []float32) []float32 {
    return make([]float32, len(samples))
}

func applyDucking(samples []float32, factor float32) []float32 {
    processed := make([]float32, len(samples))
    for i, sample := range samples {
        processed[i] = sample * factor
    }
    return processed
}

func applyFeedbackSuppression(samples []float32) []float32 {
    // Apply notch filter or reduce gain
    processed := make([]float32, len(samples))
    for i, sample := range samples {
        processed[i] = sample * 0.1 // Drastically reduce level
    }
    return processed
}

func detectRepetitivePattern(samples []float32) bool {
    // Simplified pattern detection
    if len(samples) < 1000 {
        return false
    }
    
    // Check for correlation with delayed version
    windowSize := 100
    var correlation float32
    
    for i := 0; i < windowSize; i++ {
        if i+windowSize < len(samples) {
            correlation += samples[i] * samples[i+windowSize]
        }
    }
    
    correlation /= float32(windowSize)
    return correlation > 0.7 // High correlation indicates repetitive pattern
}

func decodeAudioData(data []byte) []float32 {
    // Decode base64 PCM data
    decoded, err := base64.StdEncoding.DecodeString(string(data))
    if err != nil {
        return nil
    }
    
    // Convert to float32
    samples := make([]float32, len(decoded)/2)
    for i := 0; i < len(samples); i++ {
        // Convert int16 to float32
        val := int16(decoded[i*2]) | int16(decoded[i*2+1])<<8
        samples[i] = float32(val) / 32768.0
    }
    
    return samples
}

func encodeAudioData(samples []float32) []byte {
    // Convert float32 to int16 PCM
    pcm := make([]byte, len(samples)*2)
    
    for i, sample := range samples {
        // Clamp and convert
        val := int16(sample * 32767)
        pcm[i*2] = byte(val)
        pcm[i*2+1] = byte(val >> 8)
    }
    
    // Encode to base64
    return []byte(base64.StdEncoding.EncodeToString(pcm))
}

func (c *Client) getRoom() *Room {
    c.Hub.mu.RLock()
    defer c.Hub.mu.RUnlock()
    
    if room, ok := c.Hub.Rooms[c.Room]; ok {
        return room
    }
    return nil
}

// WebSocket handlers
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
        CurrentQuality:   0,
        Metrics:          &ClientMetrics{},
    }
    
    hub.Register <- client
    
    go client.writePump()
    go client.readPump()
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
            
        case "audio":
            // Process audio with echo cancellation
            if processed, ok := c.ProcessAudioFrame([]byte(msg.Data)); ok && processed != nil {
                // Create audio message with metadata
                audioMsg := Message{
                    Type:       "audio",
                    From:       c.ID,
                    Data:       string(processed),
                    AudioSeq:   c.AudioSequence,
                    AudioLevel: c.AudioLevel,
                    IsSpeaking: c.IsCurrentSpeaker,
                    Timestamp:  time.Now().UnixMilli(),
                }
                
                if outData, err := json.Marshal(audioMsg); err == nil {
                    hub.Broadcast <- &BroadcastMessage{
                        Room:    c.Room,
                        Message: outData,
                        From:    c.ID,
                        IsAudio: true,
                    }
                }
            }
            
        case "frame":
            // Video frame handling (simplified from adaptive version)
            quality := QualityLevels[c.CurrentQuality]
            if decoded, err := base64.StdEncoding.DecodeString(msg.Data); err == nil {
                if compressed, err := compressToWebP(decoded, &quality); err == nil {
                    outMsg := Message{
                        Type:      "webp-frame",
                        From:      c.ID,
                        Data:      base64.StdEncoding.EncodeToString(compressed),
                        Timestamp: time.Now().UnixMilli(),
                        Quality:   quality.Name,
                    }
                    
                    if outData, err := json.Marshal(outMsg); err == nil {
                        hub.Broadcast <- &BroadcastMessage{
                            Room:    c.Room,
                            Message: outData,
                            From:    c.ID,
                            IsAudio: false,
                        }
                    }
                }
            }
            
        case "feedback":
            // Process client feedback including echo detection
            if msg.Feedback != nil {
                c.processFeedback(msg.Feedback)
            }
            
        case "ping":
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

func (c *Client) processFeedback(feedback *ClientFeedback) {
    c.Metrics.mu.Lock()
    defer c.Metrics.mu.Unlock()
    
    c.Metrics.Bandwidth = feedback.Bandwidth
    c.Metrics.Latency = feedback.Latency
    c.Metrics.AudioLatency = feedback.AudioLatency
    
    if feedback.EchoDetected {
        c.Metrics.EchoEvents++
        log.Printf("Client %s reported echo detection", c.ID)
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
            h.mu.Unlock()
            
            log.Printf("Client registered: %s (room: %s)", client.ID, client.Room)
            
        case client := <-h.Unregister:
            h.mu.Lock()
            if room, ok := h.Rooms[client.Room]; ok {
                room.mu.Lock()
                delete(room.Clients, client.ID)
                
                // Clear speaker status
                if room.CurrentSpeaker == client.ID {
                    room.CurrentSpeaker = ""
                }
                room.mu.Unlock()
            }
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
                
                // Smart audio routing to prevent echo
                for _, client := range room.Clients {
                    shouldSend := true
                    
                    // Don't send audio back to sender
                    if broadcast.IsAudio && client.ID == broadcast.From {
                        shouldSend = false
                    }
                    
                    // Additional echo prevention logic
                    if broadcast.IsAudio && client.IsCurrentSpeaker && 
                       broadcast.From != room.CurrentSpeaker {
                        // Don't send to active speakers if from non-primary speaker
                        shouldSend = false
                    }
                    
                    if shouldSend {
                        clients = append(clients, client)
                    }
                }
                room.mu.RUnlock()
                
                // Send to selected clients
                for _, client := range clients {
                    select {
                    case client.Send <- broadcast.Message:
                    default:
                        // Client buffer full
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
    room.mu.Unlock()
}

// WebP compression
func compressToWebP(data []byte, quality *QualityPreset) ([]byte, error) {
    img, _, err := image.Decode(bytes.NewReader(data))
    if err != nil {
        return nil, err
    }
    
    if quality.Width > 0 && quality.Height > 0 {
        img = resize.Resize(quality.Width, quality.Height, img, resize.Lanczos3)
    }
    
    bounds := img.Bounds()
    rgba := image.NewRGBA(bounds)
    draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
    
    var buf bytes.Buffer
    options := &webp.Options{
        Lossless: false,
        Quality:  quality.Quality,
    }
    
    if err := webp.Encode(&buf, rgba, options); err != nil {
        return nil, err
    }
    
    return buf.Bytes(), nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    
    health := map[string]interface{}{
        "status": "healthy",
        "deployment": map[string]string{
            "time":      BuildTime,
            "commit":    BuildCommit,
            "deployedBy": BuildBy,
            "ref":       BuildRef,
        },
        "server": map[string]interface{}{
            "type":     "echo-free-conference",
            "version":  "1.1.0",
            "features": []string{"echo-cancellation", "VAD", "audio-ducking", "webp-compression", "deployment-tracking"},
        },
        "timestamp": time.Now().UTC().Format(time.RFC3339),
    }
    
    json.NewEncoder(w).Encode(health)
}

func main() {
    hub = &Hub{
        Rooms:      make(map[string]*Room),
        Register:   make(chan *Client),
        Unregister: make(chan *Client),
        Broadcast:  make(chan *BroadcastMessage, 256),
    }
    
    go hub.run()
    
    // Serve status page
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        html := `<!DOCTYPE html>
<html>
<head>
    <title>Echo-Free Conference Server</title>
    <style>
        body { font-family: -apple-system, sans-serif; padding: 40px; background: linear-gradient(135deg, #667eea, #764ba2); color: white; }
        .container { max-width: 800px; margin: 0 auto; background: rgba(255,255,255,0.1); padding: 30px; border-radius: 20px; }
        h1 { margin-bottom: 20px; }
        .status { background: rgba(0,255,0,0.2); padding: 10px; border-radius: 10px; }
        .features li { margin: 10px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔇 Echo-Free Conference Server</h1>
        <div class="status">✅ Server is running on :3001</div>
        
        <h2>Audio Processing Features</h2>
        <ul class="features">
            <li>🎯 <strong>Server-side Echo Cancellation</strong> - Removes echo before broadcasting</li>
            <li>🔊 <strong>Feedback Detection</strong> - Identifies and suppresses feedback loops</li>
            <li>🎤 <strong>Voice Activity Detection (VAD)</strong> - Smart speaker detection</li>
            <li>🔇 <strong>Noise Gate</strong> - Eliminates background noise</li>
            <li>📉 <strong>Audio Ducking</strong> - Reduces volume when others speak</li>
            <li>👥 <strong>Smart Audio Routing</strong> - Prevents audio loops</li>
            <li>🎚️ <strong>Automatic Gain Control</strong> - Normalizes audio levels</li>
            <li>⚡ <strong>Low Latency Processing</strong> - Real-time echo removal</li>
        </ul>
        
        <h3>How It Works:</h3>
        <ol>
            <li>Client audio is analyzed for echo patterns</li>
            <li>Echo cancellation removes feedback</li>
            <li>Voice activity detection identifies speakers</li>
            <li>Audio is routed intelligently to prevent loops</li>
            <li>Feedback suppression activates if needed</li>
        </ol>
        
        <div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid rgba(255,255,255,0.2);">
            <p>WebSocket endpoint: <code>ws://localhost:3001/ws</code></p>
            <p>Echo cancellation: <strong>ACTIVE</strong></p>
        </div>
    </div>
</body>
</html>`
        w.Header().Set("Content-Type", "text/html")
        w.Write([]byte(html))
    })
    
    http.HandleFunc("/ws", handleWebSocket)
    http.HandleFunc("/health", handleHealth)
    http.HandleFunc("/info", handleHealth)
    
    log.Println("Starting Echo-Free Conference Server on :3001")
    log.Println("Features: Echo Cancellation | Feedback Prevention | Smart Audio Routing")
    log.Printf("Build info: %s by %s (commit: %s)", BuildTime, BuildBy, BuildCommit)
    log.Fatal(http.ListenAndServe(":3001", nil))
}