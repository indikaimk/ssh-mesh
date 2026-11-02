# REST & WebSocket API Reference

All control-plane API routes are prefixed with `/api/v1`. Protected routes require an `Authorization: Bearer <JWT>` header (or `?token=<JWT>` query parameter for WebSocket connections).

---

## Authentication

### `POST /api/v1/auth/login`
Authenticates a dashboard user and returns a signed HMAC-SHA256 JWT.

**Request Body:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Response (`200 OK`):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": "admin",
  "role": "admin"
}
```

### `GET /api/v1/auth/me`
Verifies the current JWT and returns the authenticated username and role.

---

## Agents & VM Inventory

### `GET /api/v1/agents`
Returns all registered VMs that the authenticated user is permitted to access (`User.CanAccessNode`).

**Response (`200 OK`):**
```json
{
  "agents": [
    {
      "id": "vps-staging-01",
      "hostname": "vps-staging-01",
      "os": "linux",
      "arch": "amd64",
      "private_ip": "10.0.1.24",
      "tags": {
        "env": "staging",
        "team": "backend"
      },
      "version": "0.1.0",
      "status": "online"
    }
  ],
  "count": 1
}
```

### `PUT /api/v1/agents/tags` *(Admin Only)*
Updates the tags assigned to a VM in both SQLite (`node_tags`) and the live session broker.

**Request Body:**
```json
{
  "agent_id": "vps-staging-01",
  "tags": {
    "env": "staging",
    "team": "backend"
  }
}
```

---

## Team & User Management *(Admin Only)*

### `GET /api/v1/users`
Lists all dashboard user accounts.

### `POST /api/v1/users`
Creates a new user account with role and node/tag access rules.

**Request Body:**
```json
{
  "username": "dev_sarah",
  "password": "StrongPassword123",
  "role": "operator",
  "allowed_nodes": "[\"env:staging\",\"node:vps-01\"]"
}
```

### `PUT /api/v1/users`
Updates an existing user's role and allowed node/tag rules.

**Request Body:**
```json
{
  "id": "user-uuid",
  "role": "viewer",
  "allowed_nodes": "[\"env:staging\"]"
}
```

### `DELETE /api/v1/users?id=<user-id>`
Deletes a user account (prevents deleting the last remaining `admin` account).

---

## Enrollment Tokens & Installer

### `GET /api/v1/tokens` / `POST /api/v1/tokens` / `DELETE /api/v1/tokens?id=<id>`
Lists, creates (`{"name": "Prod AWS", "duration_hours": 168}`), or revokes agent enrollment tokens.

### `GET /api/v1/install-script?token=<token>&tags=<k=v,k2=v2>`
Returns a shell script that detects the target machine's OS/Arch, downloads the static agent binary, and registers a `systemd` service with the provided token and tags.

### `GET /api/v1/download/agent?os=linux&arch=amd64`
Serves the pre-compiled static `ssh-mesh-agent` binary for the requested platform.

---

## Security Auditing & Session Recordings *(Admin Only)*

### `GET /api/v1/audit/settings` / `PUT /api/v1/audit/settings`
Gets or updates the 3 opt-in auditing toggles:
```json
{
  "enable_audit_log": true,
  "enable_command_logging": true,
  "enable_session_recording": true
}
```

### `GET /api/v1/audit/logs`
Lists recorded audit events. Supports query parameters:
- `limit` (default `200`, max `1000`)
- `event_type` (prefix match, e.g. `ssh.session`, `ssh.command`, `auth.`)
- `actor` (substring match)
- `target` (substring match)

### `DELETE /api/v1/audit/logs`
Clears all records in `audit_logs` and removes `.cast` session recordings from `data/recordings/`.

### `GET /api/v1/audit/recordings?id=<recording-id>`
Streams the `asciicast` v2 `.cast` recording file for in-browser replay.

---

## WebSocket Endpoints

### `GET /api/v1/agent/tunnel`
Outbound reverse tunnel endpoint dialed by `ssh-mesh-agent`. Upgrades HTTP to WebSocket and wraps the connection in a `hashicorp/yamux` session.

### `GET /api/v1/terminal?vm_id=<id>&user=<linux_user>&cols=120&rows=36&token=<jwt>`
Browser terminal WebSocket endpoint. Verifies user RBAC and node permissions, opens a multiplexed stream to the target agent's embedded SSH server, allocates a PTY, and streams terminal I/O.

