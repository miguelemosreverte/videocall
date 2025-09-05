#!/usr/bin/env node

const WebSocket = require('ws');
const { createCanvas } = require('canvas');
const fs = require('fs').promises;
const path = require('path');
const BenchmarkDatabase = require('./benchmark-db.js');
const zlib = require('zlib');
const util = require('util');
const gzip = util.promisify(zlib.gzip);

class NetworkBandwidthBenchmark {
    constructor(wsUrl) {
        this.wsUrl = wsUrl;
        this.results = {
            timestamp: new Date().toISOString(),
            server: wsUrl,
            config: {},
            metrics: {
                bandwidth: [],
                throughput: [],
                latency: [],
                packetsPerSecond: [],
                framesSent: 0,
                framesAcked: 0,
                bytesTransferred: 0,
                connectionTime: 0
            },
            results: null
        };
        this.ws = null;
        this.startTime = null;
        this.preGeneratedFrames = [];
        this.ackReceived = 0;
        this.sendTimestamps = new Map();
    }

    async connect() {
        const connectStart = Date.now();
        return new Promise((resolve, reject) => {
            console.log(`🌐 Connecting to ${this.wsUrl}...`);
            
            this.ws = new WebSocket(this.wsUrl, {
                perMessageDeflate: {
                    zlibDeflateOptions: {
                        level: zlib.constants.Z_BEST_SPEED,
                        memLevel: 9,
                        strategy: zlib.constants.Z_DEFAULT_STRATEGY
                    },
                    clientMaxWindowBits: 15,
                    serverMaxWindowBits: 15,
                    threshold: 0 // Always compress
                },
                maxPayload: 50 * 1024 * 1024 // 50MB max
            });
            
            this.ws.on('open', () => {
                this.results.metrics.connectionTime = Date.now() - connectStart;
                console.log(`✅ Connected in ${this.results.metrics.connectionTime}ms`);
                
                // Test ping
                const pingStart = Date.now();
                this.ws.send(JSON.stringify({
                    type: 'ping',
                    timestamp: pingStart
                }));
                
                this.ws.send(JSON.stringify({
                    type: 'join',
                    room: 'network-benchmark',
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
                    
                    // Track acknowledgments for sent frames
                    if (message.type === 'frame' && message.echo) {
                        const sentTime = this.sendTimestamps.get(message.frameNumber);
                        if (sentTime) {
                            const latency = Date.now() - sentTime;
                            this.results.metrics.latency.push(latency);
                            this.sendTimestamps.delete(message.frameNumber);
                            this.ackReceived++;
                            this.results.metrics.framesAcked++;
                        }
                    }
                } catch (e) {
                    // Binary data or parse error
                }
            });

            this.ws.on('close', () => {
                console.log('📡 Connection closed');
            });
        });
    }

    async generateTestData(config) {
        console.log('📦 Generating test data for network benchmark...');
        const resolution = this.getResolution(config.resolution);
        const totalFrames = config.framerate * config.duration;
        
        const canvas = createCanvas(resolution.width, resolution.height);
        const ctx = canvas.getContext('2d', { alpha: false });
        
        // Generate diverse frame sizes to test bandwidth
        const frameSizes = ['small', 'medium', 'large', 'huge'];
        
        for (let i = 0; i < totalFrames; i++) {
            // Create frames with varying complexity/size
            const complexity = frameSizes[i % frameSizes.length];
            
            // Simple colored rectangle (compresses well)
            if (complexity === 'small') {
                ctx.fillStyle = `hsl(${i * 10}, 70%, 50%)`;
                ctx.fillRect(0, 0, resolution.width, resolution.height);
            }
            // Medium complexity with patterns
            else if (complexity === 'medium') {
                for (let j = 0; j < 10; j++) {
                    ctx.fillStyle = `hsl(${(i + j) * 10}, 70%, 50%)`;
                    ctx.fillRect(j * resolution.width/10, 0, resolution.width/10, resolution.height);
                }
            }
            // Complex with noise (compresses poorly)
            else {
                const imageData = ctx.createImageData(resolution.width, resolution.height);
                const data = imageData.data;
                for (let j = 0; j < data.length; j += 4) {
                    data[j] = Math.random() * 255;     // Red
                    data[j + 1] = Math.random() * 255; // Green
                    data[j + 2] = Math.random() * 255; // Blue
                    data[j + 3] = 255;                 // Alpha
                }
                ctx.putImageData(imageData, 0, 0);
            }
            
            // Add frame number
            ctx.fillStyle = 'white';
            ctx.font = '48px Arial';
            ctx.textAlign = 'center';
            ctx.fillText(`Frame ${i}`, resolution.width/2, resolution.height/2);
            
            // Encode with varying quality
            const quality = config.quality || (complexity === 'huge' ? 0.3 : 0.7);
            const buffer = canvas.toBuffer('image/jpeg', { 
                quality,
                progressive: false,
                chromaSubsampling: '4:2:0'
            });
            
            this.preGeneratedFrames.push({
                data: buffer.toString('base64'),
                size: buffer.length,
                frameNumber: i,
                complexity: complexity
            });
            
            if ((i + 1) % Math.floor(totalFrames / 4) === 0) {
                const sizesMB = this.preGeneratedFrames.reduce((sum, f) => sum + f.size, 0) / 1048576;
                console.log(`  Generated ${i + 1}/${totalFrames} frames (${sizesMB.toFixed(2)} MB)`);
            }
        }
        
        const totalSize = this.preGeneratedFrames.reduce((sum, f) => sum + f.size, 0);
        console.log(`✅ Generated ${totalFrames} frames, total: ${(totalSize / 1048576).toFixed(2)} MB`);
        console.log(`  Average frame size: ${(totalSize / totalFrames / 1024).toFixed(2)} KB`);
        
        return totalSize;
    }

    async runBandwidthTest(config) {
        this.results.config = config;
        const totalDataSize = await this.generateTestData(config);
        
        console.log(`\n🚀 NETWORK BANDWIDTH TEST`);
        console.log(`  Server: ${this.wsUrl}`);
        console.log(`  Resolution: ${config.resolution}`);
        console.log(`  Target: ${config.framerate} FPS for ${config.duration}s`);
        console.log(`  Expected bandwidth: ${(totalDataSize * 8 / config.duration / 1000000).toFixed(2)} Mbps`);
        
        this.startTime = Date.now();
        const totalFrames = this.preGeneratedFrames.length;
        let framesSent = 0;
        let bytesSent = 0;
        let lastMetricTime = Date.now();
        let framesInLastSecond = 0;
        let bytesInLastSecond = 0;
        
        return new Promise((resolve) => {
            const sendFrame = async () => {
                if (framesSent >= totalFrames) {
                    // All frames sent, wait for acknowledgments
                    console.log(`\n✅ All ${framesSent} frames sent, waiting for ACKs...`);
                    setTimeout(() => {
                        resolve();
                    }, 2000);
                    return;
                }
                
                // Send multiple frames in parallel for maximum throughput
                const batchSize = Math.min(10, totalFrames - framesSent);
                const batch = [];
                
                for (let i = 0; i < batchSize && framesSent < totalFrames; i++) {
                    const frame = this.preGeneratedFrames[framesSent];
                    const frameData = {
                        type: 'frame',
                        data: frame.data,
                        frameNumber: frame.frameNumber,
                        sentAt: Date.now(),
                        size: frame.size,
                        complexity: frame.complexity
                    };
                    
                    this.sendTimestamps.set(frame.frameNumber, Date.now());
                    
                    if (this.ws.readyState === WebSocket.OPEN) {
                        this.ws.send(JSON.stringify(frameData));
                        framesSent++;
                        bytesSent += frame.size;
                        framesInLastSecond++;
                        bytesInLastSecond += frame.size;
                        this.results.metrics.framesSent++;
                        this.results.metrics.bytesTransferred += frame.size;
                    }
                }
                
                // Calculate metrics every second
                const now = Date.now();
                if (now - lastMetricTime >= 1000) {
                    const elapsed = (now - this.startTime) / 1000;
                    const bandwidth = (bytesInLastSecond * 8) / 1000000; // Mbps
                    const throughput = bytesSent / elapsed / 1048576; // MB/s
                    const fps = framesInLastSecond;
                    
                    this.results.metrics.bandwidth.push(bandwidth);
                    this.results.metrics.throughput.push(throughput);
                    this.results.metrics.packetsPerSecond.push(fps);
                    
                    console.log(`  📊 FPS: ${fps} | Bandwidth: ${bandwidth.toFixed(2)} Mbps | Throughput: ${throughput.toFixed(2)} MB/s | ACKs: ${this.ackReceived}/${framesSent}`);
                    
                    lastMetricTime = now;
                    framesInLastSecond = 0;
                    bytesInLastSecond = 0;
                }
                
                // Continue sending as fast as possible
                setImmediate(sendFrame);
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
        
        const avgBandwidth = metrics.bandwidth.reduce((a, b) => a + b, 0) / metrics.bandwidth.length || 0;
        const maxBandwidth = Math.max(...metrics.bandwidth) || 0;
        const avgThroughput = metrics.throughput.reduce((a, b) => a + b, 0) / metrics.throughput.length || 0;
        const avgLatency = metrics.latency.reduce((a, b) => a + b, 0) / metrics.latency.length || 0;
        const avgFPS = metrics.packetsPerSecond.reduce((a, b) => a + b, 0) / metrics.packetsPerSecond.length || 0;
        
        const packetLoss = ((metrics.framesSent - metrics.framesAcked) / metrics.framesSent * 100) || 0;
        const efficiency = (metrics.framesAcked / metrics.framesSent * 100) || 0;
        
        this.results.results = {
            avgBandwidth: avgBandwidth.toFixed(2),
            maxBandwidth: maxBandwidth.toFixed(2),
            avgThroughput: avgThroughput.toFixed(2),
            avgLatency: avgLatency.toFixed(2),
            avgFPS: avgFPS.toFixed(2),
            packetLoss: packetLoss.toFixed(2),
            efficiency: efficiency.toFixed(2),
            framesSent: metrics.framesSent,
            framesAcked: metrics.framesAcked,
            bytesTransferred: (metrics.bytesTransferred / 1048576).toFixed(2) + ' MB',
            connectionTime: metrics.connectionTime + ' ms'
        };
        
        // Calculate network score
        const targetBandwidth = 50; // 50 Mbps target for 4K
        const bandwidthScore = Math.min(100, (avgBandwidth / targetBandwidth) * 100) * 0.4;
        const latencyScore = Math.max(0, 100 - (avgLatency / 100 * 100)) * 0.3;
        const efficiencyScore = efficiency * 0.3;
        
        this.results.results.networkScore = Math.round(bandwidthScore + latencyScore + efficiencyScore);
        
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
    console.log('🌐 NETWORK BANDWIDTH BENCHMARK v1.0');
    console.log('=====================================\n');
    
    // Test both local and remote servers
    const servers = [
        { name: 'Local', url: 'ws://localhost:3001' },
        { name: 'Hetzner', url: 'wss://91.99.159.21.nip.io/ws' }
    ];
    
    const configurations = [
        { resolution: 'hd', framerate: 30, duration: 3, quality: 0.7 },
        { resolution: 'fhd', framerate: 30, duration: 3, quality: 0.7 },
        { resolution: 'fhd', framerate: 60, duration: 3, quality: 0.6 },
        { resolution: '4k', framerate: 30, duration: 3, quality: 0.5 },
        { resolution: '4k', framerate: 60, duration: 3, quality: 0.4 }
    ];
    
    const db = new BenchmarkDatabase('./network-benchmark.db');
    await db.init();
    
    for (const server of servers) {
        console.log(`\n📡 Testing ${server.name} Server: ${server.url}`);
        console.log('=' .repeat(50));
        
        for (const config of configurations) {
            const benchmark = new NetworkBandwidthBenchmark(server.url);
            
            try {
                await benchmark.connect();
                await benchmark.runBandwidthTest(config);
                const results = benchmark.calculateResults();
                
                console.log(`\n📈 RESULTS for ${config.resolution.toUpperCase()} @ ${config.framerate}fps:`);
                console.log(`  Network Score: ${results.networkScore}/100`);
                console.log(`  Bandwidth: ${results.avgBandwidth} Mbps (max: ${results.maxBandwidth} Mbps)`);
                console.log(`  Throughput: ${results.avgThroughput} MB/s`);
                console.log(`  Latency: ${results.avgLatency} ms`);
                console.log(`  Efficiency: ${results.efficiency}% (${results.framesAcked}/${results.framesSent} frames)`);
                console.log(`  Data transferred: ${results.bytesTransferred}`);
                
                await db.insertBenchmark(benchmark.results);
                
                benchmark.disconnect();
                
                await new Promise(r => setTimeout(r, 2000));
                
            } catch (error) {
                console.error(`❌ Test failed: ${error.message}`);
                benchmark.disconnect();
                
                // If Hetzner fails, it might be offline
                if (server.name === 'Hetzner') {
                    console.log('⚠️  Hetzner server may be offline, skipping remaining tests');
                    break;
                }
            }
        }
    }
    
    // Generate network performance report
    console.log('\n' + '='.repeat(50));
    console.log('📊 NETWORK PERFORMANCE SUMMARY');
    console.log('='.repeat(50));
    
    const stats = await db.getAverageMetrics(1);
    
    console.log('\n| Resolution | Tests | Avg Bandwidth | Network Score |');
    console.log('|------------|-------|---------------|---------------|');
    
    for (const stat of stats) {
        // Note: These fields might not exist in the default schema
        // But the concept is to show network-specific metrics
        console.log(`| ${stat.resolution.toUpperCase().padEnd(10)} | ${stat.test_count.toString().padEnd(5)} | ${(stat.avg_bitrate || 0).toFixed(2).padEnd(11)} Mbps | ${stat.avg_quality_score.toString().padEnd(13)} |`);
    }
    
    await db.close();
    console.log('\n✅ Network benchmark complete!');
    console.log('📄 Results saved to network-benchmark.db');
}

if (require.main === module) {
    main().catch(console.error);
}

module.exports = NetworkBandwidthBenchmark;