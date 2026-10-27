package auth

import (
	"testing"
	"time"
)

func TestAuthManager(t *testing.T) {
	mgr := NewManager("admin", "securepass123", "default-token")

	// 1. Test Login & JWT
	token, err := mgr.AuthenticateUser("admin", "securepass123")
	if err != nil {
		t.Fatalf("Failed to authenticate valid credentials: %v", err)
	}

	sub, err := mgr.VerifyJWT(token)
	if err != nil || sub != "admin" {
		t.Fatalf("Failed to verify valid JWT: %v (sub: %s)", err, sub)
	}

	// 2. Test Invalid Login
	_, err = mgr.AuthenticateUser("admin", "wrongpass")
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

	newTok := mgr.CreateEnrollmentToken("Dev Cluster", 1*time.Hour)
	if !mgr.ValidateAgentToken(newTok.Token) {
		t.Fatalf("Expected newly created token %s to be valid", newTok.Token)
	}

	// Revoke token
	if !mgr.RevokeEnrollmentToken(newTok.ID) {
		t.Fatalf("Failed to revoke token %s", newTok.ID)
	}
	if mgr.ValidateAgentToken(newTok.Token) {
		t.Fatalf("Revoked token should not be valid")
	}
}
