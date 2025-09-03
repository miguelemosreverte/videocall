#!/bin/bash
set -e

echo "==========================================="
echo "Installing Janus WebRTC Gateway"
echo "==========================================="

# Install dependencies
apt-get update
apt-get install -y \
    libmicrohttpd-dev libjansson-dev \
    libssl-dev libsofia-sip-ua-dev libglib2.0-dev \
    libopus-dev libogg-dev libcurl4-openssl-dev liblua5.3-dev \
    libconfig-dev pkg-config gengetopt libtool automake \
    python3 python3-pip python3-setuptools python3-wheel \
    cmake

# Install libnice
cd /tmp
git clone https://gitlab.freedesktop.org/libnice/libnice
cd libnice
meson --prefix=/usr build && ninja -C build && ninja -C build install

# Install libsrtp
cd /tmp
wget https://github.com/cisco/libsrtp/archive/v2.5.0.tar.gz
tar xzf v2.5.0.tar.gz
cd libsrtp-2.5.0
./configure --prefix=/usr --enable-openssl
make shared_library && make install

# Install Janus
cd /opt
git clone https://github.com/meetecho/janus-gateway.git
cd janus-gateway
sh autogen.sh
./configure --prefix=/opt/janus
make
make install

# Create Janus config
mkdir -p /opt/janus/etc/janus
cat > /opt/janus/etc/janus/janus.jcfg << 'EOF'
general: {
    debug_level = 4
    admin_secret = "janusoverlord"
}

nat: {
    stun_server = "stun.l.google.com"
    stun_port = 19302
    nice_debug = false
    full_trickle = true
}

media: {
    ipv6 = false
    max_nack_queue = 300
}

transports: {
    disable = ""
}

plugins: {
    disable = ""
}

events: {
    broadcast = true
}
EOF

# Configure HTTP transport
cat > /opt/janus/etc/janus/janus.transport.http.jcfg << 'EOF'
general: {
    json = "indented"
    base_path = "/janus"
    http = true
    port = 8088
    https = false
    cors = true
    allow_origin = "*"
}

admin: {
    admin_base_path = "/admin"
    admin_http = false
}
EOF

# Configure WebSocket transport
cat > /opt/janus/etc/janus/janus.transport.websockets.jcfg << 'EOF'
general: {
    json = "indented"
    ws = true
    ws_port = 8188
    wss = false
}
EOF

# Configure VideoRoom plugin
cat > /opt/janus/etc/janus/janus.plugin.videoroom.jcfg << 'EOF'
general: {
    admin_key = "supersecret"
    lock_rtp_forward = true
}

room-1234: {
    description = "Demo Room"
    is_private = false
    secret = ""
    publishers = 6
    bitrate = 128000
    fir_freq = 10
    audiocodec = "opus"
    videocodec = "vp8"
    video_svc = false
}
EOF

# Create systemd service
cat > /etc/systemd/system/janus.service << 'EOF'
[Unit]
Description=Janus WebRTC Server
After=network.target

[Service]
Type=simple
ExecStart=/opt/janus/bin/janus
Restart=always
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# Configure Nginx for Janus
cat > /etc/nginx/sites-available/janus << 'EOF'
upstream janus_http {
    server localhost:8088;
}

upstream janus_ws {
    server localhost:8188;
}

server {
    listen 80;
    server_name _;

    # Janus HTTP API
    location /janus {
        proxy_pass http://janus_http/janus;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # Janus WebSocket
    location /ws {
        proxy_pass http://janus_ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }

    # Static files
    location / {
        root /var/www/janus;
        try_files $uri $uri/ =404;
    }
}
EOF

# Create web directory
mkdir -p /var/www/janus

# Enable and start services
ln -sf /etc/nginx/sites-available/janus /etc/nginx/sites-enabled/default
systemctl daemon-reload
systemctl enable janus
systemctl restart janus
systemctl restart nginx

echo "==========================================="
echo "Janus WebRTC Gateway installed!"
echo "HTTP API: http://<server-ip>/janus"
echo "WebSocket: ws://<server-ip>/ws"
echo "==========================================="