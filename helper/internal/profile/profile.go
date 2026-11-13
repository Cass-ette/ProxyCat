package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/subscription"
)

type Profile struct {
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	URL              string                `json:"url"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
	Active           bool                  `json:"active"`
	Traffic          *subscription.Traffic `json:"traffic,omitempty"`
	TrafficCheckedAt *time.Time            `json:"trafficCheckedAt,omitempty"`
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
	for i := range profiles {
		loadTraffic(profilesDir, &profiles[i])
	}
	return profiles, nil
}

func SaveAll(profilesDir string, profiles []Profile) error {
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		return err
	}
	data, err := EncodeAll(profiles)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(profilesDir, indexFile), data, 0o644)
}

func EncodeAll(profiles []Profile) ([]byte, error) {
	persisted := make([]Profile, len(profiles))
	copy(persisted, profiles)
	for i := range persisted {
		persisted[i].Traffic, persisted[i].TrafficCheckedAt = nil, nil
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return nil, err
	}
	return data, nil
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

func Activate(profilesDir string, profileID string, activeConfigPath string) error {
	src := filepath.Join(profilesDir, profileID, "config.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := AtomicWrite(activeConfigPath, data, 0o644); err != nil {
		return err
	}
	profiles, err := LoadAll(profilesDir)
	if err != nil {
		return err
	}
	for i := range profiles {
		profiles[i].Active = profiles[i].ID == profileID
	}
	return SaveAll(profilesDir, profiles)
}

func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	return AtomicWriteSet(map[string][]byte{path: data}, mode)
}

func AtomicWriteSet(updates map[string][]byte, mode os.FileMode) error {
	type stagedFile struct {
		path, temporary, backup string
		committed               bool
	}
	paths := make([]string, 0, len(updates))
	for path := range updates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	staged := make([]stagedFile, 0, len(paths))
	cleanup := func() {
		for _, item := range staged {
			if item.temporary != "" {
				_ = os.Remove(item.temporary)
			}
			if item.backup != "" {
				_ = os.Remove(item.backup)
			}
		}
	}
	rollback := func() {
		for i := len(staged) - 1; i >= 0; i-- {
			item := staged[i]
			if item.committed {
				_ = os.Remove(item.path)
			}
			if item.backup != "" {
				_ = os.Rename(item.backup, item.path)
			}
		}
		cleanup()
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			cleanup()
			return err
		}
		if info, err := os.Lstat(path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				cleanup()
				return fmt.Errorf("refusing to replace non-regular config file")
			}
		} else if !os.IsNotExist(err) {
			cleanup()
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(path), ".config-update-*")
		if err != nil {
			cleanup()
			return err
		}
		item := stagedFile{path: path, temporary: tmp.Name()}
		staged = append(staged, item)
		if err := tmp.Chmod(mode); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
		if _, err := tmp.Write(updates[path]); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			cleanup()
			return err
		}
		if err := tmp.Close(); err != nil {
			cleanup()
			return err
		}
	}
	for i := range staged {
		item := &staged[i]
		if _, err := os.Lstat(item.path); err == nil {
			backup, err := os.CreateTemp(filepath.Dir(item.path), ".config-backup-*")
			if err != nil {
				rollback()
				return err
			}
			item.backup = backup.Name()
			if err := backup.Close(); err != nil {
				rollback()
				return err
			}
			if err := os.Remove(item.backup); err != nil {
				rollback()
				return err
			}
			if err := os.Rename(item.path, item.backup); err != nil {
				item.backup = ""
				rollback()
				return err
			}
		}
		if err := os.Rename(item.temporary, item.path); err != nil {
			rollback()
			return err
		}
		item.temporary = ""
		item.committed = true
	}
	cleanup()
	return nil
}

func ProfileConfigPath(profilesDir string, profileID string) string {
	return filepath.Join(profilesDir, profileID, "config.yaml")
}

func EnsureProfileDir(profilesDir string, profileID string) (string, error) {
	p := filepath.Join(profilesDir, profileID)
	return p, os.MkdirAll(p, 0o755)
}

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
