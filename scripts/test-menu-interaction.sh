#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/proxycat-menu.XXXXXX")"
trap 'rm -rf "$TEST_DIR"' EXIT

# Compile the local, MIT-licensed battery dependency without package downloads.
BATTERY_SOURCES=()
while IFS= read -r path; do BATTERY_SOURCES+=("$path"); done < <(rg --files "$REPO_ROOT/app/ProxyCat/Vendor/BatteryView/Sources" -g '*.swift')
swiftc -swift-version 5 -module-cache-path "$TEST_DIR/ModuleCache" \
    -emit-library -emit-module -module-name BatteryView \
    -emit-module-path "$TEST_DIR/BatteryView.swiftmodule" \
    "${BATTERY_SOURCES[@]}" -o "$TEST_DIR/libBatteryView.dylib"

APP_SOURCES=()
while IFS= read -r path; do APP_SOURCES+=("$path"); done < <(rg --files "$REPO_ROOT/app/ProxyCat/ProxyCat" -g '*.swift' -g '!ProxyCatApp.swift')
swiftc -swift-version 5 -module-cache-path "$TEST_DIR/ModuleCache" \
    -I "$TEST_DIR" -L "$TEST_DIR" -lBatteryView \
    -Xlinker -rpath -Xlinker "$TEST_DIR" \
    "${APP_SOURCES[@]}" "$REPO_ROOT/app/ProxyCat/Tests/MenuInteractionTests.swift" \
    -o "$TEST_DIR/menu-tests"
if ! "$TEST_DIR/menu-tests" > "$TEST_DIR/result.log" 2>&1; then
    cat "$TEST_DIR/result.log"
    exit 1
fi
cat "$TEST_DIR/result.log"
if ! rg -q '^Passed isolated native menu ' "$TEST_DIR/result.log"; then
    echo "Menu checks did not finish; run from a macOS GUI session with WindowServer access." >&2
    exit 1
fi
