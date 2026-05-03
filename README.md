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

## Screenshots

The menu bar app shows:
- Current proxy status (On/Off)
- Core status and system proxy state
- Quick actions: toggle proxy, update subscription, test connection, restart core
- Diagnostics and logs access

## Building

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
- App auto-update
- Apple Developer ID signing/notarization
- Windows/Linux GUI

## License

[License TBD - see LICENSE file]

## Contributing

See [docs/superpowers/specs/2026-05-02-proxycat-design.md](docs/superpowers/specs/2026-05-02-proxycat-design.md) for design specifications.
