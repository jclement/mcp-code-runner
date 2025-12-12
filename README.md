# MCP Code Sandbox Server

A Model Context Protocol (MCP) compatible HTTP server that executes Python and TypeScript code in isolated Docker containers with persistent file storage.

## Overview

This server implements the [MCP specification](https://spec.modelcontextprotocol.io/) (version 2024-11-05) using HTTP with Server-Sent Events (SSE) transport. It provides secure, sandboxed code execution for AI assistants like Claude, allowing them to run code and generate files that persist across executions.

**Key Features:**
- 🔒 **Secure sandboxing** - Code runs in isolated Docker containers with no network access
- 🗂️ **Persistent storage** - Files created in `/data` persist across executions per conversation
- 📦 **Multi-language** - Python and TypeScript/JavaScript support out of the box
- 🔐 **Authentication** - Bearer token auth with hashed directory security
- 📤 **File uploads** - Upload data files for analysis before code execution
- ⚡ **Fast TypeScript** - Powered by Bun for 4x faster startup than Node.js

## Quick Start

### Option 1: Using Start Script (Recommended)

```bash
# Clone and setup
git clone <repo-url>
cd code-runner

# Build and start
./start.sh

# Server will be available at http://localhost:8080
```

The `start.sh` script automatically:
- Loads configuration from `.env`
- Creates sandbox directories
- Checks Docker connectivity
- Builds runner images if needed
- Starts the server

### Option 2: Docker Compose

```bash
# Clone and setup
git clone <repo-url>
cd code-runner

# Copy environment template
cp .env.example .env
# Edit .env with your tokens

# Build runner images
./build.sh

# Start server
docker-compose up -d

# View logs
docker-compose logs -f
```

### Option 3: Direct Binary

```bash
# Build
go build -o mcp-code-sandbox ./cmd/server

# Run with environment variables
source .env
./mcp-code-sandbox
```

## Architecture

### System Design

```
┌─────────────────────────────────────────┐
│  MCP Client (Claude, n8n, etc.)        │
└───────────────┬─────────────────────────┘
                │ HTTPS + Bearer Token
                │ JSON-RPC 2.0
                ▼
┌─────────────────────────────────────────┐
│  MCP Server (Go)                        │
│  - HTTP + SSE Transport                 │
│  - Tools: upload_file, run_code         │
│  - Hashed directory security            │
└───────────────┬─────────────────────────┘
                │ Docker API
                ▼
┌─────────────────────────────────────────┐
│  Runner Containers (ephemeral)          │
│  - Python 3.12 (numpy, pandas, etc.)    │
│  - TypeScript/Bun (postgres, csv, etc.) │
│  - Bind mount: /data → sandbox dir      │
└─────────────────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│  Sandbox Filesystem                     │
│  /sandboxes/                            │
│    └── {SHA256(conversationId+secret)}/ │
│        ├── data.csv (uploaded)          │
│        └── plot.png (generated)         │
└─────────────────────────────────────────┘
```

### Components

1. **HTTP Server** - Handles MCP JSON-RPC requests (POST) and SSE streams (GET)
2. **Runner Registry** - Auto-discovers available language runners via Docker labels
3. **Container Executor** - Manages Docker container lifecycle with resource limits
4. **Sandbox Manager** - Handles per-conversation filesystem isolation with hashed directories
5. **Authentication** - Bearer token middleware for API security

### How Code Execution Works

1. Client uploads data file via `upload_file` tool (optional)
2. Client calls `run_code` tool with language and code
3. Server creates hashed sandbox directory for conversation
4. Server spins up ephemeral runner container with `/data` bind mount
5. Container executes code as non-root user (UID 1000)
6. Server lists files in sandbox and returns URLs
7. Client can download files via public URLs (no auth needed - security via hash)

## Configuration

### Environment Variables

Create a `.env` file (or copy `.env.example`):

```bash
# HTTP server
MCP_HTTP_ADDR=:8080
PUBLIC_BASE_URL=http://localhost:8080

# Authentication
MCP_API_TOKEN=your-secret-token-here

# Sandbox filesystem
SANDBOX_ROOT=/var/sandboxes          # Path inside server container
SANDBOX_HOST_PATH=/tmp/sandboxes     # Actual host path for Docker bind mounts
FILE_SECRET=your-file-signing-secret # Used for hashing conversation IDs

# Optional: Cloudflare Tunnel
TUNNEL_TOKEN=                        # Leave empty if not using Cloudflare
```

**Important Configuration Notes:**

- **`SANDBOX_ROOT`** - Path from the server's perspective (container or process)
- **`SANDBOX_HOST_PATH`** - Absolute path on the Docker host for bind mounts
  - When running server directly: Same as `SANDBOX_ROOT`
  - When running in Docker: Must point to actual host path
  - Example: Server in container sees `/var/sandboxes`, but mounts `/home/user/sandboxes` from host
- **`FILE_SECRET`** - Used to hash conversation IDs into directory names. Must be:
  - At least 32 characters
  - Randomly generated: `openssl rand -base64 32`
  - Kept secret - protects file access
- **`MCP_API_TOKEN`** - Bearer token for API authentication. Generate with: `openssl rand -hex 32`

### Dual-Path Architecture

The server uses a dual-path system to support both:
1. Direct execution (server process accesses local filesystem)
2. Docker Compose deployment (server in container, runners in sibling containers)

**Example: Docker Compose**
```yaml
# Server container
environment:
  SANDBOX_ROOT: /var/sandboxes              # Server's view
  SANDBOX_HOST_PATH: /host/sandbox-data     # Host's actual path

volumes:
  - ./sandbox-data:/var/sandboxes           # Mount host dir into server
  # Server will tell runners to mount: /host/sandbox-data:/data
```

**Example: Direct Execution**
```bash
# Both paths are the same
SANDBOX_ROOT=/tmp/sandboxes
SANDBOX_HOST_PATH=/tmp/sandboxes
```

## MCP Protocol

### Transport

The server implements **HTTP with SSE** transport (single endpoint):
- **POST `/mcp`** - Send JSON-RPC requests, receive JSON responses
- **GET `/mcp`** - Establish SSE stream for server-initiated messages

### Authentication

All `/mcp` requests require:
```
Authorization: Bearer <MCP_API_TOKEN>
```

### Methods

#### `initialize` - MCP Handshake

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "clientInfo": {"name": "client", "version": "1.0"}
  }
}
```

Response includes server capabilities (tools).

#### `tools/list` - List Available Tools

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list"
}
```

Returns three tools:
- `upload_file` - Upload data files to sandbox
- `run_code` - Execute code in sandboxed container
- `list_runners` - List available language runners

#### `tools/call` - Execute a Tool

See "Tools" section below for detailed examples.

## Tools

### `upload_file`

Upload a file to the conversation's sandbox before running code.

**Arguments:**
- `conversationId` (string) - Unique conversation identifier
- `filename` (string) - Name of file to create (e.g., `data.csv`)
- `content` (string) - Base64-encoded file content

**Example:**

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "upload_file",
      "arguments": {
        "conversationId": "session-123",
        "filename": "data.csv",
        "content": "bmFtZSxhZ2UKQWxpY2UsMzAKQm9iLDI1"
      }
    }
  }'
```

**Response:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [{
      "type": "text",
      "text": "{\"success\":true,\"message\":\"File 'data.csv' uploaded successfully (18 bytes)\",\"file\":{\"name\":\"data.csv\",\"url\":\"http://localhost:8080/files/abc123.../data.csv\"}}"
    }]
  }
}
```

### `run_code`

Execute code in a sandboxed Docker container.

**Arguments:**
- `conversationId` (string) - Unique conversation identifier
- `language` (string) - Language to execute: `python` or `typescript`
- `code` (string) - Source code to execute
- `network` (boolean, optional) - Enable network access (default: false)
- `environment` (object, optional) - Environment variables (e.g., API keys)

**Available Libraries:**
- **Python**: `requests`, `numpy`, `pandas`, `matplotlib`, `psycopg2`
- **TypeScript**: `postgres`, `pg`, `csv-parser`, `papaparse`

**Example: Python Data Analysis**

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "run_code",
      "arguments": {
        "conversationId": "session-123",
        "language": "python",
        "code": "import pandas as pd\nimport matplotlib.pyplot as plt\n\ndf = pd.read_csv(\"/data/data.csv\")\nprint(df.describe())\n\nplt.bar(df[\"name\"], df[\"age\"])\nplt.savefig(\"/data/chart.png\")\nprint(\"Chart saved!\")"
      }
    }
  }'
```

**Response:**

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [{
      "type": "text",
      "text": "{\"success\":true,\"output\":\"       age\\ncount   2.0\\nmean   27.5\\n...\\nChart saved!\\n\",\"files\":[{\"name\":\"data.csv\",\"url\":\"...\"},{\"name\":\"chart.png\",\"url\":\"...\"}]}"
    }]
  }
}
```

**Example: TypeScript with Network Access**

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "run_code",
      "arguments": {
        "conversationId": "session-123",
        "language": "typescript",
        "network": true,
        "code": "const response = await fetch(\"https://api.example.com/data\");\nconst data = await response.json();\nconsole.log(data);\n\nconst fs = require(\"fs\");\nfs.writeFileSync(\"/data/result.json\", JSON.stringify(data, null, 2));"
      }
    }
  }'
```

### `list_runners`

List available language runners and their Docker images.

**Example:**

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "list_runners"
    }
  }'
```

## Security

### Container Isolation

**Network Isolation:**
- Containers run with `NetworkDisabled: true` by default
- Only enabled when `network: true` explicitly passed
- Prevents unintended external connections

**User Permissions:**
- All runners execute as non-root user (UID 1000)
- Sandbox directories pre-created with `1000:1000` ownership
- Prevents privilege escalation

**Resource Limits:**
- **CPU**: 0.5 cores per container
- **Memory**: 256MB per container
- **Timeout**: 30 seconds maximum execution
- **Auto-cleanup**: Containers removed after execution

**Minimal Images:**
- Alpine Linux base for smaller attack surface
- Only essential packages installed
- No shells or unnecessary tools

### Hashed Directory Security

Conversation data is stored in directories named using SHA256 hashing:

```
Directory path: /sandboxes/{SHA256(conversationId + FILE_SECRET)}/
File URL: https://example.com/files/{hash}/{filename}
```

**Security Properties:**
1. **Unpredictable** - Cannot guess hash without knowing `FILE_SECRET`
2. **Filesystem-safe** - Hash is always valid hex (64 chars: `[0-9a-f]`)
3. **No path traversal** - No `..` or `/` possible in hash
4. **Brute-force resistant** - 2^256 possible values

**No signatures needed** - The hash itself provides security, eliminating need for HMAC signatures on URLs.

### Authentication

**API Endpoints:**
- All `/mcp` requests require `Authorization: Bearer <token>`
- Token validated via middleware before processing

**File Downloads:**
- No authentication required (security via hashed directory)
- Path traversal prevention
- Only serves files within sandbox root

### Production Recommendations

1. **Strong secrets** - Generate with `openssl rand -base64 32`
2. **Isolated host** - Run on dedicated server or VM
3. **Docker socket** - Consider Docker-in-Docker for better isolation
4. **HTTPS** - Use Cloudflare Tunnel or reverse proxy with TLS
5. **Rate limiting** - Implement at proxy/gateway level
6. **Monitoring** - Track container creation, resource usage, errors
7. **Backups** - Regular backups of sandbox data volume

## Deployment

### Local Development

```bash
# Start with Docker Compose
docker-compose up -d

# Test endpoint
curl http://localhost:8080 \
  -H "Authorization: Bearer your-token"
```

### Production with Cloudflare Tunnel

```bash
# Set TUNNEL_TOKEN in .env
# Configure tunnel to route to http://mcp-sandbox-server:8080

# Start with Cloudflare compose file
docker-compose -f docker-compose-cloudflare.yml up -d

# Verify tunnel
docker-compose -f docker-compose-cloudflare.yml logs cloudflared
```

### File Downloads

Files are accessible via public URLs without authentication:

```bash
# Download a generated file
curl "http://localhost:8080/files/abc123.../plot.png" -o plot.png
```

## Development

### Adding a New Language Runner

1. **Create Dockerfile** (`Dockerfile-<language>`):

```dockerfile
FROM <base-image>

# Labels for discovery
LABEL sandbox.runner=true
LABEL sandbox.language=<language>

# Non-root user (UID 1000)
RUN adduser -D -u 1000 sandbox

# Install language runtime and libraries
RUN apk add --no-cache <packages>

# Create runner script
RUN cat > /usr/local/bin/runner.sh <<'EOF'
#!/bin/sh
set -e
cat > /tmp/script.<ext>
cd /data
exec <interpreter> /tmp/script.<ext>
EOF

RUN chmod +x /usr/local/bin/runner.sh

USER 1000:1000
WORKDIR /data
ENTRYPOINT ["/usr/local/bin/runner.sh"]
```

2. **Build image:**

```bash
docker build -f Dockerfile-<language> -t mcp-sandbox-runner-<language>:latest .
```

3. **Restart server** - Auto-discovery will find the new runner

### Project Structure

```
code-runner/
├── cmd/server/              # Main server application
├── internal/
│   ├── auth/               # Bearer token authentication
│   ├── config/             # Environment configuration
│   ├── filesign/           # Base URL management
│   ├── handler/            # HTTP handlers, MCP protocol
│   ├── runner/             # Docker container execution
│   └── sandbox/            # Filesystem management
├── Dockerfile-python       # Python runner image
├── Dockerfile-typescript   # TypeScript/Bun runner image
├── Dockerfile              # Server image
├── build.sh               # Build all images
├── start.sh               # Start server with env
├── docker-compose.yml     # Local deployment
└── docker-compose-cloudflare.yml  # Cloudflare deployment
```

## Monitoring

### View Active Containers

```bash
# All containers
docker ps

# Only runners
docker ps --filter "label=sandbox.runner=true"
```

### Resource Usage

```bash
# Real-time stats
docker stats

# Server only
docker stats mcp-sandbox-server
```

### Logs

```bash
# Server logs
docker-compose logs -f mcp-sandbox-server

# All logs
docker-compose logs -f
```

### Disk Usage

```bash
# Docker resources
docker system df

# Sandbox data
du -sh ./sandbox-data
```

## Troubleshooting

### Server can't connect to Docker

```bash
# Check Docker socket
ls -la /var/run/docker.sock

# Test Docker
docker ps

# Check logs
docker-compose logs mcp-sandbox-server
```

### Runner images not found

```bash
# List runners
docker images | grep mcp-sandbox-runner

# Rebuild
./build.sh

# Restart
docker-compose restart
```

### Permission errors in containers

```bash
# Check sandbox directory ownership
ls -la sandbox-data/

# Fix ownership (if needed)
sudo chown -R 1000:1000 sandbox-data/
```

### Port already in use

```bash
# Find process
lsof -i :8080

# Change port in .env
MCP_HTTP_ADDR=:8081
PUBLIC_BASE_URL=http://localhost:8081

# Restart
docker-compose down && docker-compose up -d
```

## License

MIT
