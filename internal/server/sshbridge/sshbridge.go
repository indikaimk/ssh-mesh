package sshbridge

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"github.com/ssh-mesh/ssh-mesh/internal/server/auth"
	"github.com/ssh-mesh/ssh-mesh/internal/server/broker"
	"golang.org/x/crypto/ssh"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow cross-origin for development & browser UI
	},
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
}

// Handler handles user terminal sessions coming from the browser (Xterm.js).
type Handler struct {
	broker      broker.Broker
	authMgr     *auth.Manager
	sharedToken string
}

// NewHandler creates a new terminal bridge handler.
func NewHandler(b broker.Broker, authMgr *auth.Manager, sharedToken string) *Handler {
	return &Handler{
		broker:      b,
		authMgr:     authMgr,
		sharedToken: sharedToken,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate user via JWT if authMgr is configured
	if h.authMgr != nil {
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			http.Error(w, "missing authentication token", http.StatusUnauthorized)
			return
		}
		claims, err := h.authMgr.VerifyJWT(tokenStr)
		if err != nil {
			http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}
		// Enforce RBAC: viewers cannot open shell sessions
		if claims.Role == "viewer" {
			http.Error(w, "forbidden: viewer role has read-only access and cannot open terminal sessions", http.StatusForbidden)
			return
		}
	}

	vmID := r.URL.Query().Get("vm_id")
	if vmID == "" {
		http.Error(w, "missing vm_id query param", http.StatusBadRequest)
		return
	}

	targetUser := r.URL.Query().Get("user")
	if targetUser == "" {
		targetUser = "mesh-user"
	}

	cols := uint32(80)
	rows := uint32(24)
	if c, err := strconv.Atoi(r.URL.Query().Get("cols")); err == nil && c > 0 {
		cols = uint32(c)
	}
	if rw, err := strconv.Atoi(r.URL.Query().Get("rows")); err == nil && rw > 0 {
		rows = uint32(rw)
	}

	// Upgrade browser HTTP request to WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Terminal Bridge] WebSocket upgrade failed: %v", err)
		return
	}
	defer ws.Close()

	// Dial the agent through the broker (abstracted - works with in-memory or gRPC/pubsub later)
	stream, err := h.broker.DialStream(r.Context(), vmID)
	if err != nil {
		log.Printf("[Terminal Bridge] Failed to connect to agent %s: %v", vmID, err)
		_ = ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m[SSH-Mesh Error] Cannot connect to agent: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	defer stream.Close()

	// Connect SSH client over this stream with specified targetUser
	sshConfig := &ssh.ClientConfig{
		User: targetUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(h.sharedToken),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(stream, "agent-ssh", sshConfig)
	if err != nil {
		log.Printf("[Terminal Bridge] SSH client handshake failed for %s: %v", vmID, err)
		_ = ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m[SSH-Mesh Error] SSH handshake failed: "+err.Error()+"\x1b[0m\r\n"))
		return
	}
	defer sshConn.Close()

	sshClient := ssh.NewClient(sshConn, chans, reqs)
	defer sshClient.Close()

	sshSession, err := sshClient.NewSession()
	if err != nil {
		log.Printf("[Terminal Bridge] Failed to create SSH session for %s: %v", vmID, err)
		return
	}
	defer sshSession.Close()

	// Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sshSession.RequestPty("xterm-256color", int(rows), int(cols), modes); err != nil {
		log.Printf("[Terminal Bridge] RequestPty error for %s: %v", vmID, err)
		return
	}

	stdinPipe, err := sshSession.StdinPipe()
	if err != nil {
		log.Printf("[Terminal Bridge] StdinPipe error: %v", err)
		return
	}
	defer stdinPipe.Close()

	stdoutPipe, err := sshSession.StdoutPipe()
	if err != nil {
		log.Printf("[Terminal Bridge] StdoutPipe error: %v", err)
		return
	}

	stderrPipe, err := sshSession.StderrPipe()
	if err != nil {
		log.Printf("[Terminal Bridge] StderrPipe error: %v", err)
		return
	}

	if err := sshSession.Shell(); err != nil {
		log.Printf("[Terminal Bridge] Failed to start shell: %v", err)
		return
	}

	var once sync.Once
	closeAll := func() {
		_ = ws.Close()
		_ = sshSession.Close()
	}

	// Forward SSH stdout -> WebSocket
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				if writeErr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		once.Do(closeAll)
	}()

	// Forward SSH stderr -> WebSocket
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				if writeErr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		once.Do(closeAll)
	}()

	// Read from WebSocket -> SSH Stdin (and intercept resize messages)
	for {
		msgType, msg, err := ws.ReadMessage()
		if err != nil {
			break
		}

		// Check if message is a JSON resize event
		if msgType == websocket.TextMessage && len(msg) > 0 && msg[0] == '{' {
			var resize protocol.TerminalResizeMsg
			if jsonErr := json.Unmarshal(msg, &resize); jsonErr == nil && resize.Type == "resize" {
				if resize.Cols > 0 && resize.Rows > 0 {
					_ = sshSession.WindowChange(int(resize.Rows), int(resize.Cols))
					continue
				}
			}
		}

		// Plain input keystrokes
		if _, writeErr := stdinPipe.Write(msg); writeErr != nil {
			break
		}
	}

	once.Do(closeAll)
}

func init() {
	// Quiet io.EOF logs on regular closure
	_ = io.EOF
}
