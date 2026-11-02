# SSH Mesh 🌐

A modern, high-performance web tool to securely SSH into cloud VMs and VPS instances located in **private subnets with private IPs**, requiring **zero public IPs, zero port forwarding, and zero inbound firewall rules**.

---

## ⚡ Architecture & How It Works (Approach B)

```
┌─────────────────────────────────┐
│ Browser (React + Xterm.js)      │
└────────────────┬────────────────┘
                 │ User WebSocket (Terminal I/O + Window Resize)
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ SSH Mesh Central Control Plane (Go Backend)                 │
│                                                             │
│   ├── REST API (/api/v1/agents, /api/v1/install-script)     │
│   ├── Session Broker (Decoupled abstraction for scaling)    │
│   ├── Reverse Tunnel Endpoint (/api/v1/agent/tunnel)        │
│   └── SSH Bridge (Dials Agent's SSH server over Yamux)      │
└────────────────▲────────────────────────────────────────────┘
                 │
                 │ Outbound-only TLS WebSocket
                 │ Upgraded to Yamux (Multiplexed Stream)
                 │
┌────────────────┴────────────────────────────────────────────┐
│ Target VPS / Cloud VM (Private Subnet)                      │
│                                                             │
│   ├── Agent Binary (Reverse Tunnel Dialer)                  │
│   └── Embedded Go SSH Server (golang.org/x/crypto/ssh)      │
│       └── Linux PTY (/bin/bash, SIGWINCH resize support)    │
└─────────────────────────────────────────────────────────────┘
```

1. **Outbound Reverse Tunnel**: The lightweight agent binary running on the VPS connects **outbound** to the central server (`/api/v1/agent/tunnel`) over TLS WebSocket.
2. **Multiplexing with Yamux**: The WebSocket connection is wrapped into a bidirectional stream multiplexer (`hashicorp/yamux`).
3. **Embedded Go SSH Server**: The agent runs an embedded SSH server (`golang.org/x/crypto/ssh`) listening directly on the virtual streams accepted from the Yamux session. No OpenSSH daemon or open port 22 is needed on the VPS.
4. **Interactive Web Terminal**: When a user opens a terminal session in the React UI, the Go server requests a new stream on the agent's Yamux session, initiates an SSH client handshake, starts an interactive shell with pseudo-terminal (`github.com/creack/pty`), and streams standard I/O into `xterm.js`.

---

## 🚀 Scalability Design

The connection routing is isolated behind the **`broker.Broker`** interface in [`internal/server/broker/broker.go`](file:///home/indika/projects/ssh-mesh/internal/server/broker/broker.go):

```go
type Broker interface {
    Register(info *protocol.AgentInfo, session *yamux.Session) error
    Unregister(agentID string)
    Get(agentID string) (*protocol.AgentInfo, error)
    List() []*protocol.AgentInfo
    DialStream(ctx context.Context, agentID string) (net.Conn, error)
    UpdateHeartbeat(agentID string)
}
```

- **Current Implementation**: Fast in-memory broker (`broker.MemoryBroker`) for single-node deployments.
- **Future Scaling (gRPC Mesh or Redis Pub/Sub)**: When scaling to a cluster of control planes behind a load balancer, any instance can implement `DialStream` to proxy streams across nodes (e.g. via gRPC streaming or a distributed mesh) **without touching any agent code or SSH protocol logic**.

---

## 🛠️ Quick Start

### 1. Build Binaries
```bash
# Build the server and agent Go binaries
go build -o bin/ssh-mesh-server ./cmd/server
go build -o bin/ssh-mesh-agent ./cmd/agent

# Build the React web dashboard
cd web && npm install && npm run build && cd ..
```

### 2. Start the Control Plane Server
```bash
./bin/ssh-mesh-server \
  -addr :8080 \
  -admin-user admin \
  -admin-pass admin \
  -token mesh-secret-token \
  -web-dir ./web/dist
```
Open **`http://localhost:8080`** in your browser and sign in with `admin` / `admin`.

---

## 🔒 Security, RBAC & Auditing

- **Web Dashboard Authentication**: Protected with HMAC-SHA256 JWT tokens and persistent SQLite user storage (`bcrypt` password hashing).
- **Multi-User RBAC & Tag/Node Access Control**:
  - **Roles**: `admin` (full control), `operator` (interactive SSH access), and `viewer` (read-only inventory access).
  - **Granular VM Restrictions**: Restrict users to specific VMs (`node:vps-01`) or VM tags (`env:staging`, `team:backend`) via the visual VM & tag picker.
- **3-Layer Opt-In Security Auditing & Session Replay** (Admin-controlled, disabled by default):
  1. **Control-Plane & Session Audit Trail**: Tracks logins, RBAC changes, token events, agent connections, and SSH session durations.
  2. **SSH Command History Capture**: Logs executed shell commands (`[ubuntu] $ ...`) per user and VM.
  3. **Full Terminal Session Recording & Replay**: Records sessions in `asciicast` v2 (`.cast`) format with built-in `xterm.js` playback in the dashboard.
- **Dynamic Enrollment Tokens**: Generate named agent tokens with custom expiration (e.g. 24h, 7d) and one-click revocation from the web dashboard.
- **Target Linux User Selection**: When launching a terminal, choose to drop privileges to `ubuntu`, `root`, `ec2-user`, or any local Unix user.
- **Private Subnet Isolation**: Agents only dial **outbound** over port 443/80. No public IPs or inbound firewall rules required on the target VMs.

### 3. Connect a VPS / Node
On any remote machine with internet access (even behind NAT/firewall):

```bash
# Direct binary execution (with optional RBAC tags):
./bin/ssh-mesh-agent \
  -server "ws://<YOUR_SERVER_HOST>:8080/api/v1/agent/tunnel" \
  -token "mesh-secret-token" \
  -id "production-db-node-1" \
  -tags "env=production,team=core"
```

Or run the one-click auto-detecting installer script:
```bash
curl -sSL "http://<YOUR_SERVER_HOST>:8080/api/v1/install-script?token=mesh-secret-token&tags=env=staging,team=backend" | sudo bash
```

Once connected, the VM will immediately light up green in the web dashboard! Click on the node to launch an interactive terminal session.

---

## 📚 Documentation

Full documentation is located in the [`docs/`](file:///home/indika/projects/ssh-mesh/docs/) directory (powered by **VitePress** and published to GitHub Pages):

- [**Getting Started & Quickstart**](file:///home/indika/projects/ssh-mesh/docs/getting-started.md)
- [**Architecture & Reverse Tunnel Design**](file:///home/indika/projects/ssh-mesh/docs/architecture.md)
- [**RBAC & Tag-Based VM Access Control**](file:///home/indika/projects/ssh-mesh/docs/rbac-and-tags.md)
- [**Security Auditing, Command Logs & Session Replay**](file:///home/indika/projects/ssh-mesh/docs/auditing-and-replay.md)
- [**Production Deployment Guide**](file:///home/indika/projects/ssh-mesh/docs/deployment.md)
- [**REST & WebSocket API Reference**](file:///home/indika/projects/ssh-mesh/docs/api-reference.md)

---

## 🧪 Running Tests

Run the unit and end-to-end integration tests:

```bash
go test -v ./internal/... ./test/...
```
