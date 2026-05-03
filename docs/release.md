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
