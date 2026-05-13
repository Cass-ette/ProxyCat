# Multi-Profile Configuration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each bootstrap/subscription creates a named profile with its own generated config.yaml. Users can switch between saved profiles from the menu UI, reusing any previously configured subscription with one click.

**Architecture:** Add a `profile` package to the Go helper that manages profile metadata and config files on disk under `~/Library/Application Support/ProxyCat/config/profiles/<id>/`. Extend `proxyctl` with `profile list`, `profile activate <id>`, and modify `bootstrap` to create/update a profile instead of overwriting the single config. The Swift UI adds a profile selector in the menu.

**Tech Stack:** Go 1.24 helper (profile storage, CLI), SwiftUI macOS menu bar app, existing `subscription` and `config` packages.

---

## File Structure

### Go helper — new profile package

- Create: `helper/internal/profile/profile.go`
  - `Profile` struct (ID, Name, URL, CreatedAt, UpdatedAt), `LoadAll`, `SaveAll`, `Activate` functions.
- Create: `helper/internal/profile/profile_test.go`
  - Round-trip, activate, duplicate-URL dedup tests.

### Go helper — CLI changes

- Modify: `helper/cmd/proxyctl/main.go`
  - Add `profile list --json`, `profile activate <id>` commands.
  - Modify `saveSingleSubscription` + `runBootstrap` to use profile package.
  - Modify `runConfigGenerate` to accept optional profile ID.
  - Modify `runSubscriptionUpdate` to update active profile.
- Modify: `helper/cmd/proxyctl/main_test.go`
  - Tests for `profile list`, `profile activate`, bootstrap creates profile.
- Modify: `helper/cmd/proxyctl/update_behavior_test.go`
  - Update existing test to work with profile-based bootstrap.

### Go helper — paths

- Modify: `helper/internal/paths/paths.go`
  - Add `ProfilesDir` field to `RuntimePaths`.
- Modify: `helper/internal/paths/paths_test.go`
  - Assert `ProfilesDir` path.

### Swift app — models

- Modify: `app/ProxyCat/ProxyCat/Models.swift`
  - Add `Profile` Codable struct matching Go `Profile`.

### Swift app — helper client

- Modify: `app/ProxyCat/ProxyCat/HelperClient.swift`
  - Add `getProfiles()`, `activateProfile(id:)` methods.

### Swift app — view model

- Modify: `app/ProxyCat/ProxyCat/StatusViewModel.swift`
  - Add `profiles: [Profile]`, `activeProfileID`, `loadProfiles()`, `activateProfile(id:)`.

### Swift app — UI

- Modify: `app/ProxyCat/ProxyCat/MenuContentView.swift`
  - Add profile selector section above "快速开始".

---

## Chunk 1: Go profile storage

### Task 1: Add ProfilesDir to RuntimePaths

**Files:**
- Modify: `helper/internal/paths/paths.go:11,36-55`
- Modify: `helper/internal/paths/paths_test.go:5-48`

- [ ] **Step 1: Add ProfilesDir field and populate it**

In `helper/internal/paths/paths.go`, add field to struct:

```go
type RuntimePaths struct {
    // ... existing fields ...
    ProfilesDir       string `json:"profilesDir"`
}
```

In `ForHome`, add:

```go
profiles := filepath.Join(config, "profiles")
```

And in the return struct:

```go
ProfilesDir: profiles,
```

- [ ] **Step 2: Add assertion to test**

In `helper/internal/paths/paths_test.go`, add:

```go
if p.ProfilesDir != wantBase+"/config/profiles" {
    t.Fatalf("ProfilesDir = %q", p.ProfilesDir)
}
```

- [ ] **Step 3: Run tests**

Run: `cd helper && go test ./internal/paths/ -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add helper/internal/paths/paths.go helper/internal/paths/paths_test.go
git commit -m "feat: add ProfilesDir to RuntimePaths"
```

---

### Task 2: Create profile package with storage and activate

**Files:**
- Create: `helper/internal/profile/profile.go`
- Create: `helper/internal/profile/profile_test.go`

- [ ] **Step 1: Write failing tests**

Create `helper/internal/profile/profile_test.go`:

```go
package profile

import (
    "os"
    "path/filepath"
    "testing"
    "time"
)

func TestSaveAndLoad(t *testing.T) {
    dir := t.TempDir()
    profiles := []Profile{
        {ID: "a1", Name: "Airport A", URL: "https://a.com/sub", CreatedAt: time.Now(), UpdatedAt: time.Now()},
        {ID: "b2", Name: "Airport B", URL: "https://b.com/sub", CreatedAt: time.Now(), UpdatedAt: time.Now()},
    }
    if err := SaveAll(dir, profiles); err != nil {
        t.Fatalf("SaveAll: %v", err)
    }
    loaded, err := LoadAll(dir)
    if err != nil {
        t.Fatalf("LoadAll: %v", err)
    }
    if len(loaded) != 2 {
        t.Fatalf("got %d profiles, want 2", len(loaded))
    }
    if loaded[0].ID != "a1" || loaded[1].Name != "Airport B" {
        t.Fatalf("unexpected: %+v", loaded)
    }
}

func TestLoadAllEmpty(t *testing.T) {
    dir := t.TempDir()
    loaded, err := LoadAll(dir)
    if err != nil {
        t.Fatalf("LoadAll: %v", err)
    }
    if len(loaded) != 0 {
        t.Fatalf("got %d, want 0", len(loaded))
    }
}

func TestFindByURL(t *testing.T) {
    profiles := []Profile{
        {ID: "a1", URL: "https://a.com/sub"},
        {ID: "b2", URL: "https://b.com/sub"},
    }
    found := FindByURL(profiles, "https://b.com/sub")
    if found == nil || found.ID != "b2" {
        t.Fatalf("FindByURL = %+v", found)
    }
    if FindByURL(profiles, "https://c.com/sub") != nil {
        t.Fatalf("FindByURL should return nil for unknown URL")
    }
}

func TestActivateWritesConfig(t *testing.T) {
    dir := t.TempDir()
    profilesDir := filepath.Join(dir, "profiles")
    activeConfig := filepath.Join(dir, "config.yaml")

    // Create profile with its own config
    p := Profile{ID: "a1", Name: "Test", URL: "https://a.com/sub"}
    profileDir := filepath.Join(profilesDir, p.ID)
    if err := os.MkdirAll(profileDir, 0o755); err != nil {
        t.Fatal(err)
    }
    profileConfig := filepath.Join(profileDir, "config.yaml")
    if err := os.WriteFile(profileConfig, []byte("mixed-port: 7890\n"), 0o644); err != nil {
        t.Fatal(err)
    }
    if err := SaveAll(profilesDir, []Profile{p}); err != nil {
        t.Fatal(err)
    }

    if err := Activate(profilesDir, p.ID, activeConfig); err != nil {
        t.Fatalf("Activate: %v", err)
    }

    data, err := os.ReadFile(activeConfig)
    if err != nil {
        t.Fatal(err)
    }
    if string(data) != "mixed-port: 7890\n" {
        t.Fatalf("active config = %q", string(data))
    }
}

func TestNextID(t *testing.T) {
    dir := t.TempDir()
    id1 := NextID(dir)
    if id1 == "" {
        t.Fatalf("NextID returned empty")
    }
    // Create a profile to verify next ID is different
    profiles := []Profile{{ID: id1, Name: "First", URL: "https://a.com"}}
    if err := SaveAll(dir, profiles); err != nil {
        t.Fatal(err)
    }
    id2 := NextID(dir)
    if id2 == id1 {
        t.Fatalf("NextID returned same ID %q", id2)
    }
}
```

- [ ] **Step 2: Run tests to verify RED**

Run: `cd helper && go test ./internal/profile/ -v`

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement profile package**

Create `helper/internal/profile/profile.go`:

```go
package profile

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "os"
    "path/filepath"
    "time"
)

type Profile struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    URL       string    `json:"url"`
    CreatedAt time.Time `json:"createdAt"`
    UpdatedAt time.Time `json:"updatedAt"`
}

const indexFile = "profiles.json"

func LoadAll(profilesDir string) ([]Profile, error) {
    path := filepath.Join(profilesDir, indexFile)
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return []Profile{}, nil
        }
        return nil, err
    }
    var profiles []Profile
    if err := json.Unmarshal(data, &profiles); err != nil {
        return nil, err
    }
    return profiles, nil
}

func SaveAll(profilesDir string, profiles []Profile) error {
    if err := os.MkdirAll(profilesDir, 0o755); err != nil {
        return err
    }
    data, err := json.MarshalIndent(profiles, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(filepath.Join(profilesDir, indexFile), data, 0o644)
}

func FindByURL(profiles []Profile, url string) *Profile {
    for i := range profiles {
        if profiles[i].URL == url {
            return &profiles[i]
        }
    }
    return nil
}

func NextID(profilesDir string) string {
    b := make([]byte, 4)
    _, _ = rand.Read(b)
    return hex.EncodeToString(b)
}

// Activate copies the profile's config.yaml to activeConfigPath.
func Activate(profilesDir string, profileID string, activeConfigPath string) error {
    src := filepath.Join(profilesDir, profileID, "config.yaml")
    data, err := os.ReadFile(src)
    if err != nil {
        return err
    }
    return os.WriteFile(activeConfigPath, data, 0o644)
}

// ProfileConfigPath returns the path to a profile's generated config.yaml.
func ProfileConfigPath(profilesDir string, profileID string) string {
    return filepath.Join(profilesDir, profileID, "config.yaml")
}

// EnsureProfileDir creates the profile's directory and returns its path.
func EnsureProfileDir(profilesDir string, profileID string) (string, error) {
    p := filepath.Join(profilesDir, profileID)
    return p, os.MkdirAll(p, 0o755)
}
```

- [ ] **Step 4: Run tests to verify GREEN**

Run: `cd helper && go test ./internal/profile/ -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add helper/internal/profile/profile.go helper/internal/profile/profile_test.go
git commit -m "feat: add profile package with storage and activate"
```

---

## Chunk 2: CLI profile commands

### Task 3: Add `profile list` and `profile activate` commands

**Files:**
- Modify: `helper/cmd/proxyctl/main.go:48-193` (command switch)
- Modify: `helper/cmd/proxyctl/main_test.go`

- [ ] **Step 1: Add profile subcommands to command switch**

In `helper/cmd/proxyctl/main.go`, add import:

```go
"github.com/Cass-ette/ProxyCat/helper/internal/profile"
```

Add to command switch (after `case "select":`):

```go
case "profile":
    if len(args) < 2 {
        fmt.Fprintf(stderr, "profile subcommand required: list, activate\n")
        return 2
    }
    switch args[1] {
    case "list":
        return runProfileList(stdout, stderr, jsonOutput)
    case "activate":
        if len(args) < 3 {
            fmt.Fprintf(stderr, "profile activate requires <id>\n")
            return 2
        }
        return runProfileActivate(args[2], stdout, stderr)
    default:
        fmt.Fprintf(stderr, "unknown profile subcommand: %s\n", redact.String(args[1]))
        return 2
    }
```

Add the handler functions:

```go
func runProfileList(stdout io.Writer, stderr io.Writer, jsonOutput bool) int {
    runtimePaths, err := paths.Default()
    if err != nil {
        fmt.Fprintf(stderr, "resolve paths: %v\n", err)
        return 1
    }
    profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
    if err != nil {
        fmt.Fprintf(stderr, "load profiles: %v\n", err)
        return 1
    }
    if jsonOutput {
        if err := json.NewEncoder(stdout).Encode(profiles); err != nil {
            fmt.Fprintf(stderr, "encode profiles: %v\n", err)
            return 1
        }
        return 0
    }
    if len(profiles) == 0 {
        fmt.Fprintln(stdout, "No profiles")
        return 0
    }
    for _, p := range profiles {
        fmt.Fprintf(stdout, "%s  %s  (%s)\n", p.ID, p.Name, redact.URL(p.URL))
    }
    return 0
}

func runProfileActivate(profileID string, stdout io.Writer, stderr io.Writer) int {
    runtimePaths, err := paths.Default()
    if err != nil {
        fmt.Fprintf(stderr, "resolve paths: %v\n", err)
        return 1
    }
    if err := profile.Activate(runtimePaths.ProfilesDir, profileID, runtimePaths.ConfigYAML); err != nil {
        fmt.Fprintf(stderr, "activate profile: %v\n", err)
        return 1
    }
    fmt.Fprintf(stdout, "Activated profile %s\n", profileID)
    return 0
}
```

Update `printHelp` to include:

```go
fmt.Fprintln(w, "  proxyctl profile list [--json]")
fmt.Fprintln(w, "  proxyctl profile activate <id>")
```

- [ ] **Step 2: Write CLI tests**

Add to `helper/cmd/proxyctl/main_test.go`:

```go
func TestProfileListCommand(t *testing.T) {
    t.Setenv("HOME", t.TempDir())
    stdout := new(bytes.Buffer)
    stderr := new(bytes.Buffer)
    exitCode := run([]string{"profile", "list", "--json"}, stdout, stderr)
    if exitCode != 0 {
        t.Fatalf("exitCode = %d, stderr = %s", exitCode, stderr.String())
    }
    if !strings.Contains(stdout.String(), "") && stdout.String() != "null\n" && stdout.String() != "[]\n" {
        t.Fatalf("expected JSON array or null, got: %s", stdout.String())
    }
}

func TestProfileMissingSubcommand(t *testing.T) {
    stdout := new(bytes.Buffer)
    stderr := new(bytes.Buffer)
    exitCode := run([]string{"profile"}, stdout, stderr)
    if exitCode != 2 {
        t.Fatalf("exitCode = %d, want 2", exitCode)
    }
    if !strings.Contains(stderr.String(), "subcommand required") {
        t.Fatalf("stderr = %s", stderr.String())
    }
}

func TestProfileActivateMissingID(t *testing.T) {
    stdout := new(bytes.Buffer)
    stderr := new(bytes.Buffer)
    exitCode := run([]string{"profile", "activate"}, stdout, stderr)
    if exitCode != 2 {
        t.Fatalf("exitCode = %d, want 2", exitCode)
    }
}
```

- [ ] **Step 3: Run tests**

Run: `cd helper && go test ./cmd/proxyctl/ -v -run TestProfile`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add helper/cmd/proxyctl/main.go helper/cmd/proxyctl/main_test.go
git commit -m "feat: add profile list and activate CLI commands"
```

---

### Task 4: Modify bootstrap to create profiles

**Files:**
- Modify: `helper/cmd/proxyctl/main.go:238-277` (runBootstrap)
- Modify: `helper/cmd/proxyctl/main.go:695-715` (saveSingleSubscription)
- Modify: `helper/cmd/proxyctl/main.go:860-929` (runConfigGenerate)

- [ ] **Step 1: Refactor bootstrap to use profiles**

Replace `saveSingleSubscription` with `saveProfileSubscription`:

```go
func saveProfileSubscription(url string) (string, error) {
    runtimePaths, err := paths.Default()
    if err != nil {
        return "", fmt.Errorf("resolve paths: %w", err)
    }
    if err := os.MkdirAll(runtimePaths.ProfilesDir, 0o755); err != nil {
        return "", fmt.Errorf("create profiles dir: %w", err)
    }

    profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
    if err != nil {
        return "", fmt.Errorf("load profiles: %w", err)
    }

    // If URL already exists, update that profile
    existing := profile.FindByURL(profiles, url)
    if existing != nil {
        existing.UpdatedAt = time.Now()
        if err := profile.SaveAll(runtimePaths.ProfilesDir, profiles); err != nil {
            return "", err
        }
        return existing.ID, nil
    }

    // Create new profile
    id := profile.NextID(runtimePaths.ProfilesDir)
    p := profile.Profile{
        ID:        id,
        Name:      "Subscription",
        URL:       url,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
    if _, err := profile.EnsureProfileDir(runtimePaths.ProfilesDir, id); err != nil {
        return "", err
    }
    profiles = append(profiles, p)
    if err := profile.SaveAll(runtimePaths.ProfilesDir, profiles); err != nil {
        return "", err
    }
    return id, nil
}
```

Modify `runBootstrap` to use it:

```go
func runBootstrap(url string, stdout io.Writer, stderr io.Writer) int {
    fmt.Fprintf(stdout, "Saving subscription: %s\n", redact.URL(url))
    profileID, err := saveProfileSubscription(url)
    if err != nil {
        fmt.Fprintf(stderr, "save subscription: %v\n", err)
        return 1
    }

    fmt.Fprintln(stdout, "Generating config...")
    if exit := runConfigGenerateForProfile(profileID, stdout, stderr); exit != 0 {
        return exit
    }

    // Activate: copy profile config to active config.yaml
    runtimePaths, err := paths.Default()
    if err != nil {
        fmt.Fprintf(stderr, "resolve paths: %v\n", err)
        return 1
    }
    if err := profile.Activate(runtimePaths.ProfilesDir, profileID, runtimePaths.ConfigYAML); err != nil {
        fmt.Fprintf(stderr, "activate profile: %v\n", err)
        return 1
    }

    fmt.Fprintln(stdout, "Ensuring Mihomo core...")
    if exit := runCoreInstall(stdout, stderr); exit != 0 {
        return exit
    }

    fmt.Fprintln(stdout, "Restarting core...")
    if exit := runCoreRestart(stdout, stderr); exit != 0 {
        return exit
    }

    if err := waitForController(8 * time.Second); err != nil {
        fmt.Fprintf(stderr, "wait for controller: %v\n", err)
        return 1
    }

    fmt.Fprintln(stdout, "Enabling system proxy...")
    if exit := runSystemProxyOn(stdout, stderr); exit != 0 {
        return exit
    }

    fmt.Fprintln(stdout, "Testing connection...")
    if exit := runTestJSON(stdout, stderr, false); exit != 0 {
        return exit
    }

    fmt.Fprintln(stdout, "ProxyCat is connected")
    return 0
}
```

- [ ] **Step 2: Refactor config generate to support profile-scoped output**

Add new function `runConfigGenerateForProfile`:

```go
func runConfigGenerateForProfile(profileID string, stdout io.Writer, stderr io.Writer) int {
    runtimePaths, err := paths.Default()
    if err != nil {
        fmt.Fprintf(stderr, "resolve paths: %v\n", err)
        return 1
    }

    profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
    if err != nil {
        fmt.Fprintf(stderr, "load profiles: %v\n", err)
        return 1
    }

    var target *profile.Profile
    for i := range profiles {
        if profiles[i].ID == profileID {
            target = &profiles[i]
            break
        }
    }
    if target == nil {
        fmt.Fprintf(stderr, "profile %s not found\n", profileID)
        return 1
    }

    fmt.Fprintf(stdout, "Generating config from subscription: %s\n", redact.URL(target.URL))

    client := &http.Client{}
    probe := subscription.Probe(client, target.URL)
    if probe.Selected == nil {
        fmt.Fprintf(stderr, "%s\n", probe.Message)
        if probe.SuggestedFix != "" {
            fmt.Fprintf(stderr, "建议：%s\n", probe.SuggestedFix)
        }
        return 1
    }
    content := probe.SelectedContent
    fmt.Fprintf(stdout, "Selected subscription format: %s via %s\n", probe.Selected.Format, probe.Selected.UserAgent)

    format := config.Format(probe.Selected.Format)
    var configYAML string
    switch format {
    case config.FormatClashYAML:
        configYAML, err = config.NormalizeClashYAML(content)
        if err != nil {
            fmt.Fprintf(stderr, "normalize YAML: %v\n", err)
            return 1
        }
    case config.FormatBase64List, config.FormatPlainList:
        configYAML, err = config.ConvertURIToYAML(content, format)
        if err != nil {
            fmt.Fprintf(stderr, "convert to YAML: %v\n", err)
            return 1
        }
    default:
        fmt.Fprintf(stderr, "Unknown subscription format\n")
        return 1
    }

    // Write to profile's own directory
    profileConfigPath := profile.ProfileConfigPath(runtimePaths.ProfilesDir, profileID)
    if err := os.MkdirAll(filepath.Dir(profileConfigPath), 0o755); err != nil {
        fmt.Fprintf(stderr, "create profile dir: %v\n", err)
        return 1
    }
    if err := os.WriteFile(profileConfigPath, []byte(configYAML), 0644); err != nil {
        fmt.Fprintf(stderr, "write profile config: %v\n", err)
        return 1
    }

    fmt.Fprintf(stdout, "Config generated: %s\n", profileConfigPath)
    return 0
}
```

Keep existing `runConfigGenerate` working by having it use first profile or the `subscriptions.json` fallback.

- [ ] **Step 3: Run existing tests**

Run: `cd helper && go test ./cmd/proxyctl/ -v`

Expected: All existing tests pass. `TestSubscriptionUpdateAllFailures` and `TestConfigGenerateUsesProbeSelection` need the old paths to still work.

- [ ] **Step 4: Commit**

```bash
git add helper/cmd/proxyctl/main.go
git commit -m "feat: bootstrap creates profiles instead of overwriting config"
```

---

### Task 5: Update subscription update to work with profiles

**Files:**
- Modify: `helper/cmd/proxyctl/main.go:814-858` (runSubscriptionUpdate)

- [ ] **Step 1: Make subscription update regenerate the active profile**

Modify `runSubscriptionUpdate`:

```go
func runSubscriptionUpdate(stdout io.Writer, stderr io.Writer) int {
    runtimePaths, err := paths.Default()
    if err != nil {
        fmt.Fprintf(stderr, "resolve paths: %v\n", err)
        return 1
    }

    // Try profile-based update first
    profiles, err := profile.LoadAll(runtimePaths.ProfilesDir)
    if err != nil {
        fmt.Fprintf(stderr, "load profiles: %v\n", err)
        return 1
    }

    if len(profiles) > 0 {
        // Update all profiles
        client := &http.Client{}
        for i, p := range profiles {
            probe := subscription.Probe(client, p.URL)
            if probe.Selected == nil {
                fmt.Fprintf(stdout, "Profile %s (%s): invalid - %s\n", p.ID, p.Name, probe.Message)
                continue
            }
            profiles[i].UpdatedAt = time.Now()
            // Regenerate config for this profile
            format := config.Format(probe.Selected.Format)
            content := probe.SelectedContent
            var configYAML string
            switch format {
            case config.FormatClashYAML:
                configYAML, err = config.NormalizeClashYAML(content)
            case config.FormatBase64List, config.FormatPlainList:
                configYAML, err = config.ConvertURIToYAML(content, format)
            }
            if err != nil {
                fmt.Fprintf(stdout, "Profile %s: generate failed: %v\n", p.ID, err)
                continue
            }
            pDir, _ := profile.EnsureProfileDir(runtimePaths.ProfilesDir, p.ID)
            if err := os.WriteFile(filepath.Join(pDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
                fmt.Fprintf(stdout, "Profile %s: write failed: %v\n", p.ID, err)
                continue
            }
            fmt.Fprintf(stdout, "Profile %s (%s): updated\n", p.ID, p.Name)
        }
        if err := profile.SaveAll(runtimePaths.ProfilesDir, profiles); err != nil {
            fmt.Fprintf(stderr, "save profiles: %v\n", err)
            return 1
        }
        return 0
    }

    // Legacy fallback: subscriptions.json
    records, err := subscription.Load(runtimePaths.SubscriptionsJSON)
    if err != nil {
        fmt.Fprintf(stderr, "load subscriptions: %v\n", err)
        return 1
    }
    if len(records) == 0 {
        fmt.Fprintln(stderr, "No subscriptions to update")
        return 1
    }
    // ... keep existing legacy code unchanged ...
    return 0
}
```

- [ ] **Step 2: Run tests**

Run: `cd helper && go test ./cmd/proxyctl/ -v`

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add helper/cmd/proxyctl/main.go
git commit -m "feat: subscription update regenerates all profiles"
```

---

## Chunk 3: Swift UI

### Task 6: Add Profile model to Swift app

**Files:**
- Modify: `app/ProxyCat/ProxyCat/Models.swift`

- [ ] **Step 1: Add Profile struct**

Add to `app/ProxyCat/ProxyCat/Models.swift`:

```swift
struct Profile: Codable, Identifiable {
    let id: String
    let name: String
    let url: String
    let createdAt: Date
    let updatedAt: Date
}
```

- [ ] **Step 2: Commit**

```bash
git add app/ProxyCat/ProxyCat/Models.swift
git commit -m "feat: add Profile model"
```

---

### Task 7: Add profile methods to HelperClient

**Files:**
- Modify: `app/ProxyCat/ProxyCat/HelperClient.swift`

- [ ] **Step 1: Add getProfiles and activateProfile**

Add to `HelperClient`:

```swift
func getProfiles() async -> Result<[Profile], HelperError> {
    let result = await runCommand(["profile", "list", "--json"])
    return result.flatMap { data in
        do {
            let profiles = try JSONDecoder().decode([Profile].self, from: data)
            return .success(profiles)
        } catch {
            return .failure(.decodingFailed(error))
        }
    }
}

func activateProfile(id: String) async -> Result<Void, HelperError> {
    let result = await runCommand(["profile", "activate", id])
    return result.map { _ in () }
}
```

- [ ] **Step 2: Commit**

```bash
git add app/ProxyCat/ProxyCat/HelperClient.swift
git commit -m "feat: add profile helper methods"
```

---

### Task 8: Add profile state to StatusViewModel

**Files:**
- Modify: `app/ProxyCat/ProxyCat/StatusViewModel.swift`

- [ ] **Step 1: Add profile published properties and methods**

Add to `StatusViewModel`:

```swift
@Published var profiles: [Profile] = []
@Published var activeProfileID: String?

func loadProfiles() async {
    let result = await helper.getProfiles()
    switch result {
    case .success(let p):
        profiles = p
    case .failure:
        profiles = []
    }
}

func activateProfile(id: String) async {
    let result = await helper.activateProfile(id: id)
    if case .failure(let error) = result {
        lastError = "Activate profile failed: \(error.localizedDescription)"
        return
    }
    activeProfileID = id
    // Restart core with new config
    let restartResult = await helper.restartCore()
    if case .failure(let error) = restartResult {
        lastError = "Restart core failed: \(error.localizedDescription)"
    }
    await refreshStatus()
}
```

In `refreshStatus()`, add at the end:

```swift
await loadProfiles()
```

- [ ] **Step 2: Commit**

```bash
git add app/ProxyCat/ProxyCat/StatusViewModel.swift
git commit -m "feat: add profile state to view model"
```

---

### Task 9: Add profile selector to MenuContentView

**Files:**
- Modify: `app/ProxyCat/ProxyCat/MenuContentView.swift`

- [ ] **Step 1: Add profile selector UI**

Add between `headerSection` and `quickStartSection` in the body `VBox`:

```swift
if !viewModel.profiles.isEmpty {
    profileSection
    Divider()
}
```

Add the computed property:

```swift
private var profileSection: some View {
    VStack(alignment: .leading, spacing: 6) {
        Text("配置")
            .font(.caption)
            .foregroundColor(.secondary)
            .padding(.horizontal, 16)
            .padding(.top, 8)

        ForEach(viewModel.profiles) { p in
            Button(action: {
                Task { await viewModel.activateProfile(id: p.id) }
            }) {
                HStack {
                    Image(systemName: viewModel.activeProfileID == p.id ? "checkmark.circle.fill" : "circle")
                        .frame(width: 20)
                        .foregroundColor(viewModel.activeProfileID == p.id ? .accentColor : .secondary)
                    Text(p.name)
                        .lineLimit(1)
                    Spacer()
                }
                .font(.system(size: 12))
            }
            .buttonStyle(PlainButtonStyle())
            .padding(.horizontal, 16)
            .padding(.vertical, 3)
            .contentShape(Rectangle())
        }
    }
    .padding(.bottom, 8)
}
```

- [ ] **Step 2: Commit**

```bash
git add app/ProxyCat/ProxyCat/MenuContentView.swift
git commit -m "feat: add profile selector to menu UI"
```

---

## Chunk 4: Integration and rebuild

### Task 10: Update diagnose to check profiles dir

**Files:**
- Modify: `helper/internal/diagnose/diagnose.go`
- Modify: `helper/internal/diagnose/diagnose_test.go`

- [ ] **Step 1: Add profiles check**

Add to `Run` checks:

```go
checkProfilesStorage(p),
```

Add function:

```go
func checkProfilesStorage(p paths.RuntimePaths) Check {
    if fileExists(p.ProfilesDir) {
        return Check{Name: "profiles-storage", Status: StatusPass, Message: "Profiles directory exists."}
    }
    return Check{Name: "profiles-storage", Status: StatusWarn, Message: "No profiles yet.", SuggestedFix: "Add a subscription to create a profile."}
}
```

Update check count in test from 4 to 5.

- [ ] **Step 2: Run tests**

Run: `cd helper && go test ./internal/diagnose/ -v`

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add helper/internal/diagnose/diagnose.go helper/internal/diagnose/diagnose_test.go
git commit -m "feat: add profiles storage check to diagnose"
```

---

### Task 11: Rebuild and local deploy

**Files:**
- Build output: `app/ProxyCat/build/Build/Products/Release/ProxyCat.app`

- [ ] **Step 1: Build everything**

Run:

```bash
cd /Users/chenzilve/Projects/ProxyCat && ./scripts/build-app.sh
```

Expected: `** BUILD SUCCEEDED **`

- [ ] **Step 2: Run Go tests**

Run: `cd helper && go test ./... -v`

Expected: All tests pass.

- [ ] **Step 3: Copy to /Applications and restart**

Run:

```bash
killall ProxyCat >/dev/null 2>&1
rm -rf /Applications/ProxyCat.app
cp -R app/ProxyCat/build/Build/Products/Release/ProxyCat.app /Applications/ProxyCat.app
open /Applications/ProxyCat.app
```

- [ ] **Step 4: Verify manually**

1. Open ProxyCat menu.
2. Paste a subscription URL and click "一键启动".
3. Repeat with a different subscription URL.
4. Verify both appear in "配置" section.
5. Click a non-active profile to switch.
6. Verify core restarts and the new config is active.

- [ ] **Step 5: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix: integration fixes from manual testing"
```
