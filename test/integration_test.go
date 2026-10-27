package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ssh-mesh/ssh-mesh/internal/agent/client"
	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"github.com/ssh-mesh/ssh-mesh/internal/server/api"
	"github.com/ssh-mesh/ssh-mesh/internal/server/broker"
	"github.com/ssh-mesh/ssh-mesh/internal/server/sshbridge"
	"github.com/ssh-mesh/ssh-mesh/internal/server/tunnel"
)

func TestEndToEndReverseSSH(t *testing.T) {
	testToken := "test-secret-token"
	agentID := "test-vps-node-1"

	// 1. Setup Server
	b := broker.NewMemoryBroker()
	tunnelH := tunnel.NewHandler(b, testToken)
	bridgeH := sshbridge.NewHandler(b, testToken)
	apiH := api.NewServerAPI(b, testToken, "")

	mux := http.NewServeMux()
	mux.Handle("/api/v1/agent/tunnel", tunnelH)
	mux.Handle("/api/v1/ws/terminal", bridgeH)
	apiH.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 2. Start Agent in background
	agentCtx, cancelAgent := context.WithCancel(context.Background())
	defer cancelAgent()

	agent, err := client.New(client.Config{
		ServerURL: wsURL + "/api/v1/agent/tunnel",
		AgentID:   agentID,
		Token:     testToken,
		Shell:     "/bin/sh",
		Version:   "test-0.1",
	})
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	go func() {
		_ = agent.Run(agentCtx)
	}()

	// Wait for agent to connect and register
	var agentFound bool
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		info, err := b.Get(agentID)
		if err == nil && info.Status == "online" {
			agentFound = true
			break
		}
	}
	if !agentFound {
		t.Fatalf("Agent failed to register with server within 2 seconds")
	}

	// 3. Verify REST API returns agent
	resp, err := http.Get(server.URL + "/api/v1/agents")
	if err != nil {
		t.Fatalf("Failed to query /api/v1/agents: %v", err)
	}
	defer resp.Body.Close()

	var apiResp struct {
		Agents []*protocol.AgentInfo `json:"agents"`
		Count  int                   `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("Failed to decode agents JSON: %v", err)
	}
	if apiResp.Count != 1 || apiResp.Agents[0].ID != agentID {
		t.Fatalf("Expected 1 agent with ID %s, got: %+v", agentID, apiResp.Agents)
	}

	// 4. Connect Web Terminal via WebSocket (emulating browser Xterm.js)
	terminalURL := wsURL + "/api/v1/ws/terminal?vm_id=" + agentID + "&cols=80&rows=24"
	dialer := websocket.DefaultDialer
	wsConn, _, err := dialer.Dial(terminalURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect terminal websocket: %v", err)
	}
	defer wsConn.Close()

	// Send a test command to the PTY
	command := "echo 'MESH_TEST_SUCCESS_OK'\n"
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte(command)); err != nil {
		t.Fatalf("Failed to send command to terminal: %v", err)
	}

	// Read output until we see our marker
	receivedOutput := ""
	timeout := time.After(3 * time.Second)
	foundMarker := false

	for !foundMarker {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for shell output. Received so far: %q", receivedOutput)
		default:
			_ = wsConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			_, msg, err := wsConn.ReadMessage()
			if err == nil {
				receivedOutput += string(msg)
				if strings.Contains(receivedOutput, "MESH_TEST_SUCCESS_OK") {
					foundMarker = true
				}
			}
		}
	}

	t.Logf("Successfully received shell output over reverse SSH: %s", receivedOutput)
}
