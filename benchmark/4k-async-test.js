#!/usr/bin/env node

const WebSocket = require('ws');
const { createCanvas } = require('canvas');

async function async4K60fps() {
    console.log('⚡ 4K@60FPS ASYNC BANDWIDTH TEST');
    console.log('==================================\n');
    
    // Pre-generate frames
    console.log('📦 Pre-generating 4K frames...');
    const canvas = createCanvas(3840, 2160);
    const ctx = canvas.getContext('2d', { alpha: false });
    
    const frames = [];
    const duration = 5; // seconds
    const fps = 60;
    const totalFrames = duration * fps;
    
    for (let i = 0; i < totalFrames; i++) {
        ctx.fillStyle = `hsl(${(i * 3) % 360}, 70%, 50%)`;
        ctx.fillRect(0, 0, 3840, 2160);
        
        ctx.fillStyle = 'white';
        ctx.font = '200px Arial';
        ctx.textAlign = 'center';
        ctx.fillText(`${i}`, 1920, 1080);
        
        const buffer = canvas.toBuffer('image/jpeg', { 
            quality: 0.4,
            progressive: false,
            chromaSubsampling: '4:2:0'
        });
        
        frames.push({
            data: buffer.toString('base64'),
            size: buffer.length,
            number: i
        });
    }
    
    const totalSize = frames.reduce((sum, f) => sum + f.size, 0);
    console.log(`✅ Pre-generated ${totalFrames} frames`);
    console.log(`   Total: ${(totalSize / 1048576).toFixed(2)} MB`);
    console.log(`   Per frame: ${(totalSize / totalFrames / 1024).toFixed(2)} KB`);
    console.log(`   Required: ${(totalSize * 8 / duration / 1000000).toFixed(2)} Mbps\n`);
    
    // Connect
    const ws = new WebSocket('ws://localhost:3001', {
        perMessageDeflate: false, // Disable compression for speed
        maxPayload: 100 * 1024 * 1024
    });
    
    await new Promise((resolve, reject) => {
        ws.on('open', () => {
            console.log('✅ Connected (compression OFF for max speed)\n');
            resolve();
        });
        ws.on('error', reject);
    });
    
    // Track metrics
    let framesSent = 0;
    let acksReceived = 0;
    const startTime = Date.now();
    const frameTimestamps = [];
    const targetInterval = 1000 / fps; // 16.67ms
    
    ws.on('message', (data) => {
        try {
            const msg = JSON.parse(data);
            if (msg.echo) acksReceived++;
        } catch (e) {}
    });
    
    // Async frame sender with precise timing
    console.log('🚀 Transmitting at 60 FPS (async)...');
    
    const sendFrame = (frame) => {
        if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
                type: 'frame',
                data: frame.data,
                frameNumber: frame.number,
                sentAt: Date.now()
            }));
            framesSent++;
            frameTimestamps.push(Date.now());
        }
    };
    
    // Use setInterval for precise timing
    let frameIndex = 0;
    const sender = setInterval(() => {
        if (frameIndex < totalFrames) {
            sendFrame(frames[frameIndex]);
            frameIndex++;
            
            // Log every second
            if (frameIndex % fps === 0) {
                const elapsed = (Date.now() - startTime) / 1000;
                const actualFPS = framesSent / elapsed;
                const mbps = (frames.slice(0, framesSent).reduce((s, f) => s + f.size, 0) * 8 / elapsed / 1000000);
                
                console.log(`  Second ${Math.floor(elapsed)}: ${actualFPS.toFixed(1)} FPS | ${mbps.toFixed(2)} Mbps | ACKs: ${acksReceived}/${framesSent}`);
            }
        } else {
            clearInterval(sender);
        }
    }, targetInterval);
    
    // Wait for completion
    await new Promise(r => setTimeout(r, (duration + 1) * 1000));
    
    // Calculate actual FPS from timestamps
    const actualIntervals = [];
    for (let i = 1; i < frameTimestamps.length; i++) {
        actualIntervals.push(frameTimestamps[i] - frameTimestamps[i-1]);
    }
    const avgInterval = actualIntervals.reduce((a, b) => a + b, 0) / actualIntervals.length;
    const actualFPS = 1000 / avgInterval;
    
    // Results
    const totalTime = (Date.now() - startTime) / 1000;
    const avgFPS = framesSent / totalTime;
    const bandwidth = (totalSize * 8 / totalTime / 1000000);
    const efficiency = (acksReceived / framesSent * 100);
    
    console.log('\n' + '='.repeat(40));
    console.log('📊 FINAL RESULTS:');
    console.log('='.repeat(40));
    console.log(`  Target: 4K @ 60 FPS for ${duration}s`);
    console.log(`  Frames sent: ${framesSent}/${totalFrames}`);
    console.log(`  ACKs received: ${acksReceived} (${efficiency.toFixed(1)}%)`);
    console.log(`  Average FPS: ${avgFPS.toFixed(1)}`);
    console.log(`  Actual FPS (from intervals): ${actualFPS.toFixed(1)}`);
    console.log(`  Bandwidth used: ${bandwidth.toFixed(2)} Mbps`);
    console.log(`  Data transferred: ${(totalSize / 1048576).toFixed(2)} MB`);
    
    if (actualFPS >= 59 && efficiency >= 95) {
        console.log('\n🎉 SUCCESS: TRUE 4K@60FPS ACHIEVED!');
        console.log('   ✅ Frame rate: ' + actualFPS.toFixed(1) + ' FPS');
        console.log('   ✅ Reliability: ' + efficiency.toFixed(1) + '%');
        console.log('   ✅ Bandwidth: ' + bandwidth.toFixed(2) + ' Mbps');
    } else {
        console.log(`\n⚠️  Achieved ${actualFPS.toFixed(1)} FPS (${(actualFPS/60*100).toFixed(0)}% of target)`);
    }
    
    ws.close();
}

async4K60fps().catch(console.error);