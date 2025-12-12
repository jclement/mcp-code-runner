# Bind Mount Architecture

## Overview

This document explains how the MCP Code Sandbox Server handles filesystem paths for Docker bind mounts, particularly in Docker-in-Docker (DinD) scenarios.

## The Problem

When the server runs **inside a Docker container** and creates runner containers (Docker-in-Docker), bind mount paths must be from the **Docker host's perspective**, not the server container's perspective.

### Example Scenario

```
Docker Host (macOS)
├── /Users/jsc/ai/code-runner/sandbox-data/
│   └── web-test/
│       └── hello.txt
│
└── Server Container
    └── /var/sandboxes/  (mounted from host's sandbox-data/)
        └── web-test/
            └── hello.txt
```

When the server creates a runner container:
- ❌ **WRONG**: Bind `/var/sandboxes/web-test:/data` (server container's path)
- ✅ **CORRECT**: Bind `/Users/jsc/ai/code-runner/sandbox-data/web-test:/data` (host's path)

## Solution: Dual Path Configuration

The system uses two path configurations:

### 1. SANDBOX_ROOT
- **Purpose**: Where the server performs filesystem operations (read, write, list files)
- **When running directly on host**: Absolute host path
- **When running in container**: Container's internal path (e.g., `/var/sandboxes`)

### 2. SANDBOX_HOST_PATH
- **Purpose**: Path for Docker bind mounts when creating runner containers
- **When running directly on host**: Same as SANDBOX_ROOT
- **When running in container**: Absolute path on the actual Docker host
- **Default**: Falls back to SANDBOX_ROOT if not specified

## Configuration Examples

### Running Server Directly on Host

```bash
# .env
SANDBOX_ROOT=/Users/jsc/ai/code-runner/sandbox-data
SANDBOX_HOST_PATH=/Users/jsc/ai/code-runner/sandbox-data
```

Both paths are the same because the server IS on the host.

### Running Server in Docker Container (docker-compose)

```yaml
# docker-compose.yml
environment:
  - SANDBOX_ROOT=/var/sandboxes              # Server container's path
  - SANDBOX_HOST_PATH=${PWD}/sandbox-data    # Docker host's path

volumes:
  - ${PWD}/sandbox-data:/var/sandboxes       # Mount host dir into server
```

Server reads/writes to `/var/sandboxes`, but runners bind to `${PWD}/sandbox-data`.

## Code Flow

### 1. Sandbox Manager (`internal/sandbox/manager.go`)

```go
type Manager struct {
    sandboxRoot     string  // For filesystem operations
    sandboxHostPath string  // For bind mounts
}

// Create directory using sandboxRoot (server's view)
func (m *Manager) EnsureSandboxDir(conversationID string) (string, error) {
    sandboxDir := filepath.Join(m.sandboxRoot, conversationID)
    os.MkdirAll(sandboxDir, 0o777)
    os.Chown(sandboxDir, 1000, 1000)  // For runner user
    return sandboxDir, nil
}

// Get host path for bind mount (Docker host's view)
func (m *Manager) GetSandboxHostPath(conversationID string) string {
    return filepath.Join(m.sandboxHostPath, conversationID)
}
```

### 2. MCP Handler (`internal/handler/mcp.go`)

```go
// Create directory on filesystem
_, err := h.sandbox.EnsureSandboxDir(args.ConversationID)

// Get host path for bind mounting
sandboxHostPath := h.sandbox.GetSandboxHostPath(args.ConversationID)

// Pass host path to executor
execResult := h.executor.Execute(ctx, imageName, sandboxHostPath, code, ...)
```

### 3. Executor (`internal/runner/executor.go`)

```go
// Bind mount the host path
hostConfig := &container.HostConfig{
    Binds: []string{sandboxDir + ":/data"},  // sandboxDir is host path
    ...
}
```

## Directory Permissions

The server creates each conversation directory with:
- **Permissions**: `0777` (rwxrwxrwx)
- **Ownership**: `1000:1000` (to match runner container user)

This is done via `os.Chown()` before creating the runner container.

### Why 1000:1000?

Runner containers run as user `1000:1000` (defined in Dockerfiles):
```dockerfile
# Dockerfile-python, Dockerfile-typescript
USER 1000:1000
```

By pre-chowning the host directory to `1000:1000`, the runner can write files without permission errors.

### macOS Limitations

On Docker Desktop for Mac, `os.Chown()` may not work due to how Docker handles file ownership. The code handles this gracefully:

```go
if err := os.Chown(sandboxDir, 1000, 1000); err != nil {
    // Log warning but don't fail
    fmt.Printf("Warning: failed to chown %s to 1000:1000: %v\n", sandboxDir, err)
}
```

The `0777` permissions should be sufficient even if chown fails.

## Testing the Setup

1. **Check server logs** on startup:
   ```
   Sandbox Root: /Users/jsc/ai/code-runner/sandbox-data
   Sandbox Host Path: /Users/jsc/ai/code-runner/sandbox-data (for Docker bind mounts)
   ```

2. **Run code** that creates a file:
   ```python
   with open("/data/hello.txt", "w") as f:
       f.write("Hello, World!")
   ```

3. **Verify file exists** on host:
   ```bash
   ls -la /Users/jsc/ai/code-runner/sandbox-data/web-test/
   # Should show hello.txt owned by UID 1000
   ```

4. **Check runner container** (while running):
   ```bash
   docker ps --filter "label=sandbox.runner=true"
   docker inspect <container-id> | jq '.[0].HostConfig.Binds'
   # Should show: ["/Users/jsc/ai/code-runner/sandbox-data/web-test:/data"]
   ```

## Troubleshooting

### Files not appearing on host
- Check SANDBOX_HOST_PATH is correct
- Verify server logs show the right paths
- Inspect runner container binds: `docker inspect <container> | jq '.[0].HostConfig.Binds'`

### Permission denied in runner
- Check directory ownership: `ls -la sandbox-data/`
- Verify directory has 0777 permissions
- Check runner Dockerfile has `USER 1000:1000`

### Wrong paths in bind mounts
- Ensure SANDBOX_HOST_PATH is absolute, not relative
- For docker-compose: use `${PWD}` to get absolute path
- Don't use container paths for bind mounts in DinD scenarios
