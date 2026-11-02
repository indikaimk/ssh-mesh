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
	Status      string           `json:"status"` // "online", "offline"
	Token       string           `json:"-"`
}

// TerminalResizeMsg is sent from the browser/control plane to resize the PTY.
type TerminalResizeMsg struct {
	Type string `json:"type"` // "resize"
	Cols uint32 `json:"cols"`
	Rows uint32 `json:"rows"`
}

// FileEntry represents a file or directory on a remote agent VM.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
}

// FileOpRequest is sent over the "mesh-fs" SSH channel to perform file operations.
type FileOpRequest struct {
	Op   string `json:"op"`   // "list", "download", "upload", "mkdir", "delete"
	Path string `json:"path"`
}

// FileOpResponse is returned by the agent over the "mesh-fs" SSH channel.
type FileOpResponse struct {
	OK      bool        `json:"ok"`
	Error   string      `json:"error,omitempty"`
	Cwd     string      `json:"cwd,omitempty"`
	Entries []FileEntry `json:"entries,omitempty"`
	Size    int64       `json:"size,omitempty"`
}

// Constants for tunnel protocol headers
const (
	HeaderAgentID    = "X-Agent-ID"
	HeaderAgentToken = "X-Agent-Token"
	HeaderAgentInfo  = "X-Agent-Info"
)
