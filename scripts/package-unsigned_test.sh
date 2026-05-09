#!/bin/bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$TMP_DIR/scripts"
mkdir -p "$TMP_DIR/app/ProxyCat/ProxyCat/Resources"

cp "$REPO_ROOT/scripts/package-unsigned.sh" "$TMP_DIR/scripts/package-unsigned.sh"
printf 'original helper' > "$TMP_DIR/app/ProxyCat/ProxyCat/Resources/proxyctl"
ORIGINAL_HELPER_SHA="$(shasum -a 256 "$TMP_DIR/app/ProxyCat/ProxyCat/Resources/proxyctl" | awk '{print $1}')"

cat > "$TMP_DIR/scripts/build-app.sh" <<'BUILD'
#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="$ROOT/app/ProxyCat/build/Build/Products/Release/ProxyCat.app"
mkdir -p "$APP/Contents/MacOS"
printf 'rebuilt helper' > "$ROOT/app/ProxyCat/ProxyCat/Resources/proxyctl"
printf 'fake app binary' > "$APP/Contents/MacOS/ProxyCat"
BUILD
chmod +x "$TMP_DIR/scripts/build-app.sh"

cat > "$TMP_DIR/app/ProxyCat/ProxyCat/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleShortVersionString</key>
    <string>9.8.7</string>
</dict>
</plist>
PLIST

(
    cd "$TMP_DIR"
    bash ./scripts/package-unsigned.sh
)

ZIP_PATH="$TMP_DIR/dist/ProxyCat-9.8.7.zip"
INSTALLER_ZIP_PATH="$TMP_DIR/dist/ProxyCat-9.8.7-installer.zip"
if [[ ! -f "$ZIP_PATH" ]]; then
    echo "expected package at $ZIP_PATH" >&2
    exit 1
fi
if [[ ! -f "$INSTALLER_ZIP_PATH" ]]; then
    echo "expected installer package at $INSTALLER_ZIP_PATH" >&2
    exit 1
fi

CURRENT_HELPER_SHA="$(shasum -a 256 "$TMP_DIR/app/ProxyCat/ProxyCat/Resources/proxyctl" | awk '{print $1}')"
if [[ "$CURRENT_HELPER_SHA" != "$ORIGINAL_HELPER_SHA" ]]; then
    echo "expected source proxyctl to be restored after packaging" >&2
    exit 1
fi

python3 - "$ZIP_PATH" <<'PY'
import sys
import zipfile

app_zip_path = sys.argv[1]
with zipfile.ZipFile(app_zip_path) as archive:
    app_names = set(archive.namelist())

if "ProxyCat.app/Contents/MacOS/ProxyCat" not in app_names:
    print("expected zip to contain ProxyCat.app", file=sys.stderr)
    sys.exit(1)
PY

EXTRACT_DIR="$TMP_DIR/extracted-installer"
mkdir -p "$EXTRACT_DIR"
ditto -x -k "$INSTALLER_ZIP_PATH" "$EXTRACT_DIR"

if [[ ! -f "$EXTRACT_DIR/ProxyCat 安装包/ProxyCat.app/Contents/MacOS/ProxyCat" ]]; then
    echo "expected installer zip to contain ProxyCat.app" >&2
    exit 1
fi
if [[ ! -x "$EXTRACT_DIR/ProxyCat 安装包/安装 ProxyCat.command" ]]; then
    echo "expected installer script to exist and be executable" >&2
    exit 1
fi
if [[ ! -f "$EXTRACT_DIR/ProxyCat 安装包/安装说明.txt" ]]; then
    echo "expected installer instructions to exist" >&2
    exit 1
fi
