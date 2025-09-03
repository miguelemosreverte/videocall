#!/bin/bash

# Deploy Video Streaming Application to VPS
# Run this AFTER setup-vps.sh

SERVER_IP="194.87.103.57"
SERVER_USER="root"

echo "==========================================="
echo "Deploying Video Streaming App to VPS"
echo "==========================================="

# Package the application
echo "Creating deployment package..."
cd /Users/miguel_lemos/Desktop/videocall

# Create deployment directory
mkdir -p deploy-package

# Copy WebGPU video call files
cp index-webgpu-call.html deploy-package/index.html
cp -r *.webp deploy-package/ 2>/dev/null || true

# Create improved index with VPS WebSocket connection
cat > deploy-package/index.html << 'EOF'
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>WebGPU Motion Vectors - Video Call</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        /* Your existing styles here */
    </style>
</head>
<body>
    <h1>Video Call with WebGPU Motion Vectors</h1>
    
    <!-- Your UI here -->
    
    <script>
        // VPS WebSocket connection
        const VPS_WS_URL = 'wss://194.87.103.57/ws';
        
        class VideoStreamClient {
            constructor(roomId, userId) {
                this.roomId = roomId;
                this.userId = userId;
                this.ws = null;
                this.connected = false;
            }
            
            connect() {
                const url = `${VPS_WS_URL}?room=${this.roomId}&id=${this.userId}`;
                this.ws = new WebSocket(url);
                
                this.ws.onopen = () => {
                    this.connected = true;
                    console.log('Connected to VPS WebSocket');
                    this.onConnect();
                };
                
                this.ws.onmessage = (event) => {
                    const msg = JSON.parse(event.data);
                    this.handleMessage(msg);
                };
                
                this.ws.onerror = (error) => {
                    console.error('WebSocket error:', error);
                };
                
                this.ws.onclose = () => {
                    this.connected = false;
                    console.log('Disconnected from VPS');
                    // Reconnect after 3 seconds
                    setTimeout(() => this.connect(), 3000);
                };
            }
            
            sendMotionEvents(events) {
                if (this.connected) {
                    this.ws.send(JSON.stringify({
                        type: 'motion_events',
                        data: events
                    }));
                }
            }
            
            handleMessage(msg) {
                switch(msg.type) {
                    case 'motion_events':
                        this.onMotionEvents(msg.from, msg.data);
                        break;
                    case 'joined':
                        this.onUserJoined(msg.data);
                        break;
                    case 'left':
                        this.onUserLeft(msg.data);
                        break;
                }
            }
            
            // Override these in your implementation
            onConnect() {}
            onMotionEvents(from, events) {}
            onUserJoined(data) {}
            onUserLeft(data) {}
        }
        
        // Initialize on page load
        let streamClient;
        
        window.addEventListener('load', () => {
            const roomId = 'room-' + window.location.hash.substr(1) || 'default';
            const userId = 'user-' + Math.random().toString(36).substr(2, 9);
            
            streamClient = new VideoStreamClient(roomId, userId);
            streamClient.connect();
            
            // Hook into your existing motion event detection
            streamClient.onMotionEvents = (from, events) => {
                // Handle incoming motion events
                console.log(`Received ${events.length} events from ${from}`);
                // Your reconstruction code here
            };
        });
    </script>
</body>
</html>
EOF

# Create tarball
tar -czf deploy.tar.gz -C deploy-package .

echo "Copying to VPS..."
scp deploy.tar.gz ${SERVER_USER}@${SERVER_IP}:/opt/videostream/

echo "Installing on VPS..."
ssh ${SERVER_USER}@${SERVER_IP} << 'REMOTE_COMMANDS'
cd /opt/videostream/www
tar -xzf ../deploy.tar.gz
rm ../deploy.tar.gz
chown -R videostream:videostream .

# Restart services
systemctl restart nginx
systemctl restart videostream

echo "Deployment complete!"
REMOTE_COMMANDS

# Clean up
rm -rf deploy-package deploy.tar.gz

echo "==========================================="
echo "Application deployed successfully!"
echo "Access at: https://194.87.103.57"
echo "==========================================="