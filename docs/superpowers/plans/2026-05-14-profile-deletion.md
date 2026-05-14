# Profile Deletion Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add profile deletion to ProxyCat — backend Delete function, `profile delete` CLI subcommand, Swift client/viewmodel methods, and inline × button with NSAlert confirmation in the menu UI.

**Architecture:** Layered — Go profile package provides `Delete()`, proxyctl CLI exposes `profile delete <id>`, Swift HelperClient calls the CLI, ViewModel orchestrates refresh, MenuContentView adds the × button and confirmation alert. Active profiles cannot be deleted.

**Tech Stack:** Go 1.x (backend + CLI), SwiftUI + AppKit (menu bar UI), NSAlert (confirmation)

**Spec:** `docs/superpowers/specs/2026-05-14-profile-deletion-design.md`

---

## Chunk 1: Backend — profile.Delete + tests

### Task 1: Add Delete function to profile package

**Files:**
- Modify: `helper/internal/profile/profile.go` (add `Delete` after `Activate`)
- Modify: `helper/internal/profile/profile_test.go` (add 3 test functions)

- [ ] **Step 1: Write failing tests in profile_test.go**

Append after `TestNextID`:

```go
func TestDeleteRemovesProfile(t *testing.T) {
	dir := t.TempDir()
	profilesDir := filepath.Join(dir, "profiles")

	p1 := Profile{ID: "a1", Name: "Active", URL: "https://a.com/sub"}
	p2 := Profile{ID: "b2", Name: "Other", URL: "https://b.com/sub"}
	profileDir := filepath.Join(profilesDir, p2.ID)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "config.yaml"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveAll(profilesDir, []Profile{p1, p2}); err != nil {
		t.Fatal(err)
	}
	activeConfig := filepath.Join(dir, "config.yaml")
	if err := Activate(profilesDir, p1.ID, activeConfig); err != nil {
		t.Fatal(err)
	}

	if err := Delete(profilesDir, p2.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	loaded, err := LoadAll(profilesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != "a1" {
		t.Fatalf("got %+v, want only a1", loaded)
	}
	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Fatal("profile directory should be removed")
	}
}

func TestDeleteActiveProfileFails(t *testing.T) {
	dir := t.TempDir()
	profilesDir := filepath.Join(dir, "profiles")

	p := Profile{ID: "a1", Name: "Active", URL: "https://a.com/sub"}
	profileDir := filepath.Join(profilesDir, p.ID)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "config.yaml"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveAll(profilesDir, []Profile{p}); err != nil {
		t.Fatal(err)
	}
	activeConfig := filepath.Join(dir, "config.yaml")
	if err := Activate(profilesDir, p.ID, activeConfig); err != nil {
		t.Fatal(err)
	}

	err := Delete(profilesDir, p.ID)
	if err == nil {
		t.Fatal("should reject deleting active profile")
	}
}

func TestDeleteNonexistentFails(t *testing.T) {
	dir := t.TempDir()
	profilesDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveAll(profilesDir, []Profile{}); err != nil {
		t.Fatal(err)
	}
	err := Delete(profilesDir, "nonexistent")
	if err == nil {
		t.Fatal("should reject deleting nonexistent profile")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd helper && go test ./internal/profile/ -run 'TestDelete' -v`
Expected: FAIL — `Delete` undefined

- [ ] **Step 3: Implement Delete in profile.go**

Append after `EnsureProfileDir`:

```go
func Delete(profilesDir string, profileID string) error {
	profiles, err := LoadAll(profilesDir)
	if err != nil {
		return err
	}
	found := -1
	for i, p := range profiles {
		if p.ID == profileID {
			found = i
			break
		}
	}
	if found == -1 {
		return fmt.Errorf("profile not found: %s", profileID)
	}
	if profiles[found].Active {
		return fmt.Errorf("cannot delete active profile")
	}
	profiles = append(profiles[:found], profiles[found+1:]...)
	if err := SaveAll(profilesDir, profiles); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(profilesDir, profileID))
}
```

Before this step, add `"fmt"` to the import block in `profile.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd helper && go test ./internal/profile/ -v`
Expected: All tests PASS

- [ ] **Step 5: Run gofmt**

Run: `gofmt -w helper/internal/profile/profile.go helper/internal/profile/profile_test.go`

- [ ] **Step 6: Commit**

```bash
git add helper/internal/profile/profile.go helper/internal/profile/profile_test.go
git commit -m "feat: add profile.Delete with active-profile guard"
```

---

## Chunk 2: CLI — profile delete subcommand + test

### Task 2: Add profile delete to proxyctl CLI

**Files:**
- Modify: `helper/cmd/proxyctl/main.go:191-208` (add `delete` case)
- Modify: `helper/cmd/proxyctl/main.go:375-381` (update help text)
- Modify: `helper/cmd/proxyctl/main_cli_test.go` (add CLI test)

- [ ] **Step 1: Write failing CLI test**

Append in `main_cli_test.go` after `TestProfileListJSONRedactsSubscriptionURL`:

```go
func TestProfileDeleteCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PROXYCAT_HOME", tmp)

	// Create a non-active profile to delete
	profilesDir := filepath.Join(tmp, "config", "profiles", "a1b2c3d4")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "config.yaml"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profilesData, _ := json.Marshal([]profile.Profile{
		{ID: "a1b2c3d4", Name: "ToDelete", URL: "https://x.com/sub"},
	})
	if err := os.WriteFile(filepath.Join(tmp, "config", "profiles", "profiles.json"), profilesData, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"profile", "delete", "a1b2c3d4"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deleted") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	// Verify profile is gone from profiles.json
	data, err := os.ReadFile(filepath.Join(tmp, "config", "profiles", "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	var remaining []profile.Profile
	if err := json.Unmarshal(data, &remaining); err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(remaining))
	}
}
```

Add required imports if not present: `"encoding/json"`, `"path/filepath"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd helper && go test ./cmd/proxyctl/ -run TestProfileDeleteCommand -v`
Expected: FAIL — `delete` not handled

- [ ] **Step 3: Add delete case to command dispatch**

In `main.go`, update the profile subcommand dispatch block (line 191-208):

Change line 193 from:
```go
fmt.Fprintf(stderr, "profile subcommand required: list, activate\n")
```
to:
```go
fmt.Fprintf(stderr, "profile subcommand required: list, activate, delete\n")
```

Add after `case "activate":` block (after line 204) and before `default:`:
```go
		case "delete":
			if len(args) < 3 {
				fmt.Fprintf(stderr, "profile delete requires <id>\n")
				return 2
			}
			return runProfileDelete(args[2], stdout, stderr)
```

Update `printHelp` at line 380 — add after `proxyctl profile activate <id>`:
```go
	fmt.Fprintln(w, "  proxyctl profile delete <id>")
```

- [ ] **Step 4: Add runProfileDelete function**

Append after `runProfileActivate`:

```go
func runProfileDelete(profileID string, stdout io.Writer, stderr io.Writer) int {
	runtimePaths, err := paths.Default()
	if err != nil {
		fmt.Fprintf(stderr, "resolve paths: %v\n", err)
		return 1
	}
	if err := profile.Delete(runtimePaths.ProfilesDir, profileID); err != nil {
		fmt.Fprintf(stderr, "delete profile: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Deleted profile %s\n", profileID)
	return 0
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd helper && go test ./cmd/proxyctl/ -run 'TestProfileDelete|TestProfileList|TestProfileActivate' -v`
Expected: All PASS

- [ ] **Step 6: Run gofmt**

Run: `cd helper && gofmt -w cmd/proxyctl/main.go cmd/proxyctl/main_cli_test.go`

- [ ] **Step 7: Commit**

```bash
git add helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_cli_test.go
git commit -m "feat: add profile delete subcommand to proxyctl"
```

---

## Chunk 3: Swift layer — HelperClient + ViewModel

### Task 3: Add deleteProfile to Swift HelperClient and ViewModel

**Files:**
- Modify: `app/ProxyCat/ProxyCat/HelperClient.swift:340-343` (add method after `activateProfile`)
- Modify: `app/ProxyCat/ProxyCat/StatusViewModel.swift:252-264` (add method after `activateProfile`)

- [ ] **Step 1: Add deleteProfile to HelperClient.swift**

After `activateProfile(id:)` (after line 343), add:

```swift
    func deleteProfile(id: String) async -> Result<Void, HelperError> {
        let result = await runCommand(["profile", "delete", id])
        return result.map { _ in () }
    }
```

- [ ] **Step 2: Add deleteProfile to StatusViewModel.swift**

After `activateProfile(id:)` (after line 264), add:

```swift
    func deleteProfile(id: String) async {
        let result = await helper.deleteProfile(id: id)
        switch result {
        case .success:
            await loadProfiles()
        case .failure(let error):
            lastError = "Delete profile failed: \(error.localizedDescription)"
        }
    }
```

- [ ] **Step 3: Build to verify compilation**

Run: `xcodebuild -project app/ProxyCat/ProxyCat.xcodeproj -scheme ProxyCat -configuration Debug build 2>&1 | tail -5`
Expected: BUILD SUCCEEDED

- [ ] **Step 4: Commit**

```bash
git add app/ProxyCat/ProxyCat/HelperClient.swift app/ProxyCat/ProxyCat/StatusViewModel.swift
git commit -m "feat: add deleteProfile to Swift HelperClient and ViewModel"
```

---

## Chunk 4: Menu UI — × button + confirmation alert

### Task 4: Add delete × button with NSAlert to MenuContentView

**Files:**
- Modify: `app/ProxyCat/ProxyCat/MenuContentView.swift:106-124` (profile rows)

- [ ] **Step 1: Replace profile row content in profileSection**

Replace the `ForEach` block in `profileSection` (lines 106-124) with:

```swift
            ForEach(viewModel.profiles) { p in
                HStack {
                    Image(systemName: viewModel.activeProfileID == p.id ? "checkmark.circle.fill" : "circle")
                        .frame(width: 20)
                        .foregroundColor(viewModel.activeProfileID == p.id ? .accentColor : .secondary)
                    Button(action: {
                        Task { await viewModel.activateProfile(id: p.id) }
                    }) {
                        Text(p.name)
                            .lineLimit(1)
                    }
                    .buttonStyle(PlainButtonStyle())
                    Spacer()
                    Button(action: {
                        showDeleteConfirmation(for: p)
                    }) {
                        Image(systemName: "xmark")
                            .font(.system(size: 10, weight: .bold))
                            .foregroundColor(.secondary)
                    }
                    .buttonStyle(PlainButtonStyle())
                    .disabled(p.active)
                    .help(p.active ? "无法删除正在使用的配置" : "")
                }
                .font(.system(size: 12))
                .padding(.horizontal, 16)
                .padding(.vertical, 3)
                .contentShape(Rectangle())
            }
```

- [ ] **Step 2: Add showDeleteConfirmation method**

Add a method to `MenuContentView` (inside the struct, before `profileSection`):

```swift
    private func showDeleteConfirmation(for profile: Profile) {
        let alert = NSAlert()
        alert.messageText = "删除配置"
        alert.informativeText = "确定要删除「\(profile.name)」吗？此操作无法撤销。"
        alert.addButton(withTitle: "取消")
        alert.addButton(withTitle: "删除")
        alert.alertStyle = .critical
        if alert.runModal() == .alertSecondButtonReturn {
            Task { await viewModel.deleteProfile(id: profile.id) }
        }
    }
```

- [ ] **Step 3: Build to verify compilation**

Run: `xcodebuild -project app/ProxyCat/ProxyCat.xcodeproj -scheme ProxyCat -configuration Debug build 2>&1 | tail -5`
Expected: BUILD SUCCEEDED

- [ ] **Step 4: Commit**

```bash
git add app/ProxyCat/ProxyCat/MenuContentView.swift
git commit -m "feat: add delete × button with confirmation to profile list"
```

---

## Chunk 5: Integration — build, bundle, verify

### Task 5: Full build + bundled proxyctl update + verification

**Files:**
- Binary: `app/ProxyCat/ProxyCat/Resources/proxyctl` (updated bundle)

- [ ] **Step 1: Run all Go tests**

Run: `cd helper && go test ./...`
Expected: All PASS

- [ ] **Step 2: Run Release build**

Run: `./scripts/build-app.sh`
Expected: BUILD SUCCEEDED, new `proxyctl` binary in `app/ProxyCat/build/Build/Products/Release/`

- [ ] **Step 3: Install and verify**

Run:
```bash
killall ProxyCat >/dev/null 2>&1
rm -rf "/Applications/ProxyCat.app"
cp -R "/Users/chenzilve/Projects/ProxyCat/app/ProxyCat/build/Build/Products/Release/ProxyCat.app" "/Applications/"
open /Applications/ProxyCat.app
```

- [ ] **Step 4: Verify delete subcommand exists**

Run: `"/Applications/ProxyCat.app/Contents/Resources/proxyctl" profile delete`
Expected: exit 2, stderr mentions "profile delete requires \<id\>"

- [ ] **Step 5: Commit bundled proxyctl**

```bash
git add app/ProxyCat/ProxyCat/Resources/proxyctl
git commit -m "build: update bundled proxyctl with profile delete"
```
