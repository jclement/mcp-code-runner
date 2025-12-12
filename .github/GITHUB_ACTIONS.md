# GitHub Actions CI/CD Setup

This repository uses GitHub Actions to automatically build and publish Docker images to GitHub Container Registry (GHCR).

## Workflow Overview

The workflow file is located at `.github/workflows/docker-build.yml`.

### What It Does

1. **Builds 3 separate images**:
   - Server (`mcp-code-sandbox-server`)
   - Python runner (`mcp-code-sandbox-runner-python`)
   - TypeScript runner (`mcp-code-sandbox-runner-typescript`)

2. **Pushes to GHCR** at `ghcr.io/<username>/mcp-code-sandbox-*`

3. **Multi-architecture builds**: Both `linux/amd64` and `linux/arm64` (M1/M2 Macs)

4. **Smart tagging**:
   - `latest` for main branch pushes
   - `v1.0.0`, `v1.0`, `v1` for semantic version tags
   - Branch name for feature branches

### Triggers

- **Push to `main`**: Builds and tags as `latest`
- **Push tag `v*`**: Builds and tags with version numbers
- **Pull Request**: Builds but doesn't push to `latest`
- **Manual**: Can be triggered manually from GitHub Actions UI

## First-Time Setup

### 1. Enable GitHub Actions

In your repository settings:
- Go to **Settings** → **Actions** → **General**
- Under "Workflow permissions", select **Read and write permissions**
- Enable **Allow GitHub Actions to create and approve pull requests**

### 2. Enable GitHub Packages

The workflow uses `GITHUB_TOKEN` which is automatically available - no setup needed!

### 3. Make Repository Public (Optional)

For **public images**:
- Go to **Settings** → **Danger Zone** → **Change visibility**
- Or keep private and authenticate when pulling (see GHCR_USAGE.md)

### 4. Push to Main Branch

```bash
git add .
git commit -m "Add GitHub Actions workflow"
git push origin main
```

Watch the build:
- Go to **Actions** tab in GitHub
- Click on the running workflow
- Expand each job to see build logs

## Creating a Release

To create versioned images (e.g., `v1.0.0`):

```bash
# Create and push a tag
git tag v1.0.0
git push origin v1.0.0
```

This will create images tagged with:
- `ghcr.io/<user>/mcp-code-sandbox-server:v1.0.0`
- `ghcr.io/<user>/mcp-code-sandbox-server:v1.0`
- `ghcr.io/<user>/mcp-code-sandbox-server:v1`
- `ghcr.io/<user>/mcp-code-sandbox-server:latest`

## Workflow Configuration

### Matrix Strategy

The workflow uses a matrix to build multiple images in parallel:

```yaml
strategy:
  matrix:
    include:
      - name: server
        dockerfile: Dockerfile
        image_suffix: server
      - name: runner-python
        dockerfile: Dockerfile-python
        image_suffix: runner-python
      - name: runner-typescript
        dockerfile: Dockerfile-typescript
        image_suffix: runner-typescript
```

### Build Cache

GitHub Actions cache is used to speed up builds:
```yaml
cache-from: type=gha
cache-to: type=gha,mode=max
```

This caches Docker layers between builds, making subsequent builds much faster.

### Multi-Architecture

Builds for both amd64 (Intel) and arm64 (Apple Silicon):
```yaml
platforms: linux/amd64,linux/arm64
```

## Viewing Published Images

After a successful build:

1. Go to your repository on GitHub
2. Click **Packages** (right sidebar)
3. You'll see all published images with their tags

Or visit directly:
- `https://github.com/<username>?tab=packages`

## Using the Images

See [GHCR_USAGE.md](../GHCR_USAGE.md) for detailed instructions on:
- Pulling images manually
- Using with docker-compose
- Authentication for private repos
- Local development vs production

## Troubleshooting

### Build Failing

Check the Actions tab for error logs. Common issues:

1. **Dockerfile syntax error**: Check the specific Dockerfile mentioned in logs
2. **Build context issue**: Ensure all files referenced in Dockerfile exist
3. **Out of disk space**: GitHub provides 14GB - usually enough but can be an issue

### Images Not Appearing in Packages

1. Check workflow completed successfully
2. Verify `GITHUB_TOKEN` has write permissions
3. Ensure the repository has Packages enabled

### Permission Denied When Pulling

For private repos, you need to authenticate:
```bash
docker login ghcr.io -u <username>
# Password: use a Personal Access Token with `read:packages` scope
```

### Stale Cache

If you need to rebuild from scratch:
```bash
# Add this to the workflow temporarily:
cache-from: type=gha
cache-to: type=gha,mode=max
no-cache: true  # Add this line
```

Or delete the cache manually in GitHub:
- Settings → Actions → Caches

## Cost and Limits

GitHub provides:
- **Storage**: 500MB for free accounts, 2GB for Pro
- **Bandwidth**: Unlimited for public repos
- **Build minutes**: 2,000 minutes/month for free accounts

Images are automatically compressed and should be well under the storage limit.

## Security

The workflow uses:
- `permissions: packages: write` - Minimal required permissions
- `GITHUB_TOKEN` - Automatically rotated, repo-scoped token
- No secrets needed for public images

For production deployments, consider:
- Using release tags instead of `latest`
- Implementing image scanning (Trivy, Snyk)
- Setting up branch protection rules
