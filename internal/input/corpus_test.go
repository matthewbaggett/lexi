package input

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// formatCase is one line of testdata/corpus/{markdown,html,mdx}. The want field
// holds what the real alex 11.0.1 reported. scripts/corpus/generate-formats.mjs
// records it.
type formatCase struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Kind     string   `json:"kind"`
	Label    string   `json:"label"`
	Text     string   `json:"text"`
	Want     []string `json:"want"`
	// Known names a section of KNOWN_DIFFS.md: lexi differs from alex on
	// purpose for this document.
	Known string `json:"known"`
}

var formatKinds = map[string]Kind{"markdown": Markdown, "html": HTML, "mdx": MDX}

func loadFormatCorpus(t *testing.T) []formatCase {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "corpus")
	var cases []formatCase
	for _, category := range []string{"html", "markdown", "mdx"} {
		files, err := filepath.Glob(filepath.Join(root, category, "*.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(files)
		for _, name := range files {
			f, err := os.Open(name)
			if err != nil {
				t.Fatal(err)
			}
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1<<20), 1<<24)
			for line := 1; sc.Scan(); line++ {
				var c formatCase
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
	}
	return cases
}

// formatLines renders what lexi finds the way the golden file does.
func formatLines(rs []rules.Rule, ws []rules.Word, c formatCase) []string {
	d := Extract(formatKinds[c.Category], c.Text)
	if e := d.Err(); e != nil {
		return []string{"ERROR " + e.Message}
	}
	// It cannot fail: the options set neither allow nor deny.
	found, _ := d.Check(rs, ws, rules.Options{})
	out := []string{}
	for _, f := range found {
		out = append(out, fmt.Sprintf("%d:%d-%d:%d: %s [%s:%s]", f.Line, f.Column, f.EndLine, f.EndColumn, f.Message, f.Source, f.RuleID))
	}
	return out
}

// TestFormatCorpus runs lexi over the Markdown, HTML and MDX corpus and compares
// it with what alex said. It needs no Node. Set CORPUS_DIFF_OUT to a file name
// to list every difference, not only the first twenty.
func TestFormatCorpus(t *testing.T) {
	cases := loadFormatCorpus(t)
	if len(cases) < 3000 {
		t.Fatalf("the format corpus has %d documents, want at least 3000", len(cases))
	}
	known, err := os.ReadFile(filepath.Join("..", "..", "testdata", "corpus", "KNOWN_DIFFS.md"))
	if err != nil {
		t.Fatal(err)
	}
	rs, err := rules.Load()
	if err != nil {
		t.Fatal(err)
	}
	ws, err := rules.LoadProfanity()
	if err != nil {
		t.Fatal(err)
	}
	// The matcher takes most of the time, so the documents run on all CPUs.
	lexi := make([]string, len(cases))
	var wg sync.WaitGroup
	next := make(chan int)
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				lexi[i] = strings.Join(formatLines(rs, ws, cases[i]), "\n")
			}
		}()
	}
	for i := range cases {
		next <- i
	}
	close(next)
	wg.Wait()
	var dump strings.Builder
	ids := map[string]bool{}
	checked, bad := 0, 0
	for i, c := range cases {
		if ids[c.ID] {
			t.Fatalf("duplicate corpus id %s", c.ID)
		}
		ids[c.ID] = true
		if _, ok := formatKinds[c.Category]; !ok {
			t.Fatalf("%s: unknown category %q", c.ID, c.Category)
		}
		checked++
		got := lexi[i]
		want := strings.Join(c.Want, "\n")
		if c.Known != "" {
			if !strings.Contains(string(known), "\n## "+c.Known+"\n") {
				t.Errorf("%s: KNOWN_DIFFS.md has no section %q", c.ID, c.Known)
			}
			if got == want {
				t.Errorf("%s: marked as a known difference (%s), but lexi now agrees with alex", c.ID, c.Known)
			}
			continue
		}
		if got == want {
			continue
		}
		bad++
		fmt.Fprintf(&dump, "%s/%s %s: %q\n lexi: %q\n alex: %q\n", c.Category, c.Kind, c.ID, c.Text, got, want)
		if bad <= 20 {
			t.Errorf("%s/%s %s: text %q\n lexi: %q\n alex: %q", c.Category, c.Kind, c.ID, clipText(c.Text), got, want)
		}
	}
	if out := os.Getenv("CORPUS_DIFF_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(dump.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d documents differ from alex", bad, checked)
	}
}

func clipText(s string) string {
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + fmt.Sprintf("… (%d runes)", len(r))
	}
	return s
}
