package handler

import (
	"context"
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
	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		return NewErrorResponse(req.ID, InvalidRequest, "Invalid JSON-RPC version", nil)
	}

	// Route based on method
	switch req.Method {
	case "tools/call":
		return h.handleToolCall(ctx, req)
	default:
		return NewErrorResponse(req.ID, MethodNotFound, fmt.Sprintf("Method not found: %s", req.Method), nil)
	}
}

// handleToolCall handles the tools/call method
func (h *MCPHandler) handleToolCall(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, InvalidParams, "Invalid params", err.Error())
	}

	switch params.Name {
	case "sandbox.run_code":
		return h.handleRunCode(ctx, req.ID, params.Arguments)
	case "sandbox.list_runners":
		return h.handleListRunners(req.ID)
	default:
		return NewErrorResponse(req.ID, MethodNotFound, fmt.Sprintf("Tool not found: %s", params.Name), nil)
	}
}

// handleRunCode implements the sandbox.run_code tool
func (h *MCPHandler) handleRunCode(ctx context.Context, id interface{}, argsJSON json.RawMessage) JSONRPCResponse {
	var args RunCodeArguments
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return NewErrorResponse(id, InvalidParams, "Invalid arguments", err.Error())
	}

	// Validate arguments
	if args.ConversationID == "" {
		return NewErrorResponse(id, InvalidParams, "conversationId is required", nil)
	}
	if args.Language == "" {
		return NewErrorResponse(id, InvalidParams, "language is required", nil)
	}
	if args.Code == "" {
		return NewErrorResponse(id, InvalidParams, "code is required", nil)
	}

	// Get runner for language
	runnerInfo, ok := h.registry.GetRunner(args.Language)
	if !ok {
		result := RunCodeResult{
			Success: false,
			Output:  fmt.Sprintf("Unsupported language: %s", args.Language),
			Files:   []FileDescriptor{},
		}
		return h.wrapToolResult(id, result)
	}

	// Ensure sandbox directory exists (creates on filesystem)
	_, err := h.sandbox.EnsureSandboxDir(args.ConversationID)
	if err != nil {
		log.Printf("Failed to create sandbox directory: %v", err)
		result := RunCodeResult{
			Success: false,
			Output:  fmt.Sprintf("Failed to create sandbox: %v", err),
			Files:   []FileDescriptor{},
		}
		return h.wrapToolResult(id, result)
	}

	// Get the host path for bind mounting into runner container
	sandboxHostPath := h.sandbox.GetSandboxHostPath(args.ConversationID)

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

	// Execute code in container (use host path for bind mount)
	log.Printf("Executing %s code for conversation %s (network: %v, env vars: %d)", args.Language, args.ConversationID, networkEnabled, len(env))
	execResult := h.executor.Execute(ctx, runnerInfo.Image, sandboxHostPath, args.Code, networkEnabled, env)

	// List files in sandbox
	files, err := h.sandbox.ListFiles(args.ConversationID)
	if err != nil {
		log.Printf("Failed to list files: %v", err)
		files = []string{}
	}

	// Create file descriptors with signed URLs
	fileDescriptors := make([]FileDescriptor, 0, len(files))
	for _, filename := range files {
		fileDescriptors = append(fileDescriptors, FileDescriptor{
			Name: filename,
			URL:  h.signer.MakeFileURL(args.ConversationID, filename),
		})
	}

	result := RunCodeResult{
		Success: execResult.Success,
		Output:  execResult.Output,
		Files:   fileDescriptors,
	}

	return h.wrapToolResult(id, result)
}

// handleListRunners implements the sandbox.list_runners tool
func (h *MCPHandler) handleListRunners(id interface{}) JSONRPCResponse {
	runners := h.registry.ListRunners()
	descriptors := make([]RunnerDescriptor, 0, len(runners))
	for _, r := range runners {
		descriptors = append(descriptors, RunnerDescriptor{
			Language: r.Language,
			Image:    r.Image,
		})
	}

	result := ListRunnersResult{
		Languages: descriptors,
	}

	return h.wrapToolResult(id, result)
}

// wrapToolResult wraps a result in the MCP tool result format
func (h *MCPHandler) wrapToolResult(id interface{}, data interface{}) JSONRPCResponse {
	toolResult := ToolResult{
		Content: []ContentBlock{
			{
				Type: "output",
				Data: data,
			},
		},
	}
	return NewSuccessResponse(id, toolResult)
}
