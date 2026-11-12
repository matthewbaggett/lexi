package rules

import (
	_ "embed"
	"encoding/json"
	"regexp"
)

// The profanity check comes from retext-profanities 7.2.2, which alex 11.0.1
// pins, and its word list from cuss 2.2.0. See docs/Upstream.md.

//go:embed profanity.json
var profanityJSON []byte

// Word is one profane word with its cuss rating: 0 is profane in some cases,
// 1 maybe profane, 2 profane. The list holds the singular and the plural of
// each cuss word, as retext-profanities builds it.
type Word struct {
	Word     string
	Sureness int
}

// profanitySource is the source name of a finding that comes from this check.
const profanitySource = "retext-profanities"

// regular holds the words that upstream searches with apostrophes kept. This
// is the `true` that retext-profanities passes to nlcst-search, which reads it
// as `allowApostrophes`. Quoted words stay skipped.
var regular = []string{"hell", "whore"}

// LoadProfanity returns the embedded word list.
func LoadProfanity() ([]Word, error) { return loadProfanity(profanityJSON) }

func loadProfanity(data []byte) ([]Word, error) {
	var raw [][2]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	ws := make([]Word, len(raw))
	for i, r := range raw {
		w, _ := r[0].(string)
		n, _ := r[1].(float64)
		ws[i] = Word{w, int(n)}
	}
	return ws, nil
}

var nonWord = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// CheckAll runs the inclusive-language check and the profanity check, and
// returns the findings in the order alex reports them. Allow and Deny apply
// to the words of the profanity check as well, by their rule id. A rule id
// is the word with each run of other characters turned into a dash.
func CheckAll(rs []Rule, ws []Word, text string, o Options) ([]Finding, error) {
	out, err := Check(rs, text, o)
	if err != nil {
		return nil, err
	}
	out = append(out, filter(profanity(ws, text, o), o)...)
	sortFindings(out)
	return out, nil
}

// profanity ports retext-profanities.
func profanity(ws []Word, text string, o Options) []Finding {
	sureness := map[string]int{}
	var normals, regularWords []string
	for _, w := range ws {
		if contains(o.Ignore, w.Word) {
			continue
		}
		sureness[w.Word] = w.Sureness
		if contains(regular, w.Word) {
			regularWords = append(regularWords, w.Word)
		} else {
			normals = append(normals, w.Word)
		}
	}

	var out []Finding
	nodes := mergeWords(text, tokenize(text))
	for _, para := range paragraphs(text, nodes) {
		sents := make([]sentence, len(para))
		for i, ns := range para {
			sents[i] = sentence{text, ns}
		}
		handle := func(m match, phrase string) {
			level := sureness[phrase]
			if level < o.Sureness {
				return
			}
			out = append(out, profaneFinding(text, m, phrase, level))
		}
		search(sents, normals, false, handle)
		search(sents, regularWords, true, handle)
	}
	return out
}

// profaneFinding builds the report for one match. Unlike an equality
// finding, its range runs to the end of the last word.
func profaneFinding(text string, m match, phrase string, level int) Finding {
	actual := m.text()
	lead, tail := "Be careful with", "it’s profane in some cases"
	switch level {
	case 1:
		lead, tail = "Reconsider using", "it may be profane"
	case 2:
		lead, tail = "Don’t use", "it’s profane"
	}
	f := Finding{
		Actual:  actual,
		RuleID:  nonWord.ReplaceAllString(phrase, "-"),
		Source:  profanitySource,
		Message: lead + " `" + actual + "`, " + tail,
	}
	f.Line, f.Column = position(text, m.sent.ns[m.start].start)
	f.EndLine, f.EndColumn = position(text, m.sent.ns[m.end].end)
	return f
}
