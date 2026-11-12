package input

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// formatsDir holds the fixture files and the golden file.
var formatsDir = filepath.Join("..", "..", "testdata", "formats")

// fmtCase is one file checked with one set of options.
type fmtCase struct {
	File     string   `json:"file"`
	Allow    []string `json:"allow,omitempty"`
	Deny     []string `json:"deny,omitempty"`
	NoBinary bool     `json:"noBinary,omitempty"`
	// Want is what alex printed. The golden file carries it.
	Want []string `json:"want,omitempty"`
}

var kinds = map[string]Kind{".md": Markdown, ".txt": Text, ".html": HTML, ".mdx": MDX}

// fixtureCases lists every fixture file with the default options, and some
// files again with other options.
func fixtureCases(t testing.TB) []fmtCase {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(formatsDir, "*.*"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []fmtCase
	for _, f := range files {
		if _, ok := kinds[filepath.Ext(f)]; ok {
			cases = append(cases, fmtCase{File: filepath.Base(f)})
		}
	}
	return append(cases,
		fmtCase{File: "ignore.md", Allow: []string{"he-she"}},
		fmtCase{File: "ignore.md", Deny: []string{"gals-man"}},
		fmtCase{File: "basic.md", NoBinary: true},
	)
}

// lexiLines renders what lexi finds the way alex's messages print.
func lexiLines(t testing.TB, c fmtCase) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(formatsDir, c.File))
	if err != nil {
		t.Fatal(err)
	}
	rs, err := rules.Load()
	if err != nil {
		t.Fatal(err)
	}
	d := Extract(kinds[filepath.Ext(c.File)], string(b))
	if e := d.Err(); e != nil {
		return []string{"ERROR " + e.Message}
	}
	found, err := d.Check(rs, rules.Options{Allow: c.Allow, Deny: c.Deny, NoBinary: c.NoBinary})
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, f := range found {
		out = append(out, fmt.Sprintf("%d:%d-%d:%d: %s [%s]", f.Line, f.Column, f.EndLine, f.EndColumn, f.Message, f.RuleID))
	}
	return out
}

func TestGolden(t *testing.T) {
	var cases []fmtCase
	b, err := os.ReadFile(filepath.Join(formatsDir, "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("expected the recorded cases, got %d", len(cases))
	}
	for _, c := range cases {
		name := fmt.Sprintf("%s/%v/%v/%v", c.File, c.Allow, c.Deny, c.NoBinary)
		t.Run(name, func(t *testing.T) {
			got := lexiLines(t, c)
			if strings.Join(got, "\n") != strings.Join(c.Want, "\n") {
				t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(c.Want, "\n"))
			}
		})
	}
}

const oracleScript = `
import {markdown, mdx, html, text} from 'alex'
let input = ''
for await (const chunk of process.stdin) input += chunk
const fns = {markdown, mdx, html, text}
const out = JSON.parse(input).map((c) => {
  const options = {noBinary: c.noBinary}
  if (c.allow) options.allow = c.allow
  if (c.deny) options.deny = c.deny
  try {
    // The profanity check is out of scope for lexi: leave its messages out.
    return fns[c.kind](c.text, options).messages.filter((m) => m.source === 'retext-equality').map(
      (m) => m.line + ':' + m.column + '-' + m.position.end.line + ':' + m.position.end.column + ': ' + m.reason + ' [' + m.ruleId + ']'
    )
  } catch (error) {
    return ['ERROR ' + error.message]
  }
})
console.log(JSON.stringify(out))
`

var kindNames = map[Kind]string{Markdown: "markdown", Text: "text", HTML: "html", MDX: "mdx"}

func runAlex(t *testing.T, dir string, cases []fmtCase) [][]string {
	t.Helper()
	type in struct {
		fmtCase
		Kind string `json:"kind"`
		Text string `json:"text"`
	}
	var ins []in
	for _, c := range cases {
		b, err := os.ReadFile(filepath.Join(formatsDir, c.File))
		if err != nil {
			t.Fatal(err)
		}
		ins = append(ins, in{c, kindNames[kinds[filepath.Ext(c.File)]], string(b)})
	}
	raw, err := json.Marshal(ins)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "--input-type=module", "-e", oracleScript)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(string(raw))
	outb, err := cmd.Output()
	if err != nil {
		t.Fatalf("running alex: %v", err)
	}
	var want [][]string
	if err := json.Unmarshal(outb, &want); err != nil {
		t.Fatal(err)
	}
	return want
}

// TestOracle compares lexi with the real alex on the fixture files. Set
// ALEX_ORACLE to a directory where `npm install alex@11.0.1` ran. Set
// UPDATE_GOLDEN to refresh the golden file from alex.
func TestOracle(t *testing.T) {
	dir := os.Getenv("ALEX_ORACLE")
	if dir == "" {
		t.Skip("ALEX_ORACLE is not set")
	}
	cases := fixtureCases(t)
	want := runAlex(t, dir, cases)
	var golden []fmtCase
	for i, c := range cases {
		c.Want = want[i]
		if want[i] == nil {
			c.Want = []string{}
		}
		golden = append(golden, c)
		got := lexiLines(t, c)
		if strings.Join(got, "\n") != strings.Join(want[i], "\n") {
			t.Errorf("%s allow=%v deny=%v\n lexi: %q\n alex: %q", c.File, c.Allow, c.Deny, got, want[i])
		}
	}
	if os.Getenv("UPDATE_GOLDEN") != "" {
		b, err := json.MarshalIndent(golden, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(formatsDir, "golden.json"), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
