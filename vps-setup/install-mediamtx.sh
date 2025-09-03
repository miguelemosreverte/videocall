#!/bin/bash
set -e

echo "==========================================="
echo "Installing MediaMTX Streaming Server"
echo "==========================================="

# Create directory
mkdir -p /opt/mediamtx
cd /opt/mediamtx

# Download MediaMTX
MEDIAMTX_VERSION="v1.9.3"
wget https://github.com/bluenviron/mediamtx/releases/download/${MEDIAMTX_VERSION}/mediamtx_${MEDIAMTX_VERSION}_linux_amd64.tar.gz
tar -xzf mediamtx_${MEDIAMTX_VERSION}_linux_amd64.tar.gz
rm mediamtx_${MEDIAMTX_VERSION}_linux_amd64.tar.gz

# Create configuration
cat > /opt/mediamtx/mediamtx.yml << 'EOF'
###############################################
# General parameters

# Log level
logLevel: info
logDestinations: [stdout]

# HTTP API
api: yes
apiAddress: :9997

# Metrics
metrics: yes
metricsAddress: :9998

# WebRTC parameters
webrtc: yes
webrtcAddress: :8889
webrtcServerKey: ""
webrtcServerCert: ""
webrtcAllowOrigin: "*"
webrtcTrustedProxies: []
webrtcICEServers:
  - stun:stun.l.google.com:19302
webrtcICEHostNAT1To1IPs: []
webrtcICEUDPMuxAddress: ""
webrtcICETCPMuxAddress: ""

# HTTP/HTTPS server
hlsAddress: :8888
hlsAllowOrigin: "*"

# RTSP server
rtsp: yes
protocols: [tcp, udp]
rtspAddress: :8554

# RTMP server
rtmp: yes
rtmpAddress: :1935

###############################################
# Path parameters

paths:
  # Allow all streams
  all:
    source: publisher
    sourceOnDemand: no
    sourceOnDemandStartTimeout: 10s
    sourceOnDemandCloseAfter: 10s
    maxReaders: 0
    record: no
EOF

# Create systemd service
cat > /etc/systemd/system/mediamtx.service << 'EOF'
[Unit]
Description=MediaMTX Streaming Server
After=network.target

[Service]
Type=simple
User=root
ExecStart=/opt/mediamtx/mediamtx /opt/mediamtx/mediamtx.yml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Update firewall
ufw allow 8889/tcp comment 'MediaMTX WebRTC'
ufw allow 8888/tcp comment 'MediaMTX HLS'
ufw allow 9997/tcp comment 'MediaMTX API'
ufw allow 8554/tcp comment 'MediaMTX RTSP'
ufw allow 8000:8999/udp comment 'WebRTC UDP'
ufw reload

# Start service
systemctl daemon-reload
systemctl enable mediamtx
systemctl restart mediamtx

# Wait for service to start
sleep 3

# Check status
systemctl status mediamtx --no-pager | head -10

echo "==========================================="
echo "MediaMTX installed successfully!"
echo "WebRTC: http://194.87.103.57:8889"
echo "API: http://194.87.103.57:9997"
echo "==========================================="