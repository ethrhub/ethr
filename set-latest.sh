#!/bin/bash

# Script to update the 'latest' tag to point to a specific version
# Usage: ./set-latest.sh [version]
# Example: ./set-latest.sh v1.0.0

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
        exit 1
    fi
    echo ""
}

# Function to show current latest
show_current_latest() {
    if git rev-parse latest >/dev/null 2>&1; then
        local latest_commit=$(git rev-parse latest)
        local latest_tag=$(git tag --points-at "$latest_commit" | grep "^v" | head -1)
        echo -e "${BLUE}Current 'latest' tag points to:${NC}"
        echo "  Commit: ${latest_commit:0:8}"
        if [ -n "$latest_tag" ]; then
            echo "  Version: $latest_tag"
        fi
        echo ""
    else
        echo -e "${YELLOW}No 'latest' tag currently exists${NC}"
        echo ""
    fi
}

# Show help
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "Usage: $0 [version]"
    echo ""
    echo "Update the 'latest' tag to point to a specific version."
    echo ""
    echo "Arguments:"
    echo "  version    Version tag to set as latest (e.g., v1.0.0 or 1.0.0)"
    echo "             If omitted, you'll be prompted to choose from existing versions."
    echo ""
    echo "Examples:"
    echo "  $0           # Interactive mode - choose from list"
    echo "  $0 v1.0.0    # Set v1.0.0 as latest"
    echo "  $0 1.0.0     # Same as above (v prefix optional)"
    echo ""
    exit 0
fi

# Show current latest
show_current_latest

# Get target version
if [ -z "$1" ]; then
    show_existing_versions
    echo -e "${YELLOW}Enter the version to set as 'latest'${NC}"
    read -p "Version: " TARGET_VERSION
    
    if [ -z "$TARGET_VERSION" ]; then
        echo -e "${RED}Error: Version cannot be empty${NC}"
        exit 1
    fi
else
    TARGET_VERSION=$1
fi

# Add 'v' prefix if not present
if [[ ! "$TARGET_VERSION" =~ ^v ]]; then
    TARGET_VERSION="v${TARGET_VERSION}"
fi

# Check if the target version exists
if ! git rev-parse "$TARGET_VERSION" >/dev/null 2>&1; then
    echo -e "${RED}Error: Tag ${TARGET_VERSION} does not exist${NC}"
    echo ""
    show_existing_versions
    exit 1
fi

# Confirm the action
echo -e "${YELLOW}This will update the 'latest' tag to point to: ${TARGET_VERSION}${NC}"
echo ""
echo "This means users downloading from the 'latest' release will get ${TARGET_VERSION}"
echo ""
read -p "Proceed? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Operation cancelled"
    exit 0
fi

# Delete the 'latest' tag locally and remotely if it exists
echo -e "${GREEN}Removing old 'latest' tag...${NC}"
git tag -d latest 2>/dev/null || true
git push origin :refs/tags/latest 2>/dev/null || true

# Create new 'latest' tag pointing to the target version
echo -e "${GREEN}Creating new 'latest' tag at ${TARGET_VERSION}...${NC}"
git tag latest "$TARGET_VERSION"

# Push the new latest tag
echo -e "${GREEN}Pushing 'latest' tag to GitHub...${NC}"
git push origin latest

echo ""
echo -e "${GREEN}✓ 'latest' tag successfully updated to ${TARGET_VERSION}${NC}"
echo ""
echo "Users can now download this version using:"
echo "  https://github.com/$(git remote get-url origin | sed -E 's|.*github.com[:/]||' | sed 's|.git$||')/releases/latest"
