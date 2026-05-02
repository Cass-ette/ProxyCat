# ProxyCat Design Spec

## Goal

ProxyCat is an open-source, diagnostic-first macOS menu bar app for managing user-provided Mihomo/Clash subscriptions. It should provide ClashX-like menu bar convenience while making subscription format handling, real macOS system proxy state, and proxy failure diagnosis transparent.

## Product Positioning

ProxyCat is not a replacement for the Mihomo core and does not implement proxy protocols itself. It is a macOS manager around Mihomo focused on four differentiators:

1. **Simple:** keep the common workflow short: add subscription, generate config, start core, turn system proxy on/off, test connectivity.
2. **Transparent:** show what was detected and what is actually running: subscription format, node/group/rule counts, core status, controller status, real macOS proxy state, selected strategy group.
3. **Diagnostic:** provide a one-click diagnostic flow that identifies which layer failed and suggests the next action.
4. **Safe:** redact subscription tokens and node credentials from logs, UI summaries, and diagnostic reports.

The first public positioning is a GitHub-friendly technical-user app: users can clone and build locally, and future unsigned GitHub releases are acceptable. ProxyCat does not initially promise Developer ID signing, Apple notarization, automatic updates, or no-warning install UX.

## Target Users

ProxyCat targets macOS users who:

- use airport-style subscription URLs or Mihomo/Clash configs,
- want menu bar on/off convenience,
- do not want to manually edit YAML,
- need clear diagnosis when proxy access fails,
- are comfortable building an open-source app locally during early versions.

ProxyCat does not target first-version power users who need TUN mode, advanced DNS customization, rule editing, or full Clash Verge parity.

## Non-Goals For MVP

The MVP will not include:

- self-implemented Shadowsocks, Trojan, VMess, VLESS, Hysteria, or other proxy protocols,
- TUN mode,
- advanced DNS configuration,
- complex rule editor,
- airport account/traffic/package parsing,
- app auto-update,
- Apple Developer ID signing or notarization,
- Windows/Linux GUI,
- account system,
- iCloud sync,
- official public binary install experience.

## Technical Approach

ProxyCat uses three layers:

```text
ProxyCat.app
├── SwiftUI macOS menu bar UI
├── bundled Go helper: proxyctl
└── Mihomo core binary
```

### SwiftUI Menu Bar App

The SwiftUI layer owns native macOS presentation only:

- menu bar icon,
- status menu,
- user actions,
- lightweight preferences,
- calling `proxyctl` and rendering JSON responses.

It does not parse subscriptions, generate configs, manage system proxy settings directly, or speak Mihomo controller API directly in MVP.

### Go Helper: proxyctl

`proxyctl` owns operational logic:

- subscription download and detection,
- config generation and validation,
- Mihomo process lifecycle,
- macOS system proxy state read/write,
- Mihomo controller API calls,
- connection tests,
- diagnostic reports,
- redaction.

SwiftUI calls `proxyctl` commands and reads structured JSON. This keeps core behavior testable without UI.

### Mihomo Core

Mihomo performs all actual proxy work. ProxyCat manages its config and process lifecycle but does not fork or alter the core.

## Repository Layout

```text
ProxyCat/
├── README.md
├── LICENSE
├── docs/
│   ├── superpowers/specs/
│   ├── design.md
│   ├── security.md
│   └── troubleshooting.md
├── helper/
│   ├── go.mod
│   ├── cmd/proxyctl/main.go
│   └── internal/
│       ├── subscription/
│       ├── configgen/
│       ├── core/
│       ├── sysproxy/
│       ├── controller/
│       ├── diagnose/
│       └── redact/
├── app/
│   └── ProxyCat/
│       ├── ProxyCat.xcodeproj
│       └── ProxyCat/
│           ├── App.swift
│           ├── MenuBarController.swift
│           ├── StatusViewModel.swift
│           └── HelperClient.swift
├── scripts/
│   ├── build-helper.sh
│   ├── build-app.sh
│   └── adhoc-sign.sh
└── .github/workflows/build.yml
```

MVP implementation should create only the subset needed for the current task. Do not scaffold empty directories without use.

## Runtime Data Layout

ProxyCat stores user data under:

```text
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

Default generated Mihomo runtime settings:

```yaml
mixed-port: 7890
external-controller: 127.0.0.1:9090
allow-lan: false
mode: rule
log-level: info
```

## MVP CLI Surface

The first buildable milestone is `proxyctl`, before SwiftUI. Commands:

```bash
proxyctl subscription add <url>
proxyctl subscription update
proxyctl subscription status --json

proxyctl config generate
proxyctl config validate

proxyctl core start
proxyctl core stop
proxyctl core restart
proxyctl core status --json

proxyctl system-proxy on
proxyctl system-proxy off
proxyctl system-proxy status --json

proxyctl groups --json
proxyctl select <group> <node>

proxyctl test --json
proxyctl diagnose --json
```

CLI output must avoid printing raw subscription URLs with tokens or node secrets. Human output should be concise; machine-readable output should use explicit redacted fields.

## MVP Menu Bar Surface

The first SwiftUI menu should show:

```text
ProxyCat: On / Off

Status
  Core: Running / Stopped
  System Proxy: On / Off
  Mode: Rule
  Current: <group> -> <node>

Actions
  Turn System Proxy On
  Turn System Proxy Off
  Update Subscription
  Test Connection
  Restart Core

Diagnostics
  Run Diagnose
  Open Last Report

Open
  Config Folder
  Logs

Quit
```

The UI must display real macOS system proxy state from `proxyctl system-proxy status --json`, not an internal cached boolean. This is a core product requirement because ClashX-like UI state can disagree with actual system proxy state.

## Subscription Handling

MVP supports:

1. Clash/Mihomo YAML subscriptions,
2. base64 node URI lists,
3. plain node URI lists.

Processing flow:

```text
subscription URL
→ download with configurable User-Agent
→ detect format
→ normalize into Mihomo config
→ validate minimal config shape
→ backup previous config
→ write config.yaml
→ emit redacted summary
```

A redacted summary should include:

```text
Detected: Clash YAML | Base64 URI List | Plain URI List
Nodes: <count>
Groups: <count>
Rules: <count>
Subscription host: <host>
Token: <redacted/none>
```

Do not log raw node URLs, raw server credentials, or full subscription URLs.

## System Proxy Handling

MVP may use `networksetup` and `scutil --proxy` for implementation simplicity. Later versions may use SystemConfiguration APIs directly.

`proxyctl system-proxy status --json` should report at least:

```json
{
  "service": "Wi-Fi",
  "httpEnabled": true,
  "httpHost": "127.0.0.1",
  "httpPort": 7890,
  "httpsEnabled": true,
  "httpsHost": "127.0.0.1",
  "httpsPort": 7890,
  "socksEnabled": true,
  "socksHost": "127.0.0.1",
  "socksPort": 7890,
  "matchesProxyCat": true
}
```

The status command must read real system state, not only ProxyCat's last action.

## Diagnostics

`proxyctl diagnose --json` checks:

1. subscription exists,
2. subscription can be downloaded,
3. subscription format is recognized,
4. generated `config.yaml` exists,
5. config contains usable `proxies`, `proxy-groups`, and `rules`,
6. Mihomo binary exists,
7. Mihomo process is running,
8. mixed-port `7890` is listening,
9. controller `127.0.0.1:9090` is reachable,
10. proxy groups are available,
11. real macOS system proxy state is known,
12. Google `generate_204` through proxy succeeds,
13. GitHub through proxy succeeds,
14. outbound IP through proxy is available.

Each check returns:

```json
{
  "name": "system-proxy",
  "status": "pass|warn|fail",
  "message": "System proxy is off",
  "suggestedFix": "Turn System Proxy On"
}
```

The menu bar app presents this in human-readable form.

## Security And Redaction

ProxyCat must treat these values as sensitive:

- subscription query tokens,
- full subscription URLs,
- node passwords,
- VMess/VLESS UUIDs,
- Trojan passwords,
- Shadowsocks passwords,
- raw node URI strings.

Redaction rules:

- Replace query `token`, `key`, `password`, `pass`, `uuid`, `secret` values with `<redacted>`.
- Replace userinfo in URLs with `<redacted>@`.
- Avoid writing raw subscription response bodies to logs.
- Diagnostic reports default to shareable redacted output.

ProxyCat should document that users should still avoid pasting raw subscriptions into public issues.

## Error Handling

Errors should identify the failing layer:

- subscription download failure,
- unsupported subscription format,
- invalid generated config,
- missing Mihomo binary,
- core not running,
- controller unavailable,
- system proxy off or mismatched,
- selected node unreachable,
- external connectivity failure.

Each user-facing error should include a suggested next action when possible.

## Testing Strategy

### Go Helper Tests

Use Go unit tests for:

- subscription format detection,
- redaction of URLs and node secrets,
- config validation,
- diagnostic result aggregation,
- controller response parsing,
- system proxy output parsing using fixtures,
- command JSON output shape.

Avoid tests that require real network or real system proxy mutation by default. Integration tests that change system proxy must be explicit and opt-in.

### SwiftUI Tests

MVP SwiftUI testing can be light:

- `HelperClient` parsing of fixture JSON,
- status view model state transitions,
- no raw secret rendering in view model strings.

### Manual Validation

Manual MVP validation:

- add a known subscription,
- update and generate config,
- start Mihomo,
- turn system proxy on/off,
- verify `scutil --proxy`,
- run connection tests,
- run diagnose,
- confirm no raw token appears in logs or reports.

## Initial Milestones

### Milestone 1: `proxyctl` skeleton and redaction

- Create Go helper module.
- Implement config paths.
- Implement redaction package.
- Implement `proxyctl diagnose` skeleton with static checks.
- Add tests for redaction.

### Milestone 2: subscription import and config validation

- Add subscription storage.
- Download subscription.
- Detect Clash YAML vs base64/plain URI lists.
- Validate YAML summary.
- Write backups.

### Milestone 3: core and system proxy management

- Manage Mihomo process.
- Read controller config/proxy groups.
- Read/write system proxy.
- Add `test` command.

### Milestone 4: SwiftUI menu bar app

- Create menu bar app.
- Bundle `proxyctl`.
- Render status.
- Add actions for proxy on/off, update, test, diagnose.

### Milestone 5: GitHub polish

- README with positioning, screenshots, build instructions, security notes.
- GitHub Actions build check.
- Optional unsigned release instructions.

## Open Decisions

- App name is `ProxyCat` unless changed before implementation.
- GitHub owner is `Cass-ette`.
- Repository name is `ProxyCat`.
- License is not yet chosen.
- Mihomo binary distribution mode is not yet chosen: bundled binary, user-provided binary, or download-on-first-run.

## First Implementation Recommendation

Start with Milestone 1 only. It proves the repo, Go helper structure, redaction policy, command shape, and test discipline without touching system proxy or running a proxy core. This keeps the first PR small and safe.
