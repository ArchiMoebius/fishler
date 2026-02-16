# Setup

Spin up a VPS with `Ubuntu 24.04.3 LTS` and run the following commands:

Find/Replace `eth0IP` with your VPS public IP address

```bash
apt-get update
apt-get upgrade
apt-get install nginx libnginx-mod-stream ssl-cert ca-certificates curl iptables-persistent iptables

hostnamectl set-hostname localhost
make-ssl-cert generate-default-snakeoil --force-overwrite

cat > /etc/nginx/nginx.conf < 'EOF'
user root;
worker_processes auto;
error_log /var/log/nginx/error.log notice;
pid /run/nginx.pid;

load_module /usr/lib/nginx/modules/ngx_stream_module.so;

events {
    worker_connections 1024;
}

stream {
    upstream ssh {
        server 127.0.0.1:22;
    }

    upstream web {
        server 127.0.0.1:8443;
    }

    map $ssl_preread_protocol $upstream {
        "" ssh;
        default web;
    }

    server {
        listen 443;
        proxy_pass $upstream;
        ssl_preread on;
    }
}

http {
    # Define upgrade connection map for WebSocket
    map $http_upgrade $connection_upgrade {
        default upgrade;
        '' close;
    }

    # HTTPS server
    server {
        listen 127.0.0.1:8443 ssl http2;
        server_name _;

        access_log /var/log/nginx/access_ssl.log;
        error_log /var/log/nginx/error_ssl.log;

        # SSL certificate (add missing semicolons)
        ssl_certificate /etc/ssl/certs/ssl-cert-snakeoil.pem;
        ssl_certificate_key /etc/ssl/private/ssl-cert-snakeoil.key;

        # SSL security settings
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_session_cache shared:SSL:1m;
        ssl_session_timeout 10m;
	ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;
        ssl_prefer_server_ciphers on;

        # Serve default nginx page for root and other paths
        location / {
            root /usr/share/nginx/html;
            index index.html index.htm;
        }

        # Proxy configuration for the specific route
        location /aa5024b363d7f93ec2f56b0402390ee9/ {
            # Proxy to local service on host machine via Docker gateway
            proxy_pass http://127.0.0.1:8080/;

            # Standard proxy headers
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;

            # WebSocket support
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;

            # Timeout settings for long-lived connections
            proxy_connect_timeout 7d;
            proxy_send_timeout 7d;
            proxy_read_timeout 7d;
        }
    }
}
EOF

cat > /etc/systemd/system/fishler.service << 'EOF'
[Unit]
Description=fishler
ConditionPathExists=/opt/honey/fishler
After=network.target uplink.target

[Service]
AmbientCapabilities=CAP_NET_BIND_SERVICE
Type=simple
User=root
LimitNOFILE=1024

Restart=on-failure
RestartSec=10

WorkingDirectory=/opt/honey/
ExecStart=/opt/honey/fishler --uplink-server-address 127.0.0.1:50051 serve --any-account --banner 'SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13.14' --ip <eth0IP> --port 22

# Log errors and stdout to the journal
PermissionsStartOnly=true
StandardOutput=journal
StandardError=journal
SyslogIdentifier=fishler

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/uplink.service << 'EOF'
[Unit]
Description=uplink
ConditionPathExists=/opt/honey/uplink
After=network.target

[Service]
AmbientCapabilities=CAP_NET_BIND_SERVICE
Type=simple
User=root
LimitNOFILE=1024

Restart=on-failure
RestartSec=10

WorkingDirectory=/opt/honey/
ExecStart=/opt/honey/uplink

# Log errors and stdout to the journal
PermissionsStartOnly=true
StandardOutput=journal
StandardError=journal
SyslogIdentifier=uplink

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/iptables/rules.v4 << 'EOF'
*filter
:INPUT ACCEPT [0:0]
:FORWARD ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:INPUT DROP [0:0]
:FORWARD DROP [0:0]
:OUTPUT ACCEPT [0:0]

# Custom per-protocol chains
:UDP - [0:0]
:TCP - [0:0]
:ICMP - [0:0]

# Logs
-N LOG_DROP
-A LOG_DROP -j LOG --log-level 6 --log-prefix "IPTABLES:DROP: "
-A LOG_DROP -j DROP


# Acceptable UDP traffic

# Acceptable TCP traffic
-A TCP -p tcp --dport 22 -j ACCEPT
-A TCP -p tcp --dport 443 -j ACCEPT
-A TCP -p tcp --dport 80 -j ACCEPT

# Acceptable ICMP traffic

# Drop invalid packets
-A INPUT -m conntrack --ctstate INVALID -j DROP

# Boilerplate acceptance policy
-A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
-A INPUT -i lo -j ACCEPT
-A INPUT -i DOCKER -j ACCEPT

# Pass traffic to protocol-specific chains
## Only allow new connections (established and related should already be handled)
## For TCP, additionally only allow new SYN packets since that is the only valid
## method for establishing a new TCP connection
-A INPUT -p udp -m conntrack --ctstate NEW -j UDP
-A INPUT -p tcp --syn -m conntrack --ctstate NEW -j TCP
-A INPUT -p icmp -m conntrack --ctstate NEW -j ICMP

# Reject anything that's fallen through to this point
## Try to be protocol-specific w/ rejection message
-A INPUT -p udp -j REJECT --reject-with icmp-port-unreachable
-A INPUT -p tcp -j REJECT --reject-with tcp-reset
-A INPUT -j REJECT --reject-with icmp-proto-unreachable

# Output Rules
-A OUTPUT -o lo -j ACCEPT
-A OUTPUT -o DOCKER -j ACCEPT
-A OUTPUT -p tcp --dport 443 -j ACCEPT
-A OUTPUT -p tcp --dport 80 -j ACCEPT
-A OUTPUT -p tcp --dport 22 -j ACCEPT
-A OUTPUT -p udp --dport 53 -j ACCEPT
-A OUTPUT -o eth0 -j ACCEPT
-A OUTPUT -p icmp -j ACCEPT

-A INPUT -j LOG_DROP
-A FORWARD -j LOG_DROP
-A OUTPUT -j LOG_DROP
-A UDP -j LOG_DROP
-A TCP -j LOG_DROP
-A ICMP -j LOG_DROP

# Commit the changes
COMMIT

*raw
:PREROUTING ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
COMMIT

*nat
:PREROUTING ACCEPT [0:0]
:INPUT ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:POSTROUTING ACCEPT [0:0]
COMMIT

*security
:INPUT ACCEPT [0:0]
:FORWARD ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
COMMIT

*mangle
:PREROUTING ACCEPT [0:0]
:INPUT ACCEPT [0:0]
:FORWARD ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:POSTROUTING ACCEPT [0:0]
COMMIT
EOF

cat > /etc/iptables/rules.v6 << 'EOF'
*filter
# Allow all outgoing, but drop incoming and forwarding packets by default
:INPUT DROP [0:0]
:FORWARD DROP [0:0]
:OUTPUT ACCEPT [0:0]

# Custom per-protocol chains
:UDP - [0:0]
:TCP - [0:0]
:ICMP - [0:0]

# Logs
-N LOG_DROP
-A LOG_DROP -j LOG --log-level 6 --log-prefix "IPTABLES:DROP: "
-A LOG_DROP -j DROP

-A INPUT -i lo -j ACCEPT
-A OUTPUT -o lo -j ACCEPT
-A INPUT -s fe80::0000:0000:0000:0000/16 -j ACCEPT
-A INPUT -d fe80::0000:0000:0000:0000/16 -j ACCEPT

-A INPUT -j LOG_DROP
-A FORWARD -j LOG_DROP
# Meh...
-A OUTPUT -j ACCEPT
-A UDP -j LOG_DROP
-A TCP -j LOG_DROP
-A ICMP -j LOG_DROP

# Commit the changes
COMMIT

*raw
:PREROUTING ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
COMMIT

*nat
:PREROUTING ACCEPT [0:0]
:INPUT ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:POSTROUTING ACCEPT [0:0]
COMMIT

*security
:INPUT ACCEPT [0:0]
:FORWARD ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
COMMIT

*mangle
:PREROUTING ACCEPT [0:0]
:INPUT ACCEPT [0:0]
:FORWARD ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:POSTROUTING ACCEPT [0:0]
EOF

cat > /etc/ssh/sshd_config << 'EOF'
Include /etc/ssh/sshd_config.d/*.conf
Port 22
ListenAddress 127.0.0.1
PermitRootLogin yes
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM yes
X11Forwarding yes
PrintMotd no
AcceptEnv LANG LC_*
Subsystem	sftp	/usr/lib/openssh/sftp-server
EOF

apt remove -yq $(dpkg --get-selections docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc | cut -f1)

install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc

# Add the repository to Apt sources:
tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")
Components: stable
Signed-By: /etc/apt/keyrings/docker.asc
EOF

apt update
apt install -yq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

systemctl daemon-reload

service docker start
service uplink start
service fishler start
service nginx start

systemctl enable docker
systemctl enable uplink
systemctl enable fishler
systemctl enable nginx

reboot now
```