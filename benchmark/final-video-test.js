#!/usr/bin/env node

const WebSocket = require('ws');

async function testVideoDisplayFix() {
    console.log('🎯 FINAL VIDEO DISPLAY TEST');
    console.log('===========================\n');
    
    const wsUrl = 'wss://95.217.238.72.nip.io/ws';
    
    // Create two test clients
    const sender = new WebSocket(wsUrl, { rejectUnauthorized: false });
    const receiver = new WebSocket(wsUrl, { rejectUnauthorized: false });
    
    const senderId = 'final-sender-' + Math.random().toString(36).substr(2, 9);
    const receiverId = 'final-receiver-' + Math.random().toString(36).substr(2, 9);
    
    let framesSent = 0;
    let framesReceived = 0;
    let hasDataField = false;
    
    // Setup sender
    sender.on('open', () => {
        console.log('✅ Sender connected');
        
        // Join room
        sender.send(JSON.stringify({
            type: 'join',
            room: 'global',
            userId: senderId
        }));
        
        // Send test frames with 'd' field (matching the fixed format)
        setTimeout(() => {
            console.log('\n📤 Sending test frames with correct format...');
            
            for (let i = 0; i < 5; i++) {
                const frame = {
                    t: i === 0 ? 'key' : 'delta',
                    d: i === 0 
                        ? btoa('fake-jpeg-data')  // Key frame
                        : [[10, 10, 50, 50, '#ff0000'], [60, 60, 40, 40, '#00ff00']],  // Delta regions
                    ts: Date.now(),
                    q: 'high',
                    f: i + 1
                };
                
                sender.send(JSON.stringify(frame));
                framesSent++;
                console.log(`  Sent frame ${i+1} (${frame.t})`);
            }
        }, 500);
    });
    
    // Setup receiver
    receiver.on('open', () => {
        console.log('✅ Receiver connected');
        
        // Join room
        receiver.send(JSON.stringify({
            type: 'join',
            room: 'global',
            userId: receiverId
        }));
    });
    
    receiver.on('message', (data) => {
        try {
            const packet = JSON.parse(data.toString());
            
            // Check for video frames
            if (packet.t === 'delta' || packet.t === 'key') {
                // Ignore self frames
                if (packet.from === receiverId || packet.userId === receiverId) {
                    return;
                }
                
                framesReceived++;
                
                // Check if it has the 'd' field
                if (packet.d !== undefined) {
                    hasDataField = true;
                    console.log(`📥 Received ${packet.t} frame from ${packet.from || packet.userId} WITH 'd' field ✅`);
                } else {
                    console.log(`❌ Received frame from ${packet.from || packet.userId} WITHOUT 'd' field`);
                }
            }
        } catch (e) {
            // Ignore parse errors
        }
    });
    
    // Wait for results
    await new Promise(resolve => setTimeout(resolve, 3000));
    
    // Results
    console.log('\n' + '='.repeat(50));
    console.log('RESULTS:');
    console.log(`Frames sent: ${framesSent}`);
    console.log(`Frames received by other client: ${framesReceived}`);
    
    if (hasDataField) {
        console.log('\n✅ SUCCESS! Frames now have the "d" field!');
        console.log('   Users should now be able to see each other\'s video.');
    } else if (framesReceived > 0) {
        console.log('\n⚠️  WARNING: Frames received but still missing "d" field');
        console.log('   The fix may not be deployed yet or clients need refresh');
    } else {
        console.log('\n❌ No frames received - check server connectivity');
    }
    
    console.log('='.repeat(50));
    
    sender.close();
    receiver.close();
}

testVideoDisplayFix().catch(console.error);