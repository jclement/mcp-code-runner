# Docker Deployment Guide

## Overview

The MCP Code Sandbox Server can be deployed using Docker Compose in two configurations:
1. **Local Development** - Exposes port 8080 directly
2. **Cloudflare Tunnel** - Routes traffic through Cloudflare's network

## Architecture

```
┌─────────────────────────────────────────┐
│  Docker Host                            │
│                                         │
│  ┌────────────────────────────────┐    │
│  │ mcp-sandbox-server container   │    │
│  │ - Runs Go server               │    │
│  │ - Mounts /var/run/docker.sock  │    │
│  │ - Creates runner containers    │    │
│  └────────────────────────────────┘    │
│           │                             │
│           │ Creates/manages             │
│           ▼                             │
│  ┌────────────────────────────────┐    │
│  │ Runner containers (ephemeral)  │    │
│  │ - runner-python                │    │
│  │ - runner-typescript            │    │
│  │ - Mounts sandbox volume        │    │
│  └────────────────────────────────┘    │
│                                         │
│  ┌────────────────────────────────┐    │
│  │ sandbox-data volume            │    │
│  │ /var/sandboxes/                │    │
│  │   └── <conversationId>/        │    │
│  │       └── files/               │    │
│  └────────────────────────────────┘    │
└─────────────────────────────────────────┘
```

## Prerequisites

1. **Docker & Docker Compose** installed
2. **Runner images built**:
   ```bash
   ./build.sh
   ```

## Configuration Files

### docker-compose.yml (Local)
- Exposes port 8080 on the host
- Maps `MCP_HTTP_ADDR` environment variable to configure port
- Suitable for local development and testing

### docker-compose-cloudflare.yml (Cloudflare Tunnel)
- No exposed ports (all traffic through Cloudflare)
- Requires `TUNNEL_TOKEN` environment variable
- Includes `cloudflared` sidecar container
- Suitable for production deployment

## Environment Variables

Create a `.env` file in the project root:

```bash
cp .env.example .env
```

Edit `.env` with your values:

```env
# Required for both configurations
MCP_API_TOKEN=your-secret-token
FILE_SECRET=your-file-secret
SANDBOX_ROOT=/var/sandboxes

# Local deployment
MCP_HTTP_ADDR=8080
PUBLIC_BASE_URL=http://localhost:8080

# Cloudflare deployment
# MCP_HTTP_ADDR is not needed (no port exposed)
# PUBLIC_BASE_URL=https://your-tunnel.domain.com
# TUNNEL_TOKEN=your-cloudflare-tunnel-token
```

## Local Deployment

### Start
```bash
docker-compose up -d
```

### Check Status
```bash
docker-compose ps
```

### View Logs
```bash
# All services
docker-compose logs -f

# Server only
docker-compose logs -f mcp-sandbox-server
```

### Test Endpoint
```bash
curl http://localhost:8080/mcp/events \
  -H "Authorization: Bearer your-token"
```

### Stop
```bash
docker-compose down
```

### Clean Everything (including volumes)
```bash
docker-compose down -v
```

## Cloudflare Tunnel Deployment

### Prerequisites

1. Create a Cloudflare Tunnel:
   - Go to Cloudflare Zero Trust Dashboard
   - Navigate to Networks > Tunnels
   - Create a new tunnel
   - Copy the tunnel token

2. Configure your tunnel to route traffic to `http://mcp-sandbox-server:8080`

### Start
```bash
# Set TUNNEL_TOKEN in .env first
docker-compose -f docker-compose-cloudflare.yml up -d
```

### Check Status
```bash
docker-compose -f docker-compose-cloudflare.yml ps
```

### View Logs
```bash
# All services
docker-compose -f docker-compose-cloudflare.yml logs -f

# Server only
docker-compose -f docker-compose-cloudflare.yml logs -f mcp-sandbox-server

# Cloudflared only
docker-compose -f docker-compose-cloudflare.yml logs -f cloudflared
```

### Test Endpoint
```bash
curl https://your-tunnel.domain.com/mcp/events \
  -H "Authorization: Bearer your-token"
```

### Stop
```bash
docker-compose -f docker-compose-cloudflare.yml down
```

## Volume Management

### Inspect Sandbox Data
```bash
# Find the volume
docker volume ls | grep sandbox-data

# Inspect volume
docker volume inspect code-runner_sandbox-data

# Browse volume contents (create temporary container)
docker run --rm -it \
  -v code-runner_sandbox-data:/data \
  alpine sh -c "ls -la /data"
```

### Backup Sandbox Data
```bash
docker run --rm \
  -v code-runner_sandbox-data:/data \
  -v $(pwd)/backup:/backup \
  alpine tar czf /backup/sandbox-backup.tar.gz -C /data .
```

### Restore Sandbox Data
```bash
docker run --rm \
  -v code-runner_sandbox-data:/data \
  -v $(pwd)/backup:/backup \
  alpine tar xzf /backup/sandbox-backup.tar.gz -C /data
```

## Docker Socket Security

The server container mounts `/var/run/docker.sock` to create and manage runner containers. This is required but has security implications:

### Security Considerations

1. **Container has Docker daemon access** - Can create/remove any container
2. **Effectively root access** - Can escape to host
3. **Production recommendations**:
   - Run on dedicated/isolated host
   - Use Docker-in-Docker (DinD) instead of socket mounting
   - Implement additional access controls (AppArmor, SELinux)
   - Monitor container creation/deletion

### Alternative: Docker-in-Docker

For better isolation, consider using Docker-in-Docker:

```yaml
services:
  docker-dind:
    image: docker:dind
    privileged: true
    volumes:
      - dind-storage:/var/lib/docker

  mcp-sandbox-server:
    environment:
      - DOCKER_HOST=tcp://docker-dind:2376
    depends_on:
      - docker-dind
```

## Troubleshooting

### Server can't connect to Docker daemon
```bash
# Check Docker socket permissions
ls -la /var/run/docker.sock

# Ensure Docker is running
docker ps

# Check container logs
docker-compose logs mcp-sandbox-server
```

### Runner images not found
```bash
# List images
docker images | grep runner-

# Rebuild runners
./build.sh

# Restart server
docker-compose restart mcp-sandbox-server
```

### Port already in use
```bash
# Find process using port 8080
lsof -i :8080

# Change port in .env
MCP_HTTP_ADDR=8081

# Restart
docker-compose down && docker-compose up -d
```

### Cloudflare tunnel not connecting
```bash
# Check cloudflared logs
docker-compose -f docker-compose-cloudflare.yml logs cloudflared

# Verify TUNNEL_TOKEN is set
docker-compose -f docker-compose-cloudflare.yml config | grep TUNNEL_TOKEN

# Test tunnel connectivity
docker-compose -f docker-compose-cloudflare.yml exec cloudflared cloudflared tunnel info
```

## Production Checklist

- [ ] Set strong `MCP_API_TOKEN` (minimum 32 characters)
- [ ] Set strong `FILE_SECRET` (minimum 32 characters)
- [ ] Configure `PUBLIC_BASE_URL` to match your domain
- [ ] Set up Cloudflare tunnel with proper routing
- [ ] Enable Docker restart policies (`restart: unless-stopped`)
- [ ] Set up log aggregation
- [ ] Configure volume backups
- [ ] Monitor resource usage
- [ ] Review runner image security
- [ ] Implement rate limiting at proxy level
- [ ] Set up alerts for container failures

## Resource Limits

Each runner container is limited to:
- **CPU**: 0.5 cores
- **Memory**: 256MB
- **Timeout**: 30 seconds

These limits are defined in [internal/runner/executor.go](internal/runner/executor.go:53-54).

To adjust limits, modify the `Resources` section:

```go
Resources: container.Resources{
    Memory:   512 * 1024 * 1024, // 512MB
    NanoCPUs: 1000000000,        // 1.0 CPU
},
```

## Monitoring

### View Active Containers
```bash
# All containers
docker ps

# Only runner containers
docker ps --filter "ancestor=runner-python"
docker ps --filter "ancestor=runner-typescript"
```

### Resource Usage
```bash
# Real-time stats
docker stats

# Server container only
docker stats mcp-sandbox-server
```

### Disk Usage
```bash
# All Docker resources
docker system df

# Detailed view
docker system df -v
```

## Cleanup

### Remove Stopped Containers
```bash
docker container prune
```

### Remove Unused Images
```bash
docker image prune -a
```

### Full Cleanup (WARNING: Removes all unused Docker resources)
```bash
docker system prune -a --volumes
```
