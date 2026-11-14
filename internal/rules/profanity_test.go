package rules

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func mustWords(t *testing.T) []Word {
	t.Helper()
	ws, err := LoadProfanity()
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestLoadProfanityBadJSON(t *testing.T) {
	if _, err := loadProfanity([]byte("{")); err == nil {
		t.Fatal("want error")
	}
}

func TestProfanityDataIntegrity(t *testing.T) {
	ws := mustWords(t)
	if len(ws) != 3083 {
		t.Errorf("cuss 2.2.0 with its inflections has 3083 words, got %d", len(ws))
	}
	seen := map[string]bool{}
	var count [3]int
	for _, w := range ws {
		if seen[w.Word] {
			t.Errorf("duplicate word %q", w.Word)
		}
		seen[w.Word] = true
		if w.Sureness < 0 || w.Sureness > 2 {
			t.Errorf("%q: bad sureness %d", w.Word, w.Sureness)
			continue
		}
		count[w.Sureness]++
	}
	for level, n := range count {
		if n == 0 {
			t.Errorf("no word with sureness %d", level)
		}
	}
	for _, w := range regular {
		if !seen[w] {
			t.Errorf("regular word %q is not in the list", w)
		}
	}
}

// line renders a finding as the oracle and the golden file do.
func line(f Finding) string {
	return fmt.Sprintf("%s [%s:%s]", format(f), f.Source, f.RuleID)
}

func lines(fs []Finding) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, line(f))
	}
	return out
}

func TestProfanityMessages(t *testing.T) {
	ws := mustWords(t)
	rs := mustLoad(t)
	for _, c := range []struct {
		text string
		want []string
	}{
		{"an adult film", []string{"1:4-1:9: Be careful with `adult`, it’s profane in some cases [retext-profanities:adult]"}},
		{"an abuse of power", []string{"1:4-1:9: Reconsider using `abuse`, it may be profane [retext-profanities:abuse]"}},
		{"what an africoon", []string{"1:9-1:17: Don’t use `africoon`, it’s profane [retext-profanities:africoon]"}},
		{"Is that so, alligator bait?", []string{"1:13-1:27: Don’t use `alligator bait`, it’s profane [retext-profanities:alligator-bait]"}},
		{"The word abuse\nand then\nabuses", []string{
			"1:10-1:15: Reconsider using `abuse`, it may be profane [retext-profanities:abuse]",
			"3:1-3:7: Reconsider using `abuses`, it may be profane [retext-profanities:abuses]",
		}},
		{"nothing to see", []string{}},
		{"", []string{}},
	} {
		got, err := CheckAll(rs, ws, c.text, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(lines(got), "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%q\n got: %q\nwant: %q", c.text, lines(got), c.want)
		}
	}
}

func TestProfanityLiterals(t *testing.T) {
	ws, rs := mustWords(t), mustLoad(t)
	for text, want := range map[string]int{
		`say "abuse" now`:   0, // a quoted word is a literal
		`say "hell" now`:    0, // hell and whore are literals too
		`Whore, whore, x`:   1, // a word between commas is a literal
		"abuse: a label":    0,
		"what the hell":     1,
		"ABUSE":             1,
		"Abuse and abuse":   2,
		"(abuse) and abuse": 1,
	} {
		got, err := CheckAll(rs, ws, text, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != want {
			t.Errorf("%q: want %d findings, got %q", text, want, lines(got))
		}
	}
}

func TestProfanitySureness(t *testing.T) {
	ws, rs := mustWords(t), mustLoad(t)
	text := "adult abuse africoon"
	for level, want := range []int{3, 2, 1, 0} {
		got, err := CheckAll(rs, ws, text, Options{Sureness: level})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != want {
			t.Errorf("sureness %d: want %d, got %q", level, want, lines(got))
		}
	}
}

func TestProfanityAllowDenyIgnore(t *testing.T) {
	ws, rs := mustWords(t), mustLoad(t)
	text := "adult abuse alligator bait"
	got, _ := CheckAll(rs, ws, text, Options{Allow: []string{"abuse", "alligator-bait"}})
	if len(got) != 1 || got[0].RuleID != "adult" {
		t.Errorf("allow: %q", lines(got))
	}
	got, _ = CheckAll(rs, ws, text, Options{Deny: []string{"abuse"}})
	if len(got) != 1 || got[0].RuleID != "abuse" {
		t.Errorf("deny: %q", lines(got))
	}
	got, _ = CheckAll(rs, ws, text, Options{Ignore: []string{"abuse"}})
	if len(got) != 2 {
		t.Errorf("ignore: %q", lines(got))
	}
	if _, err := CheckAll(rs, ws, text, Options{Allow: []string{"a"}, Deny: []string{"b"}}); err != ErrAllowDeny {
		t.Errorf("want ErrAllowDeny, got %v", err)
	}
}

func TestProfanityWithEquality(t *testing.T) {
	ws, rs := mustWords(t), mustLoad(t)
	got, err := CheckAll(rs, ws, "The blacklist is abuse", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Source != "retext-equality" || got[1].Source != "retext-profanities" {
		t.Errorf("got %q", lines(got))
	}
	if got[1].Line != 1 || got[1].Column != 18 {
		t.Errorf("position %+v", got[1])
	}
}

// profanityCase is one text of the profanity corpus. Want is what alex
// printed for it.
type profanityCase struct {
	Kind     string   `json:"-"`
	Text     string   `json:"text"`
	Sureness int      `json:"sureness"`
	Allow    []string `json:"allow,omitempty"`
	Deny     []string `json:"deny,omitempty"`
	NoBinary bool     `json:"noBinary,omitempty"`
	Want     []string `json:"-"`
}

func runProfanity(t *testing.T, ws []Word, rs []Rule, c profanityCase) []string {
	t.Helper()
	got, err := CheckAll(rs, ws, c.Text, Options{Allow: c.Allow, Deny: c.Deny, NoBinary: c.NoBinary, Sureness: c.Sureness})
	if err != nil {
		t.Fatal(err)
	}
	return lines(got)
}

// TestProfanityCorpusShape checks that the profanity files of the corpus
// cover all three ratings. TestCorpus compares the findings.
func TestProfanityCorpusShape(t *testing.T) {
	var levels [3]int
	total := 0
	for _, c := range loadCorpus(t, filepath.Join("..", "..", "testdata", "corpus")) {
		if c.Category == "profanity" {
			levels[c.Sureness]++
			total++
		}
	}
	if total < 300 {
		t.Fatalf("the profanity corpus needs 300 cases, got %d", total)
	}
	for level, n := range levels {
		if n < 100 {
			t.Errorf("sureness %d has %d cases, want at least 100", level, n)
		}
	}
}

// profanityTexts are the hand-written texts of the corpus. Each runs at all
// three sureness levels.
var profanityTexts = []string{
	"adult", "Adult", "ADULT", "an adult film", "adults", "abuse", "abuses", "abuse.", "(abuse)", "abuse, abuse",
	"abbo", "abbos", "alligator bait", "alligator  bait", "Alligator\nbait", "alligatorbait", "alligator-bait",
	"anal sex", "analsex", "anal-sex", "anus", "arab", "arabs", "arabush", "arse", "arse bandit", "arse-bandit",
	"africa", "African", "african", "address the angry crowd", "allah", "Allah's", "american", "amateur hour",
	"hell", "hells", "what the hell", `"hell"`, "'hell'", "“hell”", "(hell)", "hell: a place", "hell; so", "Hell yes",
	"whore", "whores", "“whore”", `"whores"`, "whorehouse", "whorefucker",
	"say \"abuse\" now", "say 'abuse' now", "say (abuse) now", "say [abuse] now", "abuse: a label", "a label: abuse",
	"abuse - a dash", "a dash - abuse", "a — abuse — b", "x, abuse, y", "abuse\n\nabuse", "abuse\nabuse",
	"Fine. Abuse. Fine.", "A sentence about nothing at all.", "", " ", "\n", "plain words only",
	"He said fuck.", "Fuck!", "fucking hell", "damn it", "damn", "shit happens", "bullshit", "crap", "piss off",
	"bitch", "bitches", "bastard", "bastards", "asshole", "dick", "cock", "pussy", "cunt", "twat", "wanker",
	"ass", "asses", "arsehole", "balls", "bollocks", "bugger", "slut", "sluts", "tit", "tits", "turd", "prick",
	"nigger", "negro", "faggot", "fag", "dyke", "retard", "spic", "chink", "kike", "gook", "wetback", "paki",
	"sex", "sexy", "sexual", "porn", "naked", "nude", "orgy", "orgasm", "penis", "vagina", "breast", "boob",
	"drunk", "drug", "drugs", "dope", "heroin", "cocaine", "weed", "pot", "joint", "stoned", "high",
	"kill", "killing", "murder", "rape", "raped", "suicide", "terror", "terrorist", "bomb", "gun", "shoot",
	"god", "jesus", "christ", "devil", "satan", "damn god", "oh my god", "hail satan",
	"The quick brown fox jumps over the lazy dog.", "We ship abuse reports daily.", "He is an adult and an addict.",
	"Reports of abuse and addiction.", "Drugs, sex, and rock and roll.", "Sex, drugs & abuse", "abuse/abuse", "abuse_abuse",
	"abuse's", "abuse’s", "ab-use", "a buse", "abuses abuse abused abusing", "Abuse Abuse ABUSE",
	"café abuse", "😀 abuse", "😀😀 abuse abuse", "日本語 abuse", "abuse 日本語", "ünïcödé adult",
	"tabs\tabuse", "a  b  abuse", "abuse...", "abuse?!", "abuse abuse", "“abuse”", "‘abuse’",
	"Is it hell? No, hell is other people.", "Whore, whore, whore", "Hell, hell, hell", "A whorehouse is not a whore.",
	"e.g. abuse", "Mr. Adult", "St. Abuse", "3.5 abuse", "1 adult", "adult1", "1adult",
}

// profanityOptions holds the cases with allow, deny and noBinary.
var profanityOptions = []profanityCase{
	{Text: "adult abuse abbo", Allow: []string{"abuse"}},
	{Text: "adult abuse abbo", Allow: []string{"abuse", "adult"}},
	{Text: "adult abuse abbo", Allow: []string{"abbo"}, Sureness: 1},
	{Text: "adult abuse abbo", Allow: []string{"abbo"}, Sureness: 2},
	{Text: "adult abuse abbo", Deny: []string{"abuse"}},
	{Text: "adult abuse abbo", Deny: []string{"abuse", "abbo"}, Sureness: 2},
	{Text: "adult abuse abbo", Deny: []string{"nothing"}},
	{Text: "alligator bait", Allow: []string{"alligator-bait"}},
	{Text: "alligator bait", Allow: []string{"alligator bait"}},
	{Text: "alligator bait", Deny: []string{"alligator-bait"}},
	{Text: "The blacklist is abuse", Allow: []string{"blacklist"}},
	{Text: "The blacklist is abuse", Deny: []string{"abuse"}},
	{Text: "The blacklist is abuse", Deny: []string{"blacklist"}},
	{Text: "The blacklist is abuse", Allow: []string{"abuse", "blacklist"}},
	{Text: "Her and his abuse", NoBinary: true},
	{Text: "Her and his abuse"},
	{Text: "hell", Allow: []string{"hell"}},
	{Text: "whores", Deny: []string{"whores"}},
	{Text: "Fuck the whitelist, it’s hell", Sureness: 1},
	{Text: "Fuck the whitelist, it’s hell", Sureness: 2},
}

// profanityCases lists the whole corpus.
func profanityCases(ws []Word) []profanityCase {
	var out []profanityCase
	for _, text := range profanityTexts {
		for level := range 3 {
			out = append(out, profanityCase{Kind: "plain", Text: text, Sureness: level})
		}
	}
	for _, c := range profanityOptions {
		c.Kind = "options"
		out = append(out, c)
	}
	// A sample of the word list, a word of each rating at a stride.
	for i, w := range ws {
		if i%25 != 0 {
			continue
		}
		out = append(out, profanityCase{Kind: "wordlist", Text: "We saw the " + w.Word + " today.", Sureness: w.Sureness},
			profanityCase{Kind: "wordlist", Text: strings.ToUpper(w.Word[:1]) + w.Word[1:] + " is here", Sureness: (w.Sureness + 1) % 3})
	}
	return append(out, profanityShuffled(ws)...)
}

// profanityShuffled glues profane words together with random separators, from
// a fixed seed, to stress the tokenizer and the literal rules.
func profanityShuffled(ws []Word) []profanityCase {
	seps := []string{" ", "  ", "\n", "\n\n", ", ", ". ", "/", " and ", "-", "'", "\u2019", `"`, "(", ")", ": ", " \u2014 ", "... ", "! ", "? ", "_", ";"}
	pool := []string{"hell", "whore", "Hell", "WHORE", "adult", "abuse", "alligator bait", "He", "x", "blacklist"}
	for i := 0; i < len(ws); i += 40 {
		pool = append(pool, ws[i].Word)
	}
	rnd := rand.New(rand.NewPCG(3, 4))
	var out []profanityCase
	for range 500 {
		var b strings.Builder
		for n := 1 + rnd.IntN(5); n > 0; n-- {
			b.WriteString(pool[rnd.IntN(len(pool))])
			b.WriteString(seps[rnd.IntN(len(seps))])
		}
		out = append(out, profanityCase{Kind: "shuffled", Text: b.String(), Sureness: rnd.IntN(3)})
	}
	return out
}

const profanityScript = `
import {text} from 'alex'
let input = ''
process.stdin.setEncoding('utf8')
for await (const chunk of process.stdin) input += chunk
const out = JSON.parse(input).map((c) => {
  const options = {noBinary: c.noBinary, profanitySureness: c.sureness || 0}
  if (c.allow) options.allow = c.allow
  if (c.deny) options.deny = c.deny
  return text(c.text, options).messages.map(
    (m) => m.line + ':' + m.column + '-' + m.position.end.line + ':' + m.position.end.column + ': ' + m.reason + ' [' + m.source + ':' + m.ruleId + ']'
  )
})
console.log(JSON.stringify(out))
`

// alexOnProfanity asks the real alex for the findings of each case.
func alexOnProfanity(t *testing.T, dir string, cases []profanityCase) [][]string {
	t.Helper()
	in, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "--input-type=module", "-e", profanityScript)
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

// TestProfanityOracle compares lexi with the real alex on the corpus. Set
// ALEX_ORACLE to a directory where `npm install alex@11.0.1` ran. Add
// UPDATE_GOLDEN=1 to record testdata/corpus/profanity again.
func TestProfanityOracle(t *testing.T) {
	dir := os.Getenv("ALEX_ORACLE")
	if dir == "" {
		t.Skip("ALEX_ORACLE is not set")
	}
	ws, rs := mustWords(t), mustLoad(t)
	cases := profanityCases(ws)
	want := alexOnProfanity(t, dir, cases)
	bad := 0
	for i, c := range cases {
		got := runProfanity(t, ws, rs, c)
		cases[i].Want = want[i]
		if strings.Join(got, "\n") != strings.Join(want[i], "\n") {
			bad++
			if bad <= 20 {
				t.Errorf("text %q sureness=%d allow=%v deny=%v\n lexi: %q\n alex: %q", c.Text, c.Sureness, c.Allow, c.Deny, got, want[i])
			}
		}
	}
	t.Logf("%d texts compared, %d differ", len(cases), bad)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		writeProfanityCorpus(t, cases)
	}
}

// writeProfanityCorpus records the cases, with what alex printed, as the
// testdata/corpus/profanity files.
func writeProfanityCorpus(t *testing.T, cases []profanityCase) {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "corpus", "profanity")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]*strings.Builder{}
	seen := map[string]bool{}
	for _, c := range cases {
		key, err := json.Marshal([]any{c.Text, c.Sureness, c.Allow, c.Deny, c.NoBinary})
		if err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprintf("%x", sha1.Sum(key))[:10]
		if seen[id] {
			continue
		}
		seen[id] = true
		rec := map[string]any{"id": id, "category": "profanity", "kind": c.Kind, "label": "positive", "text": c.Text, "sureness": c.Sureness, "want": c.Want}
		if len(c.Want) == 0 {
			rec["label"] = "negative"
		}
		if c.Allow != nil {
			rec["allow"] = c.Allow
		}
		if c.Deny != nil {
			rec["deny"] = c.Deny
		}
		if c.NoBinary {
			rec["noBinary"] = true
		}
		line, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		if files[c.Kind] == nil {
			files[c.Kind] = &strings.Builder{}
		}
		files[c.Kind].WriteString(string(line) + "\n")
	}
	for kind, b := range files {
		if err := os.WriteFile(filepath.Join(dir, kind+".jsonl"), []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
