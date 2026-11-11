package rules

import (
	"strings"
	"unicode"
)

// This file ports nlcst-search, nlcst-is-literal and the matching logic of
// retext-equality 6.6.0.

// normalize lowercases a word and strips dashes, and apostrophes unless
// apostrophes stay.
func normalize(s string, keepApostrophes bool) string {
	s = strings.ReplaceAll(strings.ToLower(s), "’", "'")
	s = strings.ReplaceAll(s, "-", "")
	if !keepApostrophes {
		s = strings.ReplaceAll(s, "'", "")
	}
	return s
}

// single marks that make the word beside them a literal, such as a label.
var single = []string{"-", "–", "—", ":", ";"}

// pairs maps an opening mark to the closing marks that wrap a literal.
var pairs = map[string][]string{
	",": {","}, "-": {"-"}, "–": {"–"}, "—": {"—"},
	`"`: {`"`}, "'": {"'"}, "‘": {"’"}, "‚": {"’"},
	"’": {"’", "‚"}, "“": {"”"}, "”": {"”"},
	"„": {"”", "“"}, "«": {"»"}, "»": {"«"},
	"‹": {"›"}, "›": {"‹"}, "(": {")"}, "[": {"]"},
	"{": {"}"}, "⟨": {"⟩"}, "「": {"」"},
}

func isOneOf(list []string, s string) bool {
	for _, l := range list {
		if l == s {
			return true
		}
	}
	return false
}

// sentence is the node list of one sentence plus its source text.
type sentence struct {
	src string
	ns  []node
}

// delimiter walks from pos in direction step. It skips white space and stops
// at a word. It returns the first other node when its text is in list.
func (s sentence) delimiter(pos, step int, list func(string) bool) (string, bool) {
	for i := pos + step; i > -1 && i < len(s.ns); i += step {
		switch s.ns[i].kind {
		case kWord:
			return "", false
		case kSpace:
			continue
		}
		t := s.ns[i].text(s.src)
		return t, list(t)
	}
	return "", false
}

func (s sentence) hasWord(lo, hi int) bool {
	for i := lo; i < hi; i++ {
		if s.ns[i].kind == kWord {
			return true
		}
	}
	return false
}

// isLiteral reports whether the word at pos is quoted or labelled, so that
// retext-equality leaves it alone.
func (s sentence) isLiteral(pos int) bool {
	inSingle := func(t string) bool { return isOneOf(single, t) }
	if !s.hasWord(0, pos) {
		if _, ok := s.delimiter(pos, 1, inSingle); ok {
			return true
		}
	}
	if !s.hasWord(pos+1, len(s.ns)) {
		if _, ok := s.delimiter(pos, -1, inSingle); ok {
			return true
		}
	}
	open, ok := s.delimiter(pos, -1, func(t string) bool { _, k := pairs[t]; return k })
	if !ok {
		return false
	}
	_, wrapped := s.delimiter(pos, 1, func(t string) bool { return isOneOf(pairs[open], t) })
	return wrapped
}

// match is one hit of a phrase in a sentence.
type match struct {
	cat        string
	sent       *sentence
	start, end int // indexes into sent.ns
}

// search finds every phrase in the words of one paragraph.
func search(para []sentence, phrases []string, keepApostrophes bool, handle func(m match, phrase string)) {
	byWord := map[string][]string{}
	for _, p := range phrases {
		first := normalize(strings.SplitN(p, " ", 2)[0], keepApostrophes)
		byWord[first] = append(byWord[first], p)
	}
	for si := range para {
		s := &para[si]
		for pos, n := range s.ns {
			if n.kind != kWord || s.isLiteral(pos) {
				continue
			}
			for _, p := range byWord[normalize(n.text(s.src), keepApostrophes)] {
				if end, ok := s.test(p, pos, keepApostrophes); ok {
					handle(match{sent: s, start: pos, end: end}, p)
				}
			}
		}
	}
}

// test checks the rest of phrase against the words after pos. It returns
// the index of the last node of the match.
func (s *sentence) test(phrase string, pos int, keepApostrophes bool) (int, bool) {
	rest := strings.Split(phrase, " ")[1:]
	at := pos + 1
	for _, want := range rest {
		for at < len(s.ns) && s.ns[at].kind == kSpace {
			at++
		}
		if at >= len(s.ns) || s.ns[at].kind != kWord ||
			(want != "*" && normalize(want, keepApostrophes) != normalize(s.ns[at].text(s.src), keepApostrophes)) {
			return 0, false
		}
		at++
	}
	return at - 1, true
}

func (m match) text() string {
	return m.sent.src[m.sent.ns[m.start].start:m.sent.ns[m.end].end]
}

// or drops a pair of matches of different categories that sit next to each
// other, joined by white space, `and`, `or` or a slash.
func or(ms []match) []match {
	var out []match
	for i := 0; i < len(ms); i++ {
		m := ms[i]
		if i+1 < len(ms) {
			next := ms[i+1]
			if next.sent == m.sent && next.cat != m.cat && joined(m, next) {
				i++
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

// joined reports whether only joining nodes sit between two matches.
func joined(m, next match) bool {
	for i := m.end + 1; i < next.start; i++ {
		n := m.sent.ns[i]
		t := n.text(m.sent.src)
		switch {
		case n.kind == kSpace:
		case n.kind == kWord && (strings.Contains(normalize(t, false), "and") || strings.Contains(normalize(t, false), "or")):
		case n.kind == kOther && t == "/":
		default:
			return false
		}
	}
	return true
}

// capitalize upper-cases the first letter of each suggestion.
func capitalize(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		r := []rune(s)
		out[i] = strings.ToUpper(string(r[:1])) + string(r[1:])
	}
	return out
}

// startsUpper reports whether s starts with a character that upper-casing
// leaves alone.
func startsUpper(s string) bool {
	r := []rune(s)[0]
	return unicode.ToUpper(r) == r
}
