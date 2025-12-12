# Using Pre-built Images from GitHub Container Registry

The MCP Code Sandbox Server and runner images are automatically built and published to GitHub Container Registry (GHCR) via GitHub Actions.

## Available Images

All images are available at `ghcr.io/<username>/mcp-code-sandbox-*`:

- **Server**: `ghcr.io/<username>/mcp-code-sandbox-server:latest`
- **Python Runner**: `ghcr.io/<username>/mcp-code-sandbox-runner-python:latest`
- **TypeScript Runner**: `ghcr.io/<username>/mcp-code-sandbox-runner-typescript:latest`

## Pulling Images Manually

```bash
# Set your GitHub username
export GITHUB_USER=jsc

# Pull the server image
docker pull ghcr.io/$GITHUB_USER/mcp-code-sandbox-server:latest

# Pull runner images (server will auto-discover these)
docker pull ghcr.io/$GITHUB_USER/mcp-code-sandbox-runner-python:latest
docker pull ghcr.io/$GITHUB_USER/mcp-code-sandbox-runner-typescript:latest
```

## Using with Docker Compose

Use the pre-built images with `docker-compose.ghcr.yml`:

```bash
# Set your GitHub username
export GITHUB_USER=jsc

# Start the server (will pull images if needed)
docker compose -f docker-compose.ghcr.yml up -d

# View logs
docker compose -f docker-compose.ghcr.yml logs -f

# Stop the server
docker compose -f docker-compose.ghcr.yml down
```

## Authentication for Private Repos

If your repository is private, you'll need to authenticate with GHCR:

```bash
# Create a Personal Access Token (PAT) at:
# https://github.com/settings/tokens/new
# Select scope: read:packages

# Login to GHCR
echo $GITHUB_TOKEN | docker login ghcr.io -u $GITHUB_USER --password-stdin

# Now pull images
docker pull ghcr.io/$GITHUB_USER/mcp-code-sandbox-server:latest
```

## Running the Server with Pre-built Images

The server will automatically discover and use the runner images if they have the correct labels:

```bash
# The runner images already have these labels:
# - sandbox.runner=true
# - sandbox.language=python (or typescript)

# Server startup will show:
# Discovered 2 runner(s):
#   - python: ghcr.io/jsc/mcp-code-sandbox-runner-python:latest
#   - typescript: ghcr.io/jsc/mcp-code-sandbox-runner-typescript:latest
```

## CI/CD Workflow

The GitHub Action workflow (`.github/workflows/docker-build.yml`) automatically:

1. **Builds all images** on every push to `main`
2. **Tags images** with:
   - `latest` (for main branch)
   - Version tags (for releases like `v1.0.0`)
   - Branch names (for feature branches)
3. **Multi-arch builds**: Supports both `linux/amd64` and `linux/arm64`
4. **Build caching**: Uses GitHub Actions cache for faster builds

## Triggering Builds

Builds are triggered by:

- **Push to main**: Builds and tags as `latest`
- **Creating a tag**: `git tag v1.0.0 && git push --tags` creates versioned images
- **Pull requests**: Builds but doesn't push to `latest`
- **Manual trigger**: Via GitHub Actions UI

## Local Development vs Production

### Local Development (build locally)
```bash
# Build and run locally with docker-compose
docker compose up -d
```

### Production (use pre-built images)
```bash
# Use pre-built images from GHCR
export GITHUB_USER=jsc
docker compose -f docker-compose.ghcr.yml up -d
```

## Image Sizes

Approximate compressed image sizes:

- **Server**: ~30MB (Go binary + Alpine)
- **Python Runner**: ~200MB (Python + numpy/pandas/matplotlib)
- **TypeScript Runner**: ~150MB (Bun + Alpine)

## Updating Images

To update to the latest version:

```bash
# Pull latest images
docker compose -f docker-compose.ghcr.yml pull

# Restart with new images
docker compose -f docker-compose.ghcr.yml up -d

# Remove old images
docker image prune -f
```

## Verifying Runner Discovery

After starting the server, check that runners are discovered:

```bash
# View server logs
docker compose -f docker-compose.ghcr.yml logs mcp-sandbox-server

# Should see:
# Discovered 2 runner(s):
#   - python: ghcr.io/jsc/mcp-code-sandbox-runner-python:latest
#   - typescript: ghcr.io/jsc/mcp-code-sandbox-runner-typescript:latest
```

If runners aren't discovered, the server will try to pull them automatically when code is executed.

## Troubleshooting

### "Permission denied" when pulling images

Make sure you're logged into GHCR:
```bash
docker login ghcr.io -u $GITHUB_USER
```

### Runner images not found

The server creates runner containers with the discovered image names. Ensure runner images are pulled:
```bash
docker images | grep mcp-code-sandbox-runner
```

### Build failing on GitHub Actions

Check:
- Repository has Actions enabled
- Workflow has `packages: write` permission (already configured)
- No syntax errors in Dockerfiles
