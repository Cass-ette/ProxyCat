#!/bin/bash
set -e

# scripts/build-app.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
APP_DIR="$REPO_ROOT/app/ProxyCat"
HELPER_DIR="$REPO_ROOT/helper"
RESOURCES_DIR="$APP_DIR/ProxyCat/Resources"

# Build proxyctl for macOS native
echo "Building proxyctl..."
mkdir -p "$RESOURCES_DIR"
cd "$HELPER_DIR"
GOOS=darwin GOARCH=amd64 go build -o "$RESOURCES_DIR/proxyctl-amd64" ./cmd/proxyctl
GOOS=darwin GOARCH=arm64 go build -o "$RESOURCES_DIR/proxyctl-arm64" ./cmd/proxyctl

# Create universal binary
echo "Creating universal binary..."
lipo -create \
    "$RESOURCES_DIR/proxyctl-amd64" \
    "$RESOURCES_DIR/proxyctl-arm64" \
    -output "$RESOURCES_DIR/proxyctl"

rm "$RESOURCES_DIR/proxyctl-amd64" "$RESOURCES_DIR/proxyctl-arm64"

echo "Building ProxyCat app..."
xcodebuild \
    -project "$APP_DIR/ProxyCat.xcodeproj" \
    -scheme ProxyCat \
    -configuration Release \
    -derivedDataPath "$APP_DIR/build" \
    CODE_SIGNING_ALLOWED=NO

echo "Build complete: $APP_DIR/build/Build/Products/Release/ProxyCat.app"
