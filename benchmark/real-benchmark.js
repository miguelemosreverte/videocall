#!/usr/bin/env node

const WebSocket = require('ws');
const { createCanvas } = require('canvas');
const fs = require('fs').promises;
const path = require('path');
const BenchmarkDatabase = require('./benchmark-db.js');

class RealBenchmark {
    constructor(wsUrl = 'ws://localhost:3001') {
        this.wsUrl = wsUrl;
        this.results = {
            timestamp: new Date().toISOString(),
            config: {},
            metrics: {
                fps: [],
                latency: [],
                bitrate: [],
                jitter: [],
                framesSent: 0,
                framesReceived: 0,
                bytesTransferred: 0
            },
            results: null
        };
        this.ws = null;
        this.startTime = null;
        this.frameCount = 0;
        this.lastFrameTime = null;
    }

    async connect() {
        return new Promise((resolve, reject) => {
            console.log(`🔌 Connecting to ${this.wsUrl}...`);
            this.ws = new WebSocket(this.wsUrl);
            
            this.ws.on('open', () => {
                console.log('✅ Connected to WebSocket server');
                this.ws.send(JSON.stringify({
                    type: 'join',
                    room: 'benchmark',
                    userId: 'bench-' + Date.now()
                }));
                resolve();
            });

            this.ws.on('error', (err) => {
                console.error('❌ WebSocket error:', err.message);
                reject(err);
            });

            this.ws.on('message', (data) => {
                try {
                    const message = JSON.parse(data);
                    if (message.type === 'frame' && message.sentAt) {
                        const now = Date.now();
                        const latency = now - message.sentAt;
                        this.results.metrics.latency.push(latency);
                        this.results.metrics.framesReceived++;
                    }
                } catch (e) {
                    // Ignore parse errors
                }
            });
        });
    }

    async runTest(config) {
        this.results.config = config;
        console.log(`\n📊 Running benchmark: ${config.resolution} @ ${config.framerate}fps for ${config.duration}s`);
        
        const resolution = this.getResolution(config.resolution);
        const canvas = createCanvas(resolution.width, resolution.height);
        const ctx = canvas.getContext('2d');
        
        this.startTime = Date.now();
        this.frameCount = 0;
        this.lastFrameTime = this.startTime;
        
        const frameInterval = 1000 / config.framerate;
        const duration = config.duration * 1000;
        
        return new Promise((resolve) => {
            const sendFrame = () => {
                const now = Date.now();
                const elapsed = now - this.startTime;
                
                if (elapsed >= duration) {
                    console.log(`✅ Test complete: ${this.frameCount} frames sent`);
                    resolve();
                    return;
                }
                
                // Generate test frame
                this.generateFrame(ctx, canvas.width, canvas.height, this.frameCount);
                
                // Calculate current FPS
                if (now - this.lastFrameTime >= 1000) {
                    const currentFps = this.frameCount / (elapsed / 1000);
                    this.results.metrics.fps.push(currentFps);
                    console.log(`  FPS: ${currentFps.toFixed(1)} | Frames: ${this.frameCount}`);
                    this.lastFrameTime = now;
                }
                
                // Send frame as base64
                const buffer = canvas.toBuffer('image/jpeg', { quality: 0.9 });
                const base64 = buffer.toString('base64');
                
                const frameData = {
                    type: 'frame',
                    data: base64,
                    quality: config.resolution,
                    sentAt: Date.now(),
                    frameNumber: this.frameCount
                };
                
                if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                    this.ws.send(JSON.stringify(frameData));
                    this.results.metrics.framesSent++;
                    this.results.metrics.bytesTransferred += buffer.length;
                    
                    // Update bitrate
                    const bitrate = (this.results.metrics.bytesTransferred * 8) / (elapsed / 1000) / 1000000;
                    this.results.metrics.bitrate.push(bitrate);
                }
                
                this.frameCount++;
                
                // Schedule next frame
                setTimeout(sendFrame, frameInterval);
            };
            
            sendFrame();
        });
    }

    generateFrame(ctx, width, height, frame) {
        // Create a complex test pattern
        const time = Date.now() / 1000;
        
        // Gradient background
        const gradient = ctx.createLinearGradient(0, 0, width, height);
        gradient.addColorStop(0, `hsl(${(frame * 2) % 360}, 100%, 50%)`);
        gradient.addColorStop(0.5, `hsl(${(frame * 2 + 120) % 360}, 100%, 50%)`);
        gradient.addColorStop(1, `hsl(${(frame * 2 + 240) % 360}, 100%, 50%)`);
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, width, height);
        
        // Add geometric patterns for complexity
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.5)';
        ctx.lineWidth = 2;
        
        for (let i = 0; i < 10; i++) {
            ctx.beginPath();
            ctx.arc(
                Math.sin(time + i) * width/3 + width/2,
                Math.cos(time + i) * height/3 + height/2,
                50 + i * 10,
                0,
                Math.PI * 2
            );
            ctx.stroke();
        }
        
        // Add text overlay
        ctx.fillStyle = 'white';
        ctx.font = 'bold 48px Arial';
        ctx.textAlign = 'center';
        ctx.fillText(`${width}x${height}`, width/2, height/2);
        ctx.fillText(`Frame ${frame}`, width/2, height/2 + 60);
    }

    getResolution(type) {
        const resolutions = {
            'hd': { width: 1280, height: 720 },
            'fhd': { width: 1920, height: 1080 },
            'qhd': { width: 2560, height: 1440 },
            '4k': { width: 3840, height: 2160 }
        };
        return resolutions[type] || resolutions['fhd'];
    }

    calculateResults() {
        const metrics = this.results.metrics;
        
        const avgFps = metrics.fps.reduce((a, b) => a + b, 0) / metrics.fps.length || 0;
        const avgLatency = metrics.latency.reduce((a, b) => a + b, 0) / metrics.latency.length || 0;
        const avgBitrate = metrics.bitrate.reduce((a, b) => a + b, 0) / metrics.bitrate.length || 0;
        
        // Calculate jitter
        const jitter = metrics.latency.length > 1 ? 
            Math.sqrt(metrics.latency.reduce((acc, val) => acc + Math.pow(val - avgLatency, 2), 0) / metrics.latency.length) : 0;
        
        // Calculate packet loss
        const expectedFrames = this.results.config.framerate * this.results.config.duration;
        const packetLoss = ((expectedFrames - metrics.framesReceived) / expectedFrames * 100) || 0;
        
        // Calculate quality score
        const targetFps = this.results.config.framerate;
        const fpsScore = Math.min(100, (avgFps / targetFps) * 100) * 0.3;
        const latencyScore = Math.max(0, 100 - (avgLatency / 50 * 100)) * 0.25;
        const bitrateScore = Math.min(100, (avgBitrate / 50) * 100) * 0.2;
        const jitterScore = Math.max(0, 100 - (jitter / 10 * 100)) * 0.15;
        const packetScore = Math.max(0, 100 - packetLoss) * 0.1;
        
        const score = Math.round(fpsScore + latencyScore + bitrateScore + jitterScore + packetScore);
        
        this.results.results = {
            avgFps: avgFps.toFixed(2),
            avgLatency: avgLatency.toFixed(2),
            avgBitrate: avgBitrate.toFixed(2),
            jitter: jitter.toFixed(2),
            packetLoss: packetLoss.toFixed(2),
            framesSent: metrics.framesSent,
            framesReceived: metrics.framesReceived,
            bytesTransferred: (metrics.bytesTransferred / 1048576).toFixed(2) + ' MB',
            score: score
        };
        
        return this.results.results;
    }

    disconnect() {
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
    }
}

async function main() {
    const args = process.argv.slice(2);
    const wsUrl = args[0] || 'ws://localhost:3001';
    
    console.log('🚀 Real WebRTC Benchmark Suite');
    console.log('==============================\n');
    
    // Test configurations
    const configurations = [
        { resolution: 'hd', framerate: 30, duration: 5 },
        { resolution: 'hd', framerate: 60, duration: 5 },
        { resolution: 'fhd', framerate: 30, duration: 5 },
        { resolution: 'fhd', framerate: 60, duration: 5 },
        { resolution: 'qhd', framerate: 30, duration: 5 },
        { resolution: 'qhd', framerate: 60, duration: 5 },
        { resolution: '4k', framerate: 30, duration: 5 },
        { resolution: '4k', framerate: 60, duration: 5 }
    ];
    
    const db = new BenchmarkDatabase('./benchmark-results.db');
    await db.init();
    
    const allResults = [];
    
    for (const config of configurations) {
        const benchmark = new RealBenchmark(wsUrl);
        
        try {
            await benchmark.connect();
            await benchmark.runTest(config);
            const results = benchmark.calculateResults();
            
            console.log(`\n📈 Results:`);
            console.log(`  Score: ${results.score}/100`);
            console.log(`  FPS: ${results.avgFps} (target: ${config.framerate})`);
            console.log(`  Latency: ${results.avgLatency}ms`);
            console.log(`  Bitrate: ${results.avgBitrate} Mbps`);
            console.log(`  Jitter: ${results.jitter}ms`);
            console.log(`  Packet Loss: ${results.packetLoss}%`);
            console.log(`  Data: ${results.bytesTransferred}`);
            
            // Save to database
            await db.insertBenchmark(benchmark.results);
            allResults.push(benchmark.results);
            
            benchmark.disconnect();
            
            // Wait between tests
            console.log('\n⏳ Waiting 2 seconds before next test...');
            await new Promise(r => setTimeout(r, 2000));
            
        } catch (error) {
            console.error(`❌ Test failed: ${error.message}`);
            benchmark.disconnect();
        }
    }
    
    // Generate reports
    console.log('\n📊 Generating reports...');
    
    const markdown = await db.generateMarkdownReport();
    const mdFile = `benchmark-report-${new Date().toISOString().split('T')[0]}.md`;
    await fs.writeFile(path.join(__dirname, mdFile), markdown);
    console.log(`✅ Markdown report: ${mdFile}`);
    
    const html = await db.generateHTMLReport();
    const htmlFile = `benchmark-report-${new Date().toISOString().split('T')[0]}.html`;
    await fs.writeFile(path.join(__dirname, htmlFile), html);
    console.log(`✅ HTML report: ${htmlFile}`);
    
    // Display summary
    console.log('\n📈 BENCHMARK SUMMARY');
    console.log('====================');
    
    for (const result of allResults) {
        const config = `${result.config.resolution.toUpperCase()} @ ${result.config.framerate}fps`;
        const bar = '█'.repeat(Math.round(result.results.score / 10));
        const empty = '░'.repeat(10 - Math.round(result.results.score / 10));
        console.log(`${config.padEnd(12)} ${bar}${empty} ${result.results.score}/100`);
    }
    
    await db.close();
    console.log('\n✨ Benchmark complete!');
}

if (require.main === module) {
    main().catch(console.error);
}

module.exports = RealBenchmark;