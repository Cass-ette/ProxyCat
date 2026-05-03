#!/bin/bash
set -e

# scripts/adhoc-sign.sh
# Ad-hoc sign the ProxyCat app for local use (no Developer ID required)
# Usage: ./scripts/adhoc-sign.sh [path-to-app]

APP_PATH="${1:-app/ProxyCat/build/Build/Products/Release/ProxyCat.app}"

if [ ! -d "$APP_PATH" ]; then
    echo "Error: App not found at $APP_PATH"
    echo "Build first with: ./scripts/build-app.sh"
    exit 1
fi

echo "Ad-hoc signing ProxyCat..."

# Sign the bundled proxyctl binary
echo "Signing proxyctl..."
codesign --sign - --force --preserve-metadata=entitlements \
    "$APP_PATH/Contents/Resources/proxyctl"

# Sign the app bundle
echo "Signing ProxyCat.app..."
codesign --sign - --force --deep --preserve-metadata=entitlements \
    "$APP_PATH"

# Verify signature
echo "Verifying signature..."
codesign --verify --verbose "$APP_PATH"

echo "Done. App is ad-hoc signed and ready for local use."
echo "Note: Ad-hoc signed apps only work on this Mac."
