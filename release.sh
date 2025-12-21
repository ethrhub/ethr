#!/bin/bash

# Release script for ethr
# Usage: ./release.sh [version] [--set-latest]
# Example: ./release.sh 1.0.0
# Example: ./release.sh 1.0.0 --set-latest

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to show existing tags
show_existing_versions() {
    echo -e "${BLUE}=== Existing versions ===${NC}"
    if git tag -l "v*" | grep -q .; then
        git tag -l "v*" --sort=-version:refname | head -20
        local tag_count=$(git tag -l "v*" | wc -l)
        if [ "$tag_count" -gt 20 ]; then
            echo "... (showing 20 of $tag_count versions)"
        fi
    else
        echo "No existing versions found"
    fi
    echo ""
}

# Function to get version interactively
get_version_interactive() {
    show_existing_versions
    
    echo -e "${YELLOW}Enter the version number for the new release${NC}"
    echo "Format: MAJOR.MINOR.PATCH (e.g., 1.0.0, 2.1.3)"
    echo "Or with pre-release: 2.0.0-alpha.1, 2.0.0-beta.2, 2.0.0-rc.1"
    echo ""
    read -p "Version: " VERSION
    
    if [ -z "$VERSION" ]; then
        echo -e "${RED}Error: Version cannot be empty${NC}"
        exit 1
    fi
    
    echo "$VERSION"
}

# Parse arguments
SET_LATEST=false
VERSION=""

for arg in "$@"; do
    case $arg in
        --set-latest)
            SET_LATEST=true
            ;;
        --help|-h)
            echo "Usage: $0 [version] [--set-latest]"
            echo ""
            echo "Arguments:"
            echo "  version       Version number (e.g., 1.0.0). If omitted, you'll be prompted."
            echo "  --set-latest  Update the 'latest' tag to point to this release"
            echo ""
            echo "Examples:"
            echo "  $0                    # Interactive mode"
            echo "  $0 1.0.0              # Create v1.0.0 release"
            echo "  $0 1.0.0 --set-latest # Create v1.0.0 and set as latest"
            echo ""
            exit 0
            ;;
        *)
            if [ -z "$VERSION" ]; then
                VERSION=$arg
            fi
            ;;
    esac
done

# Get version interactively if not provided
if [ -z "$VERSION" ]; then
    VERSION=$(get_version_interactive)
fi

TAG="v${VERSION}"

echo -e "${YELLOW}Creating release for version ${TAG}${NC}"
echo ""

# Check if we're on the right branch
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "master" ] && [ "$CURRENT_BRANCH" != "main" ]; then
    echo -e "${YELLOW}Warning: You're on branch '${CURRENT_BRANCH}', not 'master' or 'main'${NC}"
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo -e "${RED}Error: You have uncommitted changes${NC}"
    echo "Please commit or stash your changes before creating a release"
    exit 1
fi

# Check if tag already exists
if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo -e "${RED}Error: Tag ${TAG} already exists${NC}"
    echo "If you want to replace it, run:"
    echo "  git tag -d ${TAG}"
    echo "  git push origin :refs/tags/${TAG}"
    exit 1
fi

# Confirm release
echo -e "${YELLOW}This will:${NC}"
echo "  1. Create and push tag: ${TAG}"
echo "  2. Trigger GitHub Actions to build for all platforms"
echo "  3. Create a GitHub Release with binaries"
if [ "$SET_LATEST" = true ]; then
    echo "  4. Update the 'latest' tag to point to ${TAG}"
else
    echo "  4. The 'latest' tag will NOT be updated"
    echo "     (use --set-latest flag to update it)"
fi
echo ""
read -p "Proceed with release? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Release cancelled"
    exit 0
fi

# Create and push tag
echo -e "${GREEN}Creating tag ${TAG}...${NC}"
git tag -a "$TAG" -m "Release ${TAG}"

echo -e "${GREEN}Pushing tag to GitHub...${NC}"
git push origin "$TAG"

# Update latest tag if requested
if [ "$SET_LATEST" = true ]; then
    echo -e "${GREEN}Updating 'latest' tag...${NC}"
    
    # Delete the 'latest' tag locally and remotely if it exists
    git tag -d latest 2>/dev/null || true
    git push origin :refs/tags/latest 2>/dev/null || true
    
    # Create new 'latest' tag pointing to the same commit as the version tag
    git tag latest "$TAG"
    git push origin latest
    
    echo -e "${GREEN}✓ 'latest' tag updated to ${TAG}${NC}"
fi

echo ""
echo -e "${GREEN}✓ Release initiated!${NC}"
echo ""
echo "Next steps:"
echo "  1. Watch the build progress: https://github.com/$(git remote get-url origin | sed -E 's|.*github.com[:/]||' | sed 's|.git$||')/actions"
echo "  2. Once complete, check the release: https://github.com/$(git remote get-url origin | sed -E 's|.*github.com[:/]||' | sed 's|.git$||')/releases/tag/${TAG}"
echo ""
echo "The release will be available at:"
echo "  https://github.com/$(git remote get-url origin | sed -E 's|.*github.com[:/]||' | sed 's|.git$||')/releases/latest"
