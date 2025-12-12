#!/bin/bash
set -e

echo "🚀 MCP Code Sandbox Server - Quick Start"
echo "=========================================="
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo "📝 Creating .env file from template..."
    cp .env.example .env

    # Generate random tokens
    API_TOKEN=$(openssl rand -hex 32 2>/dev/null || echo "CHANGE_ME_$(date +%s)")
    FILE_SECRET=$(openssl rand -hex 32 2>/dev/null || echo "CHANGE_ME_$(date +%s)")

    # Update .env with generated tokens
    sed -i.bak "s/your-secret-token-here/$API_TOKEN/" .env
    sed -i.bak "s/your-file-signing-secret-here/$FILE_SECRET/" .env
    rm .env.bak 2>/dev/null || true

    echo "✅ Generated .env with random tokens"
    echo ""
else
    echo "✅ .env file already exists"
    echo ""
fi

# Build everything
echo "🔨 Building Docker images and binaries..."
echo ""
./build.sh

echo ""
echo "=========================================="
echo "✅ Setup complete!"
echo ""
echo "Your API token: $(grep MCP_API_TOKEN .env | cut -d= -f2)"
echo ""
echo "To start the server:"
echo ""
echo "  Option 1 - Docker Compose (recommended):"
echo "    docker-compose up -d"
echo ""
echo "  Option 2 - Binary:"
echo "    export \$(cat .env | xargs) && ./bin/mcp-sandbox-server"
echo ""
echo "Then open your browser to:"
echo "  http://localhost:8080"
echo ""
echo "=========================================="
