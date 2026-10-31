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
- **Menu bar status**: Green means the system proxy is on; gray means off. Direct mode is blue, and an enabled system proxy with a stopped engine shows an orange warning. The tooltip describes the state; colors do not certify network reachability.

## Screenshots

The menu bar app shows:
- Current proxy status (On/Off)
- Core status and system proxy state
- Quick actions: toggle proxy, update subscription, test connection, restart core
- Diagnostics and logs access

## Installation

Download `ProxyCat-X.Y.Z.dmg` from [GitHub Releases](https://github.com/Cass-ette/ProxyCat/releases/latest), open it, and drag **ProxyCat** onto **Applications**. Quit any older running version before replacing it, then open ProxyCat from Applications. Its cat icon appears in the menu bar.

The app is not yet Developer ID signed or notarized. If macOS blocks the first launch, open **System Settings → Privacy & Security → Open Anyway**. A DMG does not remove that requirement.

You can also use **检查更新** in the app, install the available update, and restart when prompted. Starting with 0.12.1, the updater prefers the DMG and still accepts older installer ZIPs. Releases include a compatibility ZIP so earlier versions can upgrade through their existing updater, even when skipping releases. Use the DMG for new installations.

Starting with 0.12.4, update checks use the release's `update.json` asset to avoid GitHub's shared anonymous API limit, with the API as a fallback. If both sources are unavailable, the update window still shows the installed version's bundled changelog and the failure reason. Older versions blocked by API limits can install the latest DMG manually.

## Campus Wi-Fi Protection

Open **校园网保护** in the menu, allow Wi-Fi name access when prompted, and add the current Wi-Fi or enter its SSID manually. SSIDs are matched exactly, including case and spaces, and the list and policy are saved locally.

- **提示** (default): Warns once when ProxyCat is active on a saved campus Wi-Fi, with an option to disconnect.
- **自动断开**: Disables system proxies before stopping Mihomo, cancels in-progress bootstrap, and blocks starting ProxyCat on that Wi-Fi.
- Wi-Fi is checked during the five-second status refresh. Switching networks can therefore take a refresh cycle to detect; this is not a guarantee that no proxy traffic occurs during the transition.
- Leaving campus Wi-Fi does not restore the proxy. Reconnect manually when appropriate.
- Detection requires access to the Wi-Fi name. If it is unavailable, the menu shows that protection is inactive. Wired campus connections and VPNs managed by other applications are not controlled by this feature.

Run the macOS policy and lifecycle regressions with `bash scripts/test-campus-network.sh` and `bash scripts/test-shutdown.sh`. The tests use isolated fake helpers and do not change the machine's network configuration.

Run connection recovery regressions with `bash scripts/test-connection-recovery.sh`. These also use isolated fake helpers, including simulated controller failures and network transitions.

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
- TUN mode
- Advanced DNS configuration
- Complex rule editor
- Airport account/traffic parsing
- Apple Developer ID signing/notarization
- Windows/Linux GUI

## License

License TBD. All rights reserved until a license is chosen. See [LICENSE](LICENSE).

## Contributing

See [docs/superpowers/specs/2026-05-02-proxycat-design.md](docs/superpowers/specs/2026-05-02-proxycat-design.md) for design specifications.
