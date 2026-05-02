package subscription

import (
	"encoding/json"
	"os"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
)

// Record represents a subscription entry with URL, display name, and last update time.
type Record struct {
	URL        string    `json:"url"`
	Name       string    `json:"name"`
	LastUpdate time.Time `json:"lastUpdate"`
}

// MarshalJSON returns a JSON representation with the URL redacted for display/logging.
// For persistence that preserves the actual URL, use Save which bypasses this method.
func (r Record) MarshalJSON() ([]byte, error) {
	type alias Record
	return json.Marshal(&struct {
		URL string `json:"url"`
		alias
	}{
		URL:   redact.URL(r.URL),
		alias: (alias)(r),
	})
}

// Load reads subscription records from the JSON file at path.
// Returns empty slice if the file does not exist.
func Load(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}

	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// Save writes subscription records to the JSON file at path.
// Preserves actual URLs on disk by bypassing Record.MarshalJSON redaction.
func Save(path string, records []Record) error {
	// Use raw type to bypass MarshalJSON and preserve actual URLs.
	type raw struct {
		URL        string    `json:"url"`
		Name       string    `json:"name"`
		LastUpdate time.Time `json:"lastUpdate"`
	}

	rawRecords := make([]raw, len(records))
	for i, r := range records {
		rawRecords[i] = raw(r)
	}

	data, err := json.MarshalIndent(rawRecords, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
