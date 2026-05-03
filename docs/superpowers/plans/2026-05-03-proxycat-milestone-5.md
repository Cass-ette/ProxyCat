# ProxyCat Milestone 5 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create README documentation, GitHub Actions CI, and release process for GitHub-friendly distribution.

**Architecture:**
- README.md: Project overview with positioning, build instructions, security notes
- GitHub Actions: Build and test workflow for Go helper and Swift app
- Release documentation: Instructions for creating unsigned releases

**Tech Stack:** Markdown, GitHub Actions YAML, shell scripts

---

## File Structure

```
ProxyCat/
├── README.md                    # Main project documentation
├── LICENSE                      # Open source license
├── .github/
│   └── workflows/
│       └── build.yml            # CI build and test workflow
├── scripts/
│   └── adhoc-sign.sh            # Optional ad-hoc signing script
└── docs/
    └── release.md               # Release process documentation
```

---

## Task 1: Create README.md

**Files:**
- Create: `README.md`

**Design:**
- Positioning: diagnostic-first macOS menu bar app for Mihomo/Clash
- Build instructions: Go helper + Xcode app
- Screenshots placeholder (text description for now)
- Security notes: redaction policy, sensitive data handling
- Badges: Build status, Go version, Swift version

- [ ] **Step 1: Write README.md**

```markdown
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
```

- [ ] **Step 2: Commit README.md**

```bash
git add README.md
git commit -m "docs: add README with build instructions and security notes"
```

---

## Task 2: Create LICENSE File

**Files:**
- Create: `LICENSE`

**Design:**
- Use MIT License for open source friendliness
- Fill in copyright year and author

- [ ] **Step 1: Create LICENSE file**

```text
MIT License

Copyright (c) 2026 Cassette

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Commit LICENSE**

```bash
git add LICENSE
git commit -m "chore: add MIT license"
```

---

## Task 3: Create GitHub Actions CI Workflow

**Files:**
- Create: `.github/workflows/build.yml`
- Modify: `helper/go.mod` (if Go version needs pinning)

**Design:**
- Build and test Go helper on macOS
- Build Swift app with xcodebuild
- Run on push to main and pull requests
- Use macOS runner for native builds

- [ ] **Step 1: Create .github directory structure**

```bash
mkdir -p .github/workflows
```

- [ ] **Step 2: Create build.yml**

```yaml
name: Build

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  build-helper:
    name: Build Go Helper
    runs-on: macos-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '^1.21'

      - name: Build proxyctl
        working-directory: helper
        run: |
          go build -o proxyctl ./cmd/proxyctl

      - name: Run Go tests
        working-directory: helper
        run: |
          go test ./... -v

  build-app:
    name: Build Swift App
    runs-on: macos-latest
    needs: build-helper
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '^1.21'

      - name: Build proxyctl universal binary
        run: |
          cd helper
          GOOS=darwin GOARCH=amd64 go build -o ../app/ProxyCat/ProxyCat/Resources/proxyctl-amd64 ./cmd/proxyctl
          GOOS=darwin GOARCH=arm64 go build -o ../app/ProxyCat/ProxyCat/Resources/proxyctl-arm64 ./cmd/proxyctl
          cd ../app/ProxyCat/ProxyCat/Resources
          lipo -create proxyctl-amd64 proxyctl-arm64 -output proxyctl
          rm proxyctl-amd64 proxyctl-arm64
          chmod +x proxyctl

      - name: Build Xcode project
        run: |
          cd app/ProxyCat
          xcodebuild \
            -project ProxyCat.xcodeproj \
            -scheme ProxyCat \
            -configuration Release \
            -derivedDataPath build \
            CODE_SIGNING_ALLOWED=NO

      - name: Verify app bundle
        run: |
          ls -la app/ProxyCat/build/Build/Products/Release/ProxyCat.app
          file app/ProxyCat/build/Build/Products/Release/ProxyCat.app/Contents/Resources/proxyctl

      - name: Upload app artifact
        uses: actions/upload-artifact@v4
        with:
          name: ProxyCat-app
          path: app/ProxyCat/build/Build/Products/Release/ProxyCat.app
          retention-days: 7
```

- [ ] **Step 3: Commit CI workflow**

```bash
git add .github/workflows/build.yml
git commit -m "ci: add GitHub Actions build workflow"
```

---

## Task 4: Create Ad-Hoc Signing Script

**Files:**
- Create: `scripts/adhoc-sign.sh`

**Design:**
- Sign app bundle with ad-hoc signature for local distribution
- No Developer ID required (works on own machine)
- Sign both app bundle and bundled proxyctl

- [ ] **Step 1: Create adhoc-sign.sh**

```bash
#!/bin/bash
set -e

# scripts/adhoc-sign.sh
# Ad-hoc sign the ProxyCat app for local use (no Developer ID required)
# Usage: ./scripts/adhoc-sign.sh [path-to-app]

APP_PATH="${1:-app/ProxyCat/build/Build/Products/Release/ProxyCat.app}"

if [ ! -d "$APP_PATH" ]; then
    echo "Error: App not found at $APP_PATH"
    echo "Build first with: ./scripts/build-app.sh"
    exit 1
fi

echo "Ad-hoc signing ProxyCat..."

# Sign the bundled proxyctl binary
echo "Signing proxyctl..."
codesign --sign - --force --preserve-metadata=entitlements \
    "$APP_PATH/Contents/Resources/proxyctl"

# Sign the app bundle
echo "Signing ProxyCat.app..."
codesign --sign - --force --deep --preserve-metadata=entitlements \
    "$APP_PATH"

# Verify signature
echo "Verifying signature..."
codesign --verify --verbose "$APP_PATH"

echo "Done. App is ad-hoc signed and ready for local use."
echo "Note: Ad-hoc signed apps only work on this Mac."
```

- [ ] **Step 2: Make script executable**

```bash
chmod +x scripts/adhoc-sign.sh
```

- [ ] **Step 3: Commit signing script**

```bash
git add scripts/adhoc-sign.sh
git commit -m "chore: add ad-hoc signing script for local distribution"
```

---

## Task 5: Create Release Documentation

**Files:**
- Create: `docs/release.md`

**Design:**
- Document how to create an unsigned release
- Manual release steps for GitHub
- Instructions for end users
- Known limitations (unsigned app warnings)

- [ ] **Step 1: Create release.md**

```markdown
# Release Process

This document describes how to create a ProxyCat release for GitHub.

## Creating a Release (Unsigned)

ProxyCat does not use Apple Developer ID signing in its initial releases. Users build locally or accept the unsigned app warning.

### Prerequisites

- macOS with Go 1.21+ and Xcode 13+
- GitHub push access to Cass-ette/ProxyCat

### Steps

1. **Update version references**
   - Update `CFBundleShortVersionString` in `app/ProxyCat/ProxyCat/Info.plist`
   - Update version badge in `README.md` if needed
   - Commit: `git commit -m "chore: bump version to X.Y.Z"`

2. **Build the app**
   ```bash
   ./scripts/build-app.sh
   ```

3. **Ad-hoc sign for verification**
   ```bash
   ./scripts/adhoc-sign.sh
   ```

4. **Create a zip archive**
   ```bash
   cd app/ProxyCat/build/Build/Products/Release
   zip -r ProxyCat-X.Y.Z.zip ProxyCat.app
   ```

5. **Create GitHub release**
   - Go to https://github.com/Cass-ette/ProxyCat/releases
   - Click "Draft a new release"
   - Choose or create tag `vX.Y.Z`
   - Title: `ProxyCat X.Y.Z`
   - Description: Summarize changes since last release
   - Attach `ProxyCat-X.Y.Z.zip`
   - Publish release

6. **Verify the CI passed**
   - Check GitHub Actions build status on main branch

## User Installation (Unsigned App)

Since ProxyCat is unsigned, macOS will show a security warning on first launch.

### Installation Steps

1. Download `ProxyCat-X.Y.Z.zip` from GitHub Releases
2. Extract the zip file
3. Move `ProxyCat.app` to `/Applications` or `~/Applications`
4. **First launch**:
   - Right-click ProxyCat.app and select "Open"
   - Click "Open" in the security dialog
   - Or: System Settings → Privacy & Security → Security → "Open Anyway"

### Known Limitations

- Ad-hoc signed apps only work on the Mac that built them
- Unsigned apps show security warnings on first launch
- No automatic updates (check GitHub releases manually)
- Not distributed via App Store

## Building From Source

Users can also build locally to avoid unsigned app warnings:

```bash
git clone https://github.com/Cass-ette/ProxyCat.git
cd ProxyCat
./scripts/build-app.sh
./scripts/adhoc-sign.sh
open app/ProxyCat/build/Build/Products/Release/ProxyCat.app
```

Building locally produces an app signed for that specific Mac, which runs without warnings.

## Future: Signed Releases

If ProxyCat obtains an Apple Developer ID in the future:

1. Build with proper signing:
   ```bash
   xcodebuild -project ProxyCat.xcodeproj \
     -scheme ProxyCat \
     -configuration Release \
     CODE_SIGN_IDENTITY="Developer ID Application: ..."
   ```

2. Notarize with Apple:
   ```bash
   xcrun notarytool submit ProxyCat.zip --apple-id ... --team-id ... --wait
   ```

3. Staple notarization:
   ```bash
   xcrun stapler staple ProxyCat.app
   ```
```

- [ ] **Step 2: Commit release documentation**

```bash
git add docs/release.md
git commit -m "docs: add release process documentation"
```

---

## Task 6: Update .gitignore for Xcode

**Files:**
- Modify: `.gitignore`

**Design:**
- Add standard Xcode/macOS entries
- Keep existing entries

- [ ] **Step 1: Update .gitignore**

```gitignore
.worktrees/
app/ProxyCat/build/
DerivedData/

# Xcode
xcuserdata/
*.xcscmblueprint
*.xccheckout
*.xcuserstate
project.xcworkspace/

# macOS
.DS_Store

# Build artifacts
*.app.dSYM.zip
*.app.dSYM
```

- [ ] **Step 2: Commit .gitignore update**

```bash
git add .gitignore
git commit -m "chore: add Xcode and macOS .gitignore entries"
```

---

## Task 7: Verify CI Works

**Files:**
- Test: `.github/workflows/build.yml`

**Design:**
- Push to GitHub and verify CI passes
- Check that artifacts are uploaded

- [ ] **Step 1: Push branch to GitHub**

```bash
git push -u origin feat/milestone-5
```

- [ ] **Step 2: Verify CI status**

Check GitHub Actions tab for build status. Wait for both jobs to pass:
- Build Go Helper
- Build Swift App

- [ ] **Step 3: Download and verify artifact**

Download the `ProxyCat-app` artifact from GitHub Actions and verify it contains a working app bundle.

---

## Task 8: Create Pull Request

**Files:**
- All files in feat/milestone-5 branch

**Design:**
- Summarize all changes for GitHub polish
- Link to design spec

- [ ] **Step 1: Push and create PR**

```bash
git push -u origin feat/milestone-5
```

Create PR with title "chore: GitHub polish (Milestone 5)" and description:

```markdown
## Summary

Milestone 5 adds GitHub-friendly documentation and CI for ProxyCat.

## Changes

- Add README.md with positioning, build instructions, security notes
- Add MIT LICENSE
- Add GitHub Actions CI workflow (Go helper + Swift app)
- Add ad-hoc signing script for local distribution
- Add release process documentation
- Update .gitignore with Xcode/macOS entries

## Verification

- [ ] CI workflow passes on this PR
- [ ] README renders correctly
- [ ] Build instructions are accurate

🤖 Generated with [Claude Code](https://claude.ai/code)
```

- [ ] **Step 2: Merge when CI passes**

After CI passes, merge to main.

---

**Implementation Notes:**

1. No CLAUDE.md files in this repo - follow general best practices
2. README should be informative but concise
3. CI should catch build failures before they reach main
4. Release process should be documented for future maintainers
5. MIT license is permissive and GitHub-friendly
