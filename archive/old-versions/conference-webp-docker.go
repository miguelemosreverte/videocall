package main

import (
    "crypto/tls"
    "log"
    "net/http"
    "net/http/httputil"
    "net/url"
    "os"
    "os/exec"
    "strings"

    "golang.org/x/crypto/acme/autocert"
    "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func main() {
    // Configuration from environment
    domain := getEnv("DOMAIN", "localhost")
    useSSL := getEnv("USE_SSL", "false") == "true"
    certEmail := getEnv("CERT_EMAIL", "admin@example.com")
    
    log.Printf("Starting WebP Conference Server")
    log.Printf("Domain: %s, SSL: %v", domain, useSSL)
    
    // Start the WebP server in the background
    go startWebPServer()
    
    // Backend WebP server
    backend, _ := url.Parse("http://localhost:3001")
    
    // Create reverse proxy
    proxy := httputil.NewSingleHostReverseProxy(backend)
    
    // WebSocket handler
    wsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.HasPrefix(r.URL.Path, "/ws") {
            // For WebSocket, we need to handle the upgrade ourselves
            backendConn, _, err := websocket.DefaultDialer.Dial("ws://localhost:3001"+r.URL.Path, nil)
            if err != nil {
                http.Error(w, "Backend unavailable", 502)
                return
            }
            defer backendConn.Close()
            
            // Upgrade client connection
            clientConn, err := upgrader.Upgrade(w, r, nil)
            if err != nil {
                return
            }
            defer clientConn.Close()
            
            // Proxy WebSocket messages
            go func() {
                for {
                    messageType, data, err := clientConn.ReadMessage()
                    if err != nil {
                        break
                    }
                    if err := backendConn.WriteMessage(messageType, data); err != nil {
                        break
                    }
                }
            }()
            
            for {
                messageType, data, err := backendConn.ReadMessage()
                if err != nil {
                    break
                }
                if err := clientConn.WriteMessage(messageType, data); err != nil {
                    break
                }
            }
        } else {
            // Regular HTTP proxy
            proxy.ServeHTTP(w, r)
        }
    })
    
    if useSSL && domain != "localhost" {
        // Production with Let's Encrypt
        log.Println("Starting with Let's Encrypt SSL")
        
        // Autocert manager
        m := &autocert.Manager{
            Cache:      autocert.DirCache("/app/certs"),
            Prompt:     autocert.AcceptTOS,
            HostPolicy: autocert.HostWhitelist(domain),
            Email:      certEmail,
        }
        
        // HTTPS server
        server := &http.Server{
            Addr:      ":443",
            Handler:   wsHandler,
            TLSConfig: &tls.Config{
                GetCertificate: m.GetCertificate,
            },
        }
        
        // Start HTTP server for ACME challenges
        go http.ListenAndServe(":80", m.HTTPHandler(nil))
        
        log.Fatal(server.ListenAndServeTLS("", ""))
    } else {
        // Development or localhost
        log.Println("Starting without SSL (development mode)")
        log.Fatal(http.ListenAndServe(":80", wsHandler))
    }
}

// Start the actual WebP conference server
func startWebPServer() {
    log.Println("Starting WebP server on :3001")
    
    // Import the conference-webp.go logic here
    // For Docker, we'll use exec to run the compiled binary
    cmd := exec.Command("./conference-webp")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    if err := cmd.Run(); err != nil {
        log.Fatalf("WebP server failed: %v", err)
    }
}