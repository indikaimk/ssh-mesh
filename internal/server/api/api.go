package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"github.com/ssh-mesh/ssh-mesh/internal/server/auth"
	"github.com/ssh-mesh/ssh-mesh/internal/server/broker"
)

// ServerAPI provides REST endpoints for the web UI.
type ServerAPI struct {
	broker     broker.Broker
	authMgr    *auth.Manager
	serverHost string
}

// NewServerAPI initializes the API handler.
func NewServerAPI(b broker.Broker, authMgr *auth.Manager, serverHost string) *ServerAPI {
	return &ServerAPI{
		broker:     b,
		authMgr:    authMgr,
		serverHost: serverHost,
	}
}

// RegisterRoutes attaches the REST handlers to an http.ServeMux
func (api *ServerAPI) RegisterRoutes(mux *http.ServeMux) {
	// Public routes
	mux.HandleFunc("/api/v1/auth/login", api.handleLogin)
	mux.HandleFunc("/api/v1/health", api.handleHealth)
	mux.HandleFunc("/api/v1/install-script", api.handleInstallScript)
	mux.HandleFunc("/api/v1/download/agent", api.handleDownloadAgent)

	// Protected routes (require JWT)
	mux.Handle("/api/v1/auth/me", api.authMgr.Middleware(http.HandlerFunc(api.handleMe)))
	mux.Handle("/api/v1/agents", api.authMgr.Middleware(http.HandlerFunc(api.handleAgents)))
	mux.Handle("/api/v1/tokens", api.authMgr.Middleware(http.HandlerFunc(api.handleTokens)))

	// User management (Admin only)
	adminOnly := api.authMgr.RequireRole("admin")
	mux.Handle("/api/v1/users", api.authMgr.Middleware(adminOnly(http.HandlerFunc(api.handleUsers))))
}

func (api *ServerAPI) handleLogin(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	token, user, err := api.authMgr.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		http.Error(w, "invalid credentials: "+err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
		"user":  user.Username,
		"role":  user.Role,
	})
}

func (api *ServerAPI) handleMe(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	user := r.Header.Get("X-Authenticated-User")
	role := r.Header.Get("X-Authenticated-Role")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"user":  user,
		"role":  role,
		"valid": true,
	})
}

func (api *ServerAPI) handleAgents(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.Header.Get("X-Authenticated-User")
	user, _ := api.authMgr.GetStore().GetUser(username)

	allAgents := api.broker.List()
	agents := make([]*protocol.AgentInfo, 0)
	for _, a := range allAgents {
		if user == nil || user.CanAccessNode(a) {
			agents = append(agents, a)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}

func (api *ServerAPI) handleUsers(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	w.Header().Set("Content-Type", "application/json")
	st := api.authMgr.GetStore()

	switch r.Method {
	case http.MethodGet:
		users, err := st.ListUsers()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"users": users})

	case http.MethodPost:
		var req struct {
			Username     string `json:"username"`
			Password     string `json:"password"`
			Role         string `json:"role"`
			AllowedNodes string `json:"allowed_nodes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Password == "" {
			http.Error(w, "username and password are required", http.StatusBadRequest)
			return
		}
		user, err := st.CreateUser(req.Username, req.Password, req.Role, req.AllowedNodes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(user)

	case http.MethodPut:
		var req struct {
			ID           string `json:"id"`
			Role         string `json:"role"`
			AllowedNodes string `json:"allowed_nodes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := st.UpdateUser(req.ID, req.Role, req.AllowedNodes); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"updated": true})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing user id", http.StatusBadRequest)
			return
		}
		if err := st.DeleteUser(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true})

	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *ServerAPI) handleTokens(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	w.Header().Set("Content-Type", "application/json")
	st := api.authMgr.GetStore()

	switch r.Method {
	case http.MethodGet:
		tokens, err := st.ListTokens()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tokens": tokens,
		})

	case http.MethodPost:
		var req struct {
			Name          string `json:"name"`
			DurationHours int    `json:"duration_hours"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			req.Name = "Custom Token"
		}
		duration := time.Duration(req.DurationHours) * time.Hour
		tokenEntry, err := st.CreateToken(req.Name, duration)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(tokenEntry)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing token id", http.StatusBadRequest)
			return
		}
		err := st.RevokeToken(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"revoked": true,
		})

	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *ServerAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"version": "0.1.0",
		"service": "ssh-mesh-control-plane",
	})
}

func (api *ServerAPI) handleDownloadAgent(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	osName := strings.ToLower(r.URL.Query().Get("os"))
	if osName == "" {
		osName = "linux"
	}

	archName := strings.ToLower(r.URL.Query().Get("arch"))
	if archName == "" {
		archName = "amd64"
	}
	switch archName {
	case "x86_64", "x64":
		archName = "amd64"
	case "aarch64", "arm":
		archName = "arm64"
	}

	candidates := []string{
		filepath.Join("bin/dist", fmt.Sprintf("ssh-mesh-agent-%s-%s", osName, archName)),
		filepath.Join("bin", fmt.Sprintf("ssh-mesh-agent-%s-%s", osName, archName)),
		filepath.Join("bin", "ssh-mesh-agent"),
	}

	var foundPath string
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			foundPath = p
			break
		}
	}

	if foundPath == "" {
		http.Error(w, fmt.Sprintf("agent binary not found for os=%s arch=%s", osName, archName), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"ssh-mesh-agent-%s-%s\"", osName, archName))
	http.ServeFile(w, r, foundPath)
}

func (api *ServerAPI) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript")

	host := api.serverHost
	if host == "" {
		host = r.Host
	}

	httpScheme := "http"
	wsScheme := "ws"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		httpScheme = "https"
		wsScheme = "wss"
	}

	serverHTTP := fmt.Sprintf("%s://%s", httpScheme, host)
	tunnelURL := fmt.Sprintf("%s://%s/api/v1/agent/tunnel", wsScheme, host)
	token := r.URL.Query().Get("token")
	if token == "" {
		// Use first active token or default
		if tokens, err := api.authMgr.GetStore().ListTokens(); err == nil {
			for _, t := range tokens {
				if !t.Revoked {
					token = t.Token
					break
				}
			}
		}
	}

	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

echo "=========================================================="
echo "          SSH-Mesh Agent Installer                        "
echo "=========================================================="

INSTALL_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/ssh-mesh-agent.service"
SERVER_HTTP="%s"
SERVER_TUNNEL="%s"
TOKEN="%s"

echo "[1/3] Detecting platform architecture..."
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  *)
    echo "Error: Unsupported architecture '${ARCH}'"
    exit 1
    ;;
esac

DOWNLOAD_URL="${SERVER_HTTP}/api/v1/download/agent?os=${OS}&arch=${ARCH}"
echo "Downloading agent binary for ${OS}/${ARCH}..."

mkdir -p "${INSTALL_DIR}"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${DOWNLOAD_URL}" -o "${INSTALL_DIR}/ssh-mesh-agent"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${INSTALL_DIR}/ssh-mesh-agent" "${DOWNLOAD_URL}"
else
  echo "Error: curl or wget is required to install the agent."
  exit 1
fi

chmod +x "${INSTALL_DIR}/ssh-mesh-agent"
echo "Binary installed to ${INSTALL_DIR}/ssh-mesh-agent"

echo "[2/3] Configuring systemd background service..."
if [ -d "/etc/systemd/system" ]; then
  cat << EOF > ${SERVICE_FILE}
[Unit]
Description=SSH Mesh Reverse Tunnel Agent
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/ssh-mesh-agent -server "${SERVER_TUNNEL}" -token "${TOKEN}"
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable --now ssh-mesh-agent
  echo "[3/3] Service created and started: ssh-mesh-agent"
  echo "Node is now connected to ${SERVER_TUNNEL}!"
else
  echo "[3/3] Systemd not found. You can run the agent in the background:"
  echo "nohup ${INSTALL_DIR}/ssh-mesh-agent -server '${SERVER_TUNNEL}' -token '${TOKEN}' > /tmp/ssh-mesh-agent.log 2>&1 &"
fi

echo "=========================================================="
echo "  Installation Complete!"
echo "=========================================================="
`, serverHTTP, tunnelURL, token)

	_, _ = w.Write([]byte(strings.TrimSpace(script) + "\n"))
}

func setCors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
