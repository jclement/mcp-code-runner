# MCP Protocol Implementation

This server implements the Model Context Protocol (MCP) specification version 2024-11-05 using **HTTP with SSE (Server-Sent Events) transport**.

## Transport Architecture

Per the MCP spec, the server uses a **single endpoint** (`/mcp`) that supports both:
- **POST requests**: Send JSON-RPC messages and receive responses (either JSON or SSE stream)
- **GET requests**: Establish SSE streams for server-initiated messages

This is known as "Streamable HTTP" transport in the MCP specification.

## Implemented Methods

### 1. `initialize`
**Description**: MCP initialization handshake

**Request**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "clientInfo": {
      "name": "client-name",
      "version": "1.0.0"
    }
  }
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "serverInfo": {
      "name": "mcp-code-sandbox",
      "version": "1.0.0"
    },
    "capabilities": {
      "tools": {}
    }
  }
}
```

### 2. `tools/list`
**Description**: List all available tools

**Request**:
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list"
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "sandbox.run_code",
        "description": "Execute code in a sandboxed Docker container. Supports: [python, typescript]. Files created in /data are persisted and accessible via download URLs.",
        "inputSchema": {
          "type": "object",
          "properties": {
            "conversationId": {
              "type": "string",
              "description": "Unique identifier for the conversation/session to isolate sandbox environments"
            },
            "language": {
              "type": "string",
              "description": "Programming language to execute. Available: [python, typescript]",
              "enum": ["python", "typescript"]
            },
            "code": {
              "type": "string",
              "description": "The code to execute. Any files written to /data will be persisted and returned as downloadable URLs."
            },
            "network": {
              "type": "boolean",
              "description": "Enable network access for the container (default: false for security)"
            },
            "environment": {
              "type": "object",
              "description": "Environment variables to pass to the container (e.g., API keys, configuration)",
              "additionalProperties": {
                "type": "string"
              }
            }
          },
          "required": ["conversationId", "language", "code"]
        }
      },
      {
        "name": "sandbox.list_runners",
        "description": "List all available code execution runners and their Docker images",
        "inputSchema": {
          "type": "object"
        }
      }
    ]
  }
}
```

### 3. `tools/call`
**Description**: Execute a specific tool

#### Tool: `sandbox.run_code`

**Request**:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "sandbox.run_code",
    "arguments": {
      "conversationId": "user-123-session-456",
      "language": "python",
      "code": "import matplotlib.pyplot as plt\nimport numpy as np\n\nx = np.linspace(0, 10, 100)\ny = np.sin(x)\n\nplt.plot(x, y)\nplt.savefig('/data/plot.png')\nprint('Plot saved!')",
      "network": false,
      "environment": {
        "API_KEY": "secret-value"
      }
    }
  }
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {
        "type": "output",
        "data": {
          "success": true,
          "output": "Plot saved!\n",
          "files": [
            {
              "name": "plot.png",
              "url": "https://example.com/files/abc123.../plot.png"
            }
          ]
        }
      }
    ]
  }
}
```

#### Tool: `sandbox.list_runners`

**Request**:
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "sandbox.list_runners",
    "arguments": {}
  }
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "result": {
    "content": [
      {
        "type": "output",
        "data": {
          "languages": [
            {
              "language": "python",
              "image": "mcp-sandbox-runner-python:latest"
            },
            {
              "language": "typescript",
              "image": "mcp-sandbox-runner-typescript:latest"
            }
          ]
        }
      }
    ]
  }
}
```

## Error Handling

Standard JSON-RPC error codes:
- `-32700`: Parse error
- `-32600`: Invalid request
- `-32601`: Method not found
- `-32602`: Invalid params
- `-32603`: Internal error

**Error Response Example**:
```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "error": {
    "code": -32602,
    "message": "conversationId is required",
    "data": null
  }
}
```

## Logging

All MCP operations are logged with the `[MCP]` prefix for easy filtering:

```
[MCP] Incoming request - Method: initialize, ID: 1
[MCP] Processing initialize request
[MCP] Initialize successful
[MCP] Incoming request - Method: tools/list, ID: 2
[MCP] Building tools list
[MCP] Found 2 runners
[MCP] Returning 2 tools
[MCP] Incoming request - Method: tools/call, ID: 3
[MCP] Tool call: sandbox.run_code
[MCP] Parsing run_code arguments
[MCP] run_code: conversationId=user-123, language=python, codeLen=150, network=false, envVars=1
[MCP] Using runner: mcp-sandbox-runner-python:latest
[MCP] Creating sandbox directory for conversation user-123
[MCP] Sandbox directory created: abc123def456...
[MCP] Sandbox host path: /sandbox-data/abc123def456...
[MCP] Executing python code for conversation user-123 (network: false, env vars: 1)
[MCP] Execution completed: success=true, exitCode=0
[MCP] Listing files in sandbox
[MCP] Found 1 files
[MCP] File available: plot.png -> https://example.com/files/abc123.../plot.png
[MCP] run_code completed successfully
```

HTTP layer logs use `[HTTP]` prefix:
```
[HTTP] MCP request from 127.0.0.1:54321
[HTTP] Request body: {"jsonrpc":"2.0",...}
[HTTP] Parsed JSON-RPC request: method=initialize, id=1
[HTTP] Sending response for method=initialize
[HTTP] Response: {"jsonrpc":"2.0","id":1,"result":{...}}
```

## Client Implementation

### n8n Setup

1. Configure HTTP Request node:
   - **Method**: POST
   - **URL**: `https://your-server.com/mcp`
   - **Authentication**: Header Auth
     - **Name**: `Authorization`
     - **Value**: `Bearer YOUR_API_TOKEN`
   - **Body**: JSON
   - **Content-Type**: `application/json`

2. Initialize connection:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05"
  }
}
```

3. List available tools:
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list"
}
```

4. Execute code:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "sandbox.run_code",
    "arguments": {
      "conversationId": "{{$json.sessionId}}",
      "language": "python",
      "code": "{{$json.code}}"
    }
  }
}
```

## Security

- All requests require `Authorization: Bearer <token>` header
- File URLs use hashed directory names (SHA256 of conversationID + secret)
- Network access disabled by default
- Resource limits enforced (256MB RAM, 0.5 CPU)
- Containers run as non-root user (UID 1000)
