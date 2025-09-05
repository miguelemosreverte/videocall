// Client-side quad-tree video encoder
// TODO: Add WebGL/hardware acceleration for better performance

class QuadTreeNode {
    constructor(x, y, width, height, depth = 0, maxDepth = 5) {
        this.x = x;
        this.y = y;
        this.width = width;
        this.height = height;
        this.depth = depth;
        this.maxDepth = maxDepth;
        this.children = null;
        this.isDirty = false;
        this.avgColor = null;
        this.variance = 0;
    }

    subdivide() {
        if (this.depth >= this.maxDepth || this.width <= 8 || this.height <= 8) {
            return false;
        }

        const halfW = Math.floor(this.width / 2);
        const halfH = Math.floor(this.height / 2);

        this.children = [
            new QuadTreeNode(this.x, this.y, halfW, halfH, this.depth + 1, this.maxDepth),
            new QuadTreeNode(this.x + halfW, this.y, this.width - halfW, halfH, this.depth + 1, this.maxDepth),
            new QuadTreeNode(this.x, this.y + halfH, halfW, this.height - halfH, this.depth + 1, this.maxDepth),
            new QuadTreeNode(this.x + halfW, this.y + halfH, this.width - halfW, this.height - halfH, this.depth + 1, this.maxDepth)
        ];
        return true;
    }

    analyze(currentData, previousData, threshold = 25) {
        let r = 0, g = 0, b = 0;
        let count = 0;
        let hasChange = false;
        let maxDiff = 0;

        const stride = currentData.width * 4;
        
        for (let y = 0; y < this.height; y += 2) { // Sample every 2 pixels for speed
            for (let x = 0; x < this.width; x += 2) {
                const idx = (this.y + y) * stride + (this.x + x) * 4;
                r += currentData.data[idx];
                g += currentData.data[idx + 1];
                b += currentData.data[idx + 2];
                count++;

                if (previousData) {
                    const diff = Math.abs(currentData.data[idx] - previousData.data[idx]) +
                                Math.abs(currentData.data[idx + 1] - previousData.data[idx + 1]) +
                                Math.abs(currentData.data[idx + 2] - previousData.data[idx + 2]);
                    maxDiff = Math.max(maxDiff, diff);
                    if (diff > threshold) {
                        hasChange = true;
                    }
                }
            }
        }

        this.avgColor = {
            r: Math.round(r / count),
            g: Math.round(g / count),
            b: Math.round(b / count)
        };

        // Calculate variance
        let variance = 0;
        for (let y = 0; y < this.height; y += 4) { // Sample for speed
            for (let x = 0; x < this.width; x += 4) {
                const idx = (this.y + y) * stride + (this.x + x) * 4;
                variance += Math.abs(currentData.data[idx] - this.avgColor.r);
                variance += Math.abs(currentData.data[idx + 1] - this.avgColor.g);
                variance += Math.abs(currentData.data[idx + 2] - this.avgColor.b);
            }
        }
        this.variance = variance / (count * 3);
        this.isDirty = hasChange;

        // Subdivide if high variance and changes detected
        if (this.variance > 30 && hasChange && maxDiff > 50 && this.subdivide()) {
            this.children.forEach(child => {
                child.analyze(currentData, previousData, threshold);
            });
        }

        return hasChange;
    }

    getDirtyRegions() {
        const regions = [];
        
        if (!this.isDirty) {
            return regions;
        }

        if (!this.children) {
            regions.push({
                x: this.x,
                y: this.y,
                w: this.width,
                h: this.height,
                c: (this.avgColor.r << 16) | (this.avgColor.g << 8) | this.avgColor.b // Pack color as int
            });
        } else {
            this.children.forEach(child => {
                regions.push(...child.getDirtyRegions());
            });
        }

        return regions;
    }
}

class QuadTreeEncoder {
    constructor(video, canvas) {
        this.video = video;
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d', { 
            willReadFrequently: true,
            alpha: false 
        });
        
        this.previousFrame = null;
        this.frameCount = 0;
        this.keyFrameInterval = 120; // Send keyframe every 2 seconds at 60fps
        
        // Audio handling with priority
        this.audioContext = null;
        this.audioSource = null;
        this.audioProcessor = null;
        this.audioBuffer = [];
        
        // Performance monitoring
        this.stats = {
            fps: 0,
            bandwidth: 0,
            audioDropped: 0,
            quality: 'high'
        };
    }

    async initAudio(stream) {
        this.audioContext = new (window.AudioContext || window.webkitAudioContext)({ 
            sampleRate: 48000,
            latencyHint: 'interactive'
        });
        
        this.audioSource = this.audioContext.createMediaStreamSource(stream);
        this.audioProcessor = this.audioContext.createScriptProcessor(2048, 1, 1);
        
        this.audioProcessor.onaudioprocess = (e) => {
            const inputData = e.inputBuffer.getChannelData(0);
            const pcmData = new Int16Array(inputData.length);
            
            for (let i = 0; i < inputData.length; i++) {
                pcmData[i] = Math.max(-32768, Math.min(32767, inputData[i] * 32768));
            }
            
            this.audioBuffer.push(pcmData);
            
            // Keep buffer size manageable
            if (this.audioBuffer.length > 10) {
                this.audioBuffer.shift();
                this.stats.audioDropped++;
            }
        };
        
        this.audioSource.connect(this.audioProcessor);
        this.audioProcessor.connect(this.audioContext.destination);
    }

    encodeFrame() {
        this.frameCount++;
        
        // Capture current frame
        this.ctx.drawImage(this.video, 0, 0, this.canvas.width, this.canvas.height);
        const currentFrame = this.ctx.getImageData(0, 0, this.canvas.width, this.canvas.height);
        
        const packet = {
            t: 'delta', // type
            f: this.frameCount, // frame number
            ts: Date.now(), // timestamp
            a: null, // audio
            d: null, // video data for compatibility
            q: 'high' // quality
        };

        // Always include audio if available (PRIORITY)
        if (this.audioBuffer.length > 0) {
            const audioData = this.audioBuffer.shift();
            packet.a = {
                d: btoa(String.fromCharCode.apply(null, new Uint8Array(audioData.buffer))),
                s: audioData.length
            };
        }

        // Check if we need a keyframe
        if (this.frameCount % this.keyFrameInterval === 1 || !this.previousFrame) {
            packet.t = 'key';
            packet.d = this.canvas.toDataURL('image/jpeg', 0.5).split(',')[1];
        } else {
            // Build quad-tree for delta encoding
            const tree = new QuadTreeNode(0, 0, this.canvas.width, this.canvas.height);
            const hasChanges = tree.analyze(currentFrame, this.previousFrame);
            
            if (hasChanges) {
                const regions = tree.getDirtyRegions();
                
                // Adaptive quality based on region count
                if (regions.length > 500) {
                    packet.q = 'low';
                    // Merge small regions
                    packet.d = this.mergeRegions(regions, 64);
                } else if (regions.length > 200) {
                    packet.q = 'medium';
                    packet.d = this.mergeRegions(regions, 32);
                } else {
                    packet.d = regions;
                }
                
                // Width and height are known from canvas size
            }
        }
        
        this.previousFrame = currentFrame;
        this.stats.quality = packet.q;
        
        return packet;
    }

    mergeRegions(regions, gridSize) {
        const grid = new Map();
        
        regions.forEach(r => {
            const gx = Math.floor(r.x / gridSize);
            const gy = Math.floor(r.y / gridSize);
            const key = `${gx},${gy}`;
            
            if (!grid.has(key)) {
                grid.set(key, {
                    x: gx * gridSize,
                    y: gy * gridSize,
                    w: gridSize,
                    h: gridSize,
                    colors: []
                });
            }
            grid.get(key).colors.push(r.c);
        });
        
        const merged = [];
        grid.forEach(cell => {
            // Average colors
            const avgColor = Math.floor(cell.colors.reduce((a, b) => a + b) / cell.colors.length);
            merged.push({
                x: cell.x,
                y: cell.y,
                w: cell.w,
                h: cell.h,
                c: avgColor
            });
        });
        
        return merged;
    }

    getStats() {
        return this.stats;
    }
}

// Decoder for receiving side
class QuadTreeDecoder {
    constructor(canvas) {
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d', { alpha: false });
        this.frameCount = 0;
        
        // Audio playback
        this.audioContext = null;
        this.audioQueue = [];
        this.audioPlaying = false;
    }

    async initAudio() {
        this.audioContext = new (window.AudioContext || window.webkitAudioContext)({ 
            sampleRate: 48000,
            latencyHint: 'interactive'
        });
        
        // Start audio playback loop
        this.startAudioPlayback();
    }

    startAudioPlayback() {
        const processAudio = () => {
            if (this.audioQueue.length > 0 && !this.audioPlaying) {
                this.audioPlaying = true;
                const audioData = this.audioQueue.shift();
                
                const buffer = this.audioContext.createBuffer(1, audioData.length, 48000);
                const channel = buffer.getChannelData(0);
                
                for (let i = 0; i < audioData.length; i++) {
                    channel[i] = audioData[i] / 32768;
                }
                
                const source = this.audioContext.createBufferSource();
                source.buffer = buffer;
                source.connect(this.audioContext.destination);
                
                source.onended = () => {
                    this.audioPlaying = false;
                };
                
                source.start();
            }
            
            requestAnimationFrame(processAudio);
        };
        
        // Actually start the loop!
        processAudio();
    }

    decodePacket(packet) {
        this.frameCount++;
        
        // Handle audio immediately (PRIORITY)
        if (packet.a && packet.a.d) {
            const binaryString = atob(packet.a.d);
            const bytes = new Uint8Array(binaryString.length);
            for (let i = 0; i < binaryString.length; i++) {
                bytes[i] = binaryString.charCodeAt(i);
            }
            const audioData = new Int16Array(bytes.buffer);
            this.audioQueue.push(audioData);
        }
        
        // Handle video
        if (packet.t === 'key' && packet.d) {
            // Decode keyframe - draw immediately to persist
            const img = new Image();
            img.src = 'data:image/jpeg;base64,' + packet.d;
            // Draw synchronously once loaded
            img.onload = () => {
                this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
                this.ctx.drawImage(img, 0, 0, this.canvas.width, this.canvas.height);
                // Store the keyframe for persistence
                this.lastKeyFrame = this.ctx.getImageData(0, 0, this.canvas.width, this.canvas.height);
            };
        } else if (packet.t === 'delta' && packet.d) {
            // Only apply deltas if we have a keyframe
            if (!this.lastKeyFrame) return;
            
            // Restore last keyframe first
            this.ctx.putImageData(this.lastKeyFrame, 0, 0);
            
            // Apply delta regions on top
            if (Array.isArray(packet.d)) {
                packet.d.forEach(region => {
                    if (region.x !== undefined) {
                        // Object format {x, y, w, h, c}
                        const r = (region.c >> 16) & 0xFF;
                        const g = (region.c >> 8) & 0xFF;
                        const b = region.c & 0xFF;
                        this.ctx.fillStyle = `rgb(${r},${g},${b})`;
                        this.ctx.fillRect(region.x, region.y, region.w, region.h);
                    } else if (Array.isArray(region)) {
                        // Array format [x, y, w, h, color]
                        const [x, y, w, h, color] = region;
                        this.ctx.fillStyle = color;
                        this.ctx.fillRect(x, y, w, h);
                    }
                });
            }
        }
        
        return {
            frame: packet.f,
            quality: packet.q,
            hasAudio: !!packet.a
        };
    }
}

// Export for use in main conference
window.QuadTreeEncoder = QuadTreeEncoder;
window.QuadTreeDecoder = QuadTreeDecoder;