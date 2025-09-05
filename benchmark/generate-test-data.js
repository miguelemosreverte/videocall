#!/usr/bin/env node

const BenchmarkDatabase = require('./benchmark-db.js');
const fs = require('fs').promises;

function generateTestData(resolution, framerate, baseScore = 85) {
    // Generate realistic variations
    const variance = Math.random() * 10 - 5;
    const score = Math.round(baseScore + variance);
    
    // Calculate realistic metrics based on resolution
    const resolutionMultiplier = {
        'hd': 1.0,
        'fhd': 0.9,
        'qhd': 0.8,
        '4k': 0.7
    }[resolution] || 1.0;
    
    const fps = framerate * resolutionMultiplier * (0.95 + Math.random() * 0.1);
    const latency = 20 + Math.random() * 30;
    const bitrate = (resolution === '4k' ? 40 : resolution === 'qhd' ? 25 : resolution === 'fhd' ? 15 : 8) + Math.random() * 10;
    const jitter = 2 + Math.random() * 8;
    const packetLoss = Math.random() * 0.5;
    
    return {
        timestamp: new Date().toISOString(),
        config: {
            resolution: resolution,
            framerate: framerate,
            duration: 5,
            bitrate: Math.round(bitrate)
        },
        metrics: {
            fps: Array(5).fill(fps),
            resolution: [],
            bitrate: Array(5).fill(bitrate),
            latency: Array(50).fill(latency),
            jitter: [],
            packetLoss: packetLoss,
            framesSent: framerate * 5,
            framesReceived: Math.floor(framerate * 5 * (1 - packetLoss/100)),
            bytesTransferred: bitrate * 5 * 125000
        },
        results: {
            avgFps: fps.toFixed(2),
            avgLatency: latency.toFixed(2),
            avgBitrate: bitrate.toFixed(2),
            jitter: jitter.toFixed(2),
            packetLoss: packetLoss.toFixed(2),
            framesSent: framerate * 5,
            framesReceived: Math.floor(framerate * 5 * (1 - packetLoss/100)),
            bytesTransferred: (bitrate * 5 * 125000 / 1048576).toFixed(2) + ' MB',
            score: score
        }
    };
}

async function main() {
    const db = new BenchmarkDatabase('./benchmark-results.db');
    await db.init();
    
    const configurations = [
        { resolution: 'hd', framerate: 30, baseScore: 95 },
        { resolution: 'hd', framerate: 60, baseScore: 92 },
        { resolution: 'fhd', framerate: 30, baseScore: 90 },
        { resolution: 'fhd', framerate: 60, baseScore: 85 },
        { resolution: 'qhd', framerate: 30, baseScore: 80 },
        { resolution: 'qhd', framerate: 60, baseScore: 75 },
        { resolution: '4k', framerate: 30, baseScore: 70 },
        { resolution: '4k', framerate: 60, baseScore: 65 }
    ];
    
    console.log('🎲 Generating test data...\n');
    
    // Generate 3 rounds of tests
    for (let round = 1; round <= 3; round++) {
        console.log(`Round ${round}:`);
        for (const config of configurations) {
            const data = generateTestData(config.resolution, config.framerate, config.baseScore);
            await db.insertBenchmark(data);
            console.log(`  ${config.resolution.toUpperCase()} @ ${config.framerate}fps: Score ${data.results.score}/100`);
        }
        console.log('');
    }
    
    console.log('📊 Generating reports...\n');
    
    // Generate reports
    const markdown = await db.generateMarkdownReport();
    await fs.writeFile('benchmark-report.md', markdown);
    console.log('✅ Markdown report: benchmark-report.md');
    
    const html = await db.generateHTMLReport();
    await fs.writeFile('benchmark-report.html', html);
    console.log('✅ HTML report: benchmark-report.html');
    
    // Show statistics
    console.log('\n📈 Statistics:\n');
    const stats = await db.getAverageMetrics(30);
    
    console.log('Resolution     Tests  Score  FPS     Latency  Bitrate');
    console.log('-------------------------------------------------------');
    for (const stat of stats) {
        console.log(
            `${stat.resolution.toUpperCase().padEnd(14)} ${stat.test_count.toString().padEnd(6)} ` +
            `${stat.avg_quality_score.toFixed(0).padEnd(6)} ${stat.avg_fps.toFixed(1).padEnd(7)} ` +
            `${stat.avg_latency.toFixed(1).padEnd(8)} ${stat.avg_bitrate.toFixed(1)}`
        );
    }
    
    await db.close();
    console.log('\n✨ Done!');
}

main().catch(console.error);