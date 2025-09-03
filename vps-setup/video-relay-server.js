const express = require('express');
const http = require('http');
const WebSocket = require('ws');
const cors = require('cors');

const app = express();
app.use(cors());
app.use(express.json());

const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

// Store all connected clients
const clients = new Map();
const rooms = new Map();

// Serve status page
app.get('/', (req, res) => {
  const roomInfo = Array.from(rooms.entries()).map(([roomId, room]) => ({
    id: roomId,
    publishers: room.publishers.size,
    viewers: room.viewers.size
  }));
  
  res.send(`
    <!DOCTYPE html>
    <html>
    <head><title>Video Relay Server</title></head>
    <body>
      <h1>Video Relay Server</h1>
      <p>Total clients: ${clients.size}</p>
      <p>Active rooms: ${rooms.size}</p>
      <pre>${JSON.stringify(roomInfo, null, 2)}</pre>
    </body>
    </html>
  `);
});

app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    clients: clients.size,
    rooms: rooms.size,
    timestamp: new Date().toISOString()
  });
});

// Get or create room
function getRoom(roomId = 'global') {
  if (!rooms.has(roomId)) {
    rooms.set(roomId, {
      publishers: new Map(),
      viewers: new Map()
    });
  }
  return rooms.get(roomId);
}

// Handle WebSocket connections
wss.on('connection', (ws) => {
  const clientId = Math.random().toString(36).substr(2, 9);
  const client = {
    id: clientId,
    ws: ws,
    room: 'global',
    role: null,
    peerId: null
  };
  
  clients.set(clientId, client);
  console.log(`Client ${clientId} connected. Total: ${clients.size}`);
  
  // Send welcome message with client ID
  ws.send(JSON.stringify({
    type: 'welcome',
    clientId: clientId
  }));
  
  // Join default room and get current publishers
  const room = getRoom(client.room);
  const existingPublishers = Array.from(room.publishers.keys());
  
  if (existingPublishers.length > 0) {
    ws.send(JSON.stringify({
      type: 'existing-publishers',
      publishers: existingPublishers
    }));
  }
  
  ws.on('message', (data) => {
    try {
      const message = JSON.parse(data);
      handleMessage(client, message);
    } catch (e) {
      console.error('Error parsing message:', e);
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
  const room = getRoom(client.room);
  
  switch (message.type) {
    case 'join-as-publisher':
      client.role = 'publisher';
      client.peerId = message.peerId || client.id;
      room.publishers.set(client.peerId, client);
      
      console.log(`Client ${client.id} joined as publisher ${client.peerId}`);
      
      // Notify all viewers about new publisher
      broadcastToViewers(room, {
        type: 'publisher-joined',
        publisherId: client.peerId
      });
      break;
      
    case 'join-as-viewer':
      client.role = 'viewer';
      room.viewers.set(client.id, client);
      
      console.log(`Client ${client.id} joined as viewer`);
      
      // Send list of current publishers
      const publishers = Array.from(room.publishers.keys());
      client.ws.send(JSON.stringify({
        type: 'current-publishers',
        publishers: publishers
      }));
      break;
      
    case 'offer':
      // Publisher sending offer to specific viewer
      if (message.to) {
        relayToClient(message.to, {
          type: 'offer',
          from: client.peerId || client.id,
          offer: message.offer
        });
      }
      break;
      
    case 'answer':
      // Viewer sending answer back to publisher
      if (message.to) {
        relayToClient(message.to, {
          type: 'answer',
          from: client.id,
          answer: message.answer
        });
      }
      break;
      
    case 'ice-candidate':
      // Relay ICE candidates
      if (message.to) {
        relayToClient(message.to, {
          type: 'ice-candidate',
          from: client.peerId || client.id,
          candidate: message.candidate
        });
      }
      break;
      
    case 'request-offer':
      // Viewer requesting offer from specific publisher
      const publisher = room.publishers.get(message.publisherId);
      if (publisher) {
        publisher.ws.send(JSON.stringify({
          type: 'offer-requested',
          viewerId: client.id
        }));
      }
      break;
  }
}

function handleDisconnect(client) {
  const room = getRoom(client.room);
  
  if (client.role === 'publisher') {
    room.publishers.delete(client.peerId);
    
    // Notify all viewers
    broadcastToViewers(room, {
      type: 'publisher-left',
      publisherId: client.peerId
    });
    
    console.log(`Publisher ${client.peerId} disconnected`);
  } else if (client.role === 'viewer') {
    room.viewers.delete(client.id);
    console.log(`Viewer ${client.id} disconnected`);
  }
  
  clients.delete(client.id);
  
  // Clean up empty rooms
  if (room.publishers.size === 0 && room.viewers.size === 0) {
    rooms.delete(client.room);
  }
}

function broadcastToViewers(room, message) {
  const data = JSON.stringify(message);
  room.viewers.forEach(viewer => {
    if (viewer.ws.readyState === WebSocket.OPEN) {
      viewer.ws.send(data);
    }
  });
}

function relayToClient(clientId, message) {
  // Try to find client by ID or peer ID
  let targetClient = clients.get(clientId);
  
  if (!targetClient) {
    // Search for client by peerId
    for (const [id, client] of clients) {
      if (client.peerId === clientId) {
        targetClient = client;
        break;
      }
    }
  }
  
  if (targetClient && targetClient.ws.readyState === WebSocket.OPEN) {
    targetClient.ws.send(JSON.stringify(message));
  } else {
    console.log(`Client ${clientId} not found or disconnected`);
  }
}

const PORT = process.env.PORT || 3000;
server.listen(PORT, () => {
  console.log(`Video relay server running on port ${PORT}`);
});