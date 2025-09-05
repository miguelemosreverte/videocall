#!/usr/bin/env node

const puppeteer = require('puppeteer-core');
const chromium = require('chrome-aws-lambda');

async function testBrowserErrors() {
    console.log('🔍 BROWSER ERROR DETECTION TEST');
    console.log('================================\n');
    
    let browser;
    try {
        // Try to use local Chrome first
        const executablePath = process.platform === 'darwin' 
            ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
            : process.platform === 'win32'
            ? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
            : '/usr/bin/google-chrome';
            
        browser = await puppeteer.launch({
            headless: true,
            executablePath,
            args: [
                '--no-sandbox',
                '--disable-setuid-sandbox',
                '--use-fake-ui-for-media-stream',
                '--use-fake-device-for-media-stream'
            ]
        }).catch(async () => {
            // Fallback to chrome-aws-lambda
            return await puppeteer.launch({
                args: chromium.args,
                defaultViewport: chromium.defaultViewport,
                executablePath: await chromium.executablePath,
                headless: chromium.headless,
            });
        });
    } catch (e) {
        // If Puppeteer not available, use simpler test
        console.log('⚠️  Puppeteer not available, using Node.js simulation...\n');
        return await simulateClientErrors();
    }
    
    const page = await browser.newPage();
    
    // Capture console errors
    const errors = [];
    const warnings = [];
    const logs = [];
    
    page.on('console', msg => {
        const text = msg.text();
        if (msg.type() === 'error') {
            errors.push(text);
            console.log('❌ Browser Error:', text);
        } else if (msg.type() === 'warning') {
            warnings.push(text);
            console.log('⚠️  Browser Warning:', text);
        } else {
            logs.push(text);
            if (text.includes('failed') || text.includes('error')) {
                console.log('📝 Log:', text);
            }
        }
    });
    
    page.on('pageerror', error => {
        errors.push(error.message);
        console.log('❌ Page Error:', error.message);
    });
    
    // Navigate to the page
    console.log('Loading https://miguelemosreverte.github.io/videocall/ ...\n');
    
    try {
        await page.goto('https://miguelemosreverte.github.io/videocall/', {
            waitUntil: 'networkidle0',
            timeout: 10000
        });
    } catch (e) {
        console.log('⚠️  Page load timeout (expected for auto-connecting page)');
    }
    
    // Wait for any async errors
    await page.waitForTimeout(3000);
    
    // Check for specific issues
    const result = await page.evaluate(() => {
        const issues = [];
        
        // Check if QuadTreeEncoder exists
        if (typeof QuadTreeEncoder === 'undefined') {
            issues.push('QuadTreeEncoder is not defined');
        }
        
        // Check if QuadTreeDecoder exists
        if (typeof QuadTreeDecoder === 'undefined') {
            issues.push('QuadTreeDecoder is not defined');
        }
        
        // Check WebSocket state
        if (typeof ws !== 'undefined' && ws) {
            issues.push(`WebSocket state: ${ws.readyState} (0=CONNECTING, 1=OPEN, 2=CLOSING, 3=CLOSED)`);
        }
        
        return issues;
    }).catch(e => [`Evaluation error: ${e.message}`]);
    
    console.log('\n' + '='.repeat(50));
    console.log('BROWSER ERROR SUMMARY:');
    console.log('='.repeat(50));
    
    if (errors.length > 0) {
        console.log('\n❌ Errors found:', errors.length);
        errors.forEach((err, i) => console.log(`  ${i+1}. ${err}`));
    } else {
        console.log('\n✅ No JavaScript errors detected');
    }
    
    if (warnings.length > 0) {
        console.log('\n⚠️  Warnings:', warnings.length);
        warnings.slice(0, 5).forEach((warn, i) => console.log(`  ${i+1}. ${warn}`));
    }
    
    if (result.length > 0) {
        console.log('\n📋 Page state:');
        result.forEach(issue => console.log(`  - ${issue}`));
    }
    
    await browser.close();
}

// Fallback simulation without Puppeteer
async function simulateClientErrors() {
    console.log('🔍 SIMULATING CLIENT-SIDE ERROR CHECKS\n');
    
    const https = require('https');
    
    // Fetch the HTML
    const html = await new Promise((resolve, reject) => {
        https.get('https://miguelemosreverte.github.io/videocall/', res => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => resolve(data));
        }).on('error', reject);
    });
    
    // Check for common issues
    const issues = [];
    
    // Check if quadtree-client.js is included
    if (!html.includes('quadtree-client.js')) {
        issues.push('❌ quadtree-client.js not included in HTML');
    } else {
        console.log('✅ quadtree-client.js is included');
    }
    
    // Check WebSocket URL
    const wsMatch = html.match(/wss?:\/\/[^\s'"]+/g);
    if (wsMatch) {
        console.log('✅ WebSocket URL found:', wsMatch[0]);
        
        // Test WebSocket connection
        const WebSocket = require('ws');
        const ws = new WebSocket(wsMatch[0].replace('wss://', 'wss://'), {
            rejectUnauthorized: false
        });
        
        await new Promise((resolve) => {
            ws.on('open', () => {
                console.log('✅ WebSocket connection successful');
                ws.close();
                resolve();
            });
            
            ws.on('error', (err) => {
                issues.push(`❌ WebSocket error: ${err.message}`);
                resolve();
            });
            
            setTimeout(() => {
                issues.push('❌ WebSocket connection timeout');
                resolve();
            }, 5000);
        });
    }
    
    // Check for undefined variables that would cause errors
    if (html.includes('encoder.encodeFrame()') && !html.includes('encoder = new QuadTreeEncoder')) {
        issues.push('❌ encoder used before initialization');
    }
    
    if (html.includes('decoder.decodePacket') && !html.includes('decoder = new QuadTreeDecoder')) {
        issues.push('❌ decoder used before initialization');
    }
    
    console.log('\n' + '='.repeat(50));
    if (issues.length > 0) {
        console.log('❌ ISSUES FOUND:');
        issues.forEach(issue => console.log(`  ${issue}`));
    } else {
        console.log('✅ No obvious errors detected');
        console.log('\nNote: This is a limited check. Actual browser may have different errors.');
    }
}

// Run the test
if (require.main === module) {
    testBrowserErrors().catch(console.error).finally(() => process.exit(0));
}