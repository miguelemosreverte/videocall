package main

import (
    "fmt"
    "log"
    "net/http"
    "time"
    
    "github.com/gorilla/websocket"
)

type TestMessage struct {
    Type      string  `json:"type"`
    Data      string  `json:"data,omitempty"`
    Size      int     `json:"size,omitempty"`
    Timestamp int64   `json:"timestamp,omitempty"`
    Seq       int     `json:"seq,omitempty"`
    Duration  float64 `json:"duration,omitempty"`
    Speed     float64 `json:"speed,omitempty"`
}

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
    ReadBufferSize:  1024 * 1024 * 10, // 10MB buffer
    WriteBufferSize: 1024 * 1024 * 10, // 10MB buffer
}

func handleBandwidthTest(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("Failed to upgrade: %v", err)
        return
    }
    defer conn.Close()
    
    log.Printf("Client connected from %s", r.RemoteAddr)
    
    for {
        var msg TestMessage
        err := conn.ReadJSON(&msg)
        if err != nil {
            log.Printf("Read error: %v", err)
            break
        }
        
        switch msg.Type {
        case "ping":
            // Simple ping-pong for latency measurement
            response := TestMessage{
                Type:      "pong",
                Timestamp: time.Now().UnixNano() / 1e6,
            }
            conn.WriteJSON(response)
            
        case "download-test":
            // Server sends data to client
            log.Printf("Starting download test: %d bytes", msg.Size)
            startTime := time.Now()
            
            // Generate test data
            testData := make([]byte, msg.Size)
            for i := range testData {
                testData[i] = byte(i % 256)
            }
            
            // Send in chunks to avoid overwhelming
            chunkSize := 1024 * 1024 // 1MB chunks
            totalSent := 0
            seq := 0
            
            for totalSent < len(testData) {
                end := totalSent + chunkSize
                if end > len(testData) {
                    end = len(testData)
                }
                
                chunk := TestMessage{
                    Type: "download-chunk",
                    Data: string(testData[totalSent:end]),
                    Size: end - totalSent,
                    Seq:  seq,
                }
                
                if err := conn.WriteJSON(chunk); err != nil {
                    log.Printf("Write error: %v", err)
                    break
                }
                
                totalSent = end
                seq++
            }
            
            // Send completion message
            duration := time.Since(startTime).Seconds()
            speedMbps := float64(msg.Size) * 8 / duration / 1e6
            
            complete := TestMessage{
                Type:     "download-complete",
                Size:     totalSent,
                Duration: duration,
                Speed:    speedMbps,
            }
            conn.WriteJSON(complete)
            log.Printf("Download test complete: %.2f Mbps", speedMbps)
            
        case "upload-test":
            // Client sends data to server
            log.Printf("Starting upload test for %d bytes", msg.Size)
            startTime := time.Now()
            totalReceived := 0
            
            // Keep receiving until we get upload-complete
            for {
                var chunk TestMessage
                if err := conn.ReadJSON(&chunk); err != nil {
                    log.Printf("Read error during upload: %v", err)
                    break
                }
                
                if chunk.Type == "upload-chunk" {
                    totalReceived += len(chunk.Data)
                } else if chunk.Type == "upload-complete" {
                    duration := time.Since(startTime).Seconds()
                    speedMbps := float64(totalReceived) * 8 / duration / 1e6
                    
                    // Send results back
                    result := TestMessage{
                        Type:     "upload-result",
                        Size:     totalReceived,
                        Duration: duration,
                        Speed:    speedMbps,
                    }
                    conn.WriteJSON(result)
                    log.Printf("Upload test complete: %.2f Mbps", speedMbps)
                    break
                }
            }
            
        case "echo":
            // Echo back for round-trip testing
            msg.Type = "echo-reply"
            msg.Timestamp = time.Now().UnixNano() / 1e6
            conn.WriteJSON(msg)
        }
    }
}

func handleHome(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Bandwidth Test Server</title>
</head>
<body>
    <h1>WebSocket Bandwidth Test Server</h1>
    <p>Server is running on port 8080</p>
    <p>WebSocket endpoint: ws://%s/ws</p>
    <p>Use the HTML client to test bandwidth</p>
</body>
</html>`, r.Host)
}

func main() {
    port := "8090"
    
    http.HandleFunc("/", handleHome)
    http.HandleFunc("/ws", handleBandwidthTest)
    
    log.Printf("Starting bandwidth test server on :%s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}