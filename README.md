# ProxyCat

[![Build Status](https://github.com/Cass-ette/ProxyCat/actions/workflows/build.yml/badge.svg)](https://github.com/Cass-ette/ProxyCat/actions)
[![Go Version](https://img.shields.io/badge/go-%5E1.21-blue)](https://golang.org)
[![Swift Version](https://img.shields.io/badge/swift-5.0-orange)](https://swift.org)

A diagnostic-first macOS menu bar app for managing Mihomo/Clash subscriptions with transparency and safety.

## Features

- **Simple workflow**: Add subscription → generate config → start core → toggle system proxy → test connectivity
- **Transparent**: Shows real macOS system proxy state, not cached guesses
- **Diagnostic**: One-click diagnosis identifies which layer failed and suggests fixes
- **Safe**: Redacts subscription tokens and node credentials from logs and UI
- **Clean shutdown**: Turns off system proxies before stopping Mihomo on quit, logout, shutdown, or restart
- **Campus Wi-Fi protection**: Matches saved SSIDs and either prompts or disconnects ProxyCat, without automatically reconnecting afterward
- **Connection recovery**: Enabling the system proxy starts a stopped core with the saved configuration and waits for fresh proxy groups. Temporary controller failures preserve the last node list and saved profiles; retry without reimporting the subscription.
- **Menu bar status**: Green means the system proxy or TUN is on; gray means both are off. Direct mode is blue, and an enabled system proxy with a stopped engine shows an orange warning. The tooltip describes the active capture method; colors do not certify network reachability.
- **Quota warnings**: A separate dot beside the cat marks fresh subscription allowance: yellow at ≤20%, orange at ≤10%, and red when exhausted or expired. Hover for details. The cat retains its connection-state color; stale metadata does not claim a current quota warning.
- **Subscription allowance**: A compact battery beside the menu title shows the selected subscription’s remaining / total traffic, with the expiry date and days left directly underneath. Expiry within seven days is orange; less than one day or expired is red. Click for usage, exact expiry time, last update and manual refresh. Reads the provider’s `Subscription-Userinfo` metadata; unavailable limits and expiry remain unknown, and old or failed refreshes are labeled as cached without a current expiry warning. Reopening the menu reuses fresh data for 30 minutes and retries failed requests at most once every five minutes. This does not change the core, TUN or system proxy. The battery uses the MIT-licensed [BatteryView](https://github.com/theMomax/BatteryView) component, vendored locally with its license.
- **Compact menu**: A native AppKit `NSPopover` manages the menu-bar anchor, frame and outside-click dismissal. Subscription input and one-click import/start stay at the top, followed by the system proxy, routing modes and TUN. Nodes, profiles, campus settings and tools expand on demand and remember your choices; only the active node group expands by default. The menu shrinks to fit collapsed content and scrolls when it reaches the available screen height (up to 640 points). Errors and ongoing operations remain visible even when their settings are collapsed.
  Clicking outside, pressing Escape in the menu, or switching Spaces dismisses it. Closing the menu does not cancel a running TUN switch; actual handoff stages and elapsed time appear when reopened.
- **Website checks**: Open **有网站打不开？点这里** for a real request through ProxyCat, socket-correlated route evidence and response classification. Retry with another observed selector or explicitly switch to global with a restore action. A probe does not prove another app uses ProxyCat. Checks send no browser cookies and do not follow redirects.
- **Readable connections**: Open **连接记录** to see sites, actual direct/proxy routes, nodes and rule explanations grouped by application. Opening the window enables process lookup for new connections without restarting the engine or changing routing/capture; existing connections and some system traffic may remain unidentified. Search by app, site or node, expand a record for route evidence, and clear ended records. The window samples every two seconds while open and retains up to 300 observed records in memory until quitting; closed windows stop sampling. Short connections can be missed, and absent records cannot prove an app bypasses ProxyCat. If an app is missing, the view suggests trying TUN without changing capture automatically.
  When Mihomo cannot supply process names on macOS, ProxyCat uses the current user's local socket table to identify unambiguous owners. It does not request administrator access or expose absolute executable paths in connection records; unknown/ambiguous owners stay unidentified.
- **Node explanations**: Show observed changes in automatic groups without guessing failure causes. Latency reads fast / average / slow / unavailable, with untested distinct and milliseconds available on hover.
- **Mode help**: Click the help icon beside Rule / Global / Direct for a brief explanation. Modes only affect traffic that enters ProxyCat.
- **Issue feedback**: Choose **问题反馈（Issue）** to report a bug or suggest a feature on GitHub.
- **TUN capture**: Enable the separate **TUN** switch to capture traffic from apps that do not use the system proxy. Administrator authorization is needed to install the system helper on first use; later toggles and app restarts reuse it. Rule / Global / Direct still determine routing. Turning TUN off restores the normal core; quitting or campus disconnection stops the tunnel session. The system helper stays idle without an active session. TUN is off by default and is not automatically reconnected. Remove the helper from **更多 → 卸载 TUN 系统助手…** after turning TUN off.
  Startup confirms the authorized service is prepared before stopping the normal core. A user-owned local relay keeps the fixed proxy and controller endpoints available across privileged TUN core handoffs; the tunnel core uses private ports. Failed handoffs wait for tunnel shutdown before restoring the previous node and mode; lifecycle commands are serialized to avoid duplicate cores. See [TUN testing](docs/tun-testing.md) for isolated checks and the separate local acceptance procedure.

## Screenshots

The menu bar app shows:
- Current proxy status (On/Off)
- Core status and system proxy state
- Current node and profile summaries, with expandable settings
- **快速开始**: paste a subscription link and import/start in one click
- **配置**: switch, delete and update saved subscriptions
- **更多**: connection tests, engine restart, updates, diagnostics, Issue feedback and logs

## Installation

Download `ProxyCat-X.Y.Z.dmg` from [GitHub Releases](https://github.com/Cass-ette/ProxyCat/releases/latest), open it, and drag **ProxyCat** onto **Applications**. Quit any older running version before replacing it, then open ProxyCat from Applications. Its cat icon appears in the menu bar.

The app is not yet Developer ID signed or notarized. If macOS blocks the first launch, open **System Settings → Privacy & Security → Open Anyway**. A DMG does not remove that requirement.

You can also use **检查更新** in the app, install the available update, and restart when prompted. Starting with 0.12.1, the updater prefers the DMG and still accepts older installer ZIPs. Releases include a compatibility ZIP so earlier versions can upgrade through their existing updater, even when skipping releases. Use the DMG for new installations.

Starting with 0.16.0, ProxyCat checks for updates in the background while the engine is running and either the system proxy or TUN is enabled, including when the app starts already connected. Successful checks are limited to once per 24 hours; failed checks stay silent and retry after 30 minutes while connected. A new-version link appears in the menu even when **更多** is collapsed, and stays available after restarting the app. Open it to review and choose installation. Automatic checks never download or install an update; **检查更新** still works at any time.

Starting with 0.12.4, update checks use the release's `update.json` asset to avoid GitHub's shared anonymous API limit, with the API as a fallback. If both sources are unavailable, the update window still shows the installed version's bundled changelog and the failure reason. Older versions blocked by API limits can install the latest DMG manually.

Checking and downloading updates require access to GitHub. Some networks may need a proxy or VPN enabled first. If a download fails, enable it and choose **重试安装**. From 0.13.1, the update window keeps this reminder visible and puts technical connection errors in expandable details.

## Campus Wi-Fi Protection

Open **校园网保护** in the menu, allow Wi-Fi name access when prompted, and add the current Wi-Fi or enter its SSID manually. SSIDs are matched exactly, including case and spaces, and the list and policy are saved locally.

- **提示** (default): Warns once when ProxyCat is active on a saved campus Wi-Fi, with an option to disconnect.
- **自动断开**: Disables system proxies before stopping Mihomo, cancels in-progress bootstrap, and blocks starting ProxyCat on that Wi-Fi.
- Wi-Fi is checked during the five-second status refresh. Switching networks can therefore take a refresh cycle to detect; this is not a guarantee that no proxy traffic occurs during the transition.
- Leaving campus Wi-Fi does not restore the proxy. Reconnect manually when appropriate.
- Detection requires access to the Wi-Fi name. If it is unavailable, the menu shows that protection is inactive. Wired campus connections and VPNs managed by other applications are not controlled by this feature.

Run the macOS policy and lifecycle regressions with `bash scripts/test-campus-network.sh` and `bash scripts/test-shutdown.sh`. The tests use isolated fake helpers and do not change the machine's network configuration.

The [network recovery notes](docs/network-recovery.md) describe the independent system-proxy guard, its isolated tests, and the distinction between crash recovery and normal shutdown.

Run connection recovery regressions with `bash scripts/test-connection-recovery.sh`. These also use isolated fake helpers, including simulated controller failures and network transitions.

Run menu interaction checks with `bash scripts/test-menu-interaction.sh` from a macOS GUI session. A separate test process verifies subscription typing, expanding/collapsing content, child windows, nested native popovers, outside clicks, Space-change notifications and Escape with isolated preferences and automatic network refresh disabled; it never starts the installed app or its core.

## Building

Version numbers follow the [versioning policy](docs/versioning.md): new features increment the minor version, fixes increment the patch version, and `Info.plist` is the single source. See the [release process](docs/release.md) for checks and packaging.

### Prerequisites

- macOS 11.0+ (Big Sur)
- Go 1.21+ (for helper)
- Xcode 13+ (for menu bar app)

### Build Steps

1. Clone the repository:
   ```bash
   git clone https://github.com/Cass-ette/ProxyCat.git
   cd ProxyCat
   ```

2. Build everything:
   ```bash
   ./scripts/build-app.sh
   ```

   This creates `app/ProxyCat/build/Build/Products/Release/ProxyCat.app`

3. Run the app:
   ```bash
   open app/ProxyCat/build/Build/Products/Release/ProxyCat.app
   ```

## Architecture

```
ProxyCat.app
├── SwiftUI macOS menu bar UI
├── bundled Go helper: proxyctl
└── Mihomo core binary
```

### Go Helper (proxyctl)

- Subscription download and detection
- Config generation and validation
- Mihomo process lifecycle
- macOS system proxy state read/write
- Connection tests and diagnostics
- Redaction of sensitive data

### SwiftUI Menu Bar App

- Menu bar icon and status menu
- User actions and lightweight preferences
- Calls proxyctl and renders JSON responses

## Security & Redaction

ProxyCat treats these as sensitive and redacts them from logs and UI:

- Subscription query tokens (`token`, `key`, `password`, `pass`)
- Node passwords and credentials
- VMess/VLESS UUIDs
- Trojan passwords
- Shadowsocks passwords
- Raw node URI strings

Redaction rules:
- URL query parameters: `token=<redacted>`
- URL userinfo: `<redacted>@host`
- Diagnostic reports default to redacted output

## Data Locations

ProxyCat stores user data under `~/Library/Application Support/ProxyCat/`:

```
~/Library/Application Support/ProxyCat/
├── bin/
│   ├── proxyctl
│   └── mihomo
├── config/
│   ├── config.yaml
│   ├── subscriptions.json
│   └── backups/
├── logs/
│   ├── proxycat.log
│   └── mihomo.log
└── reports/
    └── diagnose-latest.json
```

## Non-Goals

ProxyCat MVP does not include:
- Self-implemented proxy protocols (uses Mihomo)
- Advanced DNS configuration
- Complex rule editor
- Airport account login, billing and purchase management
- Apple Developer ID signing/notarization
- Windows/Linux GUI

## License

License TBD. All rights reserved until a license is chosen. See [LICENSE](LICENSE).

## Contributing

See [docs/superpowers/specs/2026-05-02-proxycat-design.md](docs/superpowers/specs/2026-05-02-proxycat-design.md) for design specifications.

Website routing and recovery regressions run with `bash scripts/test-connection-recovery.sh` and `go -C helper test ./internal/controller`. Set `PROXYCAT_TEST_MIHOMO` to an installed Mihomo binary to include isolated real-kernel route checks; these use loopback ports and never change the system proxy.
