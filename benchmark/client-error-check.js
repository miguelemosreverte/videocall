#!/usr/bin/env node

const https = require('https');
const vm = require('vm');

async function checkClientErrors() {
    console.log('🔍 CLIENT-SIDE ERROR CHECK');
    console.log('===========================\n');
    
    // 1. Fetch the HTML page
    console.log('1️⃣ Fetching HTML from GitHub Pages...');
    const html = await new Promise((resolve, reject) => {
        https.get('https://miguelemosreverte.github.io/videocall/', res => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => resolve(data));
        }).on('error', reject);
    });
    
    console.log('   ✅ HTML fetched (' + html.length + ' bytes)\n');
    
    // 2. Check if quadtree-client.js is loaded
    console.log('2️⃣ Checking script dependencies...');
    const hasQuadtreeScript = html.includes('quadtree-client.js');
    if (hasQuadtreeScript) {
        console.log('   ✅ quadtree-client.js is included');
    } else {
        console.log('   ❌ ERROR: quadtree-client.js NOT included!');
    }
    
    // 3. Fetch quadtree-client.js and check for errors
    console.log('\n3️⃣ Fetching quadtree-client.js...');
    const quadtreeJs = await new Promise((resolve, reject) => {
        https.get('https://miguelemosreverte.github.io/videocall/quadtree-client.js', res => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => resolve(data));
        }).on('error', reject);
    });
    
    // Check for syntax errors
    try {
        new vm.Script(quadtreeJs);
        console.log('   ✅ quadtree-client.js has valid syntax');
    } catch (e) {
        console.log('   ❌ SYNTAX ERROR in quadtree-client.js:', e.message);
    }
    
    // 4. Extract and analyze the inline JavaScript
    console.log('\n4️⃣ Analyzing inline JavaScript...');
    const scriptMatch = html.match(/<script>[\s\S]*?<\/script>/g);
    
    if (scriptMatch) {
        const inlineScript = scriptMatch[scriptMatch.length - 1]
            .replace('<script>', '')
            .replace('</script>', '');
        
        // Check for common issues
        const issues = [];
        
        // Check if encoder/decoder are properly initialized
        if (inlineScript.includes('encoder.encodeFrame()')) {
            if (!inlineScript.includes('encoder = new QuadTreeEncoder')) {
                issues.push('encoder.encodeFrame() called but encoder not initialized with QuadTreeEncoder');
            } else {
                console.log('   ✅ encoder properly initialized');
            }
        }
        
        if (inlineScript.includes('decoder.decodePacket')) {
            if (!inlineScript.includes('decoder = new QuadTreeDecoder')) {
                issues.push('decoder.decodePacket() called but decoder not initialized with QuadTreeDecoder');
            } else {
                console.log('   ✅ decoder properly initialized');
            }
        }
        
        // Check WebSocket URL
        const wsUrlMatch = inlineScript.match(/wss?:\/\/[^\s'"]+/);
        if (wsUrlMatch) {
            console.log('   ✅ WebSocket URL found:', wsUrlMatch[0]);
        } else {
            issues.push('No WebSocket URL found in script');
        }
        
        // Check for undefined variables
        const undefinedVars = [];
        
        // Common undefined variable pattern
        const varUsage = inlineScript.match(/\b([a-zA-Z_][a-zA-Z0-9_]*)\s*\(/g);
        if (varUsage) {
            varUsage.forEach(usage => {
                const varName = usage.replace('(', '').trim();
                // Check if it's a built-in or if it's defined
                if (!['console', 'document', 'window', 'setTimeout', 'setInterval', 
                     'clearInterval', 'clearTimeout', 'requestAnimationFrame',
                     'Promise', 'WebSocket', 'JSON', 'Date', 'Math', 'parseInt',
                     'btoa', 'atob', 'navigator'].includes(varName)) {
                    
                    // Check if it's defined in either script
                    const isDefined = inlineScript.includes(`function ${varName}`) ||
                                    inlineScript.includes(`const ${varName}`) ||
                                    inlineScript.includes(`let ${varName}`) ||
                                    inlineScript.includes(`var ${varName}`) ||
                                    quadtreeJs.includes(`class ${varName}`) ||
                                    quadtreeJs.includes(`window.${varName}`);
                    
                    if (!isDefined && !undefinedVars.includes(varName)) {
                        undefinedVars.push(varName);
                    }
                }
            });
        }
        
        if (undefinedVars.length > 0) {
            issues.push('Potentially undefined functions/variables: ' + undefinedVars.join(', '));
        }
        
        // Report issues
        if (issues.length > 0) {
            console.log('\n   ❌ ISSUES FOUND:');
            issues.forEach(issue => console.log('      - ' + issue));
        } else {
            console.log('   ✅ No obvious issues in inline script');
        }
    }
    
    // 5. Check WebSocket connectivity
    console.log('\n5️⃣ Testing WebSocket connection...');
    const WebSocket = require('ws');
    
    try {
        const ws = new WebSocket('wss://95.217.238.72.nip.io/ws', {
            rejectUnauthorized: false
        });
        
        await new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error('WebSocket connection timeout'));
            }, 5000);
            
            ws.on('open', () => {
                clearTimeout(timeout);
                console.log('   ✅ WebSocket connection successful');
                ws.close();
                resolve();
            });
            
            ws.on('error', (err) => {
                clearTimeout(timeout);
                reject(err);
            });
        });
    } catch (e) {
        console.log('   ❌ WebSocket ERROR:', e.message);
    }
    
    // Summary
    console.log('\n' + '='.repeat(50));
    console.log('SUMMARY:');
    console.log('If you see errors in the browser console that my tests');
    console.log('are not catching, they are likely related to:');
    console.log('1. Browser-specific APIs (getUserMedia, AudioContext)');
    console.log('2. Cross-origin restrictions');
    console.log('3. Mixed content warnings');
    console.log('4. Actual runtime errors when frames are processed');
    console.log('='.repeat(50));
}

checkClientErrors().catch(console.error);