package protocol

import (
	"time"
)

// AgentRegistration is sent by the agent when establishing the reverse tunnel.
type AgentRegistration struct {
	ID        string            `json:"id"`
	Hostname  string            `json:"hostname"`
	OS        string            `json:"os"`
	Arch      string            `json:"arch"`
	PrivateIP string            `json:"private_ip"`
	Tags      map[string]string `json:"tags,omitempty"`
	Token     string            `json:"token"`
	Version   string            `json:"version"`
}

// AgentInfo represents an enrolled agent in the control plane.
type AgentInfo struct {
	ID         string            `json:"id"`
	Hostname   string            `json:"hostname"`
	OS         string            `json:"os"`
	Arch       string            `json:"arch"`
	PrivateIP  string            `json:"private_ip"`
	Tags       map[string]string `json:"tags,omitempty"`
	Version    string            `json:"version"`
	ConnectedAt time.Time        `json:"connected_at"`
	LastSeenAt  time.Time        `json:"last_seen_at"`
	Status     string            `json:"status"` // "online", "offline"
}

// TerminalResizeMsg is sent from the browser/control plane to resize the PTY.
type TerminalResizeMsg struct {
	Type string `json:"type"` // "resize"
	Cols uint32 `json:"cols"`
	Rows uint32 `json:"rows"`
}

// Constants for tunnel protocol headers
const (
	HeaderAgentID    = "X-Agent-ID"
	HeaderAgentToken = "X-Agent-Token"
	HeaderAgentInfo  = "X-Agent-Info"
)
