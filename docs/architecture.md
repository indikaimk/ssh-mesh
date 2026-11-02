# Architecture & Reverse Tunnel Design

SSH Mesh eliminates the need for bastion hosts, VPNs, or inbound firewall rules by combining **outbound WebSockets**, **Yamux stream multiplexing**, and an **embedded Go SSH server**.

---

## High-Level Data Flow

```
┌─────────────────────────────────┐
│ Browser (React + Xterm.js)      │
└────────────────┬────────────────┘
                 │ 4. User WebSocket (/api/v1/terminal?vm_id=...)
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ SSH Mesh Central Control Plane (Go Backend)                 │
│                                                             │
│   ├── REST API & RBAC Auth (SQLite + sqlx + JWT)            │
│   ├── Audit & Session Recorder (asciicast v2 .cast)         │
│   ├── Session Broker (broker.Broker abstraction)            │
│   ├── Reverse Tunnel Endpoint (/api/v1/agent/tunnel)        │
│   └── SSH Bridge (Dials Agent's SSH server over Yamux)      │
└────────────────▲────────────────────────────────────────────┘
                 │
                 │ 1. Outbound-only TLS WebSocket (Port 443/80)
                 │ 2. Upgraded to Yamux Multiplexed Session
                 │ 3. Virtual stream per terminal tab
                 │
┌────────────────┴────────────────────────────────────────────┐
│ Target VPS / Cloud VM (Private Subnet)                      │
│                                                             │
│   ├── Agent Binary (Outbound Dialer + Exponential Backoff)  │
│   └── Embedded Go SSH Server (golang.org/x/crypto/ssh)      │
│       └── Linux PTY (/bin/bash, SIGWINCH resize, su/user)   │
└─────────────────────────────────────────────────────────────┘
```

---

## How a Connection Works Step-by-Step

### 1. Outbound Agent Registration
The lightweight agent binary on the private VM initiates an **outbound** HTTP/HTTPS connection to `/api/v1/agent/tunnel` with headers:
- `X-Agent-ID`: Node identifier
- `X-Agent-Token`: Enrollment token validated against the control plane's `enrollment_tokens` table
- `X-Agent-Info`: JSON payload containing hostname, OS, architecture, private RFC1918 IP, and initial tags

### 2. Yamux Stream Multiplexing
Once the WebSocket upgrade succeeds:
- The WebSocket connection is wrapped as a standard Go `net.Conn` (`pkg/wsconn`).
- **Role Inversion**: The **Agent** initializes `yamux.Server(netConn)` and the **Control Plane** initializes `yamux.Client(netConn)`.
- This role inversion allows the Control Plane to call `session.Open()` at any time to open multiple concurrent, independent virtual TCP streams back to the agent over the single outbound WebSocket connection.

### 3. Embedded SSH Server & Target User Switching
The agent runs an embedded SSH server (`golang.org/x/crypto/ssh`) directly on the streams accepted from `yamux.Session.Accept()`:
- **Zero Host Port Binding**: The agent never binds to a local TCP port on the VM, and does not require `sshd` (`openssh-server`) to be installed.
- **Target Linux User Selection**: When a user selects a target Linux user (e.g. `ubuntu`, `root`, `ec2-user`) in the web UI, the SSH handshake passes that username. If the agent runs as `root`, it looks up the target Unix user (`os/user.Lookup`) and drops privileges (`syscall.Credential{Uid, Gid}`) or spawns `su - <user>` inside a pseudo-terminal (`github.com/creack/pty`).
- **Dynamic Window Resizing**: Browser terminal resize events are sent as JSON frames (`{"type":"resize","cols":120,"rows":40}`) and translated into SSH `window-change` requests (`pty.Setsize`).

---

## Horizontal Scalability (`broker.Broker`)

All session routing in the control plane is decoupled behind the `broker.Broker` interface (`internal/server/broker/broker.go`):

```go
type Broker interface {
    Register(info *protocol.AgentInfo, session *yamux.Session) error
    Unregister(agentID string)
    Get(agentID string) (*protocol.AgentInfo, error)
    List() []*protocol.AgentInfo
    DialStream(ctx context.Context, agentID string) (net.Conn, error)
    UpdateHeartbeat(agentID string)
    UpdateTags(agentID string, tags map[string]string) error
}
```

- **Single-Node Deployment**: Uses `broker.MemoryBroker` for sub-millisecond in-memory stream dialing.
- **Multi-Node / Clustered Deployment**: Can be swapped with a gRPC Mesh or Redis-backed broker that routes `DialStream` calls to whichever control-plane instance holds the target agent's active Yamux session.

