package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ssh-mesh/ssh-mesh/internal/server/broker"
)

// ServerAPI provides REST endpoints for the web UI.
type ServerAPI struct {
	broker        broker.Broker
	requiredToken string
	serverHost    string
}

// NewServerAPI initializes the API handler.
func NewServerAPI(b broker.Broker, requiredToken, serverHost string) *ServerAPI {
	return &ServerAPI{
		broker:        b,
		requiredToken: requiredToken,
		serverHost:    serverHost,
	}
}

// RegisterRoutes attaches the REST handlers to an http.ServeMux
func (api *ServerAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/agents", api.handleAgents)
	mux.HandleFunc("/api/v1/health", api.handleHealth)
	mux.HandleFunc("/api/v1/install-script", api.handleInstallScript)
}

func (api *ServerAPI) handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents := api.broker.List()
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}

func (api *ServerAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"version": "0.1.0",
		"service": "ssh-mesh-control-plane",
	})
}

func (api *ServerAPI) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript")

	host := api.serverHost
	if host == "" {
		host = r.Host
	}

	scheme := "ws"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "wss"
	}

	tunnelURL := fmt.Sprintf("%s://%s/api/v1/agent/tunnel", scheme, host)
	token := api.requiredToken

	// Generate install script for Linux VPS
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

echo "========================================================"
echo "          SSH-Mesh Agent Installer                      "
echo "========================================================"

INSTALL_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/ssh-mesh-agent.service"
SERVER_URL="%s"
TOKEN="%s"

echo "[1/3] Downloading latest agent binary..."
# Note: For production, binaries can be hosted at /api/v1/download/agent
# For local development or testing, compile or copy to /usr/local/bin/ssh-mesh-agent
echo "Target server: ${SERVER_URL}"

cat << 'EOF' > /tmp/ssh-mesh-agent-run.sh
#!/usr/bin/env bash
exec /usr/local/bin/ssh-mesh-agent \
  -server "%s" \
  -token "%s"
EOF
chmod +x /tmp/ssh-mesh-agent-run.sh

echo "[2/3] Setting up systemd background service..."
if [ -d "/etc/systemd/system" ]; then
  cat << EOF > ${SERVICE_FILE}
[Unit]
Description=SSH Mesh Reverse Tunnel Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ssh-mesh-agent -server "${SERVER_URL}" -token "${TOKEN}"
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
  echo "[3/3] Service created at ${SERVICE_FILE}."
  echo "Run: systemctl daemon-reload && systemctl enable --now ssh-mesh-agent"
else
  echo "[3/3] Systemd not found. You can run the agent manually:"
  echo "ssh-mesh-agent -server '${SERVER_URL}' -token '${TOKEN}'"
fi

echo "Installation complete."
`, tunnelURL, token, tunnelURL, token)

	_, _ = w.Write([]byte(strings.TrimSpace(script) + "\n"))
}
