#!/usr/bin/env node

const puppeteer = require('puppeteer');
const fs = require('fs').promises;
const path = require('path');
const BenchmarkDatabase = require('./benchmark-db.js');

async function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

async function runBenchmark(page, config) {
    console.log(`\n🚀 Running benchmark: ${config.resolution} @ ${config.framerate}fps`);
    
    // Set configuration
    await page.select('#resolution', config.resolution);
    await page.select('#framerate', config.framerate.toString());
    await page.type('#duration', config.duration.toString(), {delay: 20});
    
    // Start benchmark
    await page.click('#startBtn');
    
    // Wait for benchmark to complete (duration + 2 seconds buffer)
    await sleep((config.duration + 2) * 1000);
    
    // Export results
    const results = await page.evaluate(() => {
        return window.benchmarkData;
    });
    
    if (!results || !results.results) {
        console.log('❌ Benchmark failed - no results');
        return null;
    }
    
    console.log(`✅ Completed: Score ${results.results.score}/100`);
    console.log(`   FPS: ${results.results.avgFps} | Latency: ${results.results.avgLatency}ms`);
    
    return results;
}

async function main() {
    const browser = await puppeteer.launch({
        headless: false, // Set to true for automated runs
        defaultViewport: null,
        args: ['--window-size=1920,1080']
    });

    try {
        const page = await browser.newPage();
        
        // Allow insecure localhost for testing
        await page.goto('https://miguelemosreverte.github.io/videocall/benchmark/benchmark.html', {
            waitUntil: 'networkidle2'
        });
        
        // Run multiple benchmark configurations
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
        
        const results = [];
        
        // Initialize database
        const db = new BenchmarkDatabase(path.join(__dirname, 'benchmark-results.db'));
        await db.init();
        
        for (const config of configurations) {
            const result = await runBenchmark(page, config);
            if (result) {
                results.push(result);
                // Save to database
                await db.insertBenchmark(result);
                
                // Wait between tests
                await sleep(2000);
            }
        }
        
        console.log('\n📊 Generating reports...');
        
        // Generate markdown report
        const markdownReport = await db.generateMarkdownReport();
        const mdFile = `benchmark-report-${new Date().toISOString().split('T')[0]}.md`;
        await fs.writeFile(path.join(__dirname, mdFile), markdownReport);
        console.log(`📝 Markdown report: ${mdFile}`);
        
        // Generate HTML report
        const htmlReport = await db.generateHTMLReport();
        const htmlFile = `benchmark-report-${new Date().toISOString().split('T')[0]}.html`;
        await fs.writeFile(path.join(__dirname, htmlFile), htmlReport);
        console.log(`🌐 HTML report: ${htmlFile}`);
        
        // Show summary
        console.log('\n📈 Benchmark Summary:');
        console.log('====================');
        
        const resolutionScores = {};
        for (const result of results) {
            const key = `${result.config.resolution}@${result.config.framerate}fps`;
            resolutionScores[key] = result.results.score;
        }
        
        // Sort by score
        const sorted = Object.entries(resolutionScores).sort((a, b) => b[1] - a[1]);
        
        for (const [config, score] of sorted) {
            const bar = '█'.repeat(Math.round(score / 10));
            const empty = '░'.repeat(10 - Math.round(score / 10));
            console.log(`${config.padEnd(12)} ${bar}${empty} ${score}/100`);
        }
        
        // Close database
        await db.close();
        
    } catch (error) {
        console.error('Error running benchmarks:', error);
    } finally {
        await browser.close();
    }
}

// Run if called directly
if (require.main === module) {
    main().catch(console.error);
}

module.exports = { runBenchmark };