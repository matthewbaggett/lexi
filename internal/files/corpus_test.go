package files

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

// corpusCase is one line of testdata/corpus/glob/<kind>.jsonl. See scripts/corpus/generate-glob.mjs.
type corpusCase struct {
	ID      string   `json:"id"`
	Tree    string   `json:"tree"`
	Kind    string   `json:"kind"`
	Pattern string   `json:"pattern"`
	Magic   bool     `json:"magic"`
	Want    []string `json:"want"`
	Files   []string `json:"files"`
	Crash   string   `json:"crash"`
	NoFiles bool     `json:"nofiles"`
}

const corpusDir = "../../testdata/corpus/glob"

// buildCorpusTree makes the files of a tree in <dir>/root. An entry is a name, or an object
// with a link and its target.
func buildCorpusTree(t *testing.T, entries []json.RawMessage) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "root")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, raw := range entries {
		var link struct{ Link, To string }
		if json.Unmarshal(raw, &link) == nil && link.Link != "" {
			p := filepath.Join(dir, link.Link)
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(strings.ReplaceAll(link.To, "__ROOT__", dir), p); err != nil {
				t.Fatal(err)
			}
			continue
		}
		var name string
		if err := json.Unmarshal(raw, &name); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// caseSensitiveFS tells whether the temporary directory tells "a" from "A".
func caseSensitiveFS(t *testing.T) bool {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := os.Stat(filepath.Join(dir, "A"))
	return err != nil
}

func readCorpus(t *testing.T) (map[string][]json.RawMessage, map[string][]corpusCase) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(corpusDir, "trees.json"))
	if err != nil {
		t.Fatal(err)
	}
	trees := map[string][]json.RawMessage{}
	if err := json.Unmarshal(b, &trees); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(corpusDir, "*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus files: %v", err)
	}
	cases := map[string][]corpusCase{}
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			var c corpusCase
			if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			cases[c.Tree] = append(cases[c.Tree], c)
		}
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	return trees, cases
}

func uniqueSorted(s []string) []string {
	out := []string{}
	for _, v := range s {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// TestGlobCorpus replays the cases that the real alex 11.0.1 recorded. It needs no Node.
func TestGlobCorpus(t *testing.T) {
	trees, cases := readCorpus(t)
	sensitive := caseSensitiveFS(t)
	var mu sync.Mutex
	var diffs []string
	total := 0
	for name, list := range cases {
		entries, ok := trees[name]
		if !ok {
			t.Fatalf("no tree %q", name)
		}
		// These trees hold names that differ only in case.
		if (name == "case" || name == "unicode") && !sensitive {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := buildCorpusTree(t, entries)
			var local []string
			for _, c := range list {
				// A file system that ignores case finds a file for a literal in the wrong case.
				if !sensitive && strings.ContainsAny(c.Pattern, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
					continue
				}
				pattern := strings.ReplaceAll(c.Pattern, "${ROOT}", root)
				if got := HasMagic(pattern); got != c.Magic {
					local = append(local, fmt.Sprintf("%s %q: HasMagic %v, want %v", name, c.Pattern, got, c.Magic))
					continue
				}
				if c.Magic {
					want := make([]string, len(c.Want))
					for i, w := range c.Want {
						want[i] = strings.ReplaceAll(w, "${ROOT}", root)
					}
					// glob sorts with ICU, which lexi does not need: its report has its own order.
					if got := Glob(pattern, root); !reflect.DeepEqual(uniqueSorted(got), uniqueSorted(want)) {
						local = append(local, fmt.Sprintf("%s %q:\n   got  %q\n   want %q", name, c.Pattern, got, want))
						continue
					}
				}
				if c.Crash != "" || c.NoFiles {
					continue
				}
				items := Find([]string{pattern}, Options{Cwd: root, Extensions: []string{".md", ".txt"}})
				var got []string
				for _, p := range paths(items) {
					got = append(got, strings.ReplaceAll(p, root, "${ROOT}"))
				}
				// The finder reads in the order of the file system, and alex reports in the order
				// that its reads end: compare the sets.
				if !reflect.DeepEqual(uniqueSorted(got), uniqueSorted(c.Files)) {
					local = append(local, fmt.Sprintf("%s %q (find):\n   got  %q\n   want %q", name, c.Pattern, got, c.Files))
				}
			}
			mu.Lock()
			diffs = append(diffs, local...)
			total += len(list)
			mu.Unlock()
		})
	}
	t.Cleanup(func() {
		if len(diffs) == 0 {
			return
		}
		shown := diffs
		if out := os.Getenv("CORPUS_DIFF_OUT"); out != "" {
			_ = os.WriteFile(out, []byte(strings.Join(diffs, "\n")+"\n"), 0o600)
		}
		if len(shown) > 25 {
			shown = shown[:25]
		}
		t.Errorf("%d of %d cases differ from alex. The first %d:\n%s", len(diffs), total, len(shown), strings.Join(shown, "\n"))
	})
}
