package main

import (
    "encoding/json"
    "log"
    "net/http"
    "time"

    "github.com/gorilla/websocket"
)

var (
    BuildTime   = "unknown"
    BuildCommit = "unknown" 
    BuildBy     = "local"
    BuildRef    = "unknown"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    
    health := map[string]interface{}{
        "status": "healthy",
        "deployment": map[string]string{
            "time":       BuildTime,
            "commit":     BuildCommit,
            "deployedBy": BuildBy,
            "ref":        BuildRef,
        },
        "server": map[string]interface{}{
            "type":     "simple-conference",
            "version":  "1.0.0",
            "features": []string{"websocket", "health-endpoint"},
        },
        "timestamp": time.Now().UTC().Format(time.RFC3339),
    }
    
    json.NewEncoder(w).Encode(health)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Print("Upgrade failed: ", err)
        return
    }
    defer conn.Close()

    log.Println("Client connected")
    
    for {
        messageType, p, err := conn.ReadMessage()
        if err != nil {
            log.Println("Read failed:", err)
            return
        }
        
        // Echo back for now
        if err := conn.WriteMessage(messageType, p); err != nil {
            log.Println("Write failed:", err)
            return
        }
    }
}

func main() {
    http.HandleFunc("/health", handleHealth)
    http.HandleFunc("/info", handleHealth)
    http.HandleFunc("/ws", handleWebSocket)
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Simple Conference Server Running"))
    })
    
    log.Printf("Simple Conference Server starting on :3001")
    log.Printf("Build: %s by %s", BuildCommit, BuildBy)
    log.Fatal(http.ListenAndServe(":3001", nil))
}