package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ssh-mesh/ssh-mesh/internal/agent/client"
)

func main() {
	serverURL := flag.String("server", getEnv("SSH_MESH_SERVER", "ws://localhost:8080/api/v1/agent/tunnel"), "Control plane WebSocket tunnel URL")
	agentID := flag.String("id", getEnv("SSH_MESH_AGENT_ID", ""), "Unique agent ID (defaults to hostname)")
	token := flag.String("token", getEnv("SSH_MESH_TOKEN", "mesh-secret-token"), "Enrollment authentication token")
	shell := flag.String("shell", getEnv("SSH_MESH_SHELL", ""), "Custom shell path (defaults to bash/sh)")
	tagsRaw := flag.String("tags", getEnv("SSH_MESH_TAGS", ""), "Comma-separated node tags (e.g. env=staging,team=backend)")
	flag.Parse()

	if *serverURL == "" {
		log.Fatalf("Error: server URL must be specified via -server or SSH_MESH_SERVER")
	}

	cfg := client.Config{
		ServerURL: *serverURL,
		AgentID:   *agentID,
		Token:     *token,
		Shell:     *shell,
		Tags:      parseTags(*tagsRaw),
		Version:   "0.1.0",
	}

	agent, err := client.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize agent: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("==================================================")
	fmt.Println("  SSH-Mesh Agent (VPS Reverse Tunnel)")
	fmt.Printf("  Target Server : %s\n", *serverURL)
	fmt.Printf("  Token Config  : %s\n", maskToken(*token))
	if len(cfg.Tags) > 0 {
		fmt.Printf("  Node Tags     : %v\n", cfg.Tags)
	}
	fmt.Println("==================================================")

	if err := agent.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Agent terminated with error: %v", err)
	}
	log.Println("Agent stopped cleanly.")
}

func parseTags(raw string) map[string]string {
	tags := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		var k, v string
		if strings.Contains(pair, "=") {
			parts := strings.SplitN(pair, "=", 2)
			k, v = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		} else if strings.Contains(pair, ":") {
			parts := strings.SplitN(pair, ":", 2)
			k, v = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
		if k != "" {
			tags[k] = v
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func maskToken(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return token[:2] + "****" + token[len(token)-2:]
}
