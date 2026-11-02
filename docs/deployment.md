# Production Deployment Guide

In production, the **SSH Mesh Control Plane** should be deployed behind a TLS-terminating reverse proxy (such as **Caddy** or **Nginx**) so that both browser sessions (`https://` / `wss://`) and outbound agent tunnels (`wss://`) are encrypted with TLS.

---

## 1. Docker & Docker Compose (Recommended)

The official multi-arch Docker image (`linux/amd64` and `linux/arm64`) bundles the compiled React web UI, the Go control plane server, and all cross-platform static agent binaries (`bin/dist/ssh-mesh-agent-*`) for the one-line installer.

### Using Docker Compose
```bash
docker compose up -d
```

### Using `docker run`
```bash
docker run -d \
  --name ssh-mesh-server \
  --restart unless-stopped \
  -p 8080:8080 \
  -e SSH_MESH_ADMIN_USER=admin \
  -e SSH_MESH_ADMIN_PASSWORD=ChangeMeImmediately! \
  -e SSH_MESH_TOKEN=mesh_prod_initial_secret \
  -v ssh_mesh_data:/app/data \
  ghcr.io/indikaimk/ssh-mesh:latest
```

---

## 2. Systemd Service (Bare-Metal / VM)

Copy `bin/ssh-mesh-server` and the compiled `web/dist` directory to `/opt/ssh-mesh/`, then create `/etc/systemd/system/ssh-mesh-server.service`:

```ini
[Unit]
Description=SSH Mesh Control Plane Server
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/ssh-mesh
ExecStart=/opt/ssh-mesh/bin/ssh-mesh-server \
  -addr 127.0.0.1:8080 \
  -db /opt/ssh-mesh/data/mesh.db \
  -web-dir /opt/ssh-mesh/web/dist
Environment="SSH_MESH_ADMIN_USER=admin"
Environment="SSH_MESH_ADMIN_PASS=ChangeMeImmediately!"
Environment="SSH_MESH_TOKEN=mesh_prod_initial_secret"
Environment="SSH_MESH_JWT_SECRET=replace-with-64-char-random-hex-secret"
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

Enable and start the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ssh-mesh-server
```

---

## 2. TLS Reverse Proxy Configuration

### Option A: Caddy (Automatic Let's Encrypt HTTPS)

Create `/etc/caddy/Caddyfile`:
```caddyfile
mesh.example.com {
    reverse_proxy 127.0.0.1:8080
}
```
Caddy automatically provisions TLS certificates, forwards `X-Forwarded-Proto: https`, and proxies WebSocket upgrades for both `/api/v1/agent/tunnel` and `/api/v1/terminal`.

### Option B: Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name mesh.example.com;

    ssl_certificate     /etc/letsencrypt/live/mesh.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/mesh.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Keep reverse tunnels and interactive SSH sessions open
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }
}
```

> [!TIP]
> Because `/api/v1/install-script` inspects `X-Forwarded-Proto: https`, the one-line installer in the web UI will automatically switch from `http://` and `ws://` to `https://` and `wss://` when served behind Caddy or Nginx.

---

## 3. Cross-Compiling Static Agent Binaries

To allow the one-line installer (`/api/v1/install-script`) to serve binaries for every OS and CPU architecture, build the static binaries into `bin/dist/`:

```bash
mkdir -p bin/dist

# Linux amd64 & arm64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/dist/ssh-mesh-agent-linux-amd64 ./cmd/agent
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/dist/ssh-mesh-agent-linux-arm64 ./cmd/agent

# macOS Apple Silicon & Intel
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/dist/ssh-mesh-agent-darwin-arm64 ./cmd/agent
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/dist/ssh-mesh-agent-darwin-amd64 ./cmd/agent
```

---

## 4. Database & Recording Backups

- **SQLite Database (`mesh.db`)**: Runs in `WAL` (Write-Ahead Logging) mode for concurrent reads and writes. Back up `mesh.db` safely online using `sqlite3 mesh.db ".backup 'mesh-backup.db'"`.
- **Session Recordings (`data/recordings/*.cast`)**: Plaintext JSON-lines files that compress at ~90% ratio with `tar -czf`.

