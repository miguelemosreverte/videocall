#!/usr/bin/env node

const WebSocket = require('ws');
const { createCanvas } = require('canvas');

async function test4K60fpsBandwidth() {
    console.log('🎯 4K@60FPS BANDWIDTH TEST');
    console.log('==========================\n');
    
    // Generate test frames
    console.log('📦 Generating 4K test frames...');
    const canvas = createCanvas(3840, 2160);
    const ctx = canvas.getContext('2d', { alpha: false });
    
    const frames = [];
    const targetFrames = 300; // 5 seconds at 60fps
    
    for (let i = 0; i < targetFrames; i++) {
        // Simple pattern that compresses reasonably
        ctx.fillStyle = `hsl(${(i * 2) % 360}, 70%, 50%)`;
        ctx.fillRect(0, 0, 3840, 2160);
        
        ctx.fillStyle = 'white';
        ctx.font = '200px Arial';
        ctx.textAlign = 'center';
        ctx.fillText(`Frame ${i}`, 1920, 1080);
        
        const buffer = canvas.toBuffer('image/jpeg', { quality: 0.5 });
        frames.push(buffer);
        
        if ((i + 1) % 60 === 0) {
            const totalMB = frames.reduce((sum, f) => sum + f.length, 0) / 1048576;
            console.log(`  Generated ${i + 1} frames (${totalMB.toFixed(2)} MB)`);
        }
    }
    
    const totalSize = frames.reduce((sum, f) => sum + f.length, 0);
    console.log(`✅ Generated ${targetFrames} frames`);
    console.log(`   Total size: ${(totalSize / 1048576).toFixed(2)} MB`);
    console.log(`   Average frame: ${(totalSize / targetFrames / 1024).toFixed(2)} KB`);
    console.log(`   Required bandwidth: ${(totalSize * 8 / 5 / 1000000).toFixed(2)} Mbps\n`);
    
    // Connect to server
    console.log('🔌 Connecting to ws://localhost:3001...');
    const ws = new WebSocket('ws://localhost:3001', {
        maxPayload: 100 * 1024 * 1024
    });
    
    await new Promise((resolve, reject) => {
        ws.on('open', resolve);
        ws.on('error', reject);
    });
    
    console.log('✅ Connected\n');
    
    // Send frames at 60fps
    console.log('📡 Transmitting at 60 FPS...');
    const startTime = Date.now();
    let framesSent = 0;
    let acksReceived = 0;
    let bytesSent = 0;
    let lastLogTime = startTime;
    
    ws.on('message', (data) => {
        try {
            const msg = JSON.parse(data);
            if (msg.echo) acksReceived++;
        } catch (e) {}
    });
    
    const frameInterval = 1000 / 60; // 16.67ms per frame
    
    for (let i = 0; i < targetFrames; i++) {
        const frameStart = Date.now();
        const frame = frames[i];
        
        ws.send(JSON.stringify({
            type: 'frame',
            data: frame.toString('base64'),
            frameNumber: i,
            sentAt: Date.now()
        }));
        
        framesSent++;
        bytesSent += frame.length;
        
        // Log every second
        if (frameStart - lastLogTime >= 1000) {
            const elapsed = (frameStart - startTime) / 1000;
            const actualFPS = framesSent / elapsed;
            const mbps = (bytesSent * 8 / elapsed / 1000000);
            const efficiency = (acksReceived / framesSent * 100).toFixed(1);
            
            console.log(`  FPS: ${actualFPS.toFixed(1)} | Bandwidth: ${mbps.toFixed(2)} Mbps | ACKs: ${acksReceived}/${framesSent} (${efficiency}%)`);
            lastLogTime = frameStart;
        }
        
        // Maintain 60fps timing
        const frameTime = Date.now() - frameStart;
        if (frameTime < frameInterval) {
            await new Promise(r => setTimeout(r, frameInterval - frameTime));
        }
    }
    
    // Wait for final ACKs
    await new Promise(r => setTimeout(r, 1000));
    
    // Final results
    const totalTime = (Date.now() - startTime) / 1000;
    const finalFPS = framesSent / totalTime;
    const finalBandwidth = (bytesSent * 8 / totalTime / 1000000);
    const finalEfficiency = (acksReceived / framesSent * 100);
    
    console.log('\n📊 FINAL RESULTS:');
    console.log('=================');
    console.log(`  Frames sent: ${framesSent}`);
    console.log(`  ACKs received: ${acksReceived}`);
    console.log(`  Efficiency: ${finalEfficiency.toFixed(1)}%`);
    console.log(`  Average FPS: ${finalFPS.toFixed(1)}`);
    console.log(`  Average bandwidth: ${finalBandwidth.toFixed(2)} Mbps`);
    console.log(`  Data transferred: ${(bytesSent / 1048576).toFixed(2)} MB`);
    
    if (finalFPS >= 59) {
        console.log('\n✅ SUCCESS: Achieved 4K@60fps!');
    } else {
        console.log(`\n⚠️  Only achieved ${finalFPS.toFixed(1)} FPS (${(finalFPS/60*100).toFixed(0)}% of target)`);
    }
    
    ws.close();
}

test4K60fpsBandwidth().catch(console.error);