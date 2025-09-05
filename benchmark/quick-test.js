#!/usr/bin/env node

const RealBenchmark = require('./real-benchmark.js');
const BenchmarkDatabase = require('./benchmark-db.js');

async function quickTest() {
    console.log('🚀 Quick Benchmark Test');
    console.log('======================\n');
    
    // Quick test configurations (2 seconds each)
    const configurations = [
        { resolution: 'hd', framerate: 30, duration: 2 },
        { resolution: 'fhd', framerate: 30, duration: 2 },
        { resolution: '4k', framerate: 60, duration: 2 }
    ];
    
    const db = new BenchmarkDatabase('./benchmark-results.db');
    await db.init();
    
    for (const config of configurations) {
        const benchmark = new RealBenchmark('ws://localhost:3001');
        
        try {
            await benchmark.connect();
            await benchmark.runTest(config);
            const results = benchmark.calculateResults();
            
            console.log(`\n✅ ${config.resolution.toUpperCase()} @ ${config.framerate}fps: Score ${results.score}/100`);
            
            // Save to database
            await db.insertBenchmark(benchmark.results);
            
            benchmark.disconnect();
            
            // Short wait between tests
            await new Promise(r => setTimeout(r, 500));
            
        } catch (error) {
            console.error(`❌ Test failed: ${error.message}`);
            benchmark.disconnect();
        }
    }
    
    // Generate quick report
    const stats = await db.getAverageMetrics(1);
    console.log('\n📊 Quick Test Summary:');
    console.log('=====================');
    
    for (const stat of stats) {
        console.log(`${stat.resolution.toUpperCase()}: Score ${stat.avg_quality_score.toFixed(0)}/100 (${stat.test_count} tests)`);
    }
    
    await db.close();
    console.log('\n✨ Quick test complete!');
}

quickTest().catch(console.error);