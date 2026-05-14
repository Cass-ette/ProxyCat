# Profile Deletion Design

## Summary

Add profile deletion to ProxyCat: a `profile delete` CLI command, supporting Swift layer, and an inline x button with confirmation alert in the menu bar UI. Active profiles cannot be deleted.

## Requirements

- User can delete a non-active profile from the menu bar UI
- Deletion requires confirmation via NSAlert
- Active profiles cannot be deleted (button disabled with tooltip)
- Backend removes profile from `profiles.json` and deletes `<profilesDir>/<id>/` directory
- CLI exposes `proxyctl profile delete <id>` for programmatic use

## Design

### 1. Profile Package (`helper/internal/profile/profile.go`)

Add `Delete(profilesDir, profileID, activeConfigPath string) error`:

1. Load all profiles via `LoadAll`
2. Find target profile by ID; return error if not found
3. If profile is active (`Active == true`), return error: "cannot delete active profile"
4. Remove entry from slice, call `SaveAll`
5. `os.RemoveAll(<profilesDir>/<id>/)` to delete config directory

### 2. Proxyctl CLI (`helper/cmd/proxyctl/main.go`)

Add `profile delete <id>` subcommand in the inline `switch args[1]` block within the `case "profile":` handler in `run()` (around line 191):

- Profile not found: exit 1, stderr "profile not found"
- Profile is active: exit 1, stderr "cannot delete active profile"
- Success: exit 0, stdout "profile <name> deleted"

Update the inline error message at line 193 to: `"profile subcommand required: list, activate, delete\n"`.
Update `printHelp()` to include `proxyctl profile delete <id>`.

### 3. Tests

- `TestProfileDeleteRemovesProfile`: create two profiles, activate one, delete the other, verify profiles.json and directory
- `TestProfileDeleteActiveProfileFails`: attempt to delete active profile, expect error
- `TestProfileDeleteNonexistentFails`: attempt to delete nonexistent ID, expect error

### 4. Swift HelperClient (`app/ProxyCat/ProxyCat/HelperClient.swift`)

Add `deleteProfile(id:) async -> Result<Void, HelperError>`:

- Runs `["profile", "delete", id]`
- Maps non-zero exit code to `HelperError`

### 5. Swift ViewModel (`app/ProxyCat/ProxyCat/StatusViewModel.swift`)

Add `deleteProfile(id:) async`:

- Calls `helper.deleteProfile(id:)`
- On success: calls `loadProfiles()` to refresh list
- On failure: shows error message via existing alert mechanism

### 6. Menu UI (`app/ProxyCat/ProxyCat/MenuContentView.swift`)

Modify `profileSection` rows to include an inline x button:

```
HStack {
  Image(systemName: active ? "checkmark.circle.fill" : "circle")
  Text(profile.name)
  Spacer()
  Button(action: { showDeleteConfirmation(for: profile) }) {
    Image(systemName: "xmark")
      .font(.system(size: 10, weight: .bold))
      .foregroundColor(.secondary)
  }
  .buttonStyle(.plain)
  .disabled(profile.active)
  .help(profile.active ? "Cannot delete the active profile" : "")
}
```

Confirmation alert on x click:

- Title: "Delete Profile"
- Message: "Are you sure you want to delete \"{name}\"? This cannot be undone."
- Buttons: "Cancel" + "Delete" (destructive)
- On confirm: call `viewModel.deleteProfile(id:)`

## Out of Scope

- Batch deletion
- Undo / recover deleted profiles
- Drag-to-reorder profiles
- Profile rename
