package main

import (
    "crypto/tls"
    "log"
    "net/http"
    "net/http/httputil"
    "net/url"
    "strings"

    "golang.org/x/crypto/acme/autocert"
    "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
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
    
    // Autocert manager
    m := &autocert.Manager{
        Cache:      autocert.DirCache("/root/certs"),
        Prompt:     autocert.AcceptTOS,
        HostPolicy: autocert.HostWhitelist("194.87.103.57.nip.io"),
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
    
    log.Println("Starting WebP Conference SSL server on :443")
    log.Fatal(server.ListenAndServeTLS("", ""))
}