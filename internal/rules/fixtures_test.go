package rules

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// upstreamCase is one test of retext-equality 6.6.0, recorded from its
// test.js. messages holds what retext-equality reports, in its own format.
type upstreamCase struct {
	Name    string `json:"name"`
	Input   string `json:"input"`
	Options struct {
		Ignore   []string `json:"ignore"`
		NoBinary bool     `json:"noBinary"`
	} `json:"options"`
	Messages []string `json:"messages"`
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// format renders a finding like String(VFileMessage) does in retext.
func format(f Finding) string {
	return fmt.Sprintf("%d:%d-%d:%d: %s", f.Line, f.Column, f.EndLine, f.EndColumn, f.Message)
}

func TestUpstreamFixtures(t *testing.T) {
	var cases []upstreamCase
	readJSON(t, filepath.Join("..", "..", "testdata", "upstream.json"), &cases)
	if len(cases) < 60 {
		t.Fatalf("expected the 60 recorded upstream cases, got %d", len(cases))
	}
	rs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := Check(rs, c.Input, Options{Ignore: c.Options.Ignore, NoBinary: c.Options.NoBinary})
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, f := range got {
				lines = append(lines, format(f))
			}
			if strings.Join(lines, "\n") != strings.Join(c.Messages, "\n") {
				t.Errorf("input %q\n got: %q\nwant: %q", c.Input, lines, c.Messages)
			}
		})
	}
}

// oracleCase is what the node script reads for each text.
type oracleCase struct {
	Text     string   `json:"text"`
	Allow    []string `json:"allow,omitempty"`
	Deny     []string `json:"deny,omitempty"`
	NoBinary bool     `json:"noBinary,omitempty"`
	// Want is what alex printed. Only the golden file carries it.
	Want []string `json:"want,omitempty"`
}

const oracleScript = `
import {text} from 'alex'
let input = ''
for await (const chunk of process.stdin) input += chunk
const out = JSON.parse(input).map((c) => {
  const options = {noBinary: c.noBinary}
  if (c.allow) options.allow = c.allow
  if (c.deny) options.deny = c.deny
  // The profanity check is out of scope for lexi: leave its messages out.
  return text(c.text, options).messages.filter((m) => m.source === 'retext-equality').map(
    (m) => m.line + ':' + m.column + '-' + m.position.end.line + ':' + m.position.end.column + ': ' + m.reason + ' [' + m.ruleId + ']'
  )
})
console.log(JSON.stringify(out))
`

// TestOracle compares lexi with the real alex. Set ALEX_ORACLE to a
// directory where `npm install alex@11.0.1` ran.
func TestOracle(t *testing.T) {
	dir := os.Getenv("ALEX_ORACLE")
	if dir == "" {
		t.Skip("ALEX_ORACLE is not set")
	}
	rs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var cases []oracleCase
	var upstream []upstreamCase
	readJSON(t, filepath.Join("..", "..", "testdata", "upstream.json"), &upstream)
	for _, u := range upstream {
		cases = append(cases, oracleCase{Text: u.Input, NoBinary: u.Options.NoBinary})
	}
	var extra []oracleCase
	readJSON(t, filepath.Join("..", "..", "testdata", "oracle.json"), &extra)
	cases = append(cases, extra...)
	cases = append(cases, generated(rs)...)

	want := runAlex(t, dir, cases)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		writeGolden(t, extra, shuffled(rs)[:goldenRandom], dir)
	}
	bad := 0
	for i, c := range cases {
		got, err := Check(rs, c.Text, Options{Allow: c.Allow, Deny: c.Deny, NoBinary: c.NoBinary})
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, f := range got {
			lines = append(lines, fmt.Sprintf("%s [%s]", format(f), f.RuleID))
		}
		if strings.Join(lines, "\n") != strings.Join(want[i], "\n") {
			bad++
			if bad <= 20 {
				t.Errorf("text %q allow=%v deny=%v noBinary=%v\n lexi: %q\n alex: %q", c.Text, c.Allow, c.Deny, c.NoBinary, lines, want[i])
			}
		}
	}
	t.Logf("%d texts compared, %d differ", len(cases), bad)
}

// goldenRandom is how many of the shuffled texts go into the golden file.
const goldenRandom = 700

// runAlex asks the real alex for the findings of each case.
func runAlex(t *testing.T, dir string, cases []oracleCase) [][]string {
	t.Helper()
	in, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "--input-type=module", "-e", oracleScript)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(string(in))
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("running alex: %v", err)
	}
	var want [][]string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	return want
}

// writeGolden records what alex says about a fixed set of texts, so that
// TestGolden can check lexi without node. Run the oracle with UPDATE_GOLDEN=1
// to refresh it.
func writeGolden(t *testing.T, sets ...any) {
	t.Helper()
	dir := sets[len(sets)-1].(string)
	var cases []oracleCase
	for _, s := range sets[:len(sets)-1] {
		cases = append(cases, s.([]oracleCase)...)
	}
	for i, w := range runAlex(t, dir, cases) {
		cases[i].Want = append([]string{}, w...)
	}
	var b strings.Builder
	b.WriteString("[\n")
	for i, c := range cases {
		line, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString("  " + string(line))
		if i < len(cases)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("]\n")
	if err := os.WriteFile(filepath.Join("..", "..", "testdata", "golden.json"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestGolden replays recorded alex output, so that the tokenizer stays in
// step with alex without node.
func TestGolden(t *testing.T) {
	var cases []oracleCase
	readJSON(t, filepath.Join("..", "..", "testdata", "golden.json"), &cases)
	if len(cases) < 700 {
		t.Fatalf("golden file is too short: %d", len(cases))
	}
	rs := mustLoad(t)
	for _, c := range cases {
		got, err := Check(rs, c.Text, Options{Allow: c.Allow, Deny: c.Deny, NoBinary: c.NoBinary})
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, f := range got {
			lines = append(lines, fmt.Sprintf("%s [%s]", format(f), f.RuleID))
		}
		if strings.Join(lines, "\n") != strings.Join(c.Want, "\n") {
			t.Errorf("text %q\n lexi: %q\n alex: %q", c.Text, lines, c.Want)
		}
	}
}

// generated builds one text for each phrase of each rule, in a few
// surroundings, so the oracle covers the whole rule set.
func generated(rs []Rule) []oracleCase {
	var out []oracleCase
	for _, r := range rs {
		for _, p := range r.Inconsiderate {
			out = append(out,
				oracleCase{Text: "We saw the " + p[0] + " today."},
				oracleCase{Text: strings.ToUpper(p[0][:1]) + p[0][1:] + " is here"},
			)
		}
	}
	return append(out, shuffled(rs)...)
}

// shuffled glues phrases together with random separators, from a fixed seed,
// to stress the tokenizer.
func shuffled(rs []Rule) []oracleCase {
	seps := []string{" ", "  ", "\n", "\n\n", ", ", ". ", ".\n", "/", " and ", " or ", "-", "'", "\u2019", `"`, "(", ")", ": ", " \u2014 ", "... ", "! ", "? ", "&", "_", ";", "\t", " w/ ", " 3.5 "}
	rnd := rand.New(rand.NewPCG(1, 2))
	var pool []string
	for _, r := range rs {
		for _, p := range r.Inconsiderate {
			pool = append(pool, p[0])
		}
	}
	pool = append(pool, "He", "She", "Mr", "St", "e.g", "A.D.H.D", "x", "O", "em", "tis")
	var out []oracleCase
	for range 1500 {
		var b strings.Builder
		for n := 1 + rnd.IntN(5); n > 0; n-- {
			b.WriteString(pool[rnd.IntN(len(pool))])
			b.WriteString(seps[rnd.IntN(len(seps))])
		}
		out = append(out, oracleCase{Text: b.String(), NoBinary: rnd.IntN(4) == 0})
	}
	return out
}
