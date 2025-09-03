const express = require('express');
const http = require('http');
const WebSocket = require('ws');
const path = require('path');

const app = express();
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

// Serve static files
app.use(express.static('.'));

// Conference rooms
const rooms = new Map();

class Room {
  constructor(name) {
    this.name = name;
    this.participants = new Map();
  }

  addParticipant(id, ws) {
    this.participants.set(id, {
      id: id,
      ws: ws,
      joinedAt: Date.now()
    });
    
    // Send welcome message with current participants
    const welcomeMsg = {
      type: 'welcome',
      yourId: id,
      room: this.name,
      participants: Array.from(this.participants.keys()).filter(pid => pid !== id)
    };
    ws.send(JSON.stringify(welcomeMsg));
    
    // Notify others about new participant
    this.broadcast({
      type: 'participant-joined',
      participantId: id,
      timestamp: Date.now()
    }, id);
    
    console.log(`[${this.name}] ${id} joined. Total: ${this.participants.size}`);
  }

  removeParticipant(id) {
    if (this.participants.has(id)) {
      this.participants.delete(id);
      
      // Notify others about participant leaving
      this.broadcast({
        type: 'participant-left',
        participantId: id,
        timestamp: Date.now()
      }, id);
      
      console.log(`[${this.name}] ${id} left. Total: ${this.participants.size}`);
    }
    
    return this.participants.size;
  }

  broadcast(message, excludeId = null) {
    const data = typeof message === 'string' ? message : JSON.stringify(message);
    
    this.participants.forEach((participant, id) => {
      if (id !== excludeId && participant.ws.readyState === WebSocket.OPEN) {
        participant.ws.send(data);
      }
    });
  }

  relay(fromId, message) {
    // Add sender info to message
    const relayMessage = {
      ...message,
      from: fromId,
      timestamp: Date.now()
    };
    
    // Send to all other participants
    this.broadcast(relayMessage, fromId);
  }
}

// WebSocket connection handler
wss.on('connection', (ws) => {
  let clientId = null;
  let currentRoom = null;
  
  console.log('New WebSocket connection');
  
  ws.on('message', (data) => {
    try {
      const message = JSON.parse(data.toString());
      
      switch (message.type) {
        case 'join':
          // Handle room join
          clientId = message.id || `user-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
          const roomName = message.room || 'default';
          
          // Get or create room
          if (!rooms.has(roomName)) {
            rooms.set(roomName, new Room(roomName));
          }
          currentRoom = rooms.get(roomName);
          
          // Add participant to room
          currentRoom.addParticipant(clientId, ws);
          break;
          
        case 'video-frame':
        case 'audio-chunk':
          // Relay media to other participants
          if (currentRoom && clientId) {
            currentRoom.relay(clientId, message);
          }
          break;
          
        case 'ice-candidate':
        case 'offer':
        case 'answer':
          // Relay WebRTC signaling (if we add it later)
          if (currentRoom && clientId) {
            currentRoom.relay(clientId, message);
          }
          break;
          
        default:
          console.log(`Unknown message type: ${message.type}`);
      }
    } catch (e) {
      console.error('Error processing message:', e);
    }
  });
  
  ws.on('close', () => {
    if (currentRoom && clientId) {
      const remaining = currentRoom.removeParticipant(clientId);
      
      // Clean up empty rooms
      if (remaining === 0) {
        rooms.delete(currentRoom.name);
        console.log(`Room ${currentRoom.name} deleted (empty)`);
      }
    }
  });
  
  ws.on('error', (error) => {
    console.error('WebSocket error:', error);
  });
});

// Status endpoint
app.get('/status', (req, res) => {
  const status = {
    rooms: Array.from(rooms.entries()).map(([name, room]) => ({
      name: name,
      participants: room.participants.size,
      ids: Array.from(room.participants.keys())
    }))
  };
  res.json(status);
});

const PORT = process.env.PORT || 3000;
server.listen(PORT, () => {
  console.log(`Conference server running on port ${PORT}`);
  console.log(`Open http://localhost:${PORT}/conference-client.html in multiple tabs to test`);
});