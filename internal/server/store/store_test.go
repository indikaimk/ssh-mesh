package store

import (
	"path/filepath"
	"testing"

	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
)

func TestNodeAccessControl(t *testing.T) {
	nodeStaging := &protocol.AgentInfo{
		ID:       "node-1",
		Hostname: "vps-staging-01",
		Tags:     map[string]string{"env": "staging", "team": "core"},
	}

	nodeProd := &protocol.AgentInfo{
		ID:       "node-2",
		Hostname: "vps-prod-01",
		Tags:     map[string]string{"env": "production", "team": "core"},
	}

	// 1. Admin can access everything
	adminUser := &User{Role: "admin", AllowedNodes: `["*"]`}
	if !adminUser.CanAccessNode(nodeStaging) || !adminUser.CanAccessNode(nodeProd) {
		t.Fatalf("Admin should have access to all nodes")
	}

	// 2. Wildcard rule
	opWildcard := &User{Role: "operator", AllowedNodes: `["*"]`}
	if !opWildcard.CanAccessNode(nodeStaging) || !opWildcard.CanAccessNode(nodeProd) {
		t.Fatalf("Wildcard user should have access to all nodes")
	}

	// 3. Tag-based rule (env:staging only)
	opStaging := &User{Role: "operator", AllowedNodes: `["env:staging"]`}
	if !opStaging.CanAccessNode(nodeStaging) {
		t.Fatalf("opStaging should be able to access node with env=staging")
	}
	if opStaging.CanAccessNode(nodeProd) {
		t.Fatalf("opStaging should NOT be able to access node with env=production")
	}

	// 4. Direct Node ID / Hostname rule
	opSpecificNode := &User{Role: "operator", AllowedNodes: `["node-1"]`}
	if !opSpecificNode.CanAccessNode(nodeStaging) {
		t.Fatalf("opSpecificNode should be able to access node-1")
	}
	if opSpecificNode.CanAccessNode(nodeProd) {
		t.Fatalf("opSpecificNode should NOT be able to access node-2")
	}
}

func TestStoreUserCRUDWithSqlx(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "store_test.db")

	st, err := NewSQLiteStore(dbPath, "admin", "adminpass", "tok-123")
	if err != nil {
		t.Fatalf("Failed to initialize store: %v", err)
	}
	defer st.Close()

	// Create user with custom allowed_nodes rule
	u, err := st.CreateUser("dev_sarah", "sarahpass", "operator", `["env:dev","node:sandbox"]`)
	if err != nil {
		t.Fatalf("Failed to create user with sqlx: %v", err)
	}
	if u.Username != "dev_sarah" || u.Role != "operator" {
		t.Fatalf("Unexpected user fields: %+v", u)
	}

	// Fetch user
	fetched, err := st.GetUser("dev_sarah")
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if fetched.AllowedNodes != `["env:dev","node:sandbox"]` {
		t.Fatalf("Expected allowed_nodes to match, got %s", fetched.AllowedNodes)
	}

	// Update user
	if err := st.UpdateUser(fetched.ID, "viewer", `["*"]`); err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}
	updated, _ := st.GetUser("dev_sarah")
	if updated.Role != "viewer" || updated.AllowedNodes != `["*"]` {
		t.Fatalf("User update verification failed: %+v", updated)
	}

	// Set and get node tags
	if err := st.SetNodeTags("vps-01", map[string]string{"env": "staging", "team": "backend"}); err != nil {
		t.Fatalf("Failed to set node tags: %v", err)
	}
	tags, err := st.GetNodeTags("vps-01")
	if err != nil {
		t.Fatalf("Failed to get node tags: %v", err)
	}
	if tags["env"] != "staging" || tags["team"] != "backend" {
		t.Fatalf("Unexpected node tags: %+v", tags)
	}

	// Verify audit settings are disabled by default
	settings, err := st.GetAuditSettings()
	if err != nil {
		t.Fatalf("GetAuditSettings failed: %v", err)
	}
	if settings.EnableAuditLog || settings.EnableCommandLogging || settings.EnableSessionRecording {
		t.Fatalf("Expected all audit settings to be disabled by default, got %+v", settings)
	}

	// LogEvent should be a no-op when disabled
	st.LogEvent("admin", "auth.login", "admin", "127.0.0.1", "success", "Login", nil)
	logs, err := st.ListAuditLogs(50, "", "", "")
	if err != nil || len(logs) != 0 {
		t.Fatalf("Expected 0 audit logs when disabled, got %d (err: %v)", len(logs), err)
	}

	// Enable audit log and command logging
	if err := st.UpdateAuditSettings(&AuditSettings{
		EnableAuditLog:         true,
		EnableCommandLogging:   true,
		EnableSessionRecording: false,
	}); err != nil {
		t.Fatalf("UpdateAuditSettings failed: %v", err)
	}

	st.LogEvent("dev_sarah", "ssh.command", "vps-01", "127.0.0.1", "success", "[ubuntu] $ ls -la", nil)
	logs, err = st.ListAuditLogs(50, "ssh.command", "dev_sarah", "vps-01")
	if err != nil || len(logs) != 1 {
		t.Fatalf("Expected 1 command audit log after enabling, got %d (err: %v)", len(logs), err)
	}
}
