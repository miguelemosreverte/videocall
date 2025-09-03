const express = require('express');
const http = require('http');
const WebSocket = require('ws');
const cors = require('cors');

const app = express();
app.use(cors());
app.use(express.json({limit: '50mb'}));

const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

// Store active streams
const streams = new Map();
const clients = new Map();

// Serve status page
app.get('/', (req, res) => {
  res.send(`
    <!DOCTYPE html>
    <html>
    <head><title>Media Relay Server</title></head>
    <body>
      <h1>Media Relay Server</h1>
      <p>Active streams: ${streams.size}</p>
      <p>Connected clients: ${clients.size}</p>
      <p>WebSocket endpoint: ws://${req.headers.host}/ws</p>
    </body>
    </html>
  `);
});

// Health check
app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    streams: streams.size,
    clients: clients.size,
    timestamp: new Date().toISOString()
  });
});

// WebSocket connection handler
wss.on('connection', (ws, req) => {
  const clientId = Math.random().toString(36).substr(2, 9);
  
  const client = {
    id: clientId,
    ws: ws,
    isPublisher: false,
    room: 'global' // Everyone in same room
  };
  
  clients.set(clientId, client);
  console.log(`Client ${clientId} connected. Total clients: ${clients.size}`);
  
  // Send welcome message
  ws.send(JSON.stringify({
    type: 'welcome',
    clientId: clientId,
    room: client.room
  }));
  
  // Notify about existing publishers
  const publishers = Array.from(clients.values())
    .filter(c => c.isPublisher && c.id !== clientId)
    .map(c => c.id);
  
  if (publishers.length > 0) {
    ws.send(JSON.stringify({
      type: 'publishers',
      publishers: publishers
    }));
  }
  
  ws.on('message', (data) => {
    try {
      const message = JSON.parse(data);
      
      switch (message.type) {
        case 'publish':
          // Client wants to publish video
          client.isPublisher = true;
          console.log(`Client ${clientId} started publishing`);
          
          // Notify all other clients about new publisher
          broadcast({
            type: 'new-publisher',
            publisherId: clientId
          }, clientId);
          break;
          
        case 'frame':
          // Client is sending a video frame
          if (client.isPublisher) {
            // Relay frame to all other clients in the room
            broadcast({
              type: 'frame',
              publisherId: clientId,
              data: message.data,
              timestamp: message.timestamp
            }, clientId);
          }
          break;
          
        case 'stop-publish':
          // Client stopped publishing
          client.isPublisher = false;
          console.log(`Client ${clientId} stopped publishing`);
          
          // Notify others
          broadcast({
            type: 'publisher-left',
            publisherId: clientId
          }, clientId);
          break;
          
        case 'subscribe':
          // Client wants to receive frames from a specific publisher
          console.log(`Client ${clientId} subscribing to ${message.publisherId}`);
          break;
      }
    } catch (e) {
      console.error('Error processing message:', e);
    }
  });
  
  ws.on('close', () => {
    console.log(`Client ${clientId} disconnected`);
    
    // If was publisher, notify others
    if (client.isPublisher) {
      broadcast({
        type: 'publisher-left',
        publisherId: clientId
      }, clientId);
    }
    
    clients.delete(clientId);
  });
  
  ws.on('error', (error) => {
    console.error(`Client ${clientId} error:`, error);
  });
});

// Broadcast message to all clients except sender
function broadcast(message, excludeId = null) {
  const data = JSON.stringify(message);
  
  clients.forEach((client) => {
    if (client.id !== excludeId && client.ws.readyState === WebSocket.OPEN) {
      client.ws.send(data);
    }
  });
}

const PORT = process.env.PORT || 3000;
server.listen(PORT, () => {
  console.log(`Media relay server running on port ${PORT}`);
});