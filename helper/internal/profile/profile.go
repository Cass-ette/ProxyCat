package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	persisted := make([]Profile, len(profiles))
	copy(persisted, profiles)
	for i := range persisted {
		persisted[i].Traffic, persisted[i].TrafficCheckedAt = nil, nil
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
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

func Activate(profilesDir string, profileID string, activeConfigPath string) error {
	src := filepath.Join(profilesDir, profileID, "config.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(activeConfigPath, data, 0o644); err != nil {
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
