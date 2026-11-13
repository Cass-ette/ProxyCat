#!/bin/bash
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_PATH="${1:?Usage: build-installer.sh /path/to/ProxyCat.app [output.app]}"
INSTALLER_PATH="${2:-$REPO_ROOT/app/ProxyCat/build/Installer/安装 ProxyCat.app}"
WORK_DIR="$REPO_ROOT/app/ProxyCat/build/Installer/objects"
SDK_PATH="$(xcrun --sdk macosx --show-sdk-path)"
mkdir -p "$WORK_DIR" "$INSTALLER_PATH/Contents/MacOS" "$INSTALLER_PATH/Contents/Resources"
for ARCHITECTURE in arm64 x86_64; do
    xcrun swiftc -swift-version 5 -O -disable-sandbox -sdk "$SDK_PATH" -target "$ARCHITECTURE-apple-macos12.0" \
        -module-cache-path "$WORK_DIR/ModuleCache" \
        "$REPO_ROOT/app/Installer/InstallerCore.swift" "$REPO_ROOT/app/Installer/InstallerApp.swift" \
        "$REPO_ROOT/app/Installer/InstallerView.swift" \
        -o "$WORK_DIR/ProxyCatInstaller-$ARCHITECTURE"
done
lipo -create "$WORK_DIR/ProxyCatInstaller-arm64" "$WORK_DIR/ProxyCatInstaller-x86_64" -output "$INSTALLER_PATH/Contents/MacOS/ProxyCatInstaller"
python3 - "$APP_PATH" "$INSTALLER_PATH" <<'PY'
import plistlib, shutil, sys
from pathlib import Path
app, installer = map(Path, sys.argv[1:])
source = plistlib.loads((app / 'Contents/Info.plist').read_bytes())
info = {
    'CFBundleIdentifier': 'com.cassette.proxycat.installer',
    'CFBundleExecutable': 'ProxyCatInstaller', 'CFBundleName': '安装 ProxyCat',
    'CFBundleDisplayName': '安装 ProxyCat', 'CFBundlePackageType': 'APPL',
    'CFBundleShortVersionString': source['CFBundleShortVersionString'],
    'CFBundleVersion': source['CFBundleVersion'], 'LSMinimumSystemVersion': '12.0',
    'CFBundleIconFile': 'AppIcon', 'CFBundleIconName': 'AppIcon', 'NSHighResolutionCapable': True,
    'LSMultipleInstancesProhibited': True,
}
(installer / 'Contents/Info.plist').write_bytes(plistlib.dumps(info))
for resource in ('AppIcon.icns', 'Assets.car'):
    shutil.copy2(app / 'Contents/Resources' / resource, installer / 'Contents/Resources' / resource)
PY
codesign --force --sign - "$INSTALLER_PATH"
codesign --verify --deep --strict "$INSTALLER_PATH"
echo "Installer built: $INSTALLER_PATH"
