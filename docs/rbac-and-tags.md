# RBAC & Tag-Based VM Access Control

SSH Mesh includes built-in multi-user authentication and fine-grained **Role-Based Access Control (RBAC)** so administrators can control exactly which VMs each team member can see and access.

---

## 1. User Roles

Every dashboard user is assigned one of three roles:

| Role | View Allowed VMs | Open SSH Terminals | Enroll Nodes / Tokens | Manage Users, Tags & Auditing |
| :--- | :---: | :---: | :---: | :---: |
| **`admin`** | All VMs (`*`) | Yes | Yes | Yes |
| **`operator`** | Only Permitted VMs | Yes (Permitted VMs) | Yes | No |
| **`viewer`** | Only Permitted VMs | **No (Read-Only)** | No | No |

---

## 2. Limiting User Access to Specific VMs or Tags

Admins can restrict any `operator` or `viewer` account to specific VMs or tags from **Team & Users** in the top navigation bar:

1. **All VMs (`*`)**: Grants access to all current and future VMs.
2. **Restrict to Specific VMs / Tags**:
   - **Visual VM Picker**: Click any connected VM pill (`node:<agent-id>`) to grant or revoke access to that specific machine.
   - **Visual Tag Picker**: Click any discovered tag pill (e.g. `env:staging`, `team:payments`) to grant access to all VMs carrying that tag.
   - **Custom Rules**: Type rules manually (e.g. `env:prod`, `node:db-replica-01`) to pre-authorize access to VMs before they come online.
3. **Editing Existing Users**: Click **Edit Access** on any user row to update their role or allowed VMs/tags in real time without recreating their account.

### How Rules Are Evaluated

When a user loads the dashboard (`GET /api/v1/agents`) or opens a terminal WebSocket (`GET /api/v1/terminal?vm_id=...`), `User.CanAccessNode(agent)` evaluates their `allowed_nodes` JSON array:
- If the user's role is `admin` or their rules include `*`, access is granted.
- If a rule matches the agent's ID or hostname (`node:vps-01` or `vps-01`), access is granted.
- If a rule matches any key-value pair in the agent's `Tags` map (`env:staging`, `env=staging`, or `tag:env=staging`), access is granted.
- Otherwise, the VM is hidden from the user's inventory list and direct terminal WebSocket connections return `403 Forbidden`.

---

## 3. Assigning & Managing VM Tags

There are three ways to assign tags to a VM:

### Method A: Live from the Web Dashboard (Admin)
1. In the left **Cloud VM Inventory** sidebar, click the **Tag icon** on any VM card.
2. Add or remove `key:value` tags (e.g. `env:staging`, `team:backend`).
3. Click **Save Tags**.
   - Tags are immediately updated in the live session broker and persisted in SQLite (`node_tags` table) so they survive agent restarts.

### Method B: During Node Enrollment
In the **Enroll Node** modal, enter comma-separated tags in the **Assign Node Tags** input (`env=staging, team=backend`). The generated one-line `curl` installer and binary command will automatically include `-tags "env=staging,team=backend"`.

### Method C: Agent CLI Flag or Environment Variable
Pass `-tags` or `SSH_MESH_TAGS` when starting `ssh-mesh-agent`:
```bash
./bin/ssh-mesh-agent \
  -server "ws://mesh.example.com:8080/api/v1/agent/tunnel" \
  -token "mesh_..." \
  -tags "env=staging,team=backend,region=ap-south-1"
```

