package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jsc/mcp-code-sandbox/internal/auth"
	"github.com/jsc/mcp-code-sandbox/internal/filesign"
	"github.com/jsc/mcp-code-sandbox/internal/sandbox"
)

// Server handles HTTP requests
type Server struct {
	mcpHandler *MCPHandler
	signer     *filesign.Signer
	sandbox    *sandbox.Manager
	apiToken   string
}

// NewServer creates a new HTTP server
func NewServer(
	mcpHandler *MCPHandler,
	signer *filesign.Signer,
	sandbox *sandbox.Manager,
	apiToken string,
) *Server {
	return &Server{
		mcpHandler: mcpHandler,
		signer:     signer,
		sandbox:    sandbox,
		apiToken:   apiToken,
	}
}

// SetupRoutes sets up all HTTP routes
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	// Homepage - web interface for testing
	mux.HandleFunc("/", s.handleHomepage)

	// MCP endpoints with authentication
	authMW := auth.Middleware(s.apiToken)
	mux.Handle("/mcp", authMW(http.HandlerFunc(s.handleMCP)))
	mux.Handle("/mcp/events", authMW(http.HandlerFunc(s.handleSSE)))

	// File download endpoint (no auth, uses signed URLs)
	mux.HandleFunc("/files/", s.handleFileDownload)
}

// handleMCP handles JSON-RPC requests
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON-RPC request
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("Failed to parse JSON-RPC request: %v", err)
		resp := NewErrorResponse(nil, ParseError, "Parse error", err.Error())
		s.writeJSONResponse(w, resp)
		return
	}

	// Handle request
	resp := s.mcpHandler.Handle(r.Context(), req)

	// Write response
	s.writeJSONResponse(w, resp)
}

// handleSSE handles Server-Sent Events for MCP notifications
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send initial comment
	fmt.Fprintf(w, ": SSE endpoint connected\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Keep connection open
	// In v1, we just keep the connection alive for heartbeat
	// Future versions can send real-time notifications here
	<-r.Context().Done()
}

// handleFileDownload handles file download requests with signature verification
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse URL path: /files/{conversationId}/{filename}
	path := strings.TrimPrefix(r.URL.Path, "/files/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	conversationID := parts[0]
	filename := parts[1]

	// Get signature from query
	sig := r.URL.Query().Get("sig")
	if sig == "" {
		http.Error(w, "Missing signature", http.StatusForbidden)
		return
	}

	// Verify signature
	if !s.signer.VerifySignature(conversationID, filename, sig) {
		log.Printf("Invalid signature for file: %s/%s", conversationID, filename)
		http.Error(w, "Invalid signature", http.StatusForbidden)
		return
	}

	// Get file path
	filePath := s.sandbox.GetFilePath(conversationID, filename)

	// Check if file exists and is regular file
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		log.Printf("Error checking file: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if info.IsDir() {
		http.Error(w, "Not a file", http.StatusBadRequest)
		return
	}

	// Prevent path traversal
	sandboxDir := s.sandbox.GetSandboxDir(conversationID)
	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(sandboxDir)) {
		log.Printf("Path traversal attempt: %s", filePath)
		http.Error(w, "Invalid file path", http.StatusForbidden)
		return
	}

	// Serve file
	log.Printf("Serving file: %s", filePath)
	http.ServeFile(w, r, filePath)
}

// handleHomepage serves the web interface
func (s *Server) handleHomepage(w http.ResponseWriter, r *http.Request) {
	// Only serve at root path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Serve the static HTML file
	http.ServeFile(w, r, "static/index.html")
}

// writeJSONResponse writes a JSON-RPC response
func (s *Server) writeJSONResponse(w http.ResponseWriter, resp JSONRPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}
