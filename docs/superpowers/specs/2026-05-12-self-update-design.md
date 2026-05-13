# Self-Update MVP Design

## Goal

Let a non-technical user update ProxyCat with one click (App menu) or one command (`proxycat update`), without understanding security checks, GitHub, or terminal workflows.

## Context

- Repo: `Cass-ette/ProxyCat`, branch `main`
- Current distribution: unsigned zip + `安装 ProxyCat.command` installer script
- No Apple Developer ID, no notarization, no Sparkle
- Target user has ProxyCat already installed at `/Applications/ProxyCat.app`
- First-time install remains manual; this feature only covers subsequent updates

## Architecture

### Two entry points, one core

```
App menu "检查更新" button
        │
        ▼
  proxyctl self-update      ◄──  proxycat update (symlink/wrapper)
        │
        ▼
  selfupdate package
```

### Core flow (`proxyctl self-update`)

1. Read current version from `/Applications/ProxyCat.app/Contents/Info.plist` (`CFBundleShortVersionString`)
2. Fetch latest release from `https://api.github.com/repos/Cass-ette/ProxyCat/releases/latest`
   - Cache result: skip API call if last check was < 1 hour ago (store timestamp in `~/Library/Application Support/ProxyCat/config/update-check.json`)
   - On 403 (rate limit): print `更新检查暂时不可用，请稍后重试` and exit 1
3. Compare semver (strict `major.minor.patch`, no pre-release tags); if current >= latest, print `已经是最新版` and exit 0
4. Find asset matching `ProxyCat-<digits>.<digits>.<digits>-installer.zip` (strict regex whitelist)
5. Download to temp directory (created via `os.UserCacheDir()` or `os.TempDir()`; cleaned up on any error or success — step 15 applies to all exit paths)
6. Download matching `.sha256` file; verify hash
7. Unzip
8. Validate bundle: `CFBundleIdentifier == com.cassette.proxycat`, `proxyctl` binary exists and is executable
9. Pre-flight: check available disk space (zip size + extracted size + backup size); abort if insufficient
10. Backup current app to `~/Library/Application Support/ProxyCat/backups/ProxyCat-<old-version>.app`
    - **If backup fails, abort the entire update** — do not proceed without a rollback path
    - Keep only the most recent backup; delete older ones
11. **Quit the running ProxyCat app** before replacing: `osascript -e 'tell application "ProxyCat" to quit'`, wait up to 5 seconds for exit; if still running, `killall ProxyCat` as fallback, then brief pause for process cleanup
12. Replace `/Applications/ProxyCat.app` with new app using `mv` (atomic on same volume; temp dir and `/Applications` are both on the boot volume)
13. `xattr -cr` to clear quarantine (same pattern as existing installer script; needed because macOS quarantines unsigned apps downloaded from the internet)
14. Relaunch: `open /Applications/ProxyCat.app`
15. Clean up temp directory
16. Print Chinese progress throughout

### Rollback contract

The atomic boundary is step 12 (replace). If replace fails (partial copy, permission error, disk full):

1. Delete the incomplete `/Applications/ProxyCat.app`
2. `mv` the backup back to `/Applications/ProxyCat.app`
3. Print `更新失败，已恢复旧版本。请截图发给我。`
4. Relaunch the restored app

Validation failures (step 8) abort before any file mutation — no rollback needed.

### Trust model

SHA256 sidecar verifies integrity (transport corruption, truncated download). It does **not** protect against a compromised GitHub source where both zip and sha256 are replaced in tandem. This is an accepted limitation for the MVP. Future improvement: embed a hardcoded Ed25519 public key and sign the sha256 file.

## Safety checks (automatic, invisible to user)

| Check | What | On failure |
|-------|------|------------|
| Source lock | Only `Cass-ette/ProxyCat` GitHub releases | Hard fail |
| Asset name whitelist | `ProxyCat-\d+\.\d+\.\d+-installer\.zip` (strict regex) | Hard fail |
| SHA256 | Compare downloaded zip against `.sha256` sidecar | Hard fail with "更新包校验失败" |
| Bundle identity | `CFBundleIdentifier == com.cassette.proxycat` | Hard fail |
| Proxyctl presence | `Contents/Resources/proxyctl` exists and executable | Hard fail |
| Disk space | Pre-flight check before download and before replace | Hard fail with disk-space message |
| Backup | Move old app before replacing | **Abort entire update** if backup fails |
| Rollback | If replace fails, restore from backup | Automatic |
| Temp cleanup | Delete temp directory on success or failure | Always |
| API rate limit | Cache last check; don't hit API more than once/hour | Soft fail with "请稍后重试" |

## Error messages (all Chinese, one line)

- `已经是最新版` — no update needed
- `更新包校验失败，请截图发给我` — SHA256 mismatch
- `下载失败，请检查网络后重试` — network error
- `更新失败：无法写入"应用程序"文件夹` — permission denied
- `更新失败：安装包格式不正确` — bundle validation failed
- `更新失败：磁盘空间不足` — disk full
- `更新失败，已恢复旧版本。请截图发给我。` — replace failed, rolled back
- `更新检查暂时不可用，请稍后重试` — API rate limit or 403

## JSON output format (`--json`)

Streams NDJSON progress lines (same pattern as `bootstrap` command in `HelperClient.swift`):

```jsonl
{"stage":"checking","message":"正在检查更新..."}
{"stage":"downloading","message":"正在下载...","progress":45}
{"stage":"verifying","message":"正在验证更新包..."}
{"stage":"installing","message":"正在安装..."}
{"stage":"done","message":"安装完成","newVersion":"0.2.0"}
{"stage":"error","message":"更新包校验失败，请截图发给我。"}
```

The Swift side reads stdout line-by-line via `AsyncStream` and displays `message` in the UI. `progress` is a percentage (0-100) when present.

## CLI entry point: `proxycat update`

Installer creates a small shell wrapper at `/usr/local/bin/proxycat`:

```bash
#!/bin/bash
exec "/Applications/ProxyCat.app/Contents/Resources/proxyctl" "$@"
```

User runs:

```bash
proxycat update
```

The wrapper always points to `/Applications/ProxyCat.app/Contents/Resources/proxyctl`, which survives app updates.

## App menu entry

Add "检查更新" button to `MenuContentView.swift` actions section. On tap, calls `viewModel.checkForUpdate()` which runs `proxyctl self-update --json` and displays the streamed `message` field as Chinese progress/status. When `stage == "installing"`, the Swift side quits the app before the replace step (proxyctl also sends quit, but the app can quit proactively for a cleaner exit).

## Release workflow

For each new version:

1. Update `Info.plist` `CFBundleShortVersionString`
2. Run `./scripts/package-unsigned.sh`
3. Generate `.sha256`: `shasum -a 256 dist/ProxyCat-<ver>-installer.zip > dist/ProxyCat-<ver>-installer.zip.sha256`
4. Create GitHub Release with both files attached
5. Tag: `v<version>`

## Scope

- In scope: `proxyctl self-update`, `proxycat` wrapper, App menu button, release script update, SHA256 generation, tests
- Out of scope: Developer ID signing, notarization, Sparkle, DMG/PKG, auto-check on launch, incremental/delta updates, rollback UI, Ed25519 release signing

## Implementation order

1. `helper/internal/selfupdate/` package: version check, download, verify, replace logic, tests
2. `proxyctl self-update` CLI command wiring, tests
3. Update `安装 ProxyCat.command` to create `/usr/local/bin/proxycat` wrapper
4. Update `scripts/package-unsigned.sh` to generate `.sha256`
5. App menu "检查更新" button in Swift
6. End-to-end manual test with a real GitHub release
