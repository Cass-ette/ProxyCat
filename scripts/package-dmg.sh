#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
APP_PATH="${1:-$REPO_ROOT/app/ProxyCat/build/Build/Products/Release/ProxyCat.app}"
DIST_DIR="${2:-$REPO_ROOT/dist}"
TOOLS_DIR="$REPO_ROOT/app/ProxyCat/build/dmg-tools"

if [[ ! -x "$APP_PATH/Contents/MacOS/ProxyCat" || ! -x "$APP_PATH/Contents/Resources/proxyctl" ]]; then
    echo "Build ProxyCat.app first with scripts/build-app.sh, or pass its path." >&2
    exit 1
fi
APP_PATH="$(cd "$(dirname "$APP_PATH")" && pwd)/$(basename "$APP_PATH")"
VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP_PATH/Contents/Info.plist")"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Invalid app version: $VERSION" >&2
    exit 1
fi

# Keep packaging tools out of the user's Python environment. Subsequent runs
# reuse this environment; no Finder automation or GUI session is required.
if [[ ! -x "$TOOLS_DIR/bin/python3" ]]; then
    python3 -m venv "$TOOLS_DIR"
fi
"$TOOLS_DIR/bin/python3" -m pip install --disable-pip-version-check -r "$SCRIPT_DIR/dmg/requirements.txt"

mkdir -p "$DIST_DIR"
DIST_DIR="$(cd "$DIST_DIR" && pwd)"
WORK_DIR="$(mktemp -d "$DIST_DIR/.proxycat-dmg.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT

# Sign a staging copy so packaging never changes the app or existing ZIPs.
# Ad-hoc signatures ensure code integrity; Developer ID trust and notarization
# still require an Apple developer account.
ditto "$APP_PATH" "$WORK_DIR/ProxyCat.app"
bash "$SCRIPT_DIR/adhoc-sign.sh" "$WORK_DIR/ProxyCat.app"

DMG_NAME="ProxyCat-$VERSION.dmg"
"$TOOLS_DIR/bin/dmgbuild" \
    -s "$SCRIPT_DIR/dmg/settings.py" \
    -D "app=$WORK_DIR/ProxyCat.app" \
    -D "background=$SCRIPT_DIR/dmg/background.png" \
    "ProxyCat $VERSION" "$WORK_DIR/$DMG_NAME"

hdiutil verify "$WORK_DIR/$DMG_NAME"
mv "$WORK_DIR/$DMG_NAME" "$DIST_DIR/$DMG_NAME"
(cd "$DIST_DIR" && shasum -a 256 "$DMG_NAME") > "$DIST_DIR/$DMG_NAME.sha256"
echo "DMG created: $DIST_DIR/$DMG_NAME"
echo "Open the DMG and drag ProxyCat into Applications."
