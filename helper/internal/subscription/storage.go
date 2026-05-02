package subscription

import (
	"encoding/json"
	"os"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/redact"
)

type Record struct {
	URL        string    `json:"url"`
	Name       string    `json:"name"`
	LastUpdate time.Time `json:"lastUpdate"`
}

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

func Save(path string, records []Record) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
