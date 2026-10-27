package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ssh-mesh/ssh-mesh/internal/server/api"
	"github.com/ssh-mesh/ssh-mesh/internal/server/auth"
	"github.com/ssh-mesh/ssh-mesh/internal/server/broker"
	"github.com/ssh-mesh/ssh-mesh/internal/server/store"
)

func TestInstallScriptAndDownloadEndpoints(t *testing.T) {
	// Create temporary store for testing
	tmpDB := filepath.Join(t.TempDir(), "test-api.db")
	st, err := store.NewSQLiteStore(tmpDB, "admin", "admin123", "default-test-token")
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer st.Close()

	authMgr := auth.NewManager(st)
	b := broker.NewMemoryBroker()
	serverAPI := api.NewServerAPI(b, authMgr, "mesh.example.com")

	mux := http.NewServeMux()
	serverAPI.RegisterRoutes(mux)

	// Test 1: Install script generation
	req := httptest.NewRequest(http.MethodGet, "/api/v1/install-script?token=custom-test-token-123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for install-script, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "TOKEN=\"custom-test-token-123\"") {
		t.Errorf("script body missing custom token, got: %s", body)
	}
	if !strings.Contains(body, "Detecting platform architecture") {
		t.Errorf("script body missing architecture detection")
	}

	// Test 2: Binary download endpoint
	distDir := filepath.Join("bin", "dist")
	_ = os.MkdirAll(distDir, 0755)
	dummyBinary := filepath.Join(distDir, "ssh-mesh-agent-linux-amd64")
	if err := os.WriteFile(dummyBinary, []byte("fake-binary-content"), 0755); err != nil {
		t.Fatalf("failed to create dummy binary: %v", err)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/download/agent?os=linux&arch=amd64", nil)
	dw := httptest.NewRecorder()
	mux.ServeHTTP(dw, downloadReq)

	if dw.Code != http.StatusOK {
		t.Fatalf("expected 200 for binary download, got %d", dw.Code)
	}
	contentType := dw.Header().Get("Content-Type")
	if contentType != "application/octet-stream" {
		t.Errorf("expected application/octet-stream, got %s", contentType)
	}
}
