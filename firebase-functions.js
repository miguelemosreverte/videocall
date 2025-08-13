// Firebase Signaling Functions Replacement

// Replace sendToPeer
async function sendToPeer(targetUuid, type, data) {
  try {
    const message = {
      type,
      data,
      from: myUuid,
      timestamp: Date.now()
    };
    
    // Push to target's message queue
    await database.ref(`messages/${targetUuid}`).push(message);
    console.log(`Sent ${type} to ${targetUuid} via Firebase`);
    return true;
  } catch (error) {
    console.error('SendToPeer error:', error);
    return false;
  }
}

// Replace registerPeer
async function registerPeer(uuid) {
  try {
    const now = Date.now();
    
    // Register in Firebase
    await database.ref(`peers/${uuid}`).set({
      timestamp: now,
      name: uuid
    });
    
    // Clean up stale peers (older than 1 minute)
    const peersSnapshot = await database.ref('peers').once('value');
    const allPeers = peersSnapshot.val() || {};
    
    for (const [peerId, peerData] of Object.entries(allPeers)) {
      if (peerId !== uuid && now - peerData.timestamp > 60000) {
        // Remove stale peer
        await database.ref(`peers/${peerId}`).remove();
        console.log(`Cleaned up stale peer: ${peerId}`);
      }
    }
    
    // Get list of active peers
    const activePeersSnapshot = await database.ref('peers').once('value');
    const activePeers = activePeersSnapshot.val() || {};
    const existingPeers = Object.keys(activePeers).filter(p => p !== uuid);
    
    console.log('Registered with Firebase as:', uuid);
    console.log('Active peers in room:', existingPeers);
    myRegistrationTime = now;
    
    return { uuid, registered: true, existingPeers, registrationTime: now };
  } catch (error) {
    console.log('Registration error:', error);
    return { uuid, registered: false };
  }
}

// Replace pollForSignaling with Firebase listeners
async function setupFirebaseListeners() {
  // Listen for messages addressed to us
  const messageRef = database.ref(`messages/${myUuid}`);
  
  messageRef.on('child_added', async (snapshot) => {
    const message = snapshot.val();
    console.log(`Received ${message.type} from ${message.from}`);
    
    // Process the message
    await handleSignalMessage(message);
    
    // Remove processed message
    snapshot.ref.remove();
  });
}

// Update cleanup on exit
window.addEventListener('beforeunload', async () => {
  if (myUuid) {
    try {
      // Remove from Firebase
      await database.ref(`peers/${myUuid}`).remove();
      await database.ref(`messages/${myUuid}`).remove();
    } catch (error) {
      console.error('Cleanup error:', error);
    }
  }
  // Close peer connections
  peers.forEach(pc => pc.close());
  if (localStream) {
    localStream.getTracks().forEach(track => track.stop());
  }
});