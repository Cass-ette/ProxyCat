package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/subscription"
)

type TrafficSnapshot struct {
	ProfileID string                `json:"profileID"`
	Traffic   *subscription.Traffic `json:"traffic"`
	CheckedAt time.Time             `json:"checkedAt"`
}

type trafficCache struct {
	TrafficSnapshot
	SubscriptionHash string `json:"subscriptionHash"`
}

func subscriptionHash(rawURL string) string {
	hash := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(hash[:])
}

// Keep metadata separate from the profile index: background refresh must not
// overwrite a concurrent activation, rename, import, or subscription update.
func SaveTraffic(profilesDir string, p Profile, traffic *subscription.Traffic, checkedAt time.Time) (TrafficSnapshot, error) {
	snapshot := TrafficSnapshot{ProfileID: p.ID, Traffic: traffic, CheckedAt: checkedAt}
	data, err := json.Marshal(trafficCache{snapshot, subscriptionHash(p.URL)})
	if err != nil {
		return snapshot, err
	}
	file, err := os.CreateTemp(filepath.Join(profilesDir, p.ID), ".traffic-*")
	if err != nil {
		return snapshot, err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return snapshot, err
	}
	if err := file.Close(); err != nil {
		return snapshot, err
	}
	return snapshot, os.Rename(file.Name(), filepath.Join(profilesDir, p.ID, "traffic.json"))
}

func loadTraffic(profilesDir string, p *Profile) {
	p.Traffic, p.TrafficCheckedAt = nil, nil
	data, err := os.ReadFile(filepath.Join(profilesDir, p.ID, "traffic.json"))
	if err != nil {
		return
	}
	var cache trafficCache
	if json.Unmarshal(data, &cache) != nil || cache.ProfileID != p.ID || cache.SubscriptionHash != subscriptionHash(p.URL) {
		return
	}
	p.Traffic, p.TrafficCheckedAt = cache.Traffic, &cache.CheckedAt
}
