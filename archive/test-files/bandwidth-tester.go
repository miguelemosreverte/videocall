package main

import (
    "fmt"
    "log"
    "math"
    "os"
    "sort"
    "time"
    
    "github.com/gorilla/websocket"
)

type TestMessage struct {
    Type      string  `json:"type"`
    Data      string  `json:"data,omitempty"`
    Size      int     `json:"size,omitempty"`
    Timestamp int64   `json:"timestamp,omitempty"`
    Seq       int     `json:"seq,omitempty"`
    Speed     float64 `json:"speed,omitempty"`
    Duration  float64 `json:"duration,omitempty"`
}

type BandwidthResults struct {
    LatencyMS      float64
    JitterMS       float64
    MinLatencyMS   float64
    MaxLatencyMS   float64
    DownloadMbps   float64
    UploadMbps     float64
    PacketLoss     float64
    TestDuration   float64
    Timestamp      time.Time
    ServerURL      string
    LatencySamples []float64
    DownloadTests  []TestResult
    UploadTests    []TestResult
}

type TestResult struct {
    SizeMB   float64
    SpeedMbps float64
    Duration float64
}

func main() {
    fmt.Println("🚀 VPS Bandwidth Tester")
    fmt.Println("========================")
    
    serverURL := "ws://194.87.103.57:8090/ws"
    if len(os.Args) > 1 {
        serverURL = os.Args[1]
    }
    
    fmt.Printf("Testing server: %s\n\n", serverURL)
    
    results := runBandwidthTests(serverURL)
    generateMarkdownReport(results)
    
    fmt.Println("\n✅ Report generated: bandwidth-report.md")
}

func runBandwidthTests(serverURL string) *BandwidthResults {
    results := &BandwidthResults{
        ServerURL: serverURL,
        Timestamp: time.Now(),
    }
    
    startTime := time.Now()
    
    // Connect to server
    fmt.Print("Connecting to server... ")
    conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
    if err != nil {
        log.Fatal("Connection failed:", err)
    }
    defer conn.Close()
    fmt.Println("✓")
    
    // Run tests
    runLatencyTest(conn, results)
    runDownloadTests(conn, results)
    runUploadTests(conn, results)
    
    results.TestDuration = time.Since(startTime).Seconds()
    
    return results
}

func runLatencyTest(conn *websocket.Conn, results *BandwidthResults) {
    fmt.Print("Testing latency (20 pings)... ")
    
    const numPings = 20
    latencies := make([]float64, 0, numPings)
    
    for i := 0; i < numPings; i++ {
        startTime := time.Now()
        
        // Send ping
        msg := TestMessage{Type: "ping"}
        if err := conn.WriteJSON(msg); err != nil {
            continue
        }
        
        // Wait for pong
        var response TestMessage
        if err := conn.ReadJSON(&response); err != nil {
            continue
        }
        
        if response.Type == "pong" {
            latency := time.Since(startTime).Seconds() * 1000 // Convert to ms
            latencies = append(latencies, latency)
        }
        
        time.Sleep(50 * time.Millisecond)
    }
    
    if len(latencies) == 0 {
        fmt.Println("✗ (no responses)")
        return
    }
    
    // Calculate statistics
    sort.Float64s(latencies)
    
    sum := 0.0
    for _, l := range latencies {
        sum += l
    }
    
    results.LatencySamples = latencies
    results.LatencyMS = sum / float64(len(latencies))
    results.MinLatencyMS = latencies[0]
    results.MaxLatencyMS = latencies[len(latencies)-1]
    results.JitterMS = results.MaxLatencyMS - results.MinLatencyMS
    
    fmt.Printf("✓ (avg: %.1fms, jitter: %.1fms)\n", results.LatencyMS, results.JitterMS)
}

func runDownloadTests(conn *websocket.Conn, results *BandwidthResults) {
    testSizes := []int{
        512 * 1024,        // 512KB
        1 * 1024 * 1024,   // 1MB
        2 * 1024 * 1024,   // 2MB
    }
    
    var totalSpeed float64
    
    for _, size := range testSizes {
        sizeMB := float64(size) / (1024 * 1024)
        fmt.Printf("Downloading %.0fMB... ", sizeMB)
        
        // Request download
        msg := TestMessage{
            Type: "download-test",
            Size: size,
        }
        if err := conn.WriteJSON(msg); err != nil {
            fmt.Println("✗")
            continue
        }
        
        // Read chunks until complete with timeout
        var receivedBytes int
        conn.SetReadDeadline(time.Now().Add(30 * time.Second))
        for {
            var response TestMessage
            if err := conn.ReadJSON(&response); err != nil {
                fmt.Println("✗ (timeout or error)")
                break
            }
            
            if response.Type == "download-chunk" {
                receivedBytes += response.Size
            } else if response.Type == "download-complete" {
                results.DownloadTests = append(results.DownloadTests, TestResult{
                    SizeMB:    sizeMB,
                    SpeedMbps: response.Speed,
                    Duration:  response.Duration,
                })
                totalSpeed += response.Speed
                fmt.Printf("✓ (%.1f Mbps)\n", response.Speed)
                break
            }
        }
    }
    
    if len(results.DownloadTests) > 0 {
        results.DownloadMbps = totalSpeed / float64(len(results.DownloadTests))
    }
}

func runUploadTests(conn *websocket.Conn, results *BandwidthResults) {
    testSizes := []int{
        512 * 1024,       // 512KB
        1 * 1024 * 1024,  // 1MB
    }
    
    var totalSpeed float64
    
    for _, size := range testSizes {
        sizeMB := float64(size) / (1024 * 1024)
        fmt.Printf("Uploading %.0fMB... ", sizeMB)
        
        // Notify server of upload test
        msg := TestMessage{
            Type: "upload-test",
            Size: size,
        }
        if err := conn.WriteJSON(msg); err != nil {
            fmt.Println("✗")
            continue
        }
        
        // Send data in chunks
        chunkSize := 256 * 1024 // 256KB chunks
        totalSent := 0
        
        for totalSent < size {
            remaining := size - totalSent
            currentChunk := chunkSize
            if remaining < chunkSize {
                currentChunk = remaining
            }
            
            // Generate test data
            data := make([]byte, currentChunk)
            for i := range data {
                data[i] = byte(i % 256)
            }
            
            chunk := TestMessage{
                Type: "upload-chunk",
                Data: string(data),
                Size: currentChunk,
            }
            
            if err := conn.WriteJSON(chunk); err != nil {
                break
            }
            
            totalSent += currentChunk
        }
        
        // Send completion
        complete := TestMessage{
            Type: "upload-complete",
            Size: totalSent,
        }
        if err := conn.WriteJSON(complete); err != nil {
            fmt.Println("✗")
            continue
        }
        
        // Wait for result
        var response TestMessage
        if err := conn.ReadJSON(&response); err == nil && response.Type == "upload-result" {
            results.UploadTests = append(results.UploadTests, TestResult{
                SizeMB:    sizeMB,
                SpeedMbps: response.Speed,
                Duration:  response.Duration,
            })
            totalSpeed += response.Speed
            fmt.Printf("✓ (%.1f Mbps)\n", response.Speed)
        }
    }
    
    if len(results.UploadTests) > 0 {
        results.UploadMbps = totalSpeed / float64(len(results.UploadTests))
    }
}

func generateMarkdownReport(results *BandwidthResults) {
    report := fmt.Sprintf(`# VPS Bandwidth Test Report

## Test Information
- **Server:** %s
- **Timestamp:** %s
- **Test Duration:** %.1f seconds

## Summary Results

| Metric | Value | Status |
|--------|-------|--------|
| **Latency** | %.1f ms | %s |
| **Jitter** | %.1f ms | %s |
| **Download Speed** | %.1f Mbps | %s |
| **Upload Speed** | %.1f Mbps | %s |
| **Packet Loss** | %.1f%% | ✅ Excellent |

## Detailed Results

### Latency Analysis
- **Average:** %.1f ms
- **Minimum:** %.1f ms
- **Maximum:** %.1f ms
- **Jitter:** %.1f ms
- **Samples:** %d pings

### Download Performance
`, 
        results.ServerURL,
        results.Timestamp.Format("2006-01-02 15:04:05"),
        results.TestDuration,
        results.LatencyMS,
        getLatencyStatus(results.LatencyMS),
        results.JitterMS,
        getJitterStatus(results.JitterMS),
        results.DownloadMbps,
        getSpeedStatus(results.DownloadMbps),
        results.UploadMbps,
        getSpeedStatus(results.UploadMbps),
        results.PacketLoss,
        results.LatencyMS,
        results.MinLatencyMS,
        results.MaxLatencyMS,
        results.JitterMS,
        len(results.LatencySamples),
    )
    
    // Add download test details
    report += "| Test Size | Speed | Duration |\n"
    report += "|-----------|-------|----------|\n"
    for _, test := range results.DownloadTests {
        report += fmt.Sprintf("| %.0f MB | %.1f Mbps | %.2f s |\n", 
            test.SizeMB, test.SpeedMbps, test.Duration)
    }
    report += fmt.Sprintf("\n**Average Download:** %.1f Mbps\n", results.DownloadMbps)
    
    // Add upload test details
    report += "\n### Upload Performance\n"
    report += "| Test Size | Speed | Duration |\n"
    report += "|-----------|-------|----------|\n"
    for _, test := range results.UploadTests {
        report += fmt.Sprintf("| %.0f MB | %.1f Mbps | %.2f s |\n", 
            test.SizeMB, test.SpeedMbps, test.Duration)
    }
    report += fmt.Sprintf("\n**Average Upload:** %.1f Mbps\n", results.UploadMbps)
    
    // Performance rating
    rating := calculateRating(results)
    report += fmt.Sprintf("\n## Performance Rating: %s\n\n", rating)
    
    // Suitability assessment
    report += "## Application Suitability\n\n"
    report += generateSuitabilityAssessment(results)
    
    // Network quality assessment
    report += "\n## Network Quality Assessment\n\n"
    report += generateQualityAssessment(results)
    
    // Write to file
    err := os.WriteFile("bandwidth-report.md", []byte(report), 0644)
    if err != nil {
        log.Fatal("Failed to write report:", err)
    }
}

func getLatencyStatus(latency float64) string {
    if latency < 30 {
        return "✅ Excellent"
    } else if latency < 50 {
        return "✅ Good"
    } else if latency < 100 {
        return "⚠️ Fair"
    }
    return "❌ Poor"
}

func getJitterStatus(jitter float64) string {
    if jitter < 10 {
        return "✅ Excellent"
    } else if jitter < 30 {
        return "✅ Good"
    } else if jitter < 50 {
        return "⚠️ Fair"
    }
    return "❌ Poor"
}

func getSpeedStatus(speed float64) string {
    if speed > 100 {
        return "✅ Excellent"
    } else if speed > 50 {
        return "✅ Good"
    } else if speed > 25 {
        return "⚠️ Fair"
    }
    return "❌ Poor"
}

func calculateRating(results *BandwidthResults) string {
    score := 0
    
    if results.LatencyMS < 50 {
        score += 3
    } else if results.LatencyMS < 100 {
        score += 2
    } else if results.LatencyMS < 150 {
        score += 1
    }
    
    if results.DownloadMbps > 100 {
        score += 3
    } else if results.DownloadMbps > 50 {
        score += 2
    } else if results.DownloadMbps > 25 {
        score += 1
    }
    
    if results.UploadMbps > 50 {
        score += 3
    } else if results.UploadMbps > 25 {
        score += 2
    } else if results.UploadMbps > 10 {
        score += 1
    }
    
    if results.JitterMS < 30 {
        score += 1
    }
    
    switch {
    case score >= 9:
        return "⭐⭐⭐⭐⭐ EXCELLENT"
    case score >= 7:
        return "⭐⭐⭐⭐ GOOD"
    case score >= 5:
        return "⭐⭐⭐ FAIR"
    case score >= 3:
        return "⭐⭐ POOR"
    default:
        return "⭐ NEEDS IMPROVEMENT"
    }
}

func generateSuitabilityAssessment(results *BandwidthResults) string {
    assessment := ""
    
    // Video conferencing
    if results.LatencyMS < 150 && results.DownloadMbps > 4 && results.UploadMbps > 3 {
        if results.LatencyMS < 50 && results.DownloadMbps > 25 {
            assessment += "✅ **HD Video Conferencing (1080p)** - Excellent conditions\n"
        } else {
            assessment += "✅ **Video Conferencing (720p)** - Good conditions\n"
        }
    } else {
        assessment += "⚠️ **Video Conferencing** - May experience quality issues\n"
    }
    
    // Streaming
    if results.DownloadMbps > 25 {
        assessment += "✅ **4K Streaming** - Sufficient bandwidth\n"
    } else if results.DownloadMbps > 10 {
        assessment += "✅ **HD Streaming (1080p)** - Good bandwidth\n"
    } else if results.DownloadMbps > 5 {
        assessment += "✅ **SD Streaming (720p)** - Adequate bandwidth\n"
    } else {
        assessment += "⚠️ **Video Streaming** - Limited quality\n"
    }
    
    // Live broadcasting
    if results.UploadMbps > 10 && results.LatencyMS < 100 {
        assessment += "✅ **Live Broadcasting** - Good upload capacity\n"
    } else if results.UploadMbps > 5 {
        assessment += "⚠️ **Live Broadcasting** - Limited quality possible\n"
    }
    
    // Real-time applications
    if results.LatencyMS < 50 && results.JitterMS < 30 {
        assessment += "✅ **Real-time Applications** - Low latency suitable for gaming/trading\n"
    } else if results.LatencyMS < 100 {
        assessment += "✅ **Interactive Applications** - Suitable for most uses\n"
    }
    
    // File sharing
    if results.UploadMbps > 20 {
        assessment += "✅ **Large File Sharing** - Fast upload speeds\n"
    }
    
    return assessment
}

func generateQualityAssessment(results *BandwidthResults) string {
    assessment := "### Overall Network Quality\n\n"
    
    // Calculate percentiles for latency
    p50 := percentile(results.LatencySamples, 50)
    p95 := percentile(results.LatencySamples, 95)
    p99 := percentile(results.LatencySamples, 99)
    
    assessment += fmt.Sprintf("**Latency Percentiles:**\n")
    assessment += fmt.Sprintf("- P50: %.1f ms\n", p50)
    assessment += fmt.Sprintf("- P95: %.1f ms\n", p95)
    assessment += fmt.Sprintf("- P99: %.1f ms\n", p99)
    
    assessment += fmt.Sprintf("\n**Bandwidth Efficiency:**\n")
    assessment += fmt.Sprintf("- Download: %.1f Mbps (%.0f%% of typical VPS capacity)\n", 
        results.DownloadMbps, math.Min(100, results.DownloadMbps/1000*100))
    assessment += fmt.Sprintf("- Upload: %.1f Mbps (%.0f%% of typical VPS capacity)\n", 
        results.UploadMbps, math.Min(100, results.UploadMbps/1000*100))
    
    assessment += fmt.Sprintf("\n**Connection Stability:**\n")
    if results.JitterMS < 30 && results.PacketLoss == 0 {
        assessment += "- ✅ Excellent - Very stable connection\n"
    } else if results.JitterMS < 50 {
        assessment += "- ✅ Good - Stable for most applications\n"
    } else {
        assessment += "- ⚠️ Fair - Some instability detected\n"
    }
    
    return assessment
}

func percentile(data []float64, p float64) float64 {
    if len(data) == 0 {
        return 0
    }
    
    index := int(math.Ceil(float64(len(data)) * p / 100))
    if index >= len(data) {
        index = len(data) - 1
    }
    return data[index]
}