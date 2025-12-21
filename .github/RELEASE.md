# GitHub Actions Workflows

This repository uses GitHub Actions for continuous integration and automated releases.

## Workflows

### 1. CI Workflow (`ci.yml`)
**Triggers:** Push to master/main, Pull Requests

**What it does:**
- Runs tests on Linux, Windows, and macOS
- Tests with multiple Go versions (1.21 and latest stable)
- Performs code linting and formatting checks
- Builds the project to ensure compilation succeeds

### 2. Release Workflow (`release.yml`)
**Triggers:** Push of version tags (e.g., `v1.0.0`, `v2.1.3`)

**What it does:**
- Builds ethr for multiple platforms:
  - Linux (amd64, arm64)
  - macOS (amd64, arm64/Apple Silicon)
  - Windows (amd64)
- Creates ZIP archives for each platform
- Creates a GitHub Release with all binaries attached
- Automatically generates release notes
- Updates the `latest` tag to always point to the most recent release

## Creating a Release

To create a new release:

### Method 1: Using the release script (Recommended)

#### Interactive mode (shows all previous versions):
```bash
./release.sh
```

#### Specify version directly:
```bash
./release.sh 1.0.0
```

#### Create release AND set as latest:
```bash
./release.sh 1.0.0 --set-latest
```

The script will:
- Show existing versions for reference
- Validate the version doesn't already exist
- Create and push the version tag
- Optionally update the `latest` tag

### Method 2: Manual process

#### 1. Update version (optional)
If you have a version file or want to update documentation, do it now.

#### 2. Commit your changes
```bash
git add .
git commit -m "Prepare for release v1.0.0"
git push origin master
```

#### 3. Create and push a version tag
```bash
# Create a tag (follow semantic versioning: vMAJOR.MINOR.PATCH)
git tag v1.0.0

# Push the tag to GitHub
git push origin v1.0.0
```

#### 4. Wait for the build
- GitHub Actions will automatically start building
- Go to the "Actions" tab in your repository to watch progress
- Build typically takes 5-10 minutes

#### 5. Release is published!
- Once complete, a new release will appear under "Releases"
- All platform binaries will be attached

## Managing the 'latest' Tag

The `latest` tag is what users reference when downloading the most recent stable version. You have full control over which release is marked as "latest".

### Set a version as latest:

#### Using the script (Recommended):
```bash
# Interactive - shows all versions and prompts you to choose
./set-latest.sh

# Specify version directly
./set-latest.sh v1.0.0
# or
./set-latest.sh 1.0.0
```

#### Manual process:
```bash
# Delete old latest tag
git tag -d latest
git push origin :refs/tags/latest

# Create new latest tag pointing to v1.0.0
git tag latest v1.0.0
git push origin latest
```

### When to update 'latest':

- **Stable releases**: Set major/minor releases as latest
- **Patch releases**: Usually update latest for bug fixes
- **Pre-releases**: Don't set alpha/beta/rc as latest
- **Experimental**: Don't set experimental versions as latest

## Downloading Latest Release

Users can always download the latest version using these URLs:

### Using wget
```bash
# Linux AMD64
wget https://github.com/YOUR_USERNAME/ethr/releases/latest/download/ethr_linux_amd64.zip

# macOS Apple Silicon (M1/M2)
wget https://github.com/YOUR_USERNAME/ethr/releases/latest/download/ethr_darwin_arm64.zip

# Windows
wget https://github.com/YOUR_USERNAME/ethr/releases/latest/download/ethr_windows_amd64.zip
```

### Using curl
```bash
# Linux AMD64
curl -L -O https://github.com/YOUR_USERNAME/ethr/releases/latest/download/ethr_linux_amd64.zip

# macOS Intel
curl -L -O https://github.com/YOUR_USERNAME/ethr/releases/latest/download/ethr_darwin_amd64.zip
```

### Direct browser download
Visit: `https://github.com/YOUR_USERNAME/ethr/releases/latest`

## Version Tag Format

Use semantic versioning with a `v` prefix:
- `v1.0.0` - Major release
- `v1.1.0` - Minor release (new features, backward compatible)
- `v1.1.1` - Patch release (bug fixes)

## Pre-releases

To create a pre-release (alpha, beta, rc):

```bash
git tag v2.0.0-alpha.1
git push origin v2.0.0-alpha.1
```

The workflow will still build and create a release, but you may want to mark it as a pre-release in the GitHub UI.

## Troubleshooting

### Build fails
- Check the Actions tab for detailed error messages
- Ensure all tests pass locally before tagging
- Verify `go.mod` and dependencies are up to date

### Tag already exists
If you need to move a tag:
```bash
# Delete local tag
git tag -d v1.0.0

# Delete remote tag
git push origin :refs/tags/v1.0.0

# Recreate and push
git tag v1.0.0
git push origin v1.0.0
```

### Latest tag not updating
The workflow automatically updates it, but you can manually do:
```bash
git tag -f latest
git push -f origin latest
```

## Repository Setup

After pushing these workflows, ensure:

1. **Actions are enabled**: Go to Settings → Actions → General
2. **Workflow permissions**: Settings → Actions → General → Workflow permissions
   - Set to "Read and write permissions"
   - Check "Allow GitHub Actions to create and approve pull requests"

## Migrating from Travis CI

If you're migrating from Travis CI:

1. These workflows replace `.travis.yml`
2. You can keep Travis CI or disable it in Travis CI settings
3. GitHub Actions is free for public repositories
4. Private repositories get 2,000 free minutes/month

## Available Platforms

Current build targets:
- `ethr_linux_amd64.zip` - Linux 64-bit (Intel/AMD)
- `ethr_linux_arm64.zip` - Linux ARM64 (Raspberry Pi, ARM servers)
- `ethr_darwin_amd64.zip` - macOS Intel
- `ethr_darwin_arm64.zip` - macOS Apple Silicon (M1/M2/M3)
- `ethr_windows_amd64.zip` - Windows 64-bit

To add more platforms, edit `.github/workflows/release.yml` and add entries to the matrix.

## Advanced: Manual Release

If you need to create a release without a tag:

```bash
# Use GitHub CLI
gh release create v1.0.0 \
  --title "Release v1.0.0" \
  --notes "Release notes here" \
  ethr_linux_amd64.zip \
  ethr_darwin_arm64.zip \
  ethr_windows_amd64.zip
```
