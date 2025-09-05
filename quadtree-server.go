package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// DeltaRegion represents a changed region in the quad-tree
type DeltaRegion struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	W     int `json:"w"`
	H     int `json:"h"`
	Color int `json:"c"`
}

// VideoPacket represents a frame packet with audio priority
type VideoPacket struct {
	Type      string       `json:"t"`  // "key" or "delta"
	Frame     int          `json:"f"`  // frame number
	Timestamp int64        `json:"ts"` // timestamp
	Audio     *AudioData   `json:"a"`  // audio data (priority)
	Video     *VideoData   `json:"v"`  // video data
	Quality   string       `json:"q"`  // quality level
	UserID    string       `json:"userId,omitempty"`
	Room      string       `json:"room,omitempty"`
}

// AudioData represents audio samples with priority handling
type AudioData struct {
	Data    string `json:"d"` // base64 encoded PCM
	Samples int    `json:"s"` // number of samples
}

// VideoData represents video frame data
type VideoData struct {
	Data    string         `json:"d,omitempty"` // keyframe data
	Regions []DeltaRegion  `json:"r,omitempty"` // delta regions
	Width   int            `json:"w,omitempty"` // frame width
	Height  int            `json:"h,omitempty"` // frame height
}

// FrameBuffer manages frame buffering and prioritization
type FrameBuffer struct {
	mu           sync.RWMutex
	audioBuffer  []AudioData
	videoBuffer  []VideoPacket
	maxAudioSize int
	maxVideoSize int
	stats        BufferStats
}

// BufferStats tracks performance metrics
type BufferStats struct {
	AudioPackets   int64
	VideoPackets   int64
	AudioDropped   int64
	VideoDropped   int64
	TotalBandwidth int64
	LastUpdate     time.Time
}

// NewFrameBuffer creates a new frame buffer with audio priority
func NewFrameBuffer() *FrameBuffer {
	return &FrameBuffer{
		audioBuffer:  make([]AudioData, 0, 100),
		videoBuffer:  make([]VideoPacket, 0, 30),
		maxAudioSize: 100, // Keep more audio for priority
		maxVideoSize: 30,  // Keep less video to save bandwidth
		stats: BufferStats{
			LastUpdate: time.Now(),
		},
	}
}

// AddPacket adds a packet with audio priority
func (fb *FrameBuffer) AddPacket(packet VideoPacket) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	// Always prioritize audio
	if packet.Audio != nil {
		fb.audioBuffer = append(fb.audioBuffer, *packet.Audio)
		fb.stats.AudioPackets++
		
		// Drop oldest audio if buffer full
		if len(fb.audioBuffer) > fb.maxAudioSize {
			fb.audioBuffer = fb.audioBuffer[1:]
			fb.stats.AudioDropped++
		}
	}

	// Add video if there's room
	if packet.Video != nil {
		// Calculate packet size for bandwidth management
		packetSize := fb.calculatePacketSize(&packet)
		fb.stats.TotalBandwidth += packetSize

		// Drop frames if buffer is full or bandwidth exceeded
		if len(fb.videoBuffer) >= fb.maxVideoSize {
			// Drop oldest non-keyframe first
			dropped := false
			for i := 0; i < len(fb.videoBuffer); i++ {
				if fb.videoBuffer[i].Type != "key" {
					fb.videoBuffer = append(fb.videoBuffer[:i], fb.videoBuffer[i+1:]...)
					fb.stats.VideoDropped++
					dropped = true
					break
				}
			}
			
			// If all keyframes, drop oldest
			if !dropped && len(fb.videoBuffer) > 0 {
				fb.videoBuffer = fb.videoBuffer[1:]
				fb.stats.VideoDropped++
			}
		}
		
		fb.videoBuffer = append(fb.videoBuffer, packet)
		fb.stats.VideoPackets++
	}
}

// GetNextPacket returns the next packet prioritizing audio
func (fb *FrameBuffer) GetNextPacket() *VideoPacket {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	packet := &VideoPacket{
		Timestamp: time.Now().UnixMilli(),
	}

	// Always send audio if available (PRIORITY)
	if len(fb.audioBuffer) > 0 {
		packet.Audio = &fb.audioBuffer[0]
		fb.audioBuffer = fb.audioBuffer[1:]
	}

	// Add video if bandwidth allows
	if len(fb.videoBuffer) > 0 {
		videoPacket := fb.videoBuffer[0]
		packet.Type = videoPacket.Type
		packet.Frame = videoPacket.Frame
		packet.Video = videoPacket.Video
		packet.Quality = videoPacket.Quality
		fb.videoBuffer = fb.videoBuffer[1:]
	}

	return packet
}

// GetStats returns current buffer statistics
func (fb *FrameBuffer) GetStats() BufferStats {
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	
	stats := fb.stats
	stats.LastUpdate = time.Now()
	return stats
}

func (fb *FrameBuffer) calculatePacketSize(packet *VideoPacket) int64 {
	size := int64(0)
	
	if packet.Audio != nil {
		// Estimate audio size from base64
		size += int64(len(packet.Audio.Data) * 3 / 4)
	}
	
	if packet.Video != nil {
		if packet.Video.Data != "" {
			// Keyframe size
			size += int64(len(packet.Video.Data) * 3 / 4)
		} else if packet.Video.Regions != nil {
			// Delta regions size (approx 16 bytes per region)
			size += int64(len(packet.Video.Regions) * 16)
		}
	}
	
	return size
}

// QuadTreeProcessor handles quad-tree codec processing
type QuadTreeProcessor struct {
	buffers map[string]*FrameBuffer // Per-user buffers
	mu      sync.RWMutex
	stats   ProcessorStats
}

// ProcessorStats tracks overall performance
type ProcessorStats struct {
	TotalPackets    int64
	ProcessingTime  time.Duration
	AverageFPS      float64
	AudioIntegrity  float64 // Percentage of audio preserved
	BandwidthMbps   float64
}

// NewQuadTreeProcessor creates a new processor
func NewQuadTreeProcessor() *QuadTreeProcessor {
	return &QuadTreeProcessor{
		buffers: make(map[string]*FrameBuffer),
	}
}

// ProcessPacket processes incoming packet with audio priority
func (qp *QuadTreeProcessor) ProcessPacket(userID string, data []byte) (*VideoPacket, error) {
	startTime := time.Now()
	defer func() {
		qp.stats.ProcessingTime += time.Since(startTime)
		qp.stats.TotalPackets++
	}()

	var packet VideoPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		return nil, err
	}

	// Get or create user buffer
	qp.mu.Lock()
	buffer, exists := qp.buffers[userID]
	if !exists {
		buffer = NewFrameBuffer()
		qp.buffers[userID] = buffer
	}
	qp.mu.Unlock()

	// Add packet to buffer with priority handling
	buffer.AddPacket(packet)

	// Get optimized packet for transmission
	optimizedPacket := buffer.GetNextPacket()
	
	// Update stats
	stats := buffer.GetStats()
	if stats.AudioPackets > 0 {
		qp.stats.AudioIntegrity = float64(stats.AudioPackets-stats.AudioDropped) / float64(stats.AudioPackets) * 100
	}
	
	return optimizedPacket, nil
}

// OptimizeForBandwidth adjusts quality based on available bandwidth
func (qp *QuadTreeProcessor) OptimizeForBandwidth(targetMbps float64) {
	qp.mu.Lock()
	defer qp.mu.Unlock()

	for _, buffer := range qp.buffers {
		currentBandwidth := float64(buffer.stats.TotalBandwidth) * 8 / 1000000 / time.Since(buffer.stats.LastUpdate).Seconds()
		
		if currentBandwidth > targetMbps {
			// Reduce video buffer size to save bandwidth
			buffer.maxVideoSize = max(10, buffer.maxVideoSize-5)
		} else if currentBandwidth < targetMbps*0.7 {
			// Increase video buffer if we have headroom
			buffer.maxVideoSize = min(50, buffer.maxVideoSize+5)
		}
		
		// Never reduce audio buffer (PRIORITY)
		buffer.maxAudioSize = 100
	}
}

// GetStats returns processor statistics
func (qp *QuadTreeProcessor) GetStats() ProcessorStats {
	qp.mu.RLock()
	defer qp.mu.RUnlock()
	
	stats := qp.stats
	
	// Calculate average FPS
	if qp.stats.ProcessingTime > 0 {
		stats.AverageFPS = float64(qp.stats.TotalPackets) / qp.stats.ProcessingTime.Seconds()
	}
	
	// Calculate total bandwidth
	totalBandwidth := int64(0)
	for _, buffer := range qp.buffers {
		totalBandwidth += buffer.stats.TotalBandwidth
	}
	
	if qp.stats.ProcessingTime > 0 {
		stats.BandwidthMbps = float64(totalBandwidth) * 8 / 1000000 / qp.stats.ProcessingTime.Seconds()
	}
	
	return stats
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// HandleQuadTreeMessage processes quad-tree codec messages in the conference server
func HandleQuadTreeMessage(hub *Hub, client *Client, message []byte) {
	processor := client.hub.quadTreeProcessor
	if processor == nil {
		processor = NewQuadTreeProcessor()
		client.hub.quadTreeProcessor = processor
	}

	optimizedPacket, err := processor.ProcessPacket(client.id, message)
	if err != nil {
		log.Printf("Error processing quad-tree packet: %v", err)
		return
	}

	// Set user info for routing
	optimizedPacket.UserID = client.userId
	optimizedPacket.Room = client.room

	// Serialize optimized packet
	data, err := json.Marshal(optimizedPacket)
	if err != nil {
		log.Printf("Error marshaling packet: %v", err)
		return
	}

	// Broadcast to room with audio priority
	hub.broadcastToRoom(client.room, data, client)
	
	// Log stats periodically
	stats := processor.GetStats()
	if stats.TotalPackets%600 == 0 { // Every ~10 seconds at 60fps
		log.Printf("QuadTree Stats - FPS: %.1f, Audio: %.1f%%, Bandwidth: %.2f Mbps",
			stats.AverageFPS, stats.AudioIntegrity, stats.BandwidthMbps)
	}
}