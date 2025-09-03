const express = require('express');
const http = require('http');
const WebSocket = require('ws');
const cors = require('cors');

const app = express();
app.use(cors());

const server = http.createServer(app);
const wss = new WebSocket.Server({ 
  server,
  maxPayload: 10 * 1024 * 1024 // 10MB max for video frames
});

// Store connected clients by room
const rooms = new Map();

app.get('/', (req, res) => {
  const roomList = Array.from(rooms.entries()).map(([name, clients]) => ({
    room: name,
    participants: clients.size
  }));
  
  res.send(`
    <!DOCTYPE html>
    <html>
    <head><title>WebSocket Video Relay</title></head>
    <body>
      <h1>Video Relay Server</h1>
      <p>Active rooms: ${rooms.size}</p>
      <pre>${JSON.stringify(roomList, null, 2)}</pre>
    </body>
    </html>
  `);
});

app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    rooms: rooms.size,
    clients: Array.from(rooms.values()).reduce((sum, r) => sum + r.size, 0)
  });
});

wss.on('connection', (ws) => {
  console.log('New WebSocket connection');
  let clientId = null;
  let roomName = null;
  
  ws.on('message', (data) => {
    try {
      // Convert Buffer to string if needed
      const messageStr = data.toString();
      
      // Try to parse as JSON
      try {
        const message = JSON.parse(messageStr);
        console.log('Received message type:', message.type);
        handleMessage(message);
      } catch (jsonError) {
        // Not JSON, must be binary frame
        if (roomName && clientId) {
          relayVideoFrame(roomName, clientId, data);
        }
      }
    } catch (e) {
      console.error('Error handling message:', e);
    }
  });
  
  function handleMessage(message) {
    switch (message.type) {
      case 'join':
        clientId = message.id || Math.random().toString(36).substr(2, 9);
        roomName = message.room || 'global';
        
        // Add to room
        if (!rooms.has(roomName)) {
          rooms.set(roomName, new Map());
        }
        const room = rooms.get(roomName);
        
        // Store client info
        room.set(clientId, {
          ws: ws,
          id: clientId
        });
        
        console.log(`Client ${clientId} joined room ${roomName}. Room size: ${room.size}`);
        
        // Send welcome with list of other participants
        const others = Array.from(room.keys()).filter(id => id !== clientId);
        ws.send(JSON.stringify({
          type: 'welcome',
          id: clientId,
          room: roomName,
          participants: others
        }));
        
        // Notify others about new participant
        broadcastToRoom(roomName, {
          type: 'participant-joined',
          id: clientId
        }, clientId);
        break;
        
      case 'video-frame':
        // Text-based frame data (base64)
        if (roomName && clientId && message.data) {
          broadcastToRoom(roomName, {
            type: 'video-frame',
            from: clientId,
            data: message.data,
            timestamp: message.timestamp
          }, clientId);
        }
        break;
    }
  }
  
  function relayVideoFrame(roomName, senderId, frameData) {
    const room = rooms.get(roomName);
    if (!room) return;
    
    // Relay binary frame to all others in room
    room.forEach((client, id) => {
      if (id !== senderId && client.ws.readyState === WebSocket.OPEN) {
        // Send metadata first
        client.ws.send(JSON.stringify({
          type: 'incoming-frame',
          from: senderId,
          size: frameData.length
        }));
        // Then send binary frame
        client.ws.send(frameData);
      }
    });
  }
  
  function broadcastToRoom(roomName, message, excludeId) {
    const room = rooms.get(roomName);
    if (!room) return;
    
    const data = JSON.stringify(message);
    room.forEach((client, id) => {
      if (id !== excludeId && client.ws.readyState === WebSocket.OPEN) {
        client.ws.send(data);
      }
    });
  }
  
  ws.on('close', () => {
    if (clientId && roomName) {
      const room = rooms.get(roomName);
      if (room) {
        room.delete(clientId);
        console.log(`Client ${clientId} left room ${roomName}`);
        
        // Notify others
        broadcastToRoom(roomName, {
          type: 'participant-left',
          id: clientId
        }, clientId);
        
        // Clean up empty rooms
        if (room.size === 0) {
          rooms.delete(roomName);
        }
      }
    }
  });
});

const PORT = 3000;
server.listen(PORT, () => {
  console.log(`WebSocket Video Relay running on port ${PORT}`);
});