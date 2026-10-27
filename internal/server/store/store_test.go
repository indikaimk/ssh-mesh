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
}
