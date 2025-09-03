package main

import (
    "crypto/tls"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "sync"
    "time"
    "golang.org/x/crypto/acme/autocert"
    "github.com/gorilla/websocket"
)

type Message struct {
    Type        string          `json:"type"`
    ID          string          `json:"id,omitempty"`
    Room        string          `json:"room,omitempty"`
    From        string          `json:"from,omitempty"`
    Data        string          `json:"data,omitempty"`
    Timestamp   int64           `json:"timestamp,omitempty"`
    YourId      string          `json:"yourId,omitempty"`
    Participants []string       `json:"participants,omitempty"`
    ParticipantId string        `json:"participantId,omitempty"`
}

type Client struct {
    ID   string
    Room string
    Conn *websocket.Conn
    Send chan []byte
    Hub  *Hub
}

type Room struct {
    ID      string
    Clients map[string]*Client
    mu      sync.RWMutex
}

type Hub struct {
    Rooms      map[string]*Room
    Register   chan *Client
    Unregister chan *Client
    mu         sync.RWMutex
}

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

func NewHub() *Hub {
    return &Hub{
        Rooms:      make(map[string]*Room),
        Register:   make(chan *Client),
        Unregister: make(chan *Client),
    }
}

func (h *Hub) Run() {
    for {
        select {
        case client := <-h.Register:
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
            participants := make([]string, 0)
            for id := range room.Clients {
                if id != client.ID {
                    participants = append(participants, id)
                }
            }
            room.mu.Unlock()

            welcome := Message{
                Type:         "welcome",
                YourId:       client.ID,
                Participants: participants,
            }
            
            if welcomeData, err := json.Marshal(welcome); err == nil {
                select {
                case client.Send <- welcomeData:
                default:
                }
            }

            room.mu.RLock()
            joinMsg := Message{
                Type:          "participant-joined",
                ParticipantId: client.ID,
            }
            if joinData, err := json.Marshal(joinMsg); err == nil {
                for _, c := range room.Clients {
                    if c.ID != client.ID {
                        select {
                        case c.Send <- joinData:
                        default:
                        }
                    }
                }
            }
            room.mu.RUnlock()

        case client := <-h.Unregister:
            h.mu.RLock()
            room := h.Rooms[client.Room]
            h.mu.RUnlock()
            
            if room != nil {
                room.mu.Lock()
                if _, ok := room.Clients[client.ID]; ok {
                    delete(room.Clients, client.ID)
                    close(client.Send)
                    
                    leaveMsg := Message{
                        Type:          "participant-left",
                        ParticipantId: client.ID,
                    }
                    if leaveData, err := json.Marshal(leaveMsg); err == nil {
                        for _, c := range room.Clients {
                            select {
                            case c.Send <- leaveData:
                            default:
                            }
                        }
                    }
                }
                room.mu.Unlock()
            }
        }
    }
}

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
    
    for {
        _, message, err := c.Conn.ReadMessage()
        if err != nil {
            break
        }
        
        var msg Message
        if err := json.Unmarshal(message, &msg); err != nil {
            continue
        }
        
        switch msg.Type {
        case "video-frame", "audio-chunk":
            msg.From = c.ID
            c.Hub.mu.RLock()
            room := c.Hub.Rooms[c.Room]
            c.Hub.mu.RUnlock()
            
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
            
            w, err := c.Conn.NextWriter(websocket.TextMessage)
            if err != nil {
                return
            }
            w.Write(message)
            
            n := len(c.Send)
            for i := 0; i < n; i++ {
                <-c.Send
            }
            
            if err := w.Close(); err != nil {
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

func handleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    
    _, message, err := conn.ReadMessage()
    if err != nil {
        conn.Close()
        return
    }
    
    var msg Message
    if err := json.Unmarshal(message, &msg); err != nil {
        conn.Close()
        return
    }
    
    if msg.Type != "join" {
        conn.Close()
        return
    }
    
    client := &Client{
        ID:   msg.ID,
        Room: msg.Room,
        Conn: conn,
        Send: make(chan []byte, 256),
        Hub:  hub,
    }
    
    client.Hub.Register <- client
    
    go client.WritePump()
    go client.ReadPump()
}

func main() {
    domain := os.Getenv("DOMAIN")
    if domain == "" {
        domain = "194.87.103.57.nip.io"
    }
    
    hub := NewHub()
    go hub.Run()
    
    mux := http.NewServeMux()
    
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html")
        fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Conference Server</title>
</head>
<body>
    <h1>🎥 Conference Server Active</h1>
    <p>WebSocket: wss://%s/ws</p>
</body>
</html>`, domain)
    })
    
    mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        handleWebSocket(hub, w, r)
    })
    
    // Autocert manager for Let's Encrypt
    certManager := autocert.Manager{
        Prompt:     autocert.AcceptTOS,
        HostPolicy: autocert.HostWhitelist(domain),
        Cache:      autocert.DirCache("/root/.cache/certs"),
    }
    
    server := &http.Server{
        Addr:    ":443",
        Handler: mux,
        TLSConfig: &tls.Config{
            GetCertificate: certManager.GetCertificate,
            MinVersion:     tls.VersionTLS12,
        },
    }
    
    // HTTP server for ACME challenges
    go http.ListenAndServe(":80", certManager.HTTPHandler(nil))
    
    log.Printf("Starting HTTPS server on :443 for domain %s", domain)
    log.Fatal(server.ListenAndServeTLS("", ""))
}
