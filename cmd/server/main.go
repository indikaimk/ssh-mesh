package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ssh-mesh/ssh-mesh/internal/server/api"
	"github.com/ssh-mesh/ssh-mesh/internal/server/broker"
	"github.com/ssh-mesh/ssh-mesh/internal/server/sshbridge"
	"github.com/ssh-mesh/ssh-mesh/internal/server/tunnel"
)

func main() {
	addr := flag.String("addr", getEnv("SSH_MESH_ADDR", ":8080"), "Control plane listen address")
	token := flag.String("token", getEnv("SSH_MESH_TOKEN", "mesh-secret-token"), "Enrollment authentication token for agents")
	webDir := flag.String("web-dir", getEnv("SSH_MESH_WEB_DIR", "./web/dist"), "Path to frontend static assets")
	flag.Parse()

	// 1. Initialize Broker (decoupled interface for scaling)
	sessionBroker := broker.NewMemoryBroker()

	// 2. Initialize Tunnel and Bridge handlers
	tunnelHandler := tunnel.NewHandler(sessionBroker, *token)
	terminalBridge := sshbridge.NewHandler(sessionBroker, *token)
	restAPI := api.NewServerAPI(sessionBroker, *token, "")

	mux := http.NewServeMux()

	// Reverse tunnel endpoint for agents
	mux.Handle("/api/v1/agent/tunnel", tunnelHandler)

	// Web terminal endpoint for browser Xterm.js
	mux.Handle("/api/v1/ws/terminal", terminalBridge)

	// REST APIs for web dashboard
	restAPI.RegisterRoutes(mux)

	// Static web frontend handler with SPA fallback
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Pass API routes through
		if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
			http.NotFound(w, r)
			return
		}

		path := filepath.Join(*webDir, filepath.Clean(r.URL.Path))
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}

		// Fallback to index.html for React SPA
		indexPath := filepath.Join(*webDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}

		// Friendly message if frontend hasn't been built yet
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>SSH-Mesh Control Plane</title></head>
<body style="font-family:sans-serif; background:#0f172a; color:#f8fafc; padding:40px; text-align:center;">
  <h2>🚀 SSH-Mesh Control Plane Running</h2>
  <p>Agent Tunnel: <code>ws://localhost%s/api/v1/agent/tunnel</code></p>
  <p>Terminal Bridge: <code>ws://localhost%s/api/v1/ws/terminal</code></p>
  <p>API Status: <a href="/api/v1/health" style="color:#38bdf8;">/api/v1/health</a> | <a href="/api/v1/agents" style="color:#38bdf8;">/api/v1/agents</a></p>
  <p style="color:#94a3b8; margin-top:20px;">Frontend assets are currently building or will be served from <code>%s</code></p>
</body>
</html>`, *addr, *addr, *webDir)
	})

	server := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Println("==================================================")
		fmt.Println("  SSH-Mesh Control Plane Server")
		fmt.Printf("  Listening on  : http://localhost%s\n", *addr)
		fmt.Printf("  Enroll Token  : %s\n", *token)
		fmt.Println("==================================================")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server Listen error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down control plane server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped cleanly.")
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
