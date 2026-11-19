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

// Config mirrors alex's .alexrc keys. A nil Allow or Deny is a key that is missing or null.
// An empty list is a list that is given, and alex treats it as given: `deny: []` reports
// nothing, and `allow: []` with `deny` is an error.
type Config struct {
	Allow             []string
	Deny              []string
	ProfanitySureness int
	NoBinary          bool
}

// ErrNull is the error of a config file that holds null. alex reads a key of it.
var ErrNull = errors.New("Cannot read properties of null (reading 'profanitySureness')") //nolint:staticcheck // the text is alex's

// list reads allow or deny. Only a string can match a rule id, so another item is kept as "".
func list(raw json.RawMessage) ([]string, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]string, len(items))
	for i, it := range items {
		var s string
		if json.Unmarshal(it, &s) == nil {
			out[i] = s
		}
	}
	return out, nil
}

// truthy tells if JavaScript reads the JSON value as true.
func truthy(raw json.RawMessage) bool {
	switch string(raw) {
	case "", "null", "false", "0", `""`, "-0", "0.0":
		return false
	}
	return true
}

// decode reads the keys of a config object. A value that is not an object has none of them, as in alex.
func decode(b []byte) (c Config, err error) {
	var top any
	if err = json.Unmarshal(b, &top); err != nil {
		return c, err
	}
	if top == nil {
		return c, ErrNull
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(b, &keys) != nil {
		return c, nil
	}
	if raw, ok := keys["allow"]; ok && string(raw) != "null" {
		if c.Allow, err = list(raw); err != nil {
			return c, fmt.Errorf("allow must be a list: %w", err)
		}
	}
	if raw, ok := keys["deny"]; ok && string(raw) != "null" {
		if c.Deny, err = list(raw); err != nil {
			return c, fmt.Errorf("deny must be a list: %w", err)
		}
	}
	c.NoBinary = truthy(keys["noBinary"])
	if raw, ok := keys["profanitySureness"]; ok && string(raw) != "null" {
		if err = json.Unmarshal(raw, &c.ProfanitySureness); err != nil {
			return c, fmt.Errorf("profanitySureness must be a whole number: %w", err)
		}
	}
	return c, nil
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
	c, err = decode(b)
	return c, true, err
}
