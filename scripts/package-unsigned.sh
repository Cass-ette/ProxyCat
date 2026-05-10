#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INFO_PLIST="$REPO_ROOT/app/ProxyCat/ProxyCat/Info.plist"
RESOURCE_PROXYCTL="$REPO_ROOT/app/ProxyCat/ProxyCat/Resources/proxyctl"
APP_PATH="$REPO_ROOT/app/ProxyCat/build/Build/Products/Release/ProxyCat.app"
DIST_DIR="$REPO_ROOT/dist"
INSTALLER_DIR_NAME="ProxyCat 安装包"
RESTORE_DIR=""

cleanup() {
    if [[ -n "$RESTORE_DIR" && -f "$RESTORE_DIR/proxyctl" ]]; then
        cp "$RESTORE_DIR/proxyctl" "$RESOURCE_PROXYCTL"
    fi
    if [[ -n "$RESTORE_DIR" ]]; then
        rm -rf "$RESTORE_DIR"
    fi
}
trap cleanup EXIT

if [[ ! -f "$INFO_PLIST" ]]; then
    echo "Error: Info.plist not found at $INFO_PLIST" >&2
    exit 1
fi

VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$INFO_PLIST")"
if [[ -z "$VERSION" ]]; then
    echo "Error: CFBundleShortVersionString is empty" >&2
    exit 1
fi

if [[ -f "$RESOURCE_PROXYCTL" ]]; then
    RESTORE_DIR="$(mktemp -d)"
    cp "$RESOURCE_PROXYCTL" "$RESTORE_DIR/proxyctl"
fi

"$SCRIPT_DIR/build-app.sh"

if [[ ! -d "$APP_PATH" ]]; then
    echo "Error: App not found at $APP_PATH" >&2
    exit 1
fi

mkdir -p "$DIST_DIR"
ZIP_PATH="$DIST_DIR/ProxyCat-$VERSION.zip"
INSTALLER_ZIP_PATH="$DIST_DIR/ProxyCat-$VERSION-installer.zip"
INSTALLER_WORKDIR="$(mktemp -d)"
INSTALLER_ROOT="$INSTALLER_WORKDIR/$INSTALLER_DIR_NAME"
rm -f "$ZIP_PATH" "$INSTALLER_ZIP_PATH"

(
    cd "$(dirname "$APP_PATH")"
    ditto -c -k --sequesterRsrc --keepParent "$(basename "$APP_PATH")" "$ZIP_PATH"
)

mkdir -p "$INSTALLER_ROOT"
ditto "$APP_PATH" "$INSTALLER_ROOT/ProxyCat.app"

cat > "$INSTALLER_ROOT/安装 ProxyCat.command" <<'INSTALLER'
#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_SOURCE="$SCRIPT_DIR/ProxyCat.app"
APP_DEST="/Applications/ProxyCat.app"

if [[ ! -d "$APP_SOURCE" ]]; then
    echo "没有找到 ProxyCat.app，请确认它和本安装脚本在同一个文件夹里。"
    read -r -p "按回车退出..."
    exit 1
fi

echo "正在安装 ProxyCat 到应用程序文件夹..."
if [[ -d "$APP_DEST" ]]; then
    rm -rf "$APP_DEST" 2>/dev/null || sudo rm -rf "$APP_DEST"
fi
cp -R "$APP_SOURCE" "$APP_DEST" 2>/dev/null || sudo cp -R "$APP_SOURCE" "$APP_DEST"

xattr -cr "$APP_DEST" 2>/dev/null || true

echo "安装完成，正在打开 ProxyCat..."
open "$APP_DEST" || true

echo ""
echo "如果系统提示无法验证开发者："
echo "1. 打开 系统设置 → 隐私与安全性"
echo "2. 在安全性区域找到 ProxyCat 被阻止的提示"
echo "3. 点击“仍要打开”"
echo ""
read -r -p "按回车退出..."
INSTALLER
chmod +x "$INSTALLER_ROOT/安装 ProxyCat.command"

cat > "$INSTALLER_ROOT/安装说明.txt" <<'README'
ProxyCat 安装说明

1. 双击“安装 ProxyCat.command”。
2. 如果系统拦截安装脚本，请右键“安装 ProxyCat.command”，选择“打开”，再点“打开”。
3. 如果安装过程要求输入密码，请输入你的 Mac 登录密码。输入时屏幕上不会显示字符，这是正常的。
4. 安装后 ProxyCat 会尝试自动打开，并出现在屏幕顶部菜单栏。
5. 如果系统提示“无法验证开发者”，请打开：系统设置 → 隐私与安全性 → 安全性，找到 ProxyCat 被阻止的提示，点击“仍要打开”。
6. 打开后点击菜单栏里的 ProxyCat 图标，粘贴订阅链接，然后点击“一键启动”。

如果遇到问题，把安装窗口里的提示截图发给我。
README

(
    cd "$INSTALLER_WORKDIR"
    ditto -c -k --sequesterRsrc --keepParent "$INSTALLER_DIR_NAME" "$INSTALLER_ZIP_PATH"
)
rm -rf "$INSTALLER_WORKDIR"

echo "Package created: $ZIP_PATH"
echo "Installer package created: $INSTALLER_ZIP_PATH"
echo "Share the installer zip with non-technical users. They should unzip it and double-click 安装 ProxyCat.command."
