# MCP Code Sandbox Server

An MCP-compatible HTTP + SSE server that executes Python and TypeScript code in sandboxed Docker containers with per-conversation persistent storage. Powered by Bun for blazing-fast TypeScript execution.

## Features

- **MCP JSON-RPC 2.0 API** - Standard protocol for AI assistants like Claude and ChatGPT
- **Multi-language support** - Python 3.11 and TypeScript/JavaScript (Bun runtime), extensible to other languages
- **Sandboxed execution** - Each code snippet runs in an isolated Docker container
- **Persistent storage** - Per-conversation `/data` directory for file I/O
- **Signed file URLs** - Secure HMAC-signed download links for generated files
- **Network isolation** - Containers run without network access for security
- **Resource limits** - CPU and memory constraints to prevent abuse
- **Web Interface** - Interactive browser UI for testing and development

## Quick Start

Get up and running in under a minute:

```bash
# Clone and enter directory
git clone <repo-url>
cd code-runner

# Run quick start script (builds images, generates tokens, creates .env)
./quickstart.sh

# Start with Docker Compose
docker-compose up -d

# Open browser
open http://localhost:8080
```

The quickstart script will:
- Build all Docker images (Python, TypeScript, server)
- Generate secure random API tokens
- Create a `.env` file with your configuration

## Architecture

### Components

- **HTTP Server** - Handles MCP JSON-RPC requests and file downloads
- **Runner Registry** - Auto-discovers available language runners from Docker images
- **Container Executor** - Manages Docker container lifecycle for code execution
- **Sandbox Manager** - Handles filesystem layout for conversation-specific storage
- **File Signer** - Generates and validates HMAC-signed download URLs

### Filesystem Layout

```
<SANDBOX_ROOT>/
  <conversationId>/
    files/          # Mounted as /data in containers
```

### Runner Images

Runner images are Docker containers labeled with:
- `sandbox.runner=true`
- `sandbox.language=<language>` (e.g., `python`, `typescript`)

Each runner:
1. Reads code from STDIN
2. Writes it to a temp file
3. Executes it with the appropriate runtime in `/data` (Python or Bun)
4. Returns stdout/stderr

**Why Bun?** TypeScript runner uses Bun instead of Node.js for 4x faster startup, 50% smaller images, and native TypeScript support without transpilation.

## Installation

### Prerequisites

- Go 1.21+
- Docker
- Unix-like system (Linux, macOS)

### Build

```bash
# Clone repository
git clone <repo-url>
cd code-runner

# Build everything (runner images + server binary)
./build.sh
```

This creates:
- Docker images:
  - `runner-python` - Python 3.11 with numpy, pandas, requests (~180MB)
  - `runner-typescript` - Bun runtime for TypeScript/JavaScript (~90MB)
  - `mcp-sandbox-server` - Go server binary (~20MB)
- Server binary: `bin/mcp-sandbox-server`

## Configuration

Copy the example environment file and edit it:

```bash
cp .env.example .env
```

Required environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `MCP_HTTP_ADDR` | Server bind address | `:8080` |
| `MCP_API_TOKEN` | Bearer token for authentication | `secret-token-123` |
| `SANDBOX_ROOT` | Root directory inside container | `/var/sandboxes` |
| `FILE_SECRET` | Secret for signing file URLs | `signing-secret-456` |
| `PUBLIC_BASE_URL` | Public URL of the server | `http://localhost:8080` |
| `SANDBOX_DATA_DIR` | Host directory for sandbox data (Docker Compose) | `./sandbox-data` |
| `TUNNEL_TOKEN` | Cloudflare tunnel token (optional) | Only needed for Cloudflare deployment |
| `DOCKER_HOST` | Docker socket (optional) | `unix:///var/run/docker.sock` |

**Notes:**
- For production, generate secure random values for `MCP_API_TOKEN` and `FILE_SECRET`
- `SANDBOX_DATA_DIR` controls where sandbox files are stored on your host machine (defaults to `./sandbox-data`)
- `SANDBOX_ROOT` is the path inside the container (always `/var/sandboxes`)

## Usage

### Starting the Server

#### Option 1: Docker Compose (Recommended)

**Local Development:**
```bash
# Start the server
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the server
docker-compose down
```

**With Cloudflare Tunnel:**
```bash
# Make sure TUNNEL_TOKEN is set in .env
docker-compose -f docker-compose-cloudflare.yml up -d

# View logs
docker-compose -f docker-compose-cloudflare.yml logs -f

# Stop the server
docker-compose -f docker-compose-cloudflare.yml down
```

#### Option 2: Binary (Local Development)

```bash
# Export environment variables
export $(cat .env | xargs)

# Run server
./bin/mcp-sandbox-server
```

Or with environment inline:

```bash
MCP_HTTP_ADDR=:8080 \
MCP_API_TOKEN=your-token \
SANDBOX_ROOT=/tmp/sandboxes \
FILE_SECRET=your-secret \
PUBLIC_BASE_URL=http://localhost:8080 \
./bin/mcp-sandbox-server
```

### Web Interface

Once the server is running, open your browser to:

```
http://localhost:8080
```

You'll see an interactive web interface where you can:
- Test code execution in Python or TypeScript
- Try pre-built examples (data analysis, file I/O, etc.)
- View execution output and generated files
- Download files directly from the browser

**Note:** You'll need to enter your `MCP_API_TOKEN` in the web interface to authenticate.

### API Endpoints

#### GET /

Web interface for testing the sandbox server (no authentication required).

#### POST /mcp

JSON-RPC 2.0 endpoint for tool calls.

Requires `Authorization: Bearer <MCP_API_TOKEN>` header.

**Example: Run Python Code**

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "tools/call",
    "params": {
      "name": "sandbox.run_code",
      "arguments": {
        "conversationId": "conv-123",
        "language": "python",
        "code": "print(\"Hello, World!\")\nwith open(\"/data/output.txt\", \"w\") as f:\n    f.write(\"test\")"
      }
    }
  }'
```

**Response:**

```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "result": {
    "content": [
      {
        "type": "output",
        "data": {
          "success": true,
          "output": "Hello, World!\n",
          "files": [
            {
              "name": "output.txt",
              "url": "http://localhost:8080/files/conv-123/output.txt?sig=abc123..."
            }
          ]
        }
      }
    ]
  }
}
```

**Example: List Available Runners**

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": "2",
    "method": "tools/call",
    "params": {
      "name": "sandbox.list_runners"
    }
  }'
```

#### GET /mcp/events

SSE endpoint for notifications (v1 stub).

Requires `Authorization: Bearer <MCP_API_TOKEN>` header.

#### GET /files/{conversationId}/{filename}?sig={signature}

Download files with signed URLs. No authentication header required.

### Tools

#### sandbox.run_code

Executes code in a sandboxed container.

**Arguments:**
- `conversationId` (string, required) - Unique conversation identifier
- `language` (string, required) - Language to execute (`python`, `typescript`)
- `code` (string, required) - Source code to execute

**Returns:**
- `success` (boolean) - Whether execution succeeded
- `output` (string) - Combined stdout/stderr
- `files` (array) - List of files in `/data` with signed download URLs

#### sandbox.list_runners

Lists available language runners.

**Returns:**
- `languages` (array) - Available runners with language and image name

## Security

### Container Isolation

- **No network access** - Containers run with `NetworkDisabled: true`
- **Non-root user** - Runners execute as non-root users (Python: `sandbox`, TypeScript: `bun`, both UID 1000)
- **Resource limits** - 0.5 CPU cores, 256MB RAM per container
- **Execution timeout** - 30 seconds maximum
- **Minimal images** - Alpine-based for smaller attack surface

### Authentication

- MCP endpoints require `Authorization: Bearer` token
- File downloads use HMAC-SHA256 signed URLs
- Constant-time signature comparison prevents timing attacks

### Path Traversal Protection

File downloads verify paths are within the conversation's sandbox directory.

## Development

### Adding a New Language Runner

Create `Dockerfile-<language>` with inline runner script:

```dockerfile
FROM <base-image>

LABEL sandbox.runner=true
LABEL sandbox.language=<language>

# Create non-root user
RUN adduser -D -u 1000 sandbox

# Install interpreter and dependencies
RUN ...

# Create runner script inline
RUN cat > /usr/local/bin/runner.sh <<'EOF'
#!/bin/sh
set -e
TEMP_FILE=$(mktemp /tmp/main-XXXXXX.<ext>)
cat > "$TEMP_FILE"
cd /data
exec <interpreter> "$TEMP_FILE"
EOF

RUN chmod +x /usr/local/bin/runner.sh && \
    chown sandbox:sandbox /usr/local/bin/runner.sh

USER sandbox
WORKDIR /data
ENTRYPOINT ["/usr/local/bin/runner.sh"]
```

Build the image:

```bash
docker build -f Dockerfile-<language> -t runner-<language> .
```

Restart the server - it will auto-discover the new runner.

### Project Structure

```
.
   cmd/server/           # Main application
   internal/
      auth/            # Authentication middleware
      config/          # Configuration management
      filesign/        # File URL signing
      handler/         # HTTP handlers and JSON-RPC
      runner/          # Container execution and registry
      sandbox/         # Filesystem management
   docker/              # Runner scripts
   Dockerfile-python    # Python runner image
   Dockerfile-typescript # TypeScript runner image
   build.sh            # Build script
```

## Testing

### Manual Testing

Test Python execution:

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": "test1",
    "method": "tools/call",
    "params": {
      "name": "sandbox.run_code",
      "arguments": {
        "conversationId": "test-conv",
        "language": "python",
        "code": "import sys\nprint(f\"Python {sys.version}\")\nwith open(\"/data/hello.txt\", \"w\") as f:\n    f.write(\"Hello from Python!\")"
      }
    }
  }' | jq
```

Test TypeScript execution:

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": "test2",
    "method": "tools/call",
    "params": {
      "name": "sandbox.run_code",
      "arguments": {
        "conversationId": "test-conv",
        "language": "typescript",
        "code": "console.log(\"Hello from TypeScript!\");\nconst fs = require(\"fs\");\nfs.writeFileSync(\"/data/output.json\", JSON.stringify({msg: \"test\"}));"
      }
    }
  }' | jq
```

Download a file (replace signature with actual value from response):

```bash
curl "http://localhost:8080/files/test-conv/hello.txt?sig=<actual-signature>"
```

## Future Enhancements

- Time-limited signed URLs with expiration
- Streaming stdout/stderr via SSE during execution
- Per-conversation resource quotas
- Container pooling for reduced latency
- Support for additional languages (JavaScript, Ruby, etc.)
- Sandbox cleanup policies

## License

MIT
