package rules

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// corpusCase is one line of a testdata/corpus file. The want field holds what
// alex 11.0.1 reported. scripts/corpus/generate.mjs records it.
type corpusCase struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Kind     string   `json:"kind"`
	Label    string   `json:"label"`
	Rule     string   `json:"rule"`
	Text     string   `json:"text"`
	Allow    []string `json:"allow"`
	Deny     []string `json:"deny"`
	NoBinary bool     `json:"noBinary"`
	Sureness int      `json:"sureness"`
	Want     []string `json:"want"`
}

// expectedDiffs lists the corpus cases where lexi differs from alex on
// purpose, by case id. Each one has an entry in testdata/corpus/KNOWN_DIFFS.md.
var expectedDiffs = map[string]bool{}

func loadCorpus(t *testing.T, dir string) []corpusCase {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	var cases []corpusCase
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for line := 1; sc.Scan(); line++ {
			var c corpusCase
			if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
				t.Fatalf("%s:%d: %v", name, line, err)
			}
			cases = append(cases, c)
		}
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	return cases
}

func clip(s string) string {
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + fmt.Sprintf("… (%d runes)", len(r))
	}
	return s
}

// TestCorpus runs lexi over the whole corpus and compares it with what alex
// said. The "equality" and "profanity" categories belong to this package.
// A profanity want line ends with [source:rule-id].
func TestCorpus(t *testing.T) {
	cases := loadCorpus(t, filepath.Join("..", "..", "testdata", "corpus"))
	if len(cases) < 5000 {
		t.Fatalf("the corpus has %d cases, want at least 5000", len(cases))
	}
	rs, ws := mustLoad(t), mustWords(t)
	ids := map[string]bool{}
	checked, bad := 0, 0
	for _, c := range cases {
		if ids[c.ID] {
			t.Fatalf("duplicate corpus id %s", c.ID)
		}
		ids[c.ID] = true
		opts := Options{Allow: c.Allow, Deny: c.Deny, NoBinary: c.NoBinary, Sureness: c.Sureness}
		var got []Finding
		var err error
		switch c.Category {
		case "equality":
			got, err = Check(rs, c.Text, opts)
		case "profanity":
			got, err = CheckAll(rs, ws, c.Text, opts)
		default:
			continue
		}
		checked++
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, f := range got {
			if c.Category == "profanity" {
				lines = append(lines, line(f))
			} else {
				lines = append(lines, fmt.Sprintf("%s [%s]", format(f), f.RuleID))
			}
		}
		if strings.Join(lines, "\n") == strings.Join(c.Want, "\n") || expectedDiffs[c.ID] {
			continue
		}
		bad++
		if bad <= 20 {
			t.Errorf("%s/%s %s: text %q allow=%v deny=%v noBinary=%v\n lexi: %q\n alex: %q",
				c.Category, c.Kind, c.ID, clip(c.Text), c.Allow, c.Deny, c.NoBinary, lines, c.Want)
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d cases differ from alex", bad, checked)
	}
	for id := range expectedDiffs {
		if !ids[id] {
			t.Errorf("expected diff %s is not in the corpus", id)
		}
	}
}
