#!/bin/bash
set -e

echo "🔄 Rebuilding MCP Code Sandbox Server (no cache)..."
echo ""
echo "This will rebuild all images from scratch to ensure latest code is used."
echo ""

# Stop and remove existing containers
echo "Stopping existing containers..."
docker-compose down 2>/dev/null || true

# Remove old images (including untagged ones with sandbox.runner label)
echo ""
echo "Removing old images..."
docker rmi -f runner-python runner-typescript mcp-sandbox-server 2>/dev/null || true
# Clean up dangling runner images to prevent discovery issues
docker images --filter "label=sandbox.runner=true" --filter "dangling=true" -q | xargs -r docker rmi 2>/dev/null || true

# Build runner images with no cache
echo ""
echo "Building Python runner image (no cache)..."
docker build --no-cache -f Dockerfile-python -t runner-python .

echo ""
echo "Building TypeScript runner image (no cache)..."
docker build --no-cache -f Dockerfile-typescript -t runner-typescript .

echo ""
echo "Building server Docker image (no cache)..."
docker build --no-cache -f Dockerfile -t mcp-sandbox-server .

# Build Go binary (optional, for local development)
echo ""
echo "Building Go server binary..."
mkdir -p bin
go build -o bin/mcp-sandbox-server ./cmd/server

echo ""
echo "✅ Rebuild complete!"
echo ""
echo "Docker images:"
docker images | grep -E "runner-python|runner-typescript|mcp-sandbox-server" || echo "  (no images found)"
echo ""
echo "To start the server:"
echo "  docker-compose up -d"
echo ""
