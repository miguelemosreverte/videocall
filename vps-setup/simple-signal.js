const express = require('express');
const http = require('http');
const WebSocket = require('ws');
const cors = require('cors');

const app = express();
app.use(cors());

const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

// Track connected clients
const clients = new Map();
const rooms = new Map();

app.get('/', (req, res) => {
  res.send(`
    <!DOCTYPE html>
    <html>
    <head><title>Signaling Server</title></head>
    <body>
      <h1>WebRTC Signaling Server</h1>
      <p>Connected clients: ${clients.size}</p>
      <p>Active rooms: ${rooms.size}</p>
      <p>WebSocket endpoint: ws://${req.headers.host}/ws</p>
    </body>
    </html>
  `);
});

app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    clients: clients.size,
    rooms: rooms.size
  });
});

wss.on('connection', (ws) => {
  const clientId = Math.random().toString(36).substr(2, 9);
  const client = {
    id: clientId,
    ws: ws,
    room: null
  };
  
  clients.set(clientId, client);
  console.log(`Client ${clientId} connected. Total: ${clients.size}`);
  
  // Send welcome with ID
  ws.send(JSON.stringify({
    type: 'welcome',
    id: clientId
  }));
  
  ws.on('message', (data) => {
    try {
      const message = JSON.parse(data);
      handleMessage(client, message);
    } catch (e) {
      console.error('Error handling message:', e);
    }
  });
  
  ws.on('close', () => {
    handleDisconnect(client);
  });
  
  ws.on('error', (error) => {
    console.error(`Client ${clientId} error:`, error);
  });
});

function handleMessage(client, message) {
  switch (message.type) {
    case 'join':
      // Join a room
      const roomName = message.room || 'global';
      joinRoom(client, roomName);
      break;
      
    case 'ready':
      // Client is ready to receive calls
      notifyRoomMembers(client);
      break;
      
    case 'offer':
    case 'answer':
    case 'ice-candidate':
      // Relay WebRTC signaling
      relayMessage(client, message);
      break;
      
    default:
      console.log('Unknown message type:', message.type);
  }
}

function joinRoom(client, roomName) {
  // Leave current room if any
  if (client.room) {
    leaveRoom(client);
  }
  
  // Join new room
  client.room = roomName;
  
  if (!rooms.has(roomName)) {
    rooms.set(roomName, new Set());
  }
  
  const room = rooms.get(roomName);
  room.add(client.id);
  
  console.log(`Client ${client.id} joined room ${roomName}. Room size: ${room.size}`);
  
  // Send list of existing peers in room
  const peers = Array.from(room).filter(id => id !== client.id);
  client.ws.send(JSON.stringify({
    type: 'peers',
    peers: peers
  }));
}

function notifyRoomMembers(client) {
  if (!client.room) return;
  
  const room = rooms.get(client.room);
  if (!room) return;
  
  // Notify all other members about new peer
  room.forEach(peerId => {
    if (peerId !== client.id) {
      const peer = clients.get(peerId);
      if (peer && peer.ws.readyState === WebSocket.OPEN) {
        peer.ws.send(JSON.stringify({
          type: 'peer-joined',
          peerId: client.id
        }));
      }
    }
  });
}

function relayMessage(client, message) {
  const targetId = message.to;
  const target = clients.get(targetId);
  
  if (target && target.ws.readyState === WebSocket.OPEN) {
    // Add sender info and relay
    message.from = client.id;
    delete message.to; // Remove 'to' field before sending
    target.ws.send(JSON.stringify(message));
  } else {
    console.log(`Target ${targetId} not found or disconnected`);
  }
}

function leaveRoom(client) {
  if (!client.room) return;
  
  const room = rooms.get(client.room);
  if (room) {
    room.delete(client.id);
    
    // Notify other room members
    room.forEach(peerId => {
      const peer = clients.get(peerId);
      if (peer && peer.ws.readyState === WebSocket.OPEN) {
        peer.ws.send(JSON.stringify({
          type: 'peer-left',
          peerId: client.id
        }));
      }
    });
    
    // Remove empty rooms
    if (room.size === 0) {
      rooms.delete(client.room);
    }
  }
  
  client.room = null;
}

function handleDisconnect(client) {
  console.log(`Client ${client.id} disconnected`);
  
  // Leave room
  leaveRoom(client);
  
  // Remove from clients
  clients.delete(client.id);
}

const PORT = process.env.PORT || 3000;
server.listen(PORT, () => {
  console.log(`Signaling server running on port ${PORT}`);
});