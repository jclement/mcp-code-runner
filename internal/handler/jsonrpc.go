package handler

import (
	"encoding/json"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// ToolCallParams represents the params for a tools/call method
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult represents the result wrapper for MCP tools
type ToolResult struct {
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block in the tool result
// Per MCP spec, content must be text, image, audio, or resource
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// RunCodeArguments represents arguments for sandbox.run_code
type RunCodeArguments struct {
	ConversationID string            `json:"conversationId"`
	Language       string            `json:"language"`
	Code           string            `json:"code"`
	Network        *bool             `json:"network,omitempty"`     // Optional: defaults to false (network disabled)
	Environment    map[string]string `json:"environment,omitempty"` // Optional: environment variables to pass to container
}

// FileDescriptor describes a file with its download URL
type FileDescriptor struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// RunCodeResult represents the result of code execution
type RunCodeResult struct {
	Success bool             `json:"success"`
	Output  string           `json:"output"`
	Files   []FileDescriptor `json:"files"`
}

// RunnerDescriptor describes an available runner
type RunnerDescriptor struct {
	Language string `json:"language"`
	Image    string `json:"image"`
}

// ListRunnersResult represents the result of listing runners
type ListRunnersResult struct {
	Languages []RunnerDescriptor `json:"languages"`
}

// NewSuccessResponse creates a successful JSON-RPC response
func NewSuccessResponse(id interface{}, result interface{}) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates an error JSON-RPC response
func NewErrorResponse(id interface{}, code int, message string, data interface{}) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}
