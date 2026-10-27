package auth

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ssh-mesh/ssh-mesh/internal/server/store"
)

func TestAuthManager(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	st, err := store.NewSQLiteStore(dbPath, "admin", "securepass123", "default-token")
	if err != nil {
		t.Fatalf("Failed to initialize test store: %v", err)
	}
	defer st.Close()

	mgr := NewManager(st)

	// 1. Test Login & JWT
	token, user, err := mgr.AuthenticateUser("admin", "securepass123")
	if err != nil {
		t.Fatalf("Failed to authenticate valid credentials: %v", err)
	}
	if user.Role != "admin" {
		t.Fatalf("Expected admin role, got %s", user.Role)
	}

	claims, err := mgr.VerifyJWT(token)
	if err != nil || claims.Username != "admin" || claims.Role != "admin" {
		t.Fatalf("Failed to verify valid JWT: %v (claims: %+v)", err, claims)
	}

	// 2. Test Invalid Login
	_, _, err = mgr.AuthenticateUser("admin", "wrongpass")
	if err == nil {
		t.Fatalf("Expected error for invalid credentials, got nil")
	}

	// 3. Test Tampered JWT
	tampered := token + "tamper"
	_, err = mgr.VerifyJWT(tampered)
	if err == nil {
		t.Fatalf("Expected error for tampered JWT, got nil")
	}

	// 4. Test Enrollment Tokens
	if !mgr.ValidateAgentToken("default-token") {
		t.Fatalf("Expected default-token to be valid")
	}

	newTok, err := st.CreateToken("Dev Cluster", 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}
	if !mgr.ValidateAgentToken(newTok.Token) {
		t.Fatalf("Expected newly created token %s to be valid", newTok.Token)
	}

	// Revoke token
	if err := st.RevokeToken(newTok.ID); err != nil {
		t.Fatalf("Failed to revoke token %s: %v", newTok.ID, err)
	}
	if mgr.ValidateAgentToken(newTok.Token) {
		t.Fatalf("Revoked token should not be valid")
	}
}
