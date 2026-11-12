// Package rules holds the inclusive-language rule set and the matcher. The
// rules and the matching logic come from retext-equality 6.6.0, the version
// that alex 11.0.1 pins. See docs/Upstream.md.
package rules

import (
	_ "embed"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode/utf16"
)

//go:embed data.json
var dataJSON []byte

// Rule is one retext-equality pattern.
type Rule struct {
	ID         string `json:"id"`
	Type       string `json:"type"` // "basic" or "or"
	Apostrophe bool   `json:"apostrophe"`
	// Inconsiderate lists each phrase to flag with its category.
	Inconsiderate [][2]string `json:"inconsiderate"`
	Considerate   []string    `json:"considerate"`
	Condition     string      `json:"condition"`
	Note          string      `json:"note"`
}

// Finding is one match in a text.
type Finding struct {
	Line, Column       int // start, 1-based, in UTF-16 units like alex
	EndLine, EndColumn int
	Actual             string
	Expected           []string
	Message            string
	RuleID             string
	Source             string // retext-equality or retext-profanities
	Note               string
}

// Options controls a check.
type Options struct {
	Allow    []string // rule IDs to skip
	Deny     []string // when set, report only these rule IDs
	NoBinary bool     // treat "or" rules as "basic" rules
	Ignore   []string // phrases to skip
	// Sureness is the lowest profanity rating to report: 0, 1 or 2.
	Sureness int
}

// ErrAllowDeny is returned when Options has both Allow and Deny. alex fails
// the same way.
var ErrAllowDeny = errors.New("do not provide both allow and deny configuration parameters")

// Load returns the embedded rule set.
func Load() ([]Rule, error) { return load(dataJSON) }

func load(data []byte) ([]Rule, error) {
	var rs []Rule
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

func contains(list []string, s string) bool {
	for _, l := range list {
		if l == s {
			return true
		}
	}
	return false
}

// Check scans text and returns findings in the order alex reports them.
func Check(rs []Rule, text string, o Options) ([]Finding, error) {
	if len(o.Allow) > 0 && len(o.Deny) > 0 {
		return nil, ErrAllowDeny
	}
	// Index the phrases like retext-equality does: one list that
	// normalizes apostrophes away, and one that keeps them.
	var plain, apos []string
	owner := map[string]int{}
	for i, r := range rs {
		for _, p := range r.Inconsiderate {
			if contains(o.Ignore, p[0]) {
				continue
			}
			owner[p[0]] = i
			if r.Apostrophe {
				apos = append(apos, p[0])
			} else {
				plain = append(plain, p[0])
			}
		}
	}

	var out []Finding
	nodes := mergeWords(text, tokenize(text))
	for _, para := range paragraphs(text, nodes) {
		sents := make([]sentence, len(para))
		for i, ns := range para {
			sents[i] = sentence{text, ns}
		}
		var order []int
		hits := map[int][]match{}
		handle := func(m match, phrase string) {
			if phrase != strings.ToLower(phrase) && m.text() != phrase {
				return
			}
			i := owner[phrase]
			for _, p := range rs[i].Inconsiderate {
				if p[0] == phrase {
					m.cat = p[1]
				}
			}
			if _, ok := hits[i]; !ok {
				order = append(order, i)
			}
			hits[i] = append(hits[i], m)
		}
		search(sents, plain, false, handle)
		search(sents, apos, true, handle)
		for _, i := range order {
			ms := hits[i]
			if rs[i].Type == "or" && !o.NoBinary {
				ms = or(ms)
			}
			for _, m := range ms {
				out = append(out, finding(text, rs[i], m))
			}
		}
	}

	out = filter(out, o)
	sortFindings(out)
	return out, nil
}

// sortFindings orders findings like vfile-sort does: by position, then by
// source, rule and message.
func sortFindings(out []Finding) {
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a], out[b]
		switch {
		case x.Line != y.Line:
			return x.Line < y.Line
		case x.Column != y.Column:
			return x.Column < y.Column
		case x.Source != y.Source:
			return x.Source < y.Source
		case x.RuleID != y.RuleID:
			return x.RuleID < y.RuleID
		}
		return x.Message < y.Message
	})
}

// filter applies Allow and Deny like alex does.
func filter(fs []Finding, o Options) []Finding {
	var out []Finding
	for _, f := range fs {
		if contains(o.Allow, f.RuleID) || (len(o.Deny) > 0 && !contains(o.Deny, f.RuleID)) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// finding builds the report for one match.
func finding(text string, r Rule, m match) Finding {
	actual := m.text()
	f := Finding{Actual: actual, RuleID: r.ID, Source: "retext-equality", Note: r.Note}
	// Like retext-equality, the position covers the first node only.
	f.Line, f.Column = position(text, m.sent.ns[m.start].start)
	f.EndLine, f.EndColumn = position(text, m.sent.ns[m.start].end)
	msg := "`" + actual + "` may be insensitive"
	if r.Condition != "" {
		msg += ", " + r.Condition
	}
	if len(r.Considerate) > 0 {
		f.Expected = r.Considerate
		if startsUpper(actual) {
			f.Expected = capitalize(r.Considerate)
		}
		quoted := make([]string, len(f.Expected))
		for i, e := range f.Expected {
			quoted[i] = "`" + e + "`"
		}
		msg += ", use " + strings.Join(quoted, ", ") + " instead"
	} else {
		msg += ", try not to use it"
	}
	f.Message = msg
	return f
}

// position converts a byte offset to a line and a column. The column counts
// UTF-16 units, as JavaScript does.
func position(text string, off int) (line, col int) {
	line = 1 + strings.Count(text[:off], "\n")
	start := strings.LastIndexByte(text[:off], '\n') + 1
	return line, len(utf16.Encode([]rune(text[start:off]))) + 1
}
