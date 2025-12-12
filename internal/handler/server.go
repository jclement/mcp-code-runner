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

	// MCP endpoint with authentication (supports both POST and GET)
	// Per MCP spec: single endpoint for HTTP + SSE transport
	authMW := auth.Middleware(s.apiToken)
	mux.Handle("/mcp", authMW(http.HandlerFunc(s.handleMCP)))

	// File download endpoint (no auth, URLs use hashed directory names for security)
	mux.HandleFunc("/files/", s.handleFileDownload)
}

// handleMCP handles MCP requests (HTTP + SSE transport)
// Per MCP spec: single endpoint supporting POST (JSON-RPC) and GET (SSE resumption)
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[HTTP] MCP %s request from %s", r.Method, r.RemoteAddr)

	switch r.Method {
	case http.MethodPost:
		s.handleMCPPost(w, r)
	case http.MethodGet:
		s.handleMCPGet(w, r)
	default:
		log.Printf("[HTTP] Invalid method: %s (only POST and GET allowed)", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleMCPPost handles POST requests with JSON-RPC messages
func (s *Server) handleMCPPost(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[HTTP] Failed to read request body: %v", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("[HTTP] Request body: %s", string(body))

	// Parse JSON-RPC request
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("[HTTP] Failed to parse JSON-RPC request: %v", err)
		resp := NewErrorResponse(nil, ParseError, "Parse error", err.Error())
		s.writeJSONResponse(w, resp)
		return
	}

	log.Printf("[HTTP] Parsed JSON-RPC request: method=%s, id=%v", req.Method, req.ID)

	// Check if client accepts SSE
	acceptsSSE := false
	acceptHeader := r.Header.Get("Accept")
	if acceptHeader != "" {
		for _, mediaType := range []string{"text/event-stream", "*/*"} {
			if contains(acceptHeader, mediaType) {
				acceptsSSE = true
				break
			}
		}
	}

	log.Printf("[HTTP] Client accepts SSE: %v (Accept: %s)", acceptsSSE, acceptHeader)

	// Handle request
	resp := s.mcpHandler.Handle(r.Context(), req)

	// For simple request/response, use JSON
	// In the future, we could use SSE for streaming responses
	log.Printf("[HTTP] Sending JSON response for method=%s", req.Method)
	s.writeJSONResponse(w, resp)
}

// handleMCPGet handles GET requests for SSE event stream resumption
func (s *Server) handleMCPGet(w http.ResponseWriter, r *http.Request) {
	log.Printf("[HTTP] SSE stream request (Last-Event-ID: %s)", r.Header.Get("Last-Event-ID"))

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Send initial connection message
	fmt.Fprintf(w, ": MCP SSE stream connected\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	log.Printf("[HTTP] SSE stream established")

	// Keep connection open
	// In the future, we can send server-initiated requests/notifications here
	<-r.Context().Done()
	log.Printf("[HTTP] SSE stream closed")
}


// handleFileDownload handles file download requests
// URLs are secure because the hashedDir is SHA256(conversationID + secret)
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse URL path: /files/{hashedDir}/{filename}
	path := strings.TrimPrefix(r.URL.Path, "/files/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	hashedDir := parts[0]
	filename := parts[1]

	// Validate hashedDir is a valid hex string (16 chars for truncated SHA256)
	if len(hashedDir) != 16 {
		http.Error(w, "Invalid directory hash", http.StatusBadRequest)
		return
	}

	// Get file path using the hashed directory
	filePath := s.sandbox.GetFilePath(hashedDir, filename)

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

	// Prevent path traversal - ensure file is within sandbox root
	sandboxRoot := s.sandbox.GetSandboxRoot()
	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(sandboxRoot)) {
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

	// Marshal to JSON for logging
	respJSON, _ := json.Marshal(resp)
	log.Printf("[HTTP] Response: %s", string(respJSON))

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[HTTP] Failed to write response: %v", err)
	}
}

// contains checks if a string contains a substring (case-insensitive for media types)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
