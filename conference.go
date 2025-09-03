package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message types
type Message struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Room      string          `json:"room,omitempty"`
	From      string          `json:"from,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp int64           `json:"timestamp,omitempty"`
}

// Client represents a connected user
type Client struct {
	ID   string
	Room string
	Conn *websocket.Conn
	Send chan []byte
}

// Room manages participants
type Room struct {
	Name    string
	Clients map[string]*Client
	mu      sync.RWMutex
}

// Hub manages all rooms
type Hub struct {
	Rooms      map[string]*Room
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan []byte
	mu         sync.RWMutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024 * 64,
	WriteBufferSize: 1024 * 64,
}

var hub = &Hub{
	Rooms:      make(map[string]*Room),
	Register:   make(chan *Client),
	Unregister: make(chan *Client),
	Broadcast:  make(chan []byte, 256),
}

// HTML client embedded in Go
const htmlClient = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>WebSocket Conference</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            min-height: 100vh;
            padding: 20px;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        h1 { text-align: center; margin-bottom: 30px; }
        .controls {
            display: flex;
            justify-content: center;
            gap: 15px;
            margin-bottom: 30px;
            flex-wrap: wrap;
        }
        button {
            padding: 12px 30px;
            font-size: 16px;
            border: none;
            border-radius: 8px;
            background: white;
            color: #764ba2;
            cursor: pointer;
            font-weight: bold;
            transition: transform 0.2s;
        }
        button:hover:not(:disabled) { transform: scale(1.05); }
        button:disabled { opacity: 0.5; cursor: not-allowed; }
        .status {
            text-align: center;
            padding: 15px;
            background: rgba(255,255,255,0.2);
            border-radius: 10px;
            margin-bottom: 20px;
        }
        .videos {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .video-container {
            background: rgba(0,0,0,0.3);
            border-radius: 10px;
            padding: 10px;
            position: relative;
        }
        .video-label {
            position: absolute;
            top: 20px;
            left: 20px;
            background: rgba(0,0,0,0.7);
            padding: 8px 15px;
            border-radius: 5px;
            z-index: 10;
        }
        video, canvas {
            width: 100%;
            height: auto;
            min-height: 300px;
            border-radius: 8px;
            background: #000;
            display: block;
        }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
            padding: 20px;
            background: rgba(255,255,255,0.1);
            border-radius: 10px;
        }
        .stat {
            text-align: center;
        }
        .stat-value {
            font-size: 24px;
            font-weight: bold;
        }
        .stat-label {
            font-size: 12px;
            opacity: 0.8;
            margin-top: 5px;
        }
        input[type="range"] {
            width: 100px;
        }
        .control-group {
            display: flex;
            align-items: center;
            gap: 10px;
            background: rgba(255,255,255,0.2);
            padding: 10px 15px;
            border-radius: 8px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎥 WebSocket Video Conference</h1>
        
        <div class="status" id="status">
            Ready to connect
        </div>
        
        <div class="controls">
            <button id="startBtn" onclick="startCall()">Start Call</button>
            <button id="stopBtn" onclick="stopCall()" disabled>End Call</button>
            <button id="muteVideoBtn" onclick="toggleVideo()" disabled>📷 Video</button>
            <button id="muteAudioBtn" onclick="toggleAudio()" disabled>🎤 Audio</button>
            
            <div class="control-group">
                <label>Quality:</label>
                <input type="range" id="quality" min="0.3" max="0.9" step="0.1" value="0.5">
                <span id="qualityValue">0.5</span>
            </div>
            
            <div class="control-group">
                <label>FPS:</label>
                <input type="range" id="fps" min="5" max="30" step="5" value="10">
                <span id="fpsValue">10</span>
            </div>
        </div>
        
        <div class="videos" id="videosContainer"></div>
        
        <div class="stats">
            <div class="stat">
                <div class="stat-value" id="participants">0</div>
                <div class="stat-label">Participants</div>
            </div>
            <div class="stat">
                <div class="stat-value" id="videoFrames">0</div>
                <div class="stat-label">Video Frames</div>
            </div>
            <div class="stat">
                <div class="stat-value" id="audioChunks">0</div>
                <div class="stat-label">Audio Chunks</div>
            </div>
            <div class="stat">
                <div class="stat-value" id="bandwidth">0</div>
                <div class="stat-label">KB/s</div>
            </div>
        </div>
    </div>

    <script>
        // Configuration
        const WS_URL = window.location.protocol === 'https:' 
            ? 'wss://' + window.location.host + '/ws'
            : 'ws://' + window.location.host + '/ws';
        
        // State
        let ws = null;
        let localStream = null;
        let myId = null;
        let isConnected = false;
        let videoEnabled = true;
        let audioEnabled = true;
        let captureInterval = null;
        let audioContext = null;
        let audioProcessor = null;
        let participants = new Map();
        
        // Stats
        let videoFramesSent = 0;
        let videoFramesReceived = 0;
        let audioChunksSent = 0;
        let audioChunksReceived = 0;
        let bytesSent = 0;
        let lastBandwidthTime = Date.now();
        
        // Quality controls
        document.getElementById('quality').addEventListener('input', (e) => {
            document.getElementById('qualityValue').textContent = e.target.value;
        });
        
        document.getElementById('fps').addEventListener('input', (e) => {
            document.getElementById('fpsValue').textContent = e.target.value;
            if (captureInterval) restartVideoCapture();
        });
        
        async function startCall() {
            try {
                // Get user media
                localStream = await navigator.mediaDevices.getUserMedia({
                    video: { width: 640, height: 480 },
                    audio: {
                        echoCancellation: true,
                        noiseSuppression: true,
                        autoGainControl: true
                    }
                });
                
                // Add local video
                addVideoElement('local', 'You (Local)', true);
                const localVideo = document.getElementById('video-local');
                if (localVideo) {
                    localVideo.srcObject = localStream;
                }
                
                // Setup audio processing
                setupAudioCapture();
                
                // Connect WebSocket
                connectWebSocket();
                
                // Update UI
                document.getElementById('startBtn').disabled = true;
                document.getElementById('stopBtn').disabled = false;
                document.getElementById('muteVideoBtn').disabled = false;
                document.getElementById('muteAudioBtn').disabled = false;
                
                updateStatus('Connecting...', '#ffc107');
                
            } catch (error) {
                console.error('Error starting call:', error);
                updateStatus('Error: ' + error.message, '#f44336');
            }
        }
        
        function connectWebSocket() {
            ws = new WebSocket(WS_URL);
            
            ws.onopen = () => {
                console.log('WebSocket connected');
                myId = 'user-' + Math.random().toString(36).substr(2, 9);
                
                // Join room
                ws.send(JSON.stringify({
                    type: 'join',
                    id: myId,
                    room: 'main'
                }));
                
                isConnected = true;
                updateStatus('Connected', '#4caf50');
                
                // Start capturing after connection
                setTimeout(() => {
                    startVideoCapture();
                }, 500);
            };
            
            ws.onmessage = (event) => {
                const message = JSON.parse(event.data);
                handleMessage(message);
            };
            
            ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                updateStatus('Connection error', '#f44336');
            };
            
            ws.onclose = () => {
                console.log('WebSocket closed');
                isConnected = false;
                updateStatus('Disconnected', '#9e9e9e');
                stopCall();
            };
        }
        
        function handleMessage(message) {
            switch (message.type) {
                case 'welcome':
                    console.log('Joined as:', message.yourId);
                    myId = message.yourId;
                    
                    // Add existing participants
                    if (message.participants) {
                        message.participants.forEach(id => {
                            if (!participants.has(id)) {
                                addParticipant(id);
                            }
                        });
                    }
                    updateParticipantCount();
                    break;
                    
                case 'participant-joined':
                    console.log('Participant joined:', message.participantId);
                    if (message.participantId !== myId && !participants.has(message.participantId)) {
                        addParticipant(message.participantId);
                        updateParticipantCount();
                    }
                    break;
                    
                case 'participant-left':
                    console.log('Participant left:', message.participantId);
                    removeParticipant(message.participantId);
                    updateParticipantCount();
                    break;
                    
                case 'video-frame':
                    if (message.from && message.from !== myId) {
                        displayVideoFrame(message.from, message.data);
                        videoFramesReceived++;
                    }
                    break;
                    
                case 'audio-chunk':
                    if (message.from && message.from !== myId) {
                        playAudioChunk(message.from, message.data);
                        audioChunksReceived++;
                    }
                    break;
            }
            
            // Update stats
            document.getElementById('videoFrames').textContent = videoFramesSent + videoFramesReceived;
            document.getElementById('audioChunks').textContent = audioChunksSent + audioChunksReceived;
        }
        
        function setupAudioCapture() {
            audioContext = new (window.AudioContext || window.webkitAudioContext)();
            const source = audioContext.createMediaStreamSource(localStream);
            audioProcessor = audioContext.createScriptProcessor(2048, 1, 1);
            
            audioProcessor.onaudioprocess = (e) => {
                if (!audioEnabled || !isConnected) return;
                
                const inputData = e.inputBuffer.getChannelData(0);
                
                // Convert to 16-bit PCM
                const pcm = new Int16Array(inputData.length);
                for (let i = 0; i < inputData.length; i++) {
                    const s = Math.max(-1, Math.min(1, inputData[i]));
                    pcm[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
                }
                
                // Convert to base64
                const buffer = pcm.buffer;
                const bytes = new Uint8Array(buffer);
                let binary = '';
                for (let i = 0; i < bytes.byteLength; i++) {
                    binary += String.fromCharCode(bytes[i]);
                }
                const base64 = btoa(binary);
                
                // Send audio chunk
                if (ws && ws.readyState === WebSocket.OPEN) {
                    ws.send(JSON.stringify({
                        type: 'audio-chunk',
                        data: base64,
                        timestamp: Date.now()
                    }));
                    audioChunksSent++;
                    bytesSent += base64.length;
                }
            };
            
            source.connect(audioProcessor);
            audioProcessor.connect(audioContext.destination);
        }
        
        function startVideoCapture() {
            if (captureInterval) return;
            
            const fps = parseInt(document.getElementById('fps').value);
            const interval = 1000 / fps;
            
            const video = document.getElementById('video-local');
            if (!video) return;
            
            const canvas = document.createElement('canvas');
            const ctx = canvas.getContext('2d');
            canvas.width = 640;
            canvas.height = 480;
            
            captureInterval = setInterval(() => {
                if (!videoEnabled || !isConnected || !ws) return;
                
                if (video.readyState === video.HAVE_ENOUGH_DATA) {
                    ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
                    
                    const quality = parseFloat(document.getElementById('quality').value);
                    
                    canvas.toBlob((blob) => {
                        if (blob) {
                            const reader = new FileReader();
                            reader.onload = () => {
                                if (ws && ws.readyState === WebSocket.OPEN) {
                                    ws.send(JSON.stringify({
                                        type: 'video-frame',
                                        data: reader.result,
                                        timestamp: Date.now()
                                    }));
                                    videoFramesSent++;
                                    bytesSent += reader.result.length;
                                }
                            };
                            reader.readAsDataURL(blob);
                        }
                    }, 'image/jpeg', quality);
                }
            }, interval);
        }
        
        function restartVideoCapture() {
            if (captureInterval) {
                clearInterval(captureInterval);
                captureInterval = null;
                startVideoCapture();
            }
        }
        
        function addParticipant(id) {
            participants.set(id, {
                audioContext: new (window.AudioContext || window.webkitAudioContext)(),
                audioQueue: []
            });
            addVideoElement(id, 'User: ' + id.substr(-6), false);
        }
        
        function removeParticipant(id) {
            const participant = participants.get(id);
            if (participant && participant.audioContext) {
                participant.audioContext.close();
            }
            participants.delete(id);
            removeVideoElement(id);
        }
        
        function addVideoElement(id, label, isLocal) {
            const container = document.createElement('div');
            container.className = 'video-container';
            container.id = 'container-' + id;
            
            const labelDiv = document.createElement('div');
            labelDiv.className = 'video-label';
            labelDiv.textContent = label;
            container.appendChild(labelDiv);
            
            if (isLocal) {
                const video = document.createElement('video');
                video.id = 'video-' + id;
                video.autoplay = true;
                video.muted = true;
                video.playsinline = true;
                container.appendChild(video);
            } else {
                const canvas = document.createElement('canvas');
                canvas.id = 'canvas-' + id;
                canvas.width = 640;
                canvas.height = 480;
                container.appendChild(canvas);
            }
            
            document.getElementById('videosContainer').appendChild(container);
        }
        
        function removeVideoElement(id) {
            const element = document.getElementById('container-' + id);
            if (element) element.remove();
        }
        
        function displayVideoFrame(participantId, imageData) {
            const canvas = document.getElementById('canvas-' + participantId);
            if (!canvas) {
                if (!participants.has(participantId)) {
                    addParticipant(participantId);
                }
                return;
            }
            
            const ctx = canvas.getContext('2d');
            const img = new Image();
            img.onload = () => {
                ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
            };
            img.src = imageData;
        }
        
        function playAudioChunk(participantId, base64Audio) {
            const participant = participants.get(participantId);
            if (!participant) return;
            
            // Decode base64
            const binary = atob(base64Audio);
            const len = binary.length;
            const bytes = new Uint8Array(len);
            for (let i = 0; i < len; i++) {
                bytes[i] = binary.charCodeAt(i);
            }
            
            // Convert to Float32
            const pcm = new Int16Array(bytes.buffer);
            const float32 = new Float32Array(pcm.length);
            for (let i = 0; i < pcm.length; i++) {
                float32[i] = pcm[i] / (pcm[i] < 0 ? 0x8000 : 0x7FFF);
            }
            
            // Create audio buffer
            const audioBuffer = participant.audioContext.createBuffer(1, float32.length, 48000);
            audioBuffer.copyToChannel(float32, 0);
            
            // Play immediately
            const source = participant.audioContext.createBufferSource();
            source.buffer = audioBuffer;
            source.connect(participant.audioContext.destination);
            source.start();
        }
        
        function toggleVideo() {
            videoEnabled = !videoEnabled;
            const btn = document.getElementById('muteVideoBtn');
            btn.textContent = videoEnabled ? '📷 Video' : '📷 Video (Off)';
            
            if (localStream) {
                localStream.getVideoTracks().forEach(track => {
                    track.enabled = videoEnabled;
                });
            }
        }
        
        function toggleAudio() {
            audioEnabled = !audioEnabled;
            const btn = document.getElementById('muteAudioBtn');
            btn.textContent = audioEnabled ? '🎤 Audio' : '🎤 Audio (Off)';
            
            if (localStream) {
                localStream.getAudioTracks().forEach(track => {
                    track.enabled = audioEnabled;
                });
            }
        }
        
        function stopCall() {
            // Stop capture
            if (captureInterval) {
                clearInterval(captureInterval);
                captureInterval = null;
            }
            
            // Stop audio
            if (audioProcessor) {
                audioProcessor.disconnect();
                audioProcessor = null;
            }
            if (audioContext) {
                audioContext.close();
                audioContext = null;
            }
            
            // Stop stream
            if (localStream) {
                localStream.getTracks().forEach(track => track.stop());
                localStream = null;
            }
            
            // Close WebSocket
            if (ws) {
                ws.close();
                ws = null;
            }
            
            // Clear participants
            participants.forEach(p => {
                if (p.audioContext) p.audioContext.close();
            });
            participants.clear();
            
            // Clear UI
            document.getElementById('videosContainer').innerHTML = '';
            
            // Reset buttons
            document.getElementById('startBtn').disabled = false;
            document.getElementById('stopBtn').disabled = true;
            document.getElementById('muteVideoBtn').disabled = true;
            document.getElementById('muteAudioBtn').disabled = true;
            
            updateStatus('Call ended', '#9e9e9e');
        }
        
        function updateStatus(text, color) {
            const status = document.getElementById('status');
            status.textContent = text;
            status.style.background = color ? 'rgba(255,255,255,0.2)' : '';
            status.style.borderLeft = color ? '4px solid ' + color : '';
        }
        
        function updateParticipantCount() {
            document.getElementById('participants').textContent = participants.size;
        }
        
        // Bandwidth monitor
        setInterval(() => {
            const now = Date.now();
            const elapsed = (now - lastBandwidthTime) / 1000;
            const bandwidth = Math.round((bytesSent / 1024) / elapsed);
            document.getElementById('bandwidth').textContent = bandwidth;
            bytesSent = 0;
            lastBandwidthTime = now;
        }, 1000);
    </script>
</body>
</html>`

// Run hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.addClient(client)

		case client := <-h.Unregister:
			h.removeClient(client)
		}
	}
}

func (h *Hub) addClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Get or create room
	room, exists := h.Rooms[client.Room]
	if !exists {
		room = &Room{
			Name:    client.Room,
			Clients: make(map[string]*Client),
		}
		h.Rooms[client.Room] = room
	}

	// Add to room
	room.mu.Lock()
	room.Clients[client.ID] = client
	participants := make([]string, 0, len(room.Clients)-1)
	for id := range room.Clients {
		if id != client.ID {
			participants = append(participants, id)
		}
	}
	room.mu.Unlock()

	// Send welcome
	welcomeData := map[string]interface{}{
		"type": "welcome",
		"yourId": client.ID,
		"room": client.Room,
		"participants": participants,
	}
	
	if data, err := json.Marshal(welcomeData); err == nil {
		client.Send <- data
	}

	// Notify others
	notification := map[string]interface{}{
		"type": "participant-joined",
		"participantId": client.ID,
		"timestamp": time.Now().UnixMilli(),
	}
	
	if data, err := json.Marshal(notification); err == nil {
		room.mu.RLock()
		for id, c := range room.Clients {
			if id != client.ID {
				select {
				case c.Send <- data:
				default:
				}
			}
		}
		room.mu.RUnlock()
	}

	log.Printf("Client %s joined room %s (total: %d)", client.ID, client.Room, len(room.Clients))
}

func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	room, exists := h.Rooms[client.Room]
	h.mu.Unlock()

	if !exists {
		return
	}

	room.mu.Lock()
	delete(room.Clients, client.ID)
	roomSize := len(room.Clients)
	room.mu.Unlock()

	close(client.Send)

	// Notify others
	if roomSize > 0 {
		notification := map[string]interface{}{
			"type": "participant-left",
			"participantId": client.ID,
			"timestamp": time.Now().UnixMilli(),
		}
		
		if data, err := json.Marshal(notification); err == nil {
			room.mu.RLock()
			for _, c := range room.Clients {
				select {
				case c.Send <- data:
				default:
				}
			}
			room.mu.RUnlock()
		}
	} else {
		// Remove empty room
		h.mu.Lock()
		delete(h.Rooms, client.Room)
		h.mu.Unlock()
	}

	log.Printf("Client %s left room %s (remaining: %d)", client.ID, client.Room, roomSize)
}

// Client handlers
func (c *Client) ReadPump() {
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
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		// Parse message
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		// Handle based on type
		switch msg.Type {
		case "video-frame", "audio-chunk":
			// Relay to others in room
			msg.From = c.ID
			
			hub.mu.RLock()
			room := hub.Rooms[c.Room]
			hub.mu.RUnlock()
			
			if room != nil {
				if relayData, err := json.Marshal(msg); err == nil {
					room.mu.RLock()
					for id, client := range room.Clients {
						if id != c.ID {
							select {
							case client.Send <- relayData:
							default:
							}
						}
					}
					room.mu.RUnlock()
				}
			}
		}

		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Wait for join message
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return
	}

	var joinMsg Message
	if err := json.Unmarshal(message, &joinMsg); err != nil || joinMsg.Type != "join" {
		conn.Close()
		return
	}

	client := &Client{
		ID:   joinMsg.ID,
		Room: joinMsg.Room,
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	hub.Register <- client

	go client.WritePump()
	go client.ReadPump()
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlClient))
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	status := map[string]interface{}{
		"rooms": len(hub.Rooms),
		"details": []map[string]interface{}{},
	}

	for name, room := range hub.Rooms {
		room.mu.RLock()
		roomInfo := map[string]interface{}{
			"name":         name,
			"participants": len(room.Clients),
		}
		room.mu.RUnlock()
		status["details"] = append(status["details"].([]map[string]interface{}), roomInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	go hub.Run()

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/status", handleStatus)

	port := "8080"
	log.Printf("Conference server starting on http://localhost:%s", port)
	log.Printf("Open http://localhost:%s in multiple tabs to test", port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}