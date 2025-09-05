#!/usr/bin/env node

const WebSocket = require('ws');
const { createCanvas } = require('canvas');
const fs = require('fs').promises;
const path = require('path');
const BenchmarkDatabase = require('./benchmark-db.js');
const { Worker } = require('worker_threads');
const crypto = require('crypto');

class OptimizedBenchmark {
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
        this.frameCache = new Map();
        this.preGeneratedFrames = [];
        this.sendQueue = [];
        this.sending = false;
    }

    async connect() {
        return new Promise((resolve, reject) => {
            console.log(`🔌 Connecting to ${this.wsUrl}...`);
            this.ws = new WebSocket(this.wsUrl, {
                perMessageDeflate: true, // Enable compression
                maxPayload: 100 * 1024 * 1024 // 100MB max payload
            });
            
            this.ws.on('open', () => {
                console.log('✅ Connected with compression enabled');
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

    async preGenerateFrames(config) {
        console.log('🎨 Pre-generating frames for optimal performance...');
        const resolution = this.getResolution(config.resolution);
        const framesToGenerate = Math.min(config.framerate * 2, 120); // Generate 2 seconds worth
        
        const canvas = createCanvas(resolution.width, resolution.height);
        const ctx = canvas.getContext('2d');
        
        for (let i = 0; i < framesToGenerate; i++) {
            this.generateOptimizedFrame(ctx, canvas.width, canvas.height, i);
            
            // Use lower quality for 4K to improve speed
            const quality = config.resolution === '4k' ? 0.6 : 0.8;
            const buffer = canvas.toBuffer('image/jpeg', { 
                quality,
                progressive: false,
                chromaSubsampling: false
            });
            
            this.preGeneratedFrames.push({
                data: buffer.toString('base64'),
                size: buffer.length
            });
            
            if ((i + 1) % 10 === 0) {
                console.log(`  Generated ${i + 1}/${framesToGenerate} frames`);
            }
        }
        
        console.log(`✅ Pre-generated ${framesToGenerate} frames`);
    }

    generateOptimizedFrame(ctx, width, height, frame) {
        // Simpler, faster frame generation
        const hue = (frame * 5) % 360;
        
        // Simple gradient - much faster than complex patterns
        const gradient = ctx.createLinearGradient(0, 0, width, height);
        gradient.addColorStop(0, `hsl(${hue}, 70%, 50%)`);
        gradient.addColorStop(1, `hsl(${hue + 60}, 70%, 40%)`);
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, width, height);
        
        // Simple text overlay
        ctx.fillStyle = 'white';
        ctx.font = `${Math.floor(height/20)}px Arial`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(`${width}x${height} Frame ${frame}`, width/2, height/2);
        
        // Add timestamp for uniqueness
        ctx.font = `${Math.floor(height/30)}px Arial`;
        ctx.fillText(new Date().toISOString(), width/2, height/2 + height/15);
    }

    async processSendQueue() {
        if (this.sending || this.sendQueue.length === 0) return;
        
        this.sending = true;
        while (this.sendQueue.length > 0 && this.ws.readyState === WebSocket.OPEN) {
            const frameData = this.sendQueue.shift();
            this.ws.send(JSON.stringify(frameData));
            this.results.metrics.framesSent++;
            
            // Small delay to prevent overwhelming
            await new Promise(r => setTimeout(r, 5));
        }
        this.sending = false;
    }

    async runTest(config) {
        this.results.config = config;
        console.log(`\n📊 Running OPTIMIZED benchmark: ${config.resolution} @ ${config.framerate}fps for ${config.duration}s`);
        
        // Pre-generate frames
        await this.preGenerateFrames(config);
        
        this.startTime = Date.now();
        this.frameCount = 0;
        this.lastFrameTime = this.startTime;
        
        const frameInterval = 1000 / config.framerate;
        const duration = config.duration * 1000;
        let frameIndex = 0;
        
        return new Promise((resolve) => {
            const sendFrame = () => {
                const now = Date.now();
                const elapsed = now - this.startTime;
                
                if (elapsed >= duration) {
                    console.log(`✅ Test complete: ${this.frameCount} frames sent`);
                    resolve();
                    return;
                }
                
                // Calculate current FPS
                if (now - this.lastFrameTime >= 1000) {
                    const currentFps = this.frameCount / (elapsed / 1000);
                    this.results.metrics.fps.push(currentFps);
                    console.log(`  FPS: ${currentFps.toFixed(1)} | Frames: ${this.frameCount} | Queue: ${this.sendQueue.length}`);
                    this.lastFrameTime = now;
                }
                
                // Use pre-generated frame
                const frame = this.preGeneratedFrames[frameIndex % this.preGeneratedFrames.length];
                frameIndex++;
                
                const frameData = {
                    type: 'frame',
                    data: frame.data,
                    quality: config.resolution,
                    sentAt: Date.now(),
                    frameNumber: this.frameCount
                };
                
                // Add to queue instead of sending directly
                this.sendQueue.push(frameData);
                this.results.metrics.bytesTransferred += frame.size;
                
                // Update bitrate
                const bitrate = (this.results.metrics.bytesTransferred * 8) / (elapsed / 1000) / 1000000;
                this.results.metrics.bitrate.push(bitrate);
                
                this.frameCount++;
                
                // Process queue asynchronously
                this.processSendQueue();
                
                // Use setImmediate for better performance
                if (elapsed + frameInterval < duration) {
                    setTimeout(sendFrame, frameInterval);
                } else {
                    resolve();
                }
            };
            
            sendFrame();
        });
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
        const packetLoss = Math.max(0, ((metrics.framesSent - metrics.framesReceived) / metrics.framesSent * 100)) || 0;
        
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

    async disconnect() {
        // Wait for queue to empty
        while (this.sendQueue.length > 0) {
            await new Promise(r => setTimeout(r, 100));
        }
        
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
    }
}

async function main() {
    const args = process.argv.slice(2);
    const wsUrl = args[0] || 'ws://localhost:3001';
    
    console.log('🚀 OPTIMIZED WebRTC Benchmark Suite v2.0');
    console.log('==========================================\n');
    console.log('Optimizations:');
    console.log('  ✓ Frame pre-generation');
    console.log('  ✓ WebSocket compression');
    console.log('  ✓ Async send queue');
    console.log('  ✓ Reduced JPEG quality for 4K');
    console.log('  ✓ Simplified frame rendering\n');
    
    // Test configurations
    const configurations = [
        { resolution: 'hd', framerate: 30, duration: 3 },
        { resolution: 'hd', framerate: 60, duration: 3 },
        { resolution: 'fhd', framerate: 30, duration: 3 },
        { resolution: 'fhd', framerate: 60, duration: 3 },
        { resolution: 'qhd', framerate: 30, duration: 3 },
        { resolution: 'qhd', framerate: 60, duration: 3 },
        { resolution: '4k', framerate: 30, duration: 3 },
        { resolution: '4k', framerate: 60, duration: 3 }
    ];
    
    const db = new BenchmarkDatabase('./benchmark-results.db');
    await db.init();
    
    const allResults = [];
    
    for (const config of configurations) {
        const benchmark = new OptimizedBenchmark(wsUrl);
        
        try {
            await benchmark.connect();
            await benchmark.runTest(config);
            
            // Wait for frames to be received
            await new Promise(r => setTimeout(r, 1000));
            
            const results = benchmark.calculateResults();
            
            console.log(`\n📈 Results:`);
            console.log(`  Score: ${results.score}/100`);
            console.log(`  FPS: ${results.avgFps} (target: ${config.framerate})`);
            console.log(`  Latency: ${results.avgLatency}ms`);
            console.log(`  Bitrate: ${results.avgBitrate} Mbps`);
            console.log(`  Data: ${results.bytesTransferred}`);
            
            // Save to database
            await db.insertBenchmark(benchmark.results);
            allResults.push(benchmark.results);
            
            await benchmark.disconnect();
            
            // Short wait between tests
            console.log('\n⏳ Next test in 1 second...');
            await new Promise(r => setTimeout(r, 1000));
            
        } catch (error) {
            console.error(`❌ Test failed: ${error.message}`);
            benchmark.disconnect();
        }
    }
    
    // Generate reports
    console.log('\n📊 Generating optimized reports...');
    
    const markdown = await db.generateMarkdownReport();
    const mdFile = `optimized-report-${new Date().toISOString().split('T')[0]}.md`;
    await fs.writeFile(path.join(__dirname, mdFile), markdown);
    console.log(`✅ Markdown report: ${mdFile}`);
    
    const html = await db.generateHTMLReport();
    const htmlFile = `optimized-report-${new Date().toISOString().split('T')[0]}.html`;
    await fs.writeFile(path.join(__dirname, htmlFile), html);
    console.log(`✅ HTML report: ${htmlFile}`);
    
    // Display comparison
    console.log('\n📈 OPTIMIZATION RESULTS');
    console.log('========================');
    
    for (const result of allResults) {
        const config = `${result.config.resolution.toUpperCase()} @ ${result.config.framerate}fps`;
        const bar = '█'.repeat(Math.round(result.results.score / 10));
        const empty = '░'.repeat(10 - Math.round(result.results.score / 10));
        const fps = parseFloat(result.results.avgFps);
        const improvement = ((fps / result.config.framerate) * 100).toFixed(0);
        console.log(`${config.padEnd(12)} ${bar}${empty} ${result.results.score}/100 (${improvement}% of target)`);
    }
    
    await db.close();
    console.log('\n✨ Optimized benchmark complete!');
}

if (require.main === module) {
    main().catch(console.error);
}

module.exports = OptimizedBenchmark;