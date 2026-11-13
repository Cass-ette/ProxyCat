#!/bin/bash
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/proxycat-installer.XXXXXX")"
trap 'rm -rf "$TEST_DIR"' EXIT
xcrun swiftc -swift-version 5 -sdk "$(xcrun --sdk macosx --show-sdk-path)" -module-cache-path "$TEST_DIR/ModuleCache" \
    "$REPO_ROOT/app/Installer/InstallerCore.swift" "$REPO_ROOT/app/Installer/InstallerTests.swift" -o "$TEST_DIR/installer-tests"
"$TEST_DIR/installer-tests"
