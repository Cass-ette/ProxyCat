# Release Process

ProxyCat ships a DMG for drag-to-Applications installation and in-app updates. Developer ID signing and notarization are not configured yet. Starting with 0.12.1, the updater prefers a version-matched DMG and checksum, with fallback to older installer ZIP releases.

## Build and package

Prerequisites: macOS, Go 1.21+, Xcode 13+, Python 3.10+ and GitHub push access. The DMG tools are pinned in `scripts/dmg/requirements.txt` and installed into an isolated environment under the ignored build directory. The first DMG build needs network access to download them.

1. Update `CFBundleShortVersionString` in `app/ProxyCat/ProxyCat/Info.plist` and both `MARKETING_VERSION` values in the Xcode project.
2. Run the regression checks:
   ```bash
   bash scripts/test-shutdown.sh
   bash scripts/test-campus-network.sh
   bash scripts/test-connection-recovery.sh
   bash scripts/package-unsigned_test.sh
   ```
3. Build the universal app and all distribution files:
   ```bash
   bash scripts/package-unsigned.sh
   ```
   This restores the source helper binary after building and produces:
   - `dist/ProxyCat-X.Y.Z.dmg`
   - `dist/ProxyCat-X.Y.Z.dmg.sha256`
   - `dist/ProxyCat-X.Y.Z.zip`
   - `dist/ProxyCat-X.Y.Z-installer.zip`
   - `dist/ProxyCat-X.Y.Z-installer.zip.sha256`
4. Mount the DMG without opening Finder and verify its contents:
   ```bash
   bash scripts/test-dmg.sh dist/ProxyCat-X.Y.Z.dmg
   ```
   This checks the image checksum, Applications link, Finder layout files, app version, Intel/Apple Silicon binaries and code signatures, then unmounts the image. It does not install or launch the app.
5. Commit the source changes, push, and wait for GitHub Actions to pass. Push a `vX.Y.Z` tag at the tested commit. The release workflow builds and verifies the app and DMG, then publishes the DMG and its checksum to GitHub. It also publishes the installer ZIP and checksum so versions up to 0.12.0 can still update, even when skipping releases. The plain app ZIP remains a local artifact; new installations should use the DMG.
6. Verify the published asset checksums match the local files and that the release points at the intended tag.

To add a DMG to an existing release without rebuilding or changing its ZIPs:

```bash
bash scripts/package-dmg.sh /path/to/ProxyCat.app
bash scripts/test-dmg.sh dist/ProxyCat-X.Y.Z.dmg
```

Use the app from that release's ZIP. DMG packaging signs a staging copy and leaves the original app unchanged. It uses `dmgbuild` to write Finder metadata without scripting Finder, so it also runs on macOS CI. To regenerate the included background image, run `swift scripts/dmg/background.swift`.

## User installation

1. Download the DMG, open it, and drag ProxyCat to Applications.
2. Quit an older running version before replacing it.
3. Open ProxyCat from Applications; its cat icon appears in the menu bar.
4. If macOS blocks the first launch, use **System Settings → Privacy & Security → Open Anyway**.

The DMG app is ad-hoc signed for code integrity. Ad-hoc signatures can be verified on other Macs, but do not establish Developer ID trust or replace Apple notarization. A DMG does not bypass Gatekeeper.

Dragging the app installs only ProxyCat.app. The optional legacy installer ZIP additionally offers a `proxycat` command in `/usr/local/bin`; in-app updates remain available with either installation method. In-app updates verify SHA-256 before mounting a DMG read-only, validate the bundle and signature, copy the app into staging, unmount, and replace the old app with a backup.

## Future signed releases

For first launch without the unverified-developer prompt, sign the app and nested executables with a Developer ID Application identity, enable the hardened runtime, notarize the final distribution with `xcrun notarytool`, and staple the ticket before publishing.
