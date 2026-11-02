# Getting Started

**SSH Mesh** lets you securely open interactive browser terminals to cloud VMs and servers located behind NAT, CGNAT, or inside private VPC subnets — without exposing public IPs or port `22`.

---

## 1. Build from Source

### Prerequisites
- **Go** `1.22+`
- **Node.js** `18+` (for building the React + Xterm.js web UI)

```bash
# 1. Build the React web dashboard
cd web && npm install && npm run build && cd ..

# 2. Build the Control Plane Server and Agent binaries
go build -o bin/ssh-mesh-server ./cmd/server
go build -o bin/ssh-mesh-agent ./cmd/agent

# (Optional) Build cross-platform static agent binaries for the auto-installer
make build-agents || (
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/dist/ssh-mesh-agent-linux-amd64 ./cmd/agent &&
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/dist/ssh-mesh-agent-linux-arm64 ./cmd/agent
)
```

---

## 2. Start the Control Plane Server

```bash
./bin/ssh-mesh-server \
  -addr :8080 \
  -admin-user admin \
  -admin-pass admin \
  -token mesh-secret-token \
  -db mesh.db \
  -web-dir ./web/dist
```

Open **`http://localhost:8080`** in your browser and sign in with your initial admin credentials (`admin` / `admin`).

### Server Flags & Environment Variables

| Flag | Environment Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `-addr` | `SSH_MESH_ADDR` | `:8080` | HTTP/WebSocket listen address |
| `-admin-user` | `SSH_MESH_ADMIN_USER` | `admin` | Initial admin username seeded into SQLite |
| `-admin-pass` | `SSH_MESH_ADMIN_PASS` | `admin` | Initial admin password seeded into SQLite |
| `-token` | `SSH_MESH_TOKEN` | `mesh-secret-token` | Default enrollment token for agents |
| `-jwt-secret` | `SSH_MESH_JWT_SECRET` | *(auto-generated)* | Secret key for signing dashboard JWTs |
| `-db` | `SSH_MESH_DB` | `mesh.db` | SQLite database file path |
| `-web-dir` | `SSH_MESH_WEB_DIR` | `./web/dist` | Path to compiled React frontend assets |

---

## 3. Enroll a Remote VM / Node

Click **Enroll Node** in the top navigation bar of the dashboard. You can optionally select a named enrollment token and assign initial **Node Tags** (e.g. `env=staging,team=backend`).

### Option A: One-Line Auto-Installer (Recommended)
```bash
curl -sSL "http://<YOUR_SERVER_HOST>:8080/api/v1/install-script?token=mesh-secret-token&tags=env=staging,team=backend" | sudo bash
```
The installer script automatically:
1. Detects the OS (`linux`/`darwin`) and CPU architecture (`amd64`/`arm64`).
2. Downloads the corresponding static binary from `/api/v1/download/agent` to `/usr/local/bin/ssh-mesh-agent`.
3. Creates and enables a `systemd` service (`/etc/systemd/system/ssh-mesh-agent.service`) configured to auto-restart on failure.

### Option B: Direct Binary Execution
```bash
./bin/ssh-mesh-agent \
  -server "ws://<YOUR_SERVER_HOST>:8080/api/v1/agent/tunnel" \
  -token "mesh-secret-token" \
  -id "vps-staging-01" \
  -tags "env=staging,team=backend"
```

### Agent Flags & Environment Variables

| Flag | Environment Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `-server` | `SSH_MESH_SERVER` | `ws://localhost:8080/api/v1/agent/tunnel` | Control plane tunnel WebSocket URL |
| `-id` | `SSH_MESH_AGENT_ID` | *(system hostname)* | Unique identifier for the node |
| `-token` | `SSH_MESH_TOKEN` | `mesh-secret-token` | Enrollment token for authenticating with server |
| `-tags` | `SSH_MESH_TAGS` | *(empty)* | Comma-separated `key=value` tags for RBAC |
| `-shell` | `SSH_MESH_SHELL` | *(auto-detected bash/sh)* | Custom shell binary path |

