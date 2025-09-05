#!/usr/bin/env node

const WebSocket = require('ws');
const http = require('http');

const server = http.createServer((req, res) => {
    if (req.url === '/health') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
            status: 'healthy',
            timestamp: new Date().toISOString()
        }));
    } else {
        res.writeHead(200);
        res.end('Test WebSocket Server');
    }
});

const wss = new WebSocket.Server({ server });

const clients = new Map();
let clientIdCounter = 0;

wss.on('connection', (ws) => {
    const clientId = ++clientIdCounter;
    clients.set(clientId, ws);
    
    console.log(`Client ${clientId} connected. Total clients: ${clients.size}`);
    
    ws.on('message', (data) => {
        try {
            const message = JSON.parse(data);
            
            // Echo back frames to simulate server processing
            if (message.type === 'frame') {
                // Broadcast to all other clients (not back to sender)
                clients.forEach((client, id) => {
                    if (id !== clientId && client.readyState === WebSocket.OPEN) {
                        client.send(data);
                    }
                });
                
                // Also echo back to sender for latency measurement
                ws.send(JSON.stringify({
                    type: 'frame',
                    sentAt: message.sentAt,
                    echo: true
                }));
            } else if (message.type === 'join') {
                console.log(`Client ${clientId} joined room: ${message.room}`);
                ws.send(JSON.stringify({
                    type: 'joined',
                    room: message.room,
                    userId: message.userId
                }));
            }
        } catch (e) {
            console.error('Error parsing message:', e);
        }
    });
    
    ws.on('close', () => {
        clients.delete(clientId);
        console.log(`Client ${clientId} disconnected. Total clients: ${clients.size}`);
    });
    
    ws.on('error', (error) => {
        console.error(`Client ${clientId} error:`, error);
    });
});

const PORT = process.env.PORT || 3001;
server.listen(PORT, () => {
    console.log(`🚀 Test WebSocket Server running on port ${PORT}`);
    console.log(`   Health check: http://localhost:${PORT}/health`);
    console.log(`   WebSocket: ws://localhost:${PORT}/ws`);
});

// Graceful shutdown
process.on('SIGINT', () => {
    console.log('\n🛑 Shutting down server...');
    clients.forEach((client) => {
        client.close();
    });
    server.close(() => {
        console.log('Server closed');
        process.exit(0);
    });
});