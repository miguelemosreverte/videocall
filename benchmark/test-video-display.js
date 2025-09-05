#!/usr/bin/env node

const WebSocket = require('ws');

// Simulate two clients connecting
async function testVideoDisplay() {
    console.log('🔍 VIDEO DISPLAY TEST');
    console.log('====================\n');
    
    const wsUrl = 'wss://95.217.238.72.nip.io/ws';
    
    // Client 1 - Sender
    const client1 = new WebSocket(wsUrl, { rejectUnauthorized: false });
    const client1Id = 'test-sender-' + Math.random().toString(36).substr(2, 9);
    
    // Client 2 - Receiver
    const client2 = new WebSocket(wsUrl, { rejectUnauthorized: false });
    const client2Id = 'test-receiver-' + Math.random().toString(36).substr(2, 9);
    
    let framesReceived = 0;
    let framesSent = 0;
    
    // Setup Client 1 (sender)
    client1.on('open', () => {
        console.log('✅ Client 1 (Sender) connected');
        
        // Join global room
        client1.send(JSON.stringify({
            type: 'join',
            room: 'global',
            userId: client1Id
        }));
        
        // Start sending frames after a delay
        setTimeout(() => {
            console.log('\n📤 Client 1 starting to send frames...');
            
            // Send a test frame every 100ms
            const sendInterval = setInterval(() => {
                const frame = {
                    t: 'delta',  // frame type
                    d: [[0, 0, 100, 100, '#ff0000']],  // dummy quad-tree data
                    ts: Date.now(),
                    q: 'high',
                    a: null  // no audio for simplicity
                };
                
                client1.send(JSON.stringify(frame));
                framesSent++;
                
                if (framesSent >= 10) {
                    clearInterval(sendInterval);
                    console.log(`\n📊 Sent ${framesSent} frames`);
                }
            }, 100);
        }, 1000);
    });
    
    // Setup Client 2 (receiver)
    client2.on('open', () => {
        console.log('✅ Client 2 (Receiver) connected');
        
        // Join global room
        client2.send(JSON.stringify({
            type: 'join',
            room: 'global',
            userId: client2Id
        }));
    });
    
    client2.on('message', (data) => {
        try {
            const packet = JSON.parse(data.toString());
            
            // Check if it's a video frame
            if (packet.t === 'delta' || packet.t === 'key') {
                framesReceived++;
                console.log(`📥 Client 2 received frame from ${packet.from || packet.userId || 'unknown'}`);
                
                // Verify it's not from self
                if (packet.from === client2Id || packet.userId === client2Id) {
                    console.log('❌ ERROR: Received own frame (echo)!');
                } else {
                    console.log('✅ Frame is from another user');
                }
                
                // Check frame structure
                if (!packet.d) {
                    console.log('❌ ERROR: Frame missing data (d) field!');
                }
                if (!packet.t) {
                    console.log('❌ ERROR: Frame missing type (t) field!');
                }
            } else if (packet.type === 'joined') {
                console.log(`📋 Client 2 joined room: ${packet.room}`);
            }
        } catch (e) {
            console.error('Error parsing message:', e);
        }
    });
    
    // Wait for test to complete
    await new Promise(resolve => setTimeout(resolve, 3000));
    
    // Results
    console.log('\n' + '='.repeat(50));
    console.log('RESULTS:');
    console.log(`Frames sent by Client 1: ${framesSent}`);
    console.log(`Frames received by Client 2: ${framesReceived}`);
    
    if (framesReceived === 0) {
        console.log('\n❌ CRITICAL: No frames received by Client 2!');
        console.log('   This means the server is NOT broadcasting to other clients.');
    } else if (framesReceived < framesSent) {
        console.log(`\n⚠️  WARNING: Only ${framesReceived}/${framesSent} frames received`);
    } else {
        console.log('\n✅ All frames received successfully!');
    }
    
    // Check what fields are in received frames
    if (framesReceived > 0) {
        console.log('\n📋 Checking frame structure on client side...');
        
        // Send one more frame and capture its structure
        const testFrame = {
            t: 'delta',
            d: [[0, 0, 100, 100, '#ff0000']],
            ts: Date.now(),
            q: 'high'
        };
        
        client2.once('message', (data) => {
            const packet = JSON.parse(data.toString());
            if (packet.t) {
                console.log('Received frame structure:', Object.keys(packet));
                console.log('Frame details:', JSON.stringify(packet, null, 2));
            }
        });
        
        client1.send(JSON.stringify(testFrame));
        await new Promise(resolve => setTimeout(resolve, 500));
    }
    
    client1.close();
    client2.close();
    
    console.log('='.repeat(50));
}

testVideoDisplay().catch(console.error);