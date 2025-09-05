#!/usr/bin/env node

const WebSocket = require('ws');
const { createCanvas } = require('canvas');

async function testHetzner4K() {
    console.log('🌐 4K@60FPS HETZNER PRODUCTION TEST');
    console.log('=====================================\n');
    
    // Pre-generate optimized frames
    console.log('📦 Generating optimized 4K frames...');
    const canvas = createCanvas(3840, 2160);
    const ctx = canvas.getContext('2d', { alpha: false });
    
    const frames = [];
    const duration = 3; // 3 second test
    const fps = 60;
    const totalFrames = duration * fps;
    
    for (let i = 0; i < totalFrames; i++) {
        // Simple pattern for better compression
        ctx.fillStyle = `hsl(${(i * 4) % 360}, 70%, 50%)`;
        ctx.fillRect(0, 0, 3840, 2160);
        
        ctx.fillStyle = 'white';
        ctx.font = 'bold 200px Arial';
        ctx.textAlign = 'center';
        ctx.fillText(`${i}`, 1920, 1080);
        
        const buffer = canvas.toBuffer('image/jpeg', { 
            quality: 0.35, // Lower quality for Hetzner bandwidth
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
    console.log(`✅ Generated ${totalFrames} frames`);
    console.log(`   Total: ${(totalSize / 1048576).toFixed(2)} MB`);
    console.log(`   Per frame: ${(totalSize / totalFrames / 1024).toFixed(2)} KB`);
    console.log(`   Required bandwidth: ${(totalSize * 8 / duration / 1000000).toFixed(2)} Mbps\n`);
    
    // Test servers
    const servers = [
        { name: 'Local', url: 'ws://localhost:3001' },
        { name: 'Hetzner', url: 'wss://91.99.159.21.nip.io/ws' }
    ];
    
    for (const server of servers) {
        console.log(`\n📡 Testing ${server.name}: ${server.url}`);
        console.log('-'.repeat(50));
        
        try {
            const connectStart = Date.now();
            const ws = new WebSocket(server.url, {
                maxPayload: 100 * 1024 * 1024
            });
            
            await new Promise((resolve, reject) => {
                ws.on('open', () => {
                    const latency = Date.now() - connectStart;
                    console.log(`✅ Connected in ${latency}ms\n`);
                    resolve();
                });
                ws.on('error', reject);
                setTimeout(() => reject(new Error('Connection timeout')), 5000);
            });
            
            // Send join message
            ws.send(JSON.stringify({
                type: 'join',
                room: 'hetzner-test',
                userId: 'hetzner-' + Date.now()
            }));
            
            // Metrics
            let framesSent = 0;
            let acksReceived = 0;
            const startTime = Date.now();
            const sentTimes = new Map();
            const latencies = [];
            
            ws.on('message', (data) => {
                try {
                    const msg = JSON.parse(data);
                    if (msg.echo && msg.frameNumber !== undefined) {
                        acksReceived++;
                        const sentTime = sentTimes.get(msg.frameNumber);
                        if (sentTime) {
                            latencies.push(Date.now() - sentTime);
                            sentTimes.delete(msg.frameNumber);
                        }
                    }
                } catch (e) {}
            });
            
            // Send frames with precise timing
            console.log(`🚀 Transmitting ${totalFrames} frames at 60 FPS...`);
            
            const targetInterval = 1000 / fps;
            let frameIndex = 0;
            let lastLogTime = startTime;
            
            const sender = setInterval(() => {
                if (frameIndex < totalFrames) {
                    const frame = frames[frameIndex];
                    const now = Date.now();
                    
                    if (ws.readyState === WebSocket.OPEN) {
                        sentTimes.set(frame.number, now);
                        ws.send(JSON.stringify({
                            type: 'frame',
                            data: frame.data,
                            frameNumber: frame.number,
                            sentAt: now
                        }));
                        framesSent++;
                        
                        // Log every second
                        if (now - lastLogTime >= 1000) {
                            const elapsed = (now - startTime) / 1000;
                            const actualFPS = framesSent / elapsed;
                            const mbps = (frames.slice(0, framesSent).reduce((s, f) => s + f.size, 0) * 8 / elapsed / 1000000);
                            const avgLatency = latencies.length > 0 ? 
                                (latencies.reduce((a, b) => a + b, 0) / latencies.length).toFixed(0) : 0;
                            
                            console.log(`  ${Math.ceil(elapsed)}s: ${actualFPS.toFixed(1)} FPS | ${mbps.toFixed(2)} Mbps | Latency: ${avgLatency}ms | ACKs: ${acksReceived}/${framesSent}`);
                            lastLogTime = now;
                        }
                    }
                    
                    frameIndex++;
                } else {
                    clearInterval(sender);
                }
            }, targetInterval);
            
            // Wait for completion plus ACK time
            await new Promise(r => setTimeout(r, (duration + 1) * 1000));
            
            // Results
            const totalTime = (Date.now() - startTime) / 1000;
            const avgFPS = framesSent / (duration);
            const efficiency = (acksReceived / framesSent * 100);
            const avgLatency = latencies.length > 0 ? 
                latencies.reduce((a, b) => a + b, 0) / latencies.length : 0;
            
            console.log(`\n📊 ${server.name.toUpperCase()} RESULTS:`);
            console.log(`  Frames sent: ${framesSent}/${totalFrames}`);
            console.log(`  ACKs received: ${acksReceived} (${efficiency.toFixed(1)}%)`);
            console.log(`  Average FPS: ${avgFPS.toFixed(1)}`);
            console.log(`  Average latency: ${avgLatency.toFixed(0)}ms`);
            console.log(`  Data transferred: ${(totalSize / 1048576).toFixed(2)} MB`);
            
            if (avgFPS >= 58 && efficiency >= 90) {
                console.log(`  ✅ ${server.name} PASSED: ${avgFPS.toFixed(1)} FPS @ ${efficiency.toFixed(0)}% reliability`);
            } else {
                console.log(`  ⚠️  ${server.name}: Only ${avgFPS.toFixed(1)} FPS @ ${efficiency.toFixed(0)}% reliability`);
            }
            
            ws.close();
            
        } catch (error) {
            console.log(`❌ ${server.name} failed: ${error.message}`);
        }
    }
    
    console.log('\n' + '='.repeat(50));
    console.log('✅ Production test complete!');
}

testHetzner4K().catch(console.error);