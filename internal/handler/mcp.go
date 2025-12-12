package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"github.com/jsc/mcp-code-sandbox/internal/filesign"
	"github.com/jsc/mcp-code-sandbox/internal/runner"
	"github.com/jsc/mcp-code-sandbox/internal/sandbox"
)

// MCPHandler handles MCP JSON-RPC requests
type MCPHandler struct {
	registry *runner.Registry
	executor *runner.Executor
	sandbox  *sandbox.Manager
	signer   *filesign.Signer
}

// NewMCPHandler creates a new MCP handler
func NewMCPHandler(
	registry *runner.Registry,
	executor *runner.Executor,
	sandbox *sandbox.Manager,
	signer *filesign.Signer,
) *MCPHandler {
	return &MCPHandler{
		registry: registry,
		executor: executor,
		sandbox:  sandbox,
		signer:   signer,
	}
}

// Handle processes a JSON-RPC request
func (h *MCPHandler) Handle(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	log.Printf("[MCP] Incoming request - Method: %s, ID: %v", req.Method, req.ID)

	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		log.Printf("[MCP] Invalid JSON-RPC version: %s", req.JSONRPC)
		return NewErrorResponse(req.ID, InvalidRequest, "Invalid JSON-RPC version", nil)
	}

	// Route based on method
	switch req.Method {
	case "initialize":
		log.Printf("[MCP] Handling initialize request")
		return h.handleInitialize(req)
	case "tools/list":
		log.Printf("[MCP] Handling tools/list request")
		return h.handleToolsList(req)
	case "tools/call":
		log.Printf("[MCP] Handling tools/call request")
		return h.handleToolCall(ctx, req)
	default:
		log.Printf("[MCP] Method not found: %s", req.Method)
		return NewErrorResponse(req.ID, MethodNotFound, fmt.Sprintf("Method not found: %s", req.Method), nil)
	}
}

// handleInitialize handles the MCP initialize method
func (h *MCPHandler) handleInitialize(req JSONRPCRequest) JSONRPCResponse {
	log.Printf("[MCP] Processing initialize request")

	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]interface{}{
			"name":    "mcp-code-sandbox",
			"version": "1.0.0",
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
	}

	log.Printf("[MCP] Initialize successful")
	return NewSuccessResponse(req.ID, result)
}

// handleToolsList handles the MCP tools/list method
func (h *MCPHandler) handleToolsList(req JSONRPCRequest) JSONRPCResponse {
	log.Printf("[MCP] Building tools list")

	// Get available runners
	runners := h.registry.ListRunners()
	log.Printf("[MCP] Found %d runners", len(runners))

	// Build language list for tool description
	languages := make([]string, 0, len(runners))
	for _, r := range runners {
		languages = append(languages, r.Language)
	}

	// Build library information per language
	libraryInfo := map[string][]string{
		"python":     {"requests", "numpy", "pandas", "matplotlib", "psycopg2"},
		"typescript": {"postgres", "pg", "csv-parser", "papaparse"},
	}

	// Create comprehensive description with examples
	description := fmt.Sprintf(`Execute code in a sandboxed Docker container. Supports: %v. Files in /data persist across executions and are accessible via download URLs.

Available libraries:
`, languages)
	for lang, libs := range libraryInfo {
		description += fmt.Sprintf("- %s: %v\n", lang, libs)
	}

	description += `
Environment variables (auto-injected):
- FILE_BASE_URL: Base URL for generated files (e.g., "https://example.com/files/abc123...")

⚠️ CRITICAL: You MUST use FILE_BASE_URL for ALL file references in markdown!
   - Correct: f"{os.environ['FILE_BASE_URL']}/chart.png"
   - WRONG: "/data/chart.png" or "chart.png" (will be broken links!)
   - File paths in markdown MUST be full URLs using FILE_BASE_URL

Working directory: Container starts in /data (files persist across executions)
- Save files: Use relative paths like "chart.png" or "./output.json"
- Read files: Use relative paths like "data.csv"
- NO need to use "/data/" prefix - you're already in that directory!

Example Python code showing CORRECT usage:
import os
import pandas as pd
import matplotlib.pyplot as plt

# Read file (relative path - already in /data)
df = pd.read_csv('data.csv')

# Create visualization (relative path)
plt.figure(figsize=(10, 6))
plt.bar(df['category'], df['value'])
plt.savefig('chart.png', dpi=150, bbox_inches='tight')

# Generate markdown with FULL URLs using FILE_BASE_URL
base_url = os.environ['FILE_BASE_URL']
report = f"""# Analysis Report

## Visualization
![Chart]({base_url}/chart.png)

## Summary
- Total records: {len(df)}
- Average value: {df['value'].mean():.2f}

[Download data]({base_url}/data.csv)
"""
print(report)

REMEMBER: ALL markdown links MUST use FILE_BASE_URL for proper rendering!`

	tools := []map[string]interface{}{
		{
			"name":        "upload_file",
			"description": "Upload a file to the sandbox for analysis. The file will be available in /data for code execution. Use this before run_code to provide data files (CSV, JSON, etc.) for analysis.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversationId": map[string]interface{}{
						"type":        "string",
						"description": "Unique identifier for the conversation/session",
					},
					"filename": map[string]interface{}{
						"type":        "string",
						"description": "Name of the file to create (e.g., 'data.csv', 'input.json')",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Base64 encoded file content",
					},
				},
				"required": []string{"conversationId", "filename", "content"},
			},
		},
		{
			"name":        "run_code",
			"description": description,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversationId": map[string]interface{}{
						"type":        "string",
						"description": "Unique identifier for the conversation/session to isolate sandbox environments",
					},
					"language": map[string]interface{}{
						"type":        "string",
						"description": fmt.Sprintf("Programming language to execute. Available: %v", languages),
						"enum":        languages,
					},
					"code": map[string]interface{}{
						"type":        "string",
						"description": "The code to execute. Any files written to /data will be persisted and returned as downloadable URLs.",
					},
					"network": map[string]interface{}{
						"type":        "boolean",
						"description": "Enable network access for the container (default: false for security)",
					},
					"environment": map[string]interface{}{
						"type":        "object",
						"description": "Environment variables to pass to the container (e.g., API keys, configuration)",
						"additionalProperties": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"required": []string{"conversationId", "language", "code"},
			},
		},
		{
			"name":        "list_runners",
			"description": "List all available code execution runners and their Docker images",
			"inputSchema": map[string]interface{}{
				"type": "object",
			},
		},
	}

	result := map[string]interface{}{
		"tools": tools,
	}

	log.Printf("[MCP] Returning %d tools", len(tools))
	return NewSuccessResponse(req.ID, result)
}

// handleToolCall handles the tools/call method
func (h *MCPHandler) handleToolCall(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("[MCP] Failed to parse tool call params: %v", err)
		return NewErrorResponse(req.ID, InvalidParams, "Invalid params", err.Error())
	}

	log.Printf("[MCP] Tool call: %s", params.Name)

	switch params.Name {
	case "upload_file":
		return h.handleUploadFile(req.ID, params.Arguments)
	case "run_code":
		return h.handleRunCode(ctx, req.ID, params.Arguments)
	case "list_runners":
		return h.handleListRunners(req.ID)
	default:
		log.Printf("[MCP] Unknown tool: %s", params.Name)
		return NewErrorResponse(req.ID, MethodNotFound, fmt.Sprintf("Tool not found: %s", params.Name), nil)
	}
}

// handleRunCode implements the sandbox.run_code tool
func (h *MCPHandler) handleRunCode(ctx context.Context, id interface{}, argsJSON json.RawMessage) JSONRPCResponse {
	log.Printf("[MCP] Parsing run_code arguments")
	var args RunCodeArguments
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		log.Printf("[MCP] Failed to parse arguments: %v", err)
		return NewErrorResponse(id, InvalidParams, "Invalid arguments", err.Error())
	}

	log.Printf("[MCP] run_code: conversationId=%s, language=%s, codeLen=%d, network=%v, envVars=%d",
		args.ConversationID, args.Language, len(args.Code), args.Network, len(args.Environment))

	// Validate arguments
	if args.ConversationID == "" {
		log.Printf("[MCP] Missing conversationId")
		return NewErrorResponse(id, InvalidParams, "conversationId is required", nil)
	}
	if args.Language == "" {
		log.Printf("[MCP] Missing language")
		return NewErrorResponse(id, InvalidParams, "language is required", nil)
	}
	if args.Code == "" {
		log.Printf("[MCP] Missing code")
		return NewErrorResponse(id, InvalidParams, "code is required", nil)
	}

	// Get runner for language
	runnerInfo, ok := h.registry.GetRunner(args.Language)
	if !ok {
		log.Printf("[MCP] Unsupported language: %s", args.Language)
		result := RunCodeResult{
			Success: false,
			Stderr:  fmt.Sprintf("Unsupported language: %s", args.Language),
		}
		return h.wrapToolResult(id, result)
	}

	log.Printf("[MCP] Using runner: %s", runnerInfo.Image)

	// Ensure sandbox directory exists (creates on filesystem)
	// Returns the hashed directory name which is safe to expose in URLs
	log.Printf("[MCP] Creating sandbox directory for conversation %s", args.ConversationID)
	hashedDir, err := h.sandbox.EnsureSandboxDir(args.ConversationID)
	if err != nil {
		log.Printf("[MCP] Failed to create sandbox directory: %v", err)
		result := RunCodeResult{
			Success: false,
			Stderr:  fmt.Sprintf("Failed to create sandbox: %v", err),
		}
		return h.wrapToolResult(id, result)
	}
	log.Printf("[MCP] Sandbox directory created: %s", hashedDir)

	// Get the host path for bind mounting into runner container
	sandboxHostPath := h.sandbox.GetSandboxHostPath(args.ConversationID)
	log.Printf("[MCP] Sandbox host path: %s", sandboxHostPath)

	// Determine network setting (defaults to false/disabled)
	networkEnabled := false
	if args.Network != nil {
		networkEnabled = *args.Network
	}

	// Use environment variables if provided, otherwise empty map
	env := args.Environment
	if env == nil {
		env = make(map[string]string)
	}

	// Inject FILE_BASE_URL so code can generate markdown with correct URLs
	fileBaseURL := fmt.Sprintf("%s/files/%s", h.signer.GetBaseURL(), hashedDir)
	env["FILE_BASE_URL"] = fileBaseURL

	// Execute code in container (use host path for bind mount)
	log.Printf("[MCP] Executing %s code for conversation %s (network: %v, env vars: %d)", args.Language, args.ConversationID, networkEnabled, len(env))
	execResult := h.executor.Execute(ctx, runnerInfo.Image, sandboxHostPath, args.Code, networkEnabled, env)
	log.Printf("[MCP] Execution completed: success=%v, exitCode=%d", execResult.Success, execResult.ExitCode)

	result := RunCodeResult{
		Success: execResult.Success,
		Stdout:  execResult.Stdout,
		Stderr:  execResult.Stderr,
	}

	log.Printf("[MCP] run_code completed successfully")
	return h.wrapToolResult(id, result)
}

// handleUploadFile implements the upload_file tool
func (h *MCPHandler) handleUploadFile(id interface{}, argsJSON json.RawMessage) JSONRPCResponse {
	log.Printf("[MCP] Parsing upload_file arguments")
	var args UploadFileArguments
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		log.Printf("[MCP] Failed to parse arguments: %v", err)
		return NewErrorResponse(id, InvalidParams, "Invalid arguments", err.Error())
	}

	log.Printf("[MCP] upload_file: conversationId=%s, filename=%s, contentLen=%d",
		args.ConversationID, args.Filename, len(args.Content))

	// Validate arguments
	if args.ConversationID == "" {
		log.Printf("[MCP] Missing conversationId")
		return NewErrorResponse(id, InvalidParams, "conversationId is required", nil)
	}
	if args.Filename == "" {
		log.Printf("[MCP] Missing filename")
		return NewErrorResponse(id, InvalidParams, "filename is required", nil)
	}
	if args.Content == "" {
		log.Printf("[MCP] Missing content")
		return NewErrorResponse(id, InvalidParams, "content is required", nil)
	}

	// Decode base64 content
	content, err := base64.StdEncoding.DecodeString(args.Content)
	if err != nil {
		log.Printf("[MCP] Failed to decode base64 content: %v", err)
		result := map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to decode base64 content: %v", err),
		}
		return h.wrapToolResult(id, result)
	}

	log.Printf("[MCP] Decoded %d bytes for file %s", len(content), args.Filename)

	// Write file to sandbox
	if err := h.sandbox.WriteFile(args.ConversationID, args.Filename, content); err != nil {
		log.Printf("[MCP] Failed to write file: %v", err)
		result := map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to write file: %v", err),
		}
		return h.wrapToolResult(id, result)
	}

	// Get the hashed directory name for URL
	hashedDir, err := h.sandbox.EnsureSandboxDir(args.ConversationID)
	if err != nil {
		log.Printf("[MCP] Failed to get hashed directory: %v", err)
		result := map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to get directory: %v", err),
		}
		return h.wrapToolResult(id, result)
	}

	// Create file URL
	baseURL := h.signer.GetBaseURL()
	fileURL := fmt.Sprintf("%s/files/%s/%s", baseURL, hashedDir, args.Filename)

	result := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("File '%s' uploaded successfully (%d bytes)", args.Filename, len(content)),
		"file": FileDescriptor{
			Name: args.Filename,
			URL:  fileURL,
		},
	}

	log.Printf("[MCP] upload_file completed: %s -> %s", args.Filename, fileURL)
	return h.wrapToolResult(id, result)
}

// handleListRunners implements the sandbox.list_runners tool
func (h *MCPHandler) handleListRunners(id interface{}) JSONRPCResponse {
	log.Printf("[MCP] Listing available runners")
	runners := h.registry.ListRunners()
	log.Printf("[MCP] Found %d runners", len(runners))

	descriptors := make([]RunnerDescriptor, 0, len(runners))
	for _, r := range runners {
		log.Printf("[MCP] Runner: %s -> %s", r.Language, r.Image)
		descriptors = append(descriptors, RunnerDescriptor{
			Language: r.Language,
			Image:    r.Image,
		})
	}

	result := ListRunnersResult{
		Languages: descriptors,
	}

	log.Printf("[MCP] list_runners completed")
	return h.wrapToolResult(id, result)
}

// wrapToolResult wraps a result in the MCP tool result format as text
func (h *MCPHandler) wrapToolResult(id interface{}, data interface{}) JSONRPCResponse {
	// Serialize data to JSON for text response
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("[MCP] Failed to marshal tool result: %v", err)
		jsonData = []byte(fmt.Sprintf("%v", data))
	}

	toolResult := ToolResult{
		Content: []ContentBlock{
			{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}
	return NewSuccessResponse(id, toolResult)
}
