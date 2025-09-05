#!/usr/bin/env node

const WebSocket = require('ws');
const sqlite3 = require('sqlite3').verbose();
const fs = require('fs');
const path = require('path');

// Configuration
const CONFIG = {
    serverUrl: 'wss://95.217.238.72.nip.io',
    testDuration: 5000, // 5 seconds
    numClients: 10,
    messageSize: 1024, // 1KB messages
    messageInterval: 10, // Send message every 10ms (100 msg/sec per client)
    dbPath: path.join(__dirname, 'benchmark-results.db')
};

// Test metrics
class Metrics {
    constructor() {
        this.messagesSent = 0;
        this.messagesReceived = 0;
        this.bytesReceived = 0;
        this.bytesSent = 0;
        this.latencies = [];
        this.errors = 0;
        this.connectTime = [];
        this.startTime = null;
        this.endTime = null;
    }

    addLatency(latency) {
        this.latencies.push(latency);
    }

    getStats() {
        const duration = (this.endTime - this.startTime) / 1000; // seconds
        const avgLatency = this.latencies.length > 0 
            ? this.latencies.reduce((a, b) => a + b, 0) / this.latencies.length 
            : 0;
        const minLatency = this.latencies.length > 0 ? Math.min(...this.latencies) : 0;
        const maxLatency = this.latencies.length > 0 ? Math.max(...this.latencies) : 0;
        const p99Latency = this.latencies.length > 0 
            ? this.latencies.sort((a, b) => a - b)[Math.floor(this.latencies.length * 0.99)]
            : 0;

        return {
            duration,
            messagesSent: this.messagesSent,
            messagesReceived: this.messagesReceived,
            messagesPerSecond: this.messagesReceived / duration,
            bytesSent: this.bytesSent,
            bytesReceived: this.bytesReceived,
            bandwidthMbps: (this.bytesReceived * 8) / (duration * 1000000),
            avgLatencyMs: avgLatency,
            minLatencyMs: minLatency,
            maxLatencyMs: maxLatency,
            p99LatencyMs: p99Latency,
            errors: this.errors,
            avgConnectTime: this.connectTime.length > 0
                ? this.connectTime.reduce((a, b) => a + b, 0) / this.connectTime.length
                : 0
        };
    }
}

// Database setup
class Database {
    constructor(dbPath) {
        this.db = new sqlite3.Database(dbPath);
        this.setupSchema();
    }

    setupSchema() {
        const schema = `
            CREATE TABLE IF NOT EXISTS test_runs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
                test_type TEXT,
                num_clients INTEGER,
                message_size INTEGER,
                duration_seconds REAL,
                messages_sent INTEGER,
                messages_received INTEGER,
                messages_per_second REAL,
                bytes_sent INTEGER,
                bytes_received INTEGER,
                bandwidth_mbps REAL,
                avg_latency_ms REAL,
                min_latency_ms REAL,
                max_latency_ms REAL,
                p99_latency_ms REAL,
                errors INTEGER,
                avg_connect_time_ms REAL,
                server_url TEXT
            );

            CREATE INDEX IF NOT EXISTS idx_timestamp ON test_runs(timestamp);
            CREATE INDEX IF NOT EXISTS idx_test_type ON test_runs(test_type);
        `;

        this.db.exec(schema, (err) => {
            if (err) console.error('Schema creation error:', err);
        });
    }

    saveResults(testType, config, stats) {
        return new Promise((resolve, reject) => {
            const sql = `
                INSERT INTO test_runs (
                    test_type, num_clients, message_size, duration_seconds,
                    messages_sent, messages_received, messages_per_second,
                    bytes_sent, bytes_received, bandwidth_mbps,
                    avg_latency_ms, min_latency_ms, max_latency_ms, p99_latency_ms,
                    errors, avg_connect_time_ms, server_url
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            `;

            this.db.run(sql, [
                testType,
                config.numClients,
                config.messageSize,
                stats.duration,
                stats.messagesSent,
                stats.messagesReceived,
                stats.messagesPerSecond,
                stats.bytesSent,
                stats.bytesReceived,
                stats.bandwidthMbps,
                stats.avgLatencyMs,
                stats.minLatencyMs,
                stats.maxLatencyMs,
                stats.p99LatencyMs,
                stats.errors,
                stats.avgConnectTime,
                config.serverUrl
            ], function(err) {
                if (err) reject(err);
                else resolve(this.lastID);
            });
        });
    }

    getRecentResults(limit = 10) {
        return new Promise((resolve, reject) => {
            const sql = `
                SELECT * FROM test_runs 
                ORDER BY timestamp DESC 
                LIMIT ?
            `;
            this.db.all(sql, [limit], (err, rows) => {
                if (err) reject(err);
                else resolve(rows);
            });
        });
    }

    close() {
        this.db.close();
    }
}

// Test client
class TestClient {
    constructor(id, config, metrics) {
        this.id = id;
        this.username = `test-client-${id}-${Date.now()}`;
        this.config = config;
        this.metrics = metrics;
        this.ws = null;
        this.messageInterval = null;
        this.messageCounter = 0;
    }

    async connect() {
        return new Promise((resolve, reject) => {
            const connectStart = Date.now();
            const wsUrl = `${this.config.serverUrl}/ws/${this.username}`;
            
            this.ws = new WebSocket(wsUrl);
            
            this.ws.on('open', () => {
                this.metrics.connectTime.push(Date.now() - connectStart);
                resolve();
            });

            this.ws.on('message', (data) => {
                try {
                    const message = JSON.parse(data.toString());
                    if (message.timestamp) {
                        const latency = Date.now() - message.timestamp;
                        this.metrics.addLatency(latency);
                        this.metrics.messagesReceived++;
                        this.metrics.bytesReceived += data.length;
                    }
                } catch (e) {
                    // Not a test message
                }
            });

            this.ws.on('error', (err) => {
                this.metrics.errors++;
                console.error(`Client ${this.id} error:`, err.message);
            });

            this.ws.on('close', () => {
                if (this.messageInterval) {
                    clearInterval(this.messageInterval);
                }
            });

            setTimeout(() => reject(new Error('Connection timeout')), 5000);
        });
    }

    startSending() {
        const payload = 'x'.repeat(this.config.messageSize - 100); // Leave room for metadata
        
        this.messageInterval = setInterval(() => {
            if (this.ws.readyState === WebSocket.OPEN) {
                const message = JSON.stringify({
                    id: this.messageCounter++,
                    from: this.username,
                    timestamp: Date.now(),
                    payload: payload
                });
                
                this.ws.send(message);
                this.metrics.messagesSent++;
                this.metrics.bytesSent += message.length;
            }
        }, this.config.messageInterval);
    }

    disconnect() {
        if (this.messageInterval) {
            clearInterval(this.messageInterval);
        }
        if (this.ws) {
            this.ws.close();
        }
    }
}

// Report generator
class ReportGenerator {
    static generateMarkdown(stats, config, historicalData) {
        let report = `# WebSocket Relay Benchmark Report\n\n`;
        report += `**Date:** ${new Date().toISOString()}\n`;
        report += `**Server:** ${config.serverUrl}\n\n`;
        
        report += `## Test Configuration\n`;
        report += `- **Clients:** ${config.numClients}\n`;
        report += `- **Message Size:** ${config.messageSize} bytes\n`;
        report += `- **Test Duration:** ${config.testDuration / 1000} seconds\n`;
        report += `- **Message Rate:** ${1000 / config.messageInterval} msg/sec per client\n\n`;
        
        report += `## Results\n`;
        report += `| Metric | Value |\n`;
        report += `|--------|-------|\n`;
        report += `| Messages Sent | ${stats.messagesSent.toLocaleString()} |\n`;
        report += `| Messages Received | ${stats.messagesReceived.toLocaleString()} |\n`;
        report += `| Throughput | ${stats.messagesPerSecond.toFixed(2)} msg/sec |\n`;
        report += `| Bandwidth | ${stats.bandwidthMbps.toFixed(2)} Mbps |\n`;
        report += `| Avg Latency | ${stats.avgLatencyMs.toFixed(2)} ms |\n`;
        report += `| Min Latency | ${stats.minLatencyMs.toFixed(2)} ms |\n`;
        report += `| Max Latency | ${stats.maxLatencyMs.toFixed(2)} ms |\n`;
        report += `| P99 Latency | ${stats.p99LatencyMs.toFixed(2)} ms |\n`;
        report += `| Errors | ${stats.errors} |\n`;
        report += `| Avg Connect Time | ${stats.avgConnectTime.toFixed(2)} ms |\n\n`;
        
        if (historicalData && historicalData.length > 0) {
            report += `## Historical Comparison\n`;
            report += `| Timestamp | Clients | Throughput | Avg Latency | Bandwidth |\n`;
            report += `|-----------|---------|------------|-------------|------------|\n`;
            
            historicalData.forEach(row => {
                report += `| ${new Date(row.timestamp).toLocaleString()} `;
                report += `| ${row.num_clients} `;
                report += `| ${row.messages_per_second.toFixed(0)} msg/s `;
                report += `| ${row.avg_latency_ms.toFixed(2)} ms `;
                report += `| ${row.bandwidth_mbps.toFixed(2)} Mbps |\n`;
            });
        }
        
        return report;
    }

    static generateHTML(stats, config, historicalData) {
        let html = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WebSocket Relay Benchmark Report</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 10px;
            margin-bottom: 30px;
        }
        h1 { margin: 0 0 10px 0; }
        .subtitle { opacity: 0.9; }
        .card {
            background: white;
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        .metrics {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-top: 20px;
        }
        .metric {
            text-align: center;
            padding: 15px;
            background: #f8f9fa;
            border-radius: 8px;
        }
        .metric-value {
            font-size: 24px;
            font-weight: bold;
            color: #667eea;
            margin: 10px 0;
        }
        .metric-label {
            font-size: 12px;
            text-transform: uppercase;
            color: #666;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        th, td {
            padding: 10px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background: #f8f9fa;
            font-weight: 600;
        }
        .success { color: #28a745; }
        .warning { color: #ffc107; }
        .danger { color: #dc3545; }
    </style>
</head>
<body>
    <div class="header">
        <h1>WebSocket Relay Benchmark Report</h1>
        <div class="subtitle">${new Date().toLocaleString()}</div>
        <div class="subtitle">${config.serverUrl}</div>
    </div>

    <div class="card">
        <h2>Test Configuration</h2>
        <div class="metrics">
            <div class="metric">
                <div class="metric-label">Clients</div>
                <div class="metric-value">${config.numClients}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Message Size</div>
                <div class="metric-value">${config.messageSize} B</div>
            </div>
            <div class="metric">
                <div class="metric-label">Duration</div>
                <div class="metric-value">${config.testDuration / 1000}s</div>
            </div>
            <div class="metric">
                <div class="metric-label">Rate per Client</div>
                <div class="metric-value">${1000 / config.messageInterval}/s</div>
            </div>
        </div>
    </div>

    <div class="card">
        <h2>Performance Metrics</h2>
        <div class="metrics">
            <div class="metric">
                <div class="metric-label">Messages Sent</div>
                <div class="metric-value">${stats.messagesSent.toLocaleString()}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Messages Received</div>
                <div class="metric-value">${stats.messagesReceived.toLocaleString()}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Throughput</div>
                <div class="metric-value">${stats.messagesPerSecond.toFixed(0)}/s</div>
            </div>
            <div class="metric">
                <div class="metric-label">Bandwidth</div>
                <div class="metric-value">${stats.bandwidthMbps.toFixed(2)} Mbps</div>
            </div>
        </div>
    </div>

    <div class="card">
        <h2>Latency Analysis</h2>
        <div class="metrics">
            <div class="metric">
                <div class="metric-label">Average</div>
                <div class="metric-value ${stats.avgLatencyMs < 50 ? 'success' : stats.avgLatencyMs < 100 ? 'warning' : 'danger'}">${stats.avgLatencyMs.toFixed(2)} ms</div>
            </div>
            <div class="metric">
                <div class="metric-label">Minimum</div>
                <div class="metric-value">${stats.minLatencyMs.toFixed(2)} ms</div>
            </div>
            <div class="metric">
                <div class="metric-label">Maximum</div>
                <div class="metric-value">${stats.maxLatencyMs.toFixed(2)} ms</div>
            </div>
            <div class="metric">
                <div class="metric-label">P99</div>
                <div class="metric-value">${stats.p99LatencyMs.toFixed(2)} ms</div>
            </div>
        </div>
    </div>`;

        if (historicalData && historicalData.length > 0) {
            html += `
    <div class="card">
        <h2>Historical Performance</h2>
        <table>
            <thead>
                <tr>
                    <th>Timestamp</th>
                    <th>Clients</th>
                    <th>Throughput</th>
                    <th>Avg Latency</th>
                    <th>Bandwidth</th>
                    <th>Errors</th>
                </tr>
            </thead>
            <tbody>`;
            
            historicalData.forEach(row => {
                html += `
                <tr>
                    <td>${new Date(row.timestamp).toLocaleString()}</td>
                    <td>${row.num_clients}</td>
                    <td>${row.messages_per_second.toFixed(0)} msg/s</td>
                    <td class="${row.avg_latency_ms < 50 ? 'success' : row.avg_latency_ms < 100 ? 'warning' : 'danger'}">${row.avg_latency_ms.toFixed(2)} ms</td>
                    <td>${row.bandwidth_mbps.toFixed(2)} Mbps</td>
                    <td class="${row.errors === 0 ? 'success' : 'danger'}">${row.errors}</td>
                </tr>`;
            });
            
            html += `
            </tbody>
        </table>
    </div>`;
        }

        html += `
</body>
</html>`;
        
        return html;
    }
}

// Main test runner
async function runBenchmark() {
    console.log('🚀 Starting WebSocket Relay Benchmark');
    console.log('=====================================\n');
    
    const metrics = new Metrics();
    const db = new Database(CONFIG.dbPath);
    const clients = [];
    
    try {
        // Create and connect clients
        console.log(`📡 Connecting ${CONFIG.numClients} clients...`);
        metrics.startTime = Date.now();
        
        for (let i = 0; i < CONFIG.numClients; i++) {
            const client = new TestClient(i, CONFIG, metrics);
            clients.push(client);
            await client.connect();
            console.log(`   Client ${i + 1}/${CONFIG.numClients} connected`);
        }
        
        console.log('\n📤 Starting message transmission...');
        
        // Start sending messages
        clients.forEach(client => client.startSending());
        
        // Run test for specified duration
        await new Promise(resolve => setTimeout(resolve, CONFIG.testDuration));
        
        metrics.endTime = Date.now();
        
        console.log('\n📊 Test completed, processing results...');
        
        // Disconnect all clients
        clients.forEach(client => client.disconnect());
        
        // Calculate statistics
        const stats = metrics.getStats();
        
        // Save to database
        await db.saveResults('text-chat', CONFIG, stats);
        
        // Get historical data
        const historicalData = await db.getRecentResults(10);
        
        // Generate reports
        const markdownReport = ReportGenerator.generateMarkdown(stats, CONFIG, historicalData);
        const htmlReport = ReportGenerator.generateHTML(stats, CONFIG, historicalData);
        
        // Save reports
        fs.writeFileSync(path.join(__dirname, 'benchmark-report.md'), markdownReport);
        fs.writeFileSync(path.join(__dirname, 'benchmark-report.html'), htmlReport);
        
        // Print summary
        console.log('\n📈 RESULTS SUMMARY');
        console.log('==================');
        console.log(`Messages: ${stats.messagesSent} sent, ${stats.messagesReceived} received`);
        console.log(`Throughput: ${stats.messagesPerSecond.toFixed(2)} msg/sec`);
        console.log(`Bandwidth: ${stats.bandwidthMbps.toFixed(2)} Mbps`);
        console.log(`Latency: ${stats.avgLatencyMs.toFixed(2)}ms avg (${stats.minLatencyMs.toFixed(2)}ms min, ${stats.maxLatencyMs.toFixed(2)}ms max)`);
        console.log(`P99 Latency: ${stats.p99LatencyMs.toFixed(2)}ms`);
        console.log(`Errors: ${stats.errors}`);
        
        console.log('\n✅ Reports saved:');
        console.log(`   - benchmark-report.md`);
        console.log(`   - benchmark-report.html`);
        console.log(`   - benchmark-results.db (SQLite database)`);
        
    } catch (error) {
        console.error('❌ Benchmark failed:', error);
    } finally {
        db.close();
        process.exit(0);
    }
}

// Run if executed directly
if (require.main === module) {
    runBenchmark();
}