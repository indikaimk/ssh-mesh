package sshbridge

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"github.com/ssh-mesh/ssh-mesh/internal/server/auth"
	"github.com/ssh-mesh/ssh-mesh/internal/server/broker"
	"golang.org/x/crypto/ssh"
)

const RecordingsDir = "data/recordings"

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow cross-origin for development & browser UI
	},
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
}

// sessionRecorder writes terminal output frames in asciicast v2 format (.cast)
type sessionRecorder struct {
	mu    sync.Mutex
	file  *os.File
	start time.Time
}

func newSessionRecorder(sessionID, title string, cols, rows uint32) (*sessionRecorder, error) {
	if err := os.MkdirAll(RecordingsDir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(RecordingsDir, sessionID+".cast")
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	header := map[string]interface{}{
		"version":   2,
		"width":     cols,
		"height":    rows,
		"timestamp": time.Now().Unix(),
		"title":     title,
	}
	headerBytes, _ := json.Marshal(header)
	_, _ = f.Write(append(headerBytes, '\n'))

	return &sessionRecorder{
		file:  f,
		start: time.Now(),
	}, nil
}

func (sr *sessionRecorder) WriteOutput(data []byte) {
	if sr == nil || sr.file == nil || len(data) == 0 {
		return
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()

	elapsed := time.Since(sr.start).Seconds()
	frame := []interface{}{elapsed, "o", string(data)}
	if raw, err := json.Marshal(frame); err == nil {
		_, _ = sr.file.Write(append(raw, '\n'))
	}
}

func (sr *sessionRecorder) Close() {
	if sr == nil || sr.file == nil {
		return
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	_ = sr.file.Close()
	sr.file = nil
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
	vmID := r.URL.Query().Get("vm_id")
	if vmID == "" {
		http.Error(w, "missing vm_id query param", http.StatusBadRequest)
		return
	}

	username := "admin"
	clientIP := r.RemoteAddr

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
		username = claims.Username
		st := h.authMgr.GetStore()

		// Enforce RBAC: viewers cannot open shell sessions
		if claims.Role == "viewer" {
			if st != nil {
				st.LogEvent(username, "ssh.access_denied", vmID, clientIP, "denied", "Viewer role attempted to open SSH terminal", nil)
			}
			http.Error(w, "forbidden: viewer role has read-only access and cannot open terminal sessions", http.StatusForbidden)
			return
		}

		// Enforce Node-level permissions
		agentInfo, err := h.broker.Get(vmID)
		if err != nil {
			http.Error(w, "agent not found or offline", http.StatusNotFound)
			return
		}
		if st != nil {
			userRecord, err := st.GetUser(claims.Username)
			if err == nil && userRecord != nil && !userRecord.CanAccessNode(agentInfo) {
				log.Printf("[Terminal Bridge] Access Denied: User '%s' does not have permission to access node '%s'", claims.Username, vmID)
				st.LogEvent(username, "ssh.access_denied", vmID, clientIP, "denied", "Restricted by node/tag RBAC rules", nil)
				http.Error(w, "forbidden: you do not have permission to access this node", http.StatusForbidden)
				return
			}
		}
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

	// Initialize session auditing & recording if enabled
	sessionStart := time.Now()
	var recorder *sessionRecorder
	var recordingIDPtr *string

	if h.authMgr != nil && h.authMgr.GetStore() != nil {
		st := h.authMgr.GetStore()
		if settings, err := st.GetAuditSettings(); err == nil && settings != nil {
			if settings.EnableSessionRecording {
				recID := uuid.New().String()
				if rec, err := newSessionRecorder(recID, fmt.Sprintf("%s@%s (%s)", username, vmID, targetUser), cols, rows); err == nil {
					recorder = rec
					recordingIDPtr = &recID
				}
			}
		}
		st.LogEvent(
			username,
			"ssh.session.start",
			vmID,
			clientIP,
			"success",
			fmt.Sprintf("Opened SSH session as Linux user '%s'", targetUser),
			recordingIDPtr,
		)
	}

	defer func() {
		if recorder != nil {
			recorder.Close()
		}
		if h.authMgr != nil && h.authMgr.GetStore() != nil {
			duration := time.Since(sessionStart).Round(time.Second)
			h.authMgr.GetStore().LogEvent(
				username,
				"ssh.session.end",
				vmID,
				clientIP,
				"success",
				fmt.Sprintf("Closed SSH session as '%s' (duration: %s)", targetUser, duration),
				recordingIDPtr,
			)
		}
	}()

	var once sync.Once
	closeAll := func() {
		_ = ws.Close()
		_ = sshSession.Close()
	}

	// Forward SSH stdout -> WebSocket (and optional session recorder)
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				if recorder != nil {
					recorder.WriteOutput(buf[:n])
				}
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

	// Forward SSH stderr -> WebSocket (and optional session recorder)
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				if recorder != nil {
					recorder.WriteOutput(buf[:n])
				}
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

	// Command buffer for Layer 2 command line auditing
	var cmdBuf []byte

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

		// Capture command lines when command logging is enabled
		if h.authMgr != nil && h.authMgr.GetStore() != nil {
			cmdBuf = processCommandInput(cmdBuf, msg, func(cmdLine string) {
				h.authMgr.GetStore().LogEvent(
					username,
					"ssh.command",
					vmID,
					clientIP,
					"success",
					fmt.Sprintf("[%s] $ %s", targetUser, cmdLine),
					recordingIDPtr,
				)
			})
		}

		// Plain input keystrokes
		if _, writeErr := stdinPipe.Write(msg); writeErr != nil {
			break
		}
	}

	once.Do(closeAll)
}

func processCommandInput(buf []byte, input []byte, onCommand func(string)) []byte {
	i := 0
	for i < len(input) {
		b := input[i]
		// Skip ANSI escape sequences (e.g. arrow keys \x1b[A)
		if b == 0x1b {
			i++
			if i < len(input) && input[i] == '[' {
				i++
				for i < len(input) && (input[i] < 0x40 || input[i] > 0x7e) {
					i++
				}
			}
			i++
			continue
		}

		switch b {
		case '\r', '\n':
			line := strings.TrimSpace(string(buf))
			if line != "" {
				onCommand(line)
			}
			buf = buf[:0]
		case 0x7f, 0x08: // Backspace / Delete
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		case 0x03, 0x15: // Ctrl+C or Ctrl+U clears current input buffer
			buf = buf[:0]
		default:
			if b >= 0x20 && b < 0x7f {
				buf = append(buf, b)
			}
		}
		i++
	}
	return buf
}

func init() {
	// Quiet io.EOF logs on regular closure
	_ = io.EOF
}
