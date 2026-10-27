package tunnel

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"github.com/ssh-mesh/ssh-mesh/internal/server/auth"
	"github.com/ssh-mesh/ssh-mesh/internal/server/broker"
	"github.com/ssh-mesh/ssh-mesh/pkg/wsconn"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow cross-origin for agents and development
	},
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
}

// Handler handles reverse tunnel connections from agents.
type Handler struct {
	broker  broker.Broker
	authMgr *auth.Manager
}

// NewHandler creates a new tunnel endpoint handler.
func NewHandler(b broker.Broker, authMgr *auth.Manager) *Handler {
	return &Handler{
		broker:  b,
		authMgr: authMgr,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	agentID := r.Header.Get(protocol.HeaderAgentID)
	token := r.Header.Get(protocol.HeaderAgentToken)
	agentInfoRaw := r.Header.Get(protocol.HeaderAgentInfo)

	// Validate token dynamically against auth manager
	if h.authMgr != nil && !h.authMgr.ValidateAgentToken(token) {
		log.Printf("[Server Tunnel] Unauthorized connection attempt from %s (invalid or revoked token)", r.RemoteAddr)
		http.Error(w, "unauthorized agent token", http.StatusUnauthorized)
		return
	}

	if agentID == "" {
		http.Error(w, "missing X-Agent-ID header", http.StatusBadRequest)
		return
	}

	var reg protocol.AgentRegistration
	if agentInfoRaw != "" {
		_ = json.Unmarshal([]byte(agentInfoRaw), &reg)
	}
	if reg.ID == "" {
		reg.ID = agentID
	}

	// Upgrade HTTP connection to WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Server Tunnel] WebSocket upgrade failed from %s: %v", r.RemoteAddr, err)
		return
	}
	defer ws.Close()

	netConn := wsconn.Wrap(ws)

	// Initialize Yamux as Client (so the server can call session.Open() to dial the agent's SSH server)
	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.EnableKeepAlive = true
	yamuxConfig.KeepAliveInterval = 15 * time.Second

	session, err := yamux.Client(netConn, yamuxConfig)
	if err != nil {
		log.Printf("[Server Tunnel] Yamux client failed for agent %s: %v", agentID, err)
		return
	}
	defer session.Close()

	agentInfo := &protocol.AgentInfo{
		ID:        reg.ID,
		Hostname:  reg.Hostname,
		OS:        reg.OS,
		Arch:      reg.Arch,
		PrivateIP: reg.PrivateIP,
		Tags:      reg.Tags,
		Version:   reg.Version,
		Status:    "online",
	}

	if err := h.broker.Register(agentInfo, session); err != nil {
		log.Printf("[Server Tunnel] Failed to register agent %s: %v", agentID, err)
		return
	}
	log.Printf("[Server Tunnel] Agent registered successfully: %s (%s, IP: %s)", agentInfo.ID, agentInfo.Hostname, agentInfo.PrivateIP)

	defer func() {
		h.broker.Unregister(agentID)
		log.Printf("[Server Tunnel] Agent disconnected: %s", agentID)
	}()

	// Wait until connection closes
	<-session.CloseChan()
}
