#!/bin/bash
set -e

# Script to pull all MCP Code Sandbox images from GitHub Container Registry

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get GitHub username from environment or prompt
if [ -z "$GITHUB_USER" ]; then
    echo -e "${YELLOW}Enter your GitHub username (where the images are published):${NC}"
    read -r GITHUB_USER
fi

REGISTRY="ghcr.io"
IMAGE_PREFIX="${REGISTRY}/${GITHUB_USER}/mcp-code-sandbox"

echo -e "${BLUE}=== Pulling MCP Code Sandbox Images ===${NC}"
echo -e "Registry: ${REGISTRY}"
echo -e "User: ${GITHUB_USER}"
echo ""

# Array of images to pull
images=(
    "server:latest"
    "runner-python:latest"
    "runner-typescript:latest"
)

# Pull each image
for image in "${images[@]}"; do
    full_image="${IMAGE_PREFIX}-${image}"
    echo -e "${GREEN}Pulling ${full_image}...${NC}"

    if docker pull "${full_image}"; then
        echo -e "${GREEN}✓ Successfully pulled ${image}${NC}"
    else
        echo -e "${YELLOW}⚠ Failed to pull ${image}${NC}"
        echo -e "${YELLOW}  If this is a private repo, make sure you're logged in:${NC}"
        echo -e "${YELLOW}  docker login ghcr.io -u ${GITHUB_USER}${NC}"
        exit 1
    fi
    echo ""
done

echo -e "${GREEN}=== All images pulled successfully! ===${NC}"
echo ""
echo "Available images:"
docker images | grep "mcp-code-sandbox" || echo "No images found"

echo ""
echo -e "${BLUE}Next steps:${NC}"
echo "1. Create/edit .env file with your configuration"
echo "2. Run: docker compose -f docker-compose.ghcr.yml up -d"
echo "3. Open: http://localhost:8080"
