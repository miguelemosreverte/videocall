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
  const roomList = Array.from(rooms.entries()).map(([name, members]) => 
    `${name}: ${members.size} members`
  ).join(', ');
  
  res.send(`
    <!DOCTYPE html>
    <html>
    <head><title>Signaling Server</title></head>
    <body>
      <h1>WebRTC Signaling Server</h1>
      <p>Connected clients: ${clients.size}</p>
      <p>Rooms: ${roomList || 'None'}</p>
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
  
  // Send welcome
  ws.send(JSON.stringify({
    type: 'welcome',
    id: clientId
  }));
  
  ws.on('message', (data) => {
    try {
      const message = JSON.parse(data);
      handleMessage(client, message);
    } catch (e) {
      console.error('Error:', e);
    }
  });
  
  ws.on('close', () => {
    console.log(`Client ${client.id} disconnected`);
    leaveRoom(client);
    clients.delete(client.id);
  });
});

function handleMessage(client, message) {
  console.log(`[${client.id}] ${message.type}`, message.to ? `to ${message.to}` : '');
  
  switch (message.type) {
    case 'join':
      joinRoom(client, message.room || 'global');
      break;
      
    case 'offer':
    case 'answer':
    case 'ice-candidate':
      relayToPeer(client.id, message);
      break;
  }
}

function joinRoom(client, roomName) {
  // Leave previous room
  if (client.room) {
    leaveRoom(client);
  }
  
  client.room = roomName;
  
  if (!rooms.has(roomName)) {
    rooms.set(roomName, new Set());
  }
  
  const room = rooms.get(roomName);
  
  // Get existing members before adding new one
  const existingMembers = Array.from(room);
  
  room.add(client.id);
  console.log(`Client ${client.id} joined room ${roomName}. Members: ${room.size}`);
  
  // Send existing peers to new member
  if (existingMembers.length > 0) {
    client.ws.send(JSON.stringify({
      type: 'peers',
      peers: existingMembers
    }));
  }
  
  // Notify existing members about new peer
  existingMembers.forEach(memberId => {
    const member = clients.get(memberId);
    if (member && member.ws.readyState === WebSocket.OPEN) {
      member.ws.send(JSON.stringify({
        type: 'peer-joined',
        peerId: client.id
      }));
    }
  });
}

function leaveRoom(client) {
  if (!client.room) return;
  
  const room = rooms.get(client.room);
  if (!room) return;
  
  room.delete(client.id);
  
  // Notify others
  room.forEach(memberId => {
    const member = clients.get(memberId);
    if (member && member.ws.readyState === WebSocket.OPEN) {
      member.ws.send(JSON.stringify({
        type: 'peer-left',
        peerId: client.id
      }));
    }
  });
  
  // Clean up empty rooms
  if (room.size === 0) {
    rooms.delete(client.room);
  }
  
  client.room = null;
}

function relayToPeer(fromId, message) {
  const targetId = message.to;
  const target = clients.get(targetId);
  
  if (target && target.ws.readyState === WebSocket.OPEN) {
    message.from = fromId;
    delete message.to;
    target.ws.send(JSON.stringify(message));
  }
}

const PORT = 3000;
server.listen(PORT, () => {
  console.log(`Signaling server on port ${PORT}`);
});