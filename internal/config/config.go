// Package config loads alex-compatible configuration (.alexrc, .alexrc.json).
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config mirrors alex's .alexrc keys.
type Config struct {
	Allow             []string `json:"allow"`
	Deny              []string `json:"deny"`
	ProfanitySureness int      `json:"profanitySureness"`
	NoBinary          bool     `json:"noBinary"`
}

var names = []string{".alexrc", ".alexrc.json"}

// abs resolves a path. Tests replace it to reach the error branch.
var abs = filepath.Abs

// Find walks up from dir looking for a config file. It returns a zero
// Config when none exists.
func Find(dir string) (Config, error) {
	dir, err := abs(dir)
	if err != nil {
		return Config{}, err
	}
	for {
		for _, n := range names {
			b, err := os.ReadFile(filepath.Join(dir, n))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return Config{}, err
			}
			var c Config
			if err := json.Unmarshal(b, &c); err != nil {
				return Config{}, err
			}
			return c, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Config{}, nil
		}
		dir = parent
	}
}
