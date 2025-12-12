#!/bin/bash
set -e

echo "Building MCP Code Sandbox Server..."

# Build runner images
echo ""
echo "Building Python runner image..."
docker build -f Dockerfile-python -t runner-python .

echo ""
echo "Building TypeScript runner image (Bun)..."
docker build -f Dockerfile-typescript -t runner-typescript .

echo ""
echo "Building server Docker image..."
docker build -f Dockerfile -t mcp-sandbox-server .

# Build Go binary (optional, for local development)
echo ""
echo "Building Go server binary..."
mkdir -p bin
go build -o bin/mcp-sandbox-server ./cmd/server

echo ""
echo "Build complete!"
echo ""
echo "Docker images:"
echo "  - runner-python (Python 3.11 + numpy, pandas, requests)"
echo "  - runner-typescript (Bun runtime - fast TypeScript/JavaScript)"
echo "  - mcp-sandbox-server"
echo ""
echo "Server binary: bin/mcp-sandbox-server"
echo ""
echo "To run with Docker Compose:"
echo "  docker-compose up -d                    # Local development"
echo "  docker-compose -f docker-compose-cloudflare.yml up -d  # With Cloudflare tunnel"
echo ""
echo "To run the binary directly:"
echo "  export \$(cat .env | xargs) && ./bin/mcp-sandbox-server"
