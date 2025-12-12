#!/bin/bash
set -e

# MCP Code Sandbox Server Startup Script
# Loads environment variables from .env and starts the server

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== MCP Code Sandbox Server ===${NC}"

# Check if .env exists
if [ ! -f .env ]; then
    echo -e "${RED}Error: .env file not found${NC}"
    echo -e "${YELLOW}Creating .env from .env.example...${NC}"

    if [ -f .env.example ]; then
        cp .env.example .env
        echo -e "${YELLOW}Please edit .env with your configuration and run again${NC}"
        exit 1
    else
        echo -e "${RED}Error: .env.example not found${NC}"
        exit 1
    fi
fi

# Load environment variables from .env
echo -e "${BLUE}Loading environment from .env...${NC}"
set -a
source .env
set +a

# Validate required environment variables
REQUIRED_VARS=(
    "MCP_API_TOKEN"
    "SANDBOX_ROOT"
    "FILE_SECRET"
    "PUBLIC_BASE_URL"
)

for var in "${REQUIRED_VARS[@]}"; do
    if [ -z "${!var}" ]; then
        echo -e "${RED}Error: Required environment variable $var is not set${NC}"
        echo -e "${YELLOW}Please configure it in .env${NC}"
        exit 1
    fi
done

# Set defaults for optional variables
export MCP_HTTP_ADDR=${MCP_HTTP_ADDR:-:8080}
export SANDBOX_HOST_PATH=${SANDBOX_HOST_PATH:-$SANDBOX_ROOT}

# Display configuration
echo -e "${BLUE}Configuration:${NC}"
echo -e "  HTTP Address: ${GREEN}${MCP_HTTP_ADDR}${NC}"
echo -e "  Public URL: ${GREEN}${PUBLIC_BASE_URL}${NC}"
echo -e "  Sandbox Root: ${GREEN}${SANDBOX_ROOT}${NC}"
if [ "$SANDBOX_HOST_PATH" != "$SANDBOX_ROOT" ]; then
    echo -e "  Sandbox Host Path: ${GREEN}${SANDBOX_HOST_PATH}${NC}"
fi
echo -e "  API Token: ${GREEN}${MCP_API_TOKEN:0:10}...${NC}"
echo -e "  File Secret: ${GREEN}${FILE_SECRET:0:10}...${NC}"

# Create sandbox directory if it doesn't exist
if [ ! -d "$SANDBOX_ROOT" ]; then
    echo -e "${YELLOW}Creating sandbox directory: $SANDBOX_ROOT${NC}"
    mkdir -p "$SANDBOX_ROOT"
fi

# Check if binary exists
if [ ! -f "./cmd/server/main.go" ]; then
    echo -e "${RED}Error: main.go not found. Are you in the project root?${NC}"
    exit 1
fi

# Check for Docker
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Docker is not installed or not in PATH${NC}"
    exit 1
fi

# Check Docker connection
if ! docker info &> /dev/null; then
    echo -e "${RED}Error: Cannot connect to Docker daemon${NC}"
    echo -e "${YELLOW}Make sure Docker is running${NC}"
    exit 1
fi

# Check for runner images
echo -e "${BLUE}Checking for runner images...${NC}"
PYTHON_IMAGE=$(docker images -q --filter "label=sandbox.language=python" | head -1)
TS_IMAGE=$(docker images -q --filter "label=sandbox.language=typescript" | head -1)

if [ -z "$PYTHON_IMAGE" ] || [ -z "$TS_IMAGE" ]; then
    echo -e "${YELLOW}Warning: Runner images not found${NC}"
    echo -e "${YELLOW}Building runner images...${NC}"

    if [ -f "./build.sh" ]; then
        ./build.sh
    else
        echo -e "${RED}Error: build.sh not found${NC}"
        echo -e "${YELLOW}Please build runner images manually:${NC}"
        echo -e "  docker build -f Dockerfile-python -t mcp-sandbox-runner-python ."
        echo -e "  docker build -f Dockerfile-typescript -t mcp-sandbox-runner-typescript ."
        exit 1
    fi
else
    echo -e "${GREEN}✓ Runner images found${NC}"
fi

# Start the server
echo -e "${BLUE}Starting MCP Code Sandbox Server...${NC}"
echo ""

# Run with go run for development, or use pre-built binary
if [ -f "./mcp-server" ]; then
    echo -e "${GREEN}Using pre-built binary${NC}"
    exec ./mcp-server
else
    echo -e "${GREEN}Running with 'go run'${NC}"
    exec go run ./cmd/server
fi
