#!/bin/bash
set -euo pipefail

DMG_PATH="${1:?Usage: bash scripts/test-dmg.sh path/to/ProxyCat-version.dmg}"
DMG_PATH="$(cd "$(dirname "$DMG_PATH")" && pwd)/$(basename "$DMG_PATH")"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/proxycat-dmg-test.XXXXXX")"
MOUNT_PATH="$TEST_DIR/mount"
MOUNTED=false
fail() {
    echo "$*" >&2
    exit 1
}
cleanup() {
    if [[ "$MOUNTED" == true ]]; then
        hdiutil detach "$MOUNT_PATH"
    fi
    rm -rf "$TEST_DIR"
}
trap cleanup EXIT

(cd "$(dirname "$DMG_PATH")" && shasum -a 256 -c "$(basename "$DMG_PATH").sha256")
hdiutil verify "$DMG_PATH"
hdiutil attach -readonly -nobrowse -noautoopen -mountpoint "$MOUNT_PATH" -plist "$DMG_PATH" > "$TEST_DIR/attach.plist"
MOUNTED=true

[[ -L "$MOUNT_PATH/Applications" && "$(readlink "$MOUNT_PATH/Applications")" == /Applications ]] || fail "Missing Applications link"
[[ -s "$MOUNT_PATH/.DS_Store" && -s "$MOUNT_PATH/.background.png" ]] || fail "Missing Finder layout or background"
"$REPO_ROOT/app/ProxyCat/build/dmg-tools/bin/python3" - "$MOUNT_PATH/.DS_Store" <<'PY'
from ds_store import DSStore
import sys
with DSStore.open(sys.argv[1], 'r') as store:
    assert store['ProxyCat.app']['Iloc'] == (170, 190)
    assert store['Applications']['Iloc'] == (490, 190)
    assert store['.']['bwsp']['WindowBounds'] == '{{200, 140}, {660, 440}}'
    assert store['.']['icvp']['backgroundType'] == 2
    assert store['.']['icvp']['iconSize'] == 104
PY
APP_PATH="$MOUNT_PATH/ProxyCat.app"
[[ -x "$APP_PATH/Contents/MacOS/ProxyCat" && -x "$APP_PATH/Contents/Resources/proxyctl" ]] || fail "Missing app executables"
[[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$APP_PATH/Contents/Info.plist")" == com.cassette.proxycat ]] || fail "Incorrect app identifier"
VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP_PATH/Contents/Info.plist")"
[[ "$(basename "$DMG_PATH")" == "ProxyCat-$VERSION.dmg" ]] || fail "DMG filename and app version disagree"
for EXECUTABLE in "$APP_PATH/Contents/MacOS/ProxyCat" "$APP_PATH/Contents/Resources/proxyctl"; do
    ARCHITECTURES="$(lipo -archs "$EXECUTABLE")"
    [[ " $ARCHITECTURES " == *" x86_64 "* && " $ARCHITECTURES " == *" arm64 "* ]] || fail "Missing universal architectures: $EXECUTABLE"
done
codesign --verify --deep --strict --verbose "$APP_PATH"
echo "Passed DMG integrity, mounted layout, version, universal binaries and code signature checks."
