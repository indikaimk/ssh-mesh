package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

	agents := api.broker.List()
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
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Password == "" {
			http.Error(w, "username and password are required", http.StatusBadRequest)
			return
		}
		user, err := st.CreateUser(req.Username, req.Password, req.Role)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(user)

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

echo "========================================================"
echo "          SSH-Mesh Agent Installer                      "
echo "========================================================"

INSTALL_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/ssh-mesh-agent.service"
SERVER_URL="%s"
TOKEN="%s"

echo "[1/3] Target control plane: ${SERVER_URL}"

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

func setCors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
