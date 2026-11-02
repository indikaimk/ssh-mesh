# Security Auditing, Command Logs & Session Replay

SSH Mesh provides a **3-layer security auditing system** that intercepts events, commands, and terminal streams directly at the control plane — requiring zero extra agents or audit daemons on target VMs.

> [!IMPORTANT]
> **Privacy & Storage by Default**: All three auditing layers are **disabled by default** (`false`). An Administrator can enable or disable each layer independently from the **Audit & Logs** modal in the top navigation bar.

---

## The 3 Auditing Layers

### Layer 1: Control-Plane & Session Audit Trail (`enable_audit_log`)
Records structured security and operational events in the SQLite `audit_logs` table:
- **Authentication**:
  - `auth.login`: Successful logins and failed login attempts (`status: denied`) with client IP.
- **SSH Terminal Lifecycle**:
  - `ssh.session.start`: User opened a terminal session on a VM (includes target Linux user).
  - `ssh.session.end`: User closed a terminal session (includes total session duration).
  - `ssh.access_denied`: Blocked terminal connection attempt (e.g. `viewer` role or unauthorized node/tag).
- **User & RBAC Administration**:
  - `user.create`, `user.update`, `user.delete`: User account and node permission changes.
  - `node.tags`: Changes to VM tags.
  - `token.create`, `token.revoke`: Enrollment token lifecycle events.
  - `audit.settings`: Audit configuration changes (always logged so changes to auditing itself are tracked).
- **Agent Connectivity**:
  - `agent.connect`, `agent.disconnect`: VM reverse tunnel online/offline events.
  - `agent.auth_failed`: Rejected agent tunnel connection with an invalid or revoked token.

---

### Layer 2: SSH Command History Capture (`enable_command_logging`)
When enabled, the control plane's terminal bridge (`internal/server/sshbridge/sshbridge.go`) inspects keystrokes sent to the remote PTY stdin:
- Reconstructs interactive command lines until `Enter` (`\r` / `\n`).
- Properly handles `Backspace`/`Delete`, resets on `Ctrl+C` / `Ctrl+U`, and strips ANSI escape sequences (such as arrow keys).
- Stores an `ssh.command` audit entry formatted as:
  ```text
  [ubuntu] $ sudo systemctl status nginx
  ```
- Associates the command with the `actor` (dashboard user), `target` (VM ID), `client_ip`, and `recording_id` (if session recording is also active).

---

### Layer 3: Full Terminal Session Recording & Replay (`enable_session_recording`)
When enabled, every SSH terminal session is recorded in the industry-standard **asciicast v2** (`.cast`) JSON-lines format under `data/recordings/<session-id>.cast`:

```json
{"version":2,"width":120,"height":36,"timestamp":1762078000,"title":"dev_sarah@vps-staging-01 (ubuntu)"}
[0.04218,"o","ubuntu@vps-staging-01:~$ "]
[1.81904,"o","ls -la\r\n"]
```

#### In-Browser Session Replay
1. Open **Audit & Logs** in the top navigation bar.
2. Any SSH session or command recorded while Layer 3 was active displays a **Replay** button.
3. Click **Replay** to launch the built-in `xterm.js` player directly inside the modal:
   - **Play / Pause / Restart** controls
   - **Playback Speed**: `1x`, `2x`, or `4x` (automatically caps long idle pauses to `1.5s` so reviewing sessions is fast)

---

## Exporting & Clearing Audit Data

From the **Audit & Logs** modal:
- **Filter & Search**: Filter by category (`All Events`, `SSH Sessions`, `Shell Commands`, `Auth & Admin`, `Agents`) or search by username, VM ID, IP address, or command substring.
- **Export JSON**: Click **Export** to download the current audit trail as a timestamped `.json` file for SIEM ingestion or compliance archiving.
- **Clear Logs**: Click the trash icon to purge all `audit_logs` records and delete `.cast` files under `data/recordings/`.

