package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/client"
	"github.com/jsc/mcp-code-sandbox/internal/config"
	"github.com/jsc/mcp-code-sandbox/internal/filesign"
	"github.com/jsc/mcp-code-sandbox/internal/handler"
	"github.com/jsc/mcp-code-sandbox/internal/runner"
	"github.com/jsc/mcp-code-sandbox/internal/sandbox"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting MCP Code Sandbox Server...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  HTTP Address: %s", cfg.HTTPAddr)
	log.Printf("  Public Base URL: %s", cfg.PublicBaseURL)
	log.Printf("  Sandbox Root: %s", cfg.SandboxRoot)
	if cfg.SandboxHostPath != cfg.SandboxRoot {
		log.Printf("  Sandbox Host Path: %s (for Docker bind mounts)", cfg.SandboxHostPath)
	}

	// Create Docker client
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer dockerClient.Close()

	// Ping Docker to ensure connection
	ctx := context.Background()
	if _, err := dockerClient.Ping(ctx); err != nil {
		log.Fatalf("Failed to connect to Docker: %v", err)
	}
	log.Println("Connected to Docker daemon")

	// Discover runner images
	registry, err := runner.NewRegistry(ctx, dockerClient)
	if err != nil {
		log.Fatalf("Failed to create runner registry: %v", err)
	}

	runners := registry.ListRunners()
	log.Printf("Discovered %d runner(s):", len(runners))
	for _, r := range runners {
		log.Printf("  - %s: %s", r.Language, r.Image)
	}

	if len(runners) == 0 {
		log.Println("WARNING: No runner images found. Please build runner images with labels:")
		log.Println("  sandbox.runner=true")
		log.Println("  sandbox.language=<language>")
	}

	// Ensure sandbox root directory exists
	if err := os.MkdirAll(cfg.SandboxRoot, 0o755); err != nil {
		log.Fatalf("Failed to create sandbox root directory: %v", err)
	}

	// Create components
	sandboxMgr := sandbox.NewManager(cfg.SandboxRoot, cfg.SandboxHostPath)
	signer := filesign.NewSigner(cfg.FileSecret, cfg.PublicBaseURL)
	executor := runner.NewExecutor(dockerClient, 30*time.Second)

	// Create handlers
	mcpHandler := handler.NewMCPHandler(registry, executor, sandboxMgr, signer)
	httpServer := handler.NewServer(mcpHandler, signer, sandboxMgr, cfg.APIToken)

	// Setup HTTP routes
	mux := http.NewServeMux()
	httpServer.SetupRoutes(mux)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
