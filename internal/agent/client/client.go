package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/ssh-mesh/ssh-mesh/internal/agent/sshserver"
	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"github.com/ssh-mesh/ssh-mesh/pkg/wsconn"
)

// Config defines the agent runtime options
type Config struct {
	ServerURL   string
	AgentID     string
	Token       string
	Shell       string
	Version     string
	Tags        map[string]string
	AutoRestart bool
}

// Agent coordinates the outbound reverse tunnel and embedded SSH server
type Agent struct {
	cfg       Config
	sshServer *sshserver.Server
}

// New creates an Agent instance
func New(cfg Config) (*Agent, error) {
	if cfg.Version == "" {
		cfg.Version = "0.1.0"
	}
	if cfg.AgentID == "" {
		host, _ := os.Hostname()
		cfg.AgentID = host
	}

	sshSrv, err := sshserver.NewServer(sshserver.Config{
		Shell:       cfg.Shell,
		SharedToken: cfg.Token,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedded SSH server: %w", err)
	}

	return &Agent{
		cfg:       cfg,
		sshServer: sshSrv,
	}, nil
}

// Run starts the agent connection loop with automatic exponential backoff reconnect
func (a *Agent) Run(ctx context.Context) error {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("[Agent] Connecting reverse tunnel to %s (Agent ID: %s)...", a.cfg.ServerURL, a.cfg.AgentID)
		err := a.connectAndServe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			log.Printf("[Agent] Connection error: %v. Retrying in %v...", err, backoff)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (a *Agent) connectAndServe(ctx context.Context) error {
	// Gather system details
	hostname, _ := os.Hostname()
	privateIP := getPrivateIP()

	reg := protocol.AgentRegistration{
		ID:        a.cfg.AgentID,
		Hostname:  hostname,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		PrivateIP: privateIP,
		Tags:      a.cfg.Tags,
		Token:     a.cfg.Token,
		Version:   a.cfg.Version,
	}

	regJSON, err := json.Marshal(reg)
	if err != nil {
		return err
	}

	headers := http.Header{}
	headers.Set(protocol.HeaderAgentID, a.cfg.AgentID)
	headers.Set(protocol.HeaderAgentToken, a.cfg.Token)
	headers.Set(protocol.HeaderAgentInfo, string(regJSON))

	dialer := websocket.DefaultDialer
	ws, resp, err := dialer.DialContext(ctx, a.cfg.ServerURL, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("server handshake failed with status %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("failed to dial websocket: %w", err)
	}
	defer ws.Close()

	log.Printf("[Agent] Reverse tunnel established successfully. Initializing Yamux session...")

	// Wrap WebSocket into net.Conn
	netConn := wsconn.Wrap(ws)

	// In Yamux, Agent runs as Server so that the Central Control Plane acts as Client and can open streams to us
	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.EnableKeepAlive = true
	yamuxConfig.KeepAliveInterval = 15 * time.Second

	session, err := yamux.Server(netConn, yamuxConfig)
	if err != nil {
		return fmt.Errorf("failed to start yamux server: %w", err)
	}
	defer session.Close()

	// Watch for context cancellation to immediately unblock session.Accept()
	closeDone := make(chan struct{})
	defer close(closeDone)

	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
			_ = ws.Close()
		case <-closeDone:
		}
	}()

	log.Printf("[Agent] Embedded SSH server listening for incoming tunnel sessions...")

	// Serve incoming Yamux streams as SSH connections
	for {
		stream, err := session.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if session.IsClosed() {
				return nil
			}
			return fmt.Errorf("yamux accept stream error: %w", err)
		}

		go a.sshServer.HandleConn(stream)
	}
}

func getPrivateIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "unknown"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ip := ipnet.IP.String()
				// Return common private RFC1918 addresses
				if strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") || strings.HasPrefix(ip, "192.168.") {
					return ip
				}
			}
		}
	}
	// Fallback to first non-loopback
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}
