package maintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Status struct {
	ConfigApplicationState   string            `json:"configApplicationState,omitempty"`
	ConfigApplicationMessage string            `json:"configApplicationMessage,omitempty"`
	ConfigSHA256             string            `json:"configSHA256,omitempty"`
	ObservedAt               string            `json:"observedAt,omitempty"`
	GeoAutoUpdate            bool              `json:"geoAutoUpdate"`
	GeoFiles                 int               `json:"geoFiles"`
	SubscriptionLastAttempt  string            `json:"subscriptionLastAttempt,omitempty"`
	SubscriptionLastSuccess  string            `json:"subscriptionLastSuccess,omitempty"`
	SubscriptionLastError    string            `json:"subscriptionLastError,omitempty"`
	SubscriptionInProgress   bool              `json:"subscriptionInProgress,omitempty"`
	GeoLastObservedChange    string            `json:"geoLastObservedChange,omitempty"`
	GeoLastError             string            `json:"geoLastError,omitempty"`
	GeoObservedFiles         map[string]string `json:"geoObservedFiles,omitempty"`
	LegacyGeoLastSuccess     string            `json:"geoLastSuccess,omitempty"`
}

func Load(path string) (Status, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, err
	}
	// The old field did not distinguish an observed file change from a verified
	// update, so do not carry its timestamp into the more specific status.
	status.LegacyGeoLastSuccess = ""
	return status, nil
}

func Save(path string, status Status) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".maintenance-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func MarkAttempt(status Status, at time.Time) Status {
	status.SubscriptionLastAttempt = at.UTC().Format(time.RFC3339)
	status.SubscriptionLastError = ""
	status.SubscriptionInProgress = true
	return status
}

func MarkSuccess(status Status, at time.Time) Status {
	status.SubscriptionLastSuccess = at.UTC().Format(time.RFC3339)
	status.SubscriptionLastError = ""
	status.SubscriptionInProgress = false
	return status
}

func MarkFailure(status Status, message string) Status {
	status.SubscriptionLastError = message
	status.SubscriptionInProgress = false
	return status
}
