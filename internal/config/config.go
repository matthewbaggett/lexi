// Package config loads alex-compatible configuration: .alexrc, .alexrc.json,
// .alexrc.yml, .alexrc.yaml and the `alex` field of package.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config mirrors alex's .alexrc keys.
type Config struct {
	Allow             []string `json:"allow"`
	Deny              []string `json:"deny"`
	ProfanitySureness int      `json:"profanitySureness"`
	NoBinary          bool     `json:"noBinary"`
}

// names lists the config files of one directory in the order unified-engine
// looks for them. JavaScript files are listed so that lexi can say it does
// not read them.
var names = []string{
	".alexrc", ".alexrc.json", ".alexrc.cjs", ".alexrc.mjs", ".alexrc.js",
	".alexrc.yaml", ".alexrc.yml", "package.json",
}

// abs resolves a path. Tests replace it to reach the error branch.
var abs = filepath.Abs

// ErrScript is returned for a JavaScript config file.
var ErrScript = errors.New("lexi cannot run JavaScript configuration; use .alexrc.json or .alexrc.yml")

// Find walks up from dir looking for a config file. It returns a zero
// Config when none exists.
func Find(dir string) (Config, error) {
	c, _, err := FindFile(dir)
	return c, err
}

// FindFile is like Find. It also returns the path of the file it read, or an
// empty string.
func FindFile(dir string) (Config, string, error) {
	dir, err := abs(dir)
	if err != nil {
		return Config{}, "", err
	}
	for {
		for _, n := range names {
			path := filepath.Join(dir, n)
			b, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return Config{}, path, err
			}
			c, found, err := parse(n, b)
			if err != nil {
				return Config{}, path, fmt.Errorf("cannot parse file `%s`: %w", path, err)
			}
			if found {
				return c, path, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Config{}, "", nil
		}
		dir = parent
	}
}

// parse reads one config file. found is false for a package.json without an
// `alex` field.
func parse(name string, b []byte) (c Config, found bool, err error) {
	switch filepath.Ext(name) {
	case ".cjs", ".mjs", ".js":
		return c, true, ErrScript
	case ".yaml", ".yml":
		var v any
		if err = yaml.Unmarshal(b, &v); err != nil {
			return c, true, err
		}
		if b, err = json.Marshal(v); err != nil {
			return c, true, err
		}
	}
	if name == "package.json" {
		var pkg struct {
			Alex *json.RawMessage `json:"alex"`
		}
		if err = json.Unmarshal(b, &pkg); err != nil || pkg.Alex == nil {
			return c, false, err
		}
		b = *pkg.Alex
	}
	err = json.Unmarshal(b, &c)
	return c, true, err
}
