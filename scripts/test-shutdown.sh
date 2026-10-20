#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/proxycat-shutdown.XXXXXX")"
trap 'rm -rf "$TEST_DIR"' EXIT

swiftc -swift-version 5 -module-cache-path "$TEST_DIR/ModuleCache" \
    "$REPO_ROOT/app/ProxyCat/ProxyCat/Models.swift" \
    "$REPO_ROOT/app/ProxyCat/ProxyCat/HelperClient.swift" \
    "$REPO_ROOT/app/ProxyCat/Tests/ShutdownTests.swift" \
    -o "$TEST_DIR/shutdown-tests"
"$TEST_DIR/shutdown-tests"
