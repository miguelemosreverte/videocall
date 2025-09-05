#!/usr/bin/env node

const sqlite3 = require('sqlite3').verbose();
const fs = require('fs').promises;
const path = require('path');

class BenchmarkDatabase {
    constructor(dbPath = './benchmark-results.db') {
        this.dbPath = dbPath;
        this.db = null;
    }

    async init() {
        return new Promise((resolve, reject) => {
            this.db = new sqlite3.Database(this.dbPath, (err) => {
                if (err) {
                    reject(err);
                    return;
                }
                console.log('Connected to SQLite database');
                this.createTables().then(resolve).catch(reject);
            });
        });
    }

    async createTables() {
        return new Promise((resolve, reject) => {
            const sql = `
                CREATE TABLE IF NOT EXISTS benchmarks (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
                    test_timestamp TEXT,
                    resolution TEXT,
                    framerate INTEGER,
                    duration INTEGER,
                    target_bitrate INTEGER,
                    avg_fps REAL,
                    avg_latency REAL,
                    avg_bitrate REAL,
                    jitter REAL,
                    packet_loss REAL,
                    frames_sent INTEGER,
                    frames_received INTEGER,
                    bytes_transferred INTEGER,
                    quality_score INTEGER,
                    raw_data TEXT
                );

                CREATE INDEX IF NOT EXISTS idx_timestamp ON benchmarks(timestamp);
                CREATE INDEX IF NOT EXISTS idx_resolution ON benchmarks(resolution);
                CREATE INDEX IF NOT EXISTS idx_quality_score ON benchmarks(quality_score);
            `;

            this.db.exec(sql, (err) => {
                if (err) {
                    reject(err);
                } else {
                    console.log('Database tables created');
                    resolve();
                }
            });
        });
    }

    async insertBenchmark(data) {
        return new Promise((resolve, reject) => {
            const sql = `
                INSERT INTO benchmarks (
                    test_timestamp, resolution, framerate, duration, target_bitrate,
                    avg_fps, avg_latency, avg_bitrate, jitter, packet_loss,
                    frames_sent, frames_received, bytes_transferred, quality_score, raw_data
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            `;

            const params = [
                data.timestamp,
                data.config.resolution,
                data.config.framerate,
                data.config.duration,
                data.config.bitrate,
                parseFloat(data.results.avgFps),
                parseFloat(data.results.avgLatency),
                parseFloat(data.results.avgBitrate),
                parseFloat(data.results.jitter),
                parseFloat(data.results.packetLoss),
                data.results.framesSent,
                data.results.framesReceived,
                data.metrics.bytesTransferred,
                data.results.score,
                JSON.stringify(data)
            ];

            this.db.run(sql, params, function(err) {
                if (err) {
                    reject(err);
                } else {
                    console.log(`Benchmark inserted with ID: ${this.lastID}`);
                    resolve(this.lastID);
                }
            });
        });
    }

    async getLatestBenchmarks(limit = 10) {
        return new Promise((resolve, reject) => {
            const sql = `
                SELECT * FROM benchmarks 
                ORDER BY timestamp DESC 
                LIMIT ?
            `;

            this.db.all(sql, [limit], (err, rows) => {
                if (err) {
                    reject(err);
                } else {
                    resolve(rows);
                }
            });
        });
    }

    async getAverageMetrics(days = 7) {
        return new Promise((resolve, reject) => {
            const sql = `
                SELECT 
                    resolution,
                    COUNT(*) as test_count,
                    AVG(avg_fps) as avg_fps,
                    AVG(avg_latency) as avg_latency,
                    AVG(avg_bitrate) as avg_bitrate,
                    AVG(jitter) as avg_jitter,
                    AVG(packet_loss) as avg_packet_loss,
                    AVG(quality_score) as avg_quality_score,
                    MIN(quality_score) as min_quality_score,
                    MAX(quality_score) as max_quality_score
                FROM benchmarks
                WHERE timestamp >= datetime('now', '-${days} days')
                GROUP BY resolution
            `;

            this.db.all(sql, (err, rows) => {
                if (err) {
                    reject(err);
                } else {
                    resolve(rows);
                }
            });
        });
    }

    async generateMarkdownReport() {
        const latest = await this.getLatestBenchmarks(20);
        const averages = await this.getAverageMetrics(7);
        
        let markdown = `# WebRTC Benchmark Performance Report

## Executive Summary
Generated: ${new Date().toISOString()}

## 7-Day Performance Averages

| Resolution | Tests | Avg FPS | Avg Latency | Avg Bitrate | Avg Jitter | Packet Loss | Quality Score |
|------------|-------|---------|-------------|-------------|------------|-------------|---------------|
`;

        for (const avg of averages) {
            markdown += `| ${avg.resolution.toUpperCase()} | ${avg.test_count} | ${avg.avg_fps.toFixed(1)} | ${avg.avg_latency.toFixed(1)}ms | ${avg.avg_bitrate.toFixed(1)} Mbps | ${avg.avg_jitter.toFixed(1)}ms | ${avg.avg_packet_loss.toFixed(2)}% | ${avg.avg_quality_score.toFixed(0)}/100 |\n`;
        }

        markdown += `
## Recent Test Results (Last 20)

| Timestamp | Resolution | FPS | Latency | Bitrate | Jitter | Loss | Score |
|-----------|------------|-----|---------|---------|--------|------|-------|
`;

        for (const test of latest) {
            const timestamp = new Date(test.timestamp).toLocaleString();
            markdown += `| ${timestamp} | ${test.resolution} | ${test.avg_fps} | ${test.avg_latency}ms | ${test.avg_bitrate} Mbps | ${test.jitter}ms | ${test.packet_loss}% | ${test.quality_score}/100 |\n`;
        }

        markdown += `
## Quality Trends

### Performance by Resolution (7 days)
`;

        for (const avg of averages) {
            const scoreBar = '█'.repeat(Math.round(avg.avg_quality_score / 10));
            const emptyBar = '░'.repeat(10 - Math.round(avg.avg_quality_score / 10));
            markdown += `
**${avg.resolution.toUpperCase()}**
- Quality Score: ${scoreBar}${emptyBar} ${avg.avg_quality_score.toFixed(0)}/100
- Range: ${avg.min_quality_score} - ${avg.max_quality_score}
- Tests Run: ${avg.test_count}
`;
        }

        markdown += `
## Recommendations

`;

        // Generate recommendations based on data
        for (const avg of averages) {
            if (avg.avg_quality_score < 80) {
                markdown += `- **${avg.resolution.toUpperCase()}**: Quality score below 80. Consider optimization.\n`;
            }
            if (avg.avg_packet_loss > 1) {
                markdown += `- **${avg.resolution.toUpperCase()}**: High packet loss detected (${avg.avg_packet_loss.toFixed(2)}%).\n`;
            }
            if (avg.avg_latency > 50) {
                markdown += `- **${avg.resolution.toUpperCase()}**: Latency exceeds target (${avg.avg_latency.toFixed(1)}ms).\n`;
            }
        }

        markdown += `
---
*Report generated automatically by benchmark-db.js*
`;

        return markdown;
    }

    async generateHTMLReport() {
        const latest = await this.getLatestBenchmarks(50);
        const averages = await this.getAverageMetrics(30);
        
        const html = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WebRTC Benchmark Report - ${new Date().toLocaleDateString()}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
        }
        .container {
            background: white;
            border-radius: 10px;
            padding: 30px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.1);
        }
        h1 {
            color: #667eea;
            border-bottom: 3px solid #667eea;
            padding-bottom: 10px;
        }
        h2 {
            color: #764ba2;
            margin-top: 30px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin: 20px 0;
        }
        th {
            background: linear-gradient(135deg, #667eea, #764ba2);
            color: white;
            padding: 12px;
            text-align: left;
        }
        td {
            padding: 10px;
            border-bottom: 1px solid #ddd;
        }
        tr:hover {
            background-color: #f5f5f5;
        }
        .metric-card {
            display: inline-block;
            background: linear-gradient(135deg, #667eea, #764ba2);
            color: white;
            padding: 20px;
            border-radius: 10px;
            margin: 10px;
            min-width: 200px;
            text-align: center;
        }
        .metric-value {
            font-size: 2em;
            font-weight: bold;
        }
        .metric-label {
            font-size: 0.9em;
            opacity: 0.9;
        }
        .chart {
            width: 100%;
            height: 300px;
            margin: 20px 0;
            border: 1px solid #ddd;
            border-radius: 5px;
            padding: 10px;
        }
        .quality-bar {
            display: inline-block;
            height: 20px;
            background: linear-gradient(90deg, #4ade80, #22d3ee);
            border-radius: 10px;
            margin-right: 10px;
        }
        .timestamp {
            color: #666;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 WebRTC Performance Benchmark Report</h1>
        <p class="timestamp">Generated: ${new Date().toISOString()}</p>
        
        <h2>📊 30-Day Overview</h2>
        <div>
            ${averages.map(avg => `
                <div class="metric-card">
                    <div class="metric-label">${avg.resolution.toUpperCase()}</div>
                    <div class="metric-value">${avg.avg_quality_score.toFixed(0)}/100</div>
                    <div class="metric-label">${avg.test_count} tests</div>
                </div>
            `).join('')}
        </div>

        <h2>📈 Performance Metrics</h2>
        <table>
            <thead>
                <tr>
                    <th>Resolution</th>
                    <th>Tests</th>
                    <th>Avg FPS</th>
                    <th>Latency</th>
                    <th>Bitrate</th>
                    <th>Jitter</th>
                    <th>Packet Loss</th>
                    <th>Quality Score</th>
                </tr>
            </thead>
            <tbody>
                ${averages.map(avg => `
                    <tr>
                        <td><strong>${avg.resolution.toUpperCase()}</strong></td>
                        <td>${avg.test_count}</td>
                        <td>${avg.avg_fps.toFixed(1)}</td>
                        <td>${avg.avg_latency.toFixed(1)}ms</td>
                        <td>${avg.avg_bitrate.toFixed(1)} Mbps</td>
                        <td>${avg.avg_jitter.toFixed(1)}ms</td>
                        <td>${avg.avg_packet_loss.toFixed(2)}%</td>
                        <td>
                            <span class="quality-bar" style="width: ${avg.avg_quality_score}px"></span>
                            ${avg.avg_quality_score.toFixed(0)}/100
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>

        <h2>📋 Recent Tests</h2>
        <table>
            <thead>
                <tr>
                    <th>Timestamp</th>
                    <th>Resolution</th>
                    <th>FPS</th>
                    <th>Latency</th>
                    <th>Score</th>
                </tr>
            </thead>
            <tbody>
                ${latest.slice(0, 10).map(test => `
                    <tr>
                        <td>${new Date(test.timestamp).toLocaleString()}</td>
                        <td>${test.resolution.toUpperCase()}</td>
                        <td>${test.avg_fps}</td>
                        <td>${test.avg_latency}ms</td>
                        <td>${test.quality_score}/100</td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    </div>
</body>
</html>`;

        return html;
    }

    close() {
        return new Promise((resolve, reject) => {
            this.db.close((err) => {
                if (err) {
                    reject(err);
                } else {
                    console.log('Database connection closed');
                    resolve();
                }
            });
        });
    }
}

// CLI interface
async function main() {
    const args = process.argv.slice(2);
    const command = args[0];
    
    const db = new BenchmarkDatabase();
    await db.init();

    try {
        switch(command) {
            case 'import':
                // Import from JSON file
                if (!args[1]) {
                    console.error('Usage: node benchmark-db.js import <file.json>');
                    process.exit(1);
                }
                const data = JSON.parse(await fs.readFile(args[1], 'utf-8'));
                await db.insertBenchmark(data);
                console.log('Benchmark data imported successfully');
                break;

            case 'report':
                // Generate markdown report
                const markdown = await db.generateMarkdownReport();
                const reportFile = `benchmark-report-${new Date().toISOString().split('T')[0]}.md`;
                await fs.writeFile(reportFile, markdown);
                console.log(`Report saved to ${reportFile}`);
                break;

            case 'html':
                // Generate HTML report
                const html = await db.generateHTMLReport();
                const htmlFile = `benchmark-report-${new Date().toISOString().split('T')[0]}.html`;
                await fs.writeFile(htmlFile, html);
                console.log(`HTML report saved to ${htmlFile}`);
                break;

            case 'latest':
                // Show latest results
                const latest = await db.getLatestBenchmarks(5);
                console.log('\nLatest Benchmark Results:');
                console.log('========================');
                for (const test of latest) {
                    console.log(`\n[${new Date(test.timestamp).toLocaleString()}]`);
                    console.log(`Resolution: ${test.resolution.toUpperCase()}`);
                    console.log(`Quality Score: ${test.quality_score}/100`);
                    console.log(`FPS: ${test.avg_fps} | Latency: ${test.avg_latency}ms | Bitrate: ${test.avg_bitrate} Mbps`);
                }
                break;

            case 'stats':
                // Show statistics
                const stats = await db.getAverageMetrics(30);
                console.log('\n30-Day Statistics:');
                console.log('==================');
                for (const stat of stats) {
                    console.log(`\n${stat.resolution.toUpperCase()}:`);
                    console.log(`  Tests: ${stat.test_count}`);
                    console.log(`  Avg Quality: ${stat.avg_quality_score.toFixed(0)}/100`);
                    console.log(`  Avg FPS: ${stat.avg_fps.toFixed(1)}`);
                    console.log(`  Avg Latency: ${stat.avg_latency.toFixed(1)}ms`);
                }
                break;

            default:
                console.log(`
Usage: node benchmark-db.js <command> [options]

Commands:
  import <file.json>  Import benchmark results from JSON file
  report             Generate markdown report
  html               Generate HTML report  
  latest             Show latest benchmark results
  stats              Show statistics

Examples:
  node benchmark-db.js import benchmark-2024-01-01.json
  node benchmark-db.js report
  node benchmark-db.js latest
`);
        }
    } catch (error) {
        console.error('Error:', error.message);
        process.exit(1);
    } finally {
        await db.close();
    }
}

// Run if called directly
if (require.main === module) {
    main().catch(console.error);
}

module.exports = BenchmarkDatabase;