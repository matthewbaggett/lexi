package rules

import (
	"regexp"
	"strings"
	"unicode"
)

// This file ports the part of parse-english (nlcst) that decides what a
// "word" is: it splits text into words, white space and other marks, merges
// the marks that sit inside a word, and groups the nodes into sentences and
// paragraphs. It follows parse-latin 5 and parse-english 5, the versions that
// alex 11.0.1 pins. Abbreviation exceptions for sentence ends are left out.

type kind int

const (
	kWord kind = iota
	kSpace
	kOther // punctuation and symbols: the matcher treats them alike
)

// node is one token. start and end are byte offsets into the text.
type node struct {
	kind       kind
	start, end int
}

func (n node) text(src string) string { return src[n.start:n.end] }

var (
	// Abbreviations that keep their full stop: parse-english lists them in
	// two groups. The first group matches in any case, the second as written.
	abbrAny = regexp.MustCompile(`^(inc|ltd|bbls?|cu|doz|fl|ft|gal|gr|gro|in|kt|lbs?|mi|oz|pt|qt|sq|tbsp|` +
		`tsp|yds?|sec|min|hr|mon|tue|tues|wed|thu|thurs|fri|sat|sun|jan|feb|mar|` +
		`apr|jun|jul|aug|sep|sept|oct|nov|dec)$`)
	abbrCase = regexp.MustCompile(`^(Mr|Mrs|Miss|Ms|Mss|Mses|Mlle|Mme|M|Messrs|Mmes|Jr|Sr|Snr|` +
		`Dr|Mgr|Atty|Prof|Hon|Rev|Fr|Msgr|Sr|Br|St|Pres|Supt|Rep|Sen|` +
		`Gov|Amb|Treas|Sec|Amd|Brig|Gen|Cdr|Col|Capt|Lt|Maj|Sgt|Po|Wo|Ph|` +
		`Ave|Blvd|Mt|Rd|Bldgs?|Nat|Natl|Rt|Rte|Co|Pk|Sq|Dr|Pt|St|Ft|Pen|Terr|Hwy|Fwy|Pkwy|` +
		`Ala|Ariz|Ark|Cal|Calif|Col|Colo|Conn|Del|Fla|Ga|Ida|Id|Ill|Ind|` +
		`Ia|Kan|Kans|Ken|Ky|La|Me|Md|Mass|Mich|Minn|Miss|Mo|Mont|Neb|` +
		`Nebr|Nev|Mex|Dak|Okla|Ok|Ore|Penna|Penn|Pa|Tenn|Tex|Ut|Vt|Va|` +
		`Wash|Wis|Wisc|Wyo|Alta|Man|Ont|Qu\x{00E9}|Que|Sask|Yuk|` +
		`Beds|Berks|Bucks|Cambs|Ches|Corn|Cumb|Derbys|Derbs|Dev|Dor|Dur|` +
		`Glos|Hants|Here|Heref|Herts|Hunts|Lancs|Leics|Lincs|Mx|Middx|Mddx|` +
		`Norf|Northants|Northumb|Northd|Notts|Oxon|Rut|Shrops|Salop|Som|` +
		`Staffs|Staf|Suff|Sy|Sx|Ssx|Warks|War|Warw|Westm|Wilts|Worcs|Yorks)$`)
	// Words that keep a full stop when they end a sentence: parse-latin.
	abbrPrefix = regexp.MustCompile(`^([0-9]{1,3}|[a-z]|al|ca|cap|cca|cent|cf|cit|con|cp|cwt|ead|etc|ff|` +
		`fl|ibid|id|nem|op|pro|seq|sic|stat|tem|viz)$`)
	lineBreak   = regexp.MustCompile(`\r\n|\r|\n`)
	affixSet    = "!\"').?»’”…›‽]}"
	newLine     = regexp.MustCompile(`^[ \t]*((\r?\n|\r)[\t ]*)+$`)
	terminal    = regexp.MustCompile(`^[!.?\x{2026}\x{203D}]+$`)
	innerSymbol = regexp.MustCompile(`^([&'\-.:=?@\x{00AD}\x{00B7}\x{2010}\x{2011}\x{2019}\x{2027}]|_+)$`)
	elisionHead = regexp.MustCompile(`^(o|ol)$`)
	elisionTail = regexp.MustCompile(`^(im|er|em|cause|twas|tis|twere|\d\ds?)$`)
)

func classify(r rune) kind {
	switch {
	case unicode.IsSpace(r):
		return kSpace
	case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r):
		return kWord
	}
	return kOther
}

// tokenize splits text into words, white space and other marks. Words and
// white space group runs. A mark groups with the same mark repeated.
func tokenize(src string) []node {
	var out []node
	var prev rune
	var left kind
	start := 0
	for i, r := range src {
		k := classify(r)
		if i > 0 && k == left && (k == kWord || k == kSpace || r == prev) {
			prev = r
			continue
		}
		if i > 0 {
			out = append(out, node{left, start, i})
		}
		start, left, prev = i, k, r
	}
	if len(src) > 0 {
		out = append(out, node{left, start, len(src)})
	}
	return out
}

func isMark(n node) bool { return n.kind == kOther }

// mergeWords applies the word-level plugins of parse-latin and parse-english
// to the tokens of one document.
func mergeWords(src string, ns []node) []node {
	ns = elision(src, ns)
	ns = initialAmpersand(src, ns)
	ns = finalHyphen(src, ns)
	ns = innerSymbols(src, ns)
	ns = innerSlash(src, ns)
	ns = initialisms(src, ns)
	return abbreviations(src, ns)
}

// elision joins `w/` and the apostrophes of o', 'em, 'tis and 'n'.
func elision(src string, ns []node) []node {
	for i := 0; i < len(ns); i++ {
		if !isMark(ns[i]) {
			continue
		}
		v := ns[i].text(src)
		switch v {
		case "/":
			if i > 0 && strings.EqualFold(ns[i-1].text(src), "w") {
				ns[i-1].end = ns[i].end
				ns = append(ns[:i], ns[i+1:]...)
				i--
			}
		case "'", "’":
			if i > 1 && i < len(ns)-1 && ns[i-1].kind == kWord && ns[i-2].kind == kSpace &&
				ns[i+1].kind == kSpace && elisionHead.MatchString(strings.ToLower(ns[i-1].text(src))) {
				ns[i-1].end = ns[i].end
				ns = append(ns[:i], ns[i+1:]...)
				i--
				continue
			}
			if i == len(ns)-1 || ns[i+1].kind != kWord || (i > 0 && ns[i-1].kind == kWord) {
				continue
			}
			w := strings.ToLower(ns[i+1].text(src))
			switch {
			case elisionTail.MatchString(w):
				ns[i+1].start = ns[i].start
				ns = append(ns[:i], ns[i+1:]...)
			case w == "n" && i < len(ns)-2 && (ns[i+2].text(src) == "'" || ns[i+2].text(src) == "’"):
				ns[i+1].start, ns[i+1].end = ns[i].start, ns[i+2].end
				ns = append(ns[:i], ns[i+1:]...)
				ns = append(ns[:i+1], ns[i+2:]...)
			}
		}
	}
	return ns
}

// initialAmpersand joins a leading `&` to the word after it.
func initialAmpersand(src string, ns []node) []node {
	for i := 0; i < len(ns); i++ {
		if !isMark(ns[i]) || ns[i].text(src) != "&" {
			continue
		}
		if (i > 0 && ns[i-1].kind == kWord) || i+1 >= len(ns) || ns[i+1].kind != kWord {
			continue
		}
		ns[i+1].start = ns[i].start
		ns = append(ns[:i], ns[i+1:]...)
		i--
	}
	return ns
}

// finalHyphen joins a `-` that follows a word and precedes no word.
func finalHyphen(src string, ns []node) []node {
	for i := 1; i < len(ns); i++ {
		if !isMark(ns[i]) || ns[i].text(src) != "-" || ns[i-1].kind != kWord {
			continue
		}
		if i+1 < len(ns) && ns[i+1].kind == kWord {
			continue
		}
		ns[i-1].end = ns[i].end
		ns = append(ns[:i], ns[i+1:]...)
		i--
	}
	return ns
}

// innerSymbols joins word, inner marks and word, such as `don't` or `a.b`.
func innerSymbols(src string, ns []node) []node {
	for i := 1; i < len(ns); i++ {
		if !isMark(ns[i]) || ns[i-1].kind != kWord {
			continue
		}
		pos, last := i-1, -1
		for pos++; pos < len(ns); pos++ {
			if ns[pos].kind == kWord {
				last = pos
			} else if !isMark(ns[pos]) || !innerSymbol.MatchString(ns[pos].text(src)) {
				break
			}
		}
		if last < 0 {
			continue
		}
		ns[i-1].end = ns[last].end
		ns = append(ns[:i], ns[last+1:]...)
		i--
	}
	return ns
}

// innerSlash joins short words around a slash, such as `w/o` or `a/b`.
func innerSlash(src string, ns []node) []node {
	for i := 1; i < len(ns); i++ {
		if !isMark(ns[i]) || ns[i].text(src) != "/" || ns[i-1].kind != kWord {
			continue
		}
		prev := ns[i-1].text(src)
		tail, count, next := i, 1, ""
		if i+1 < len(ns) && ns[i+1].kind == kWord {
			next, tail, count = ns[i+1].text(src), i+1, 2
		}
		if len([]rune(prev)) < 3 && len([]rune(next)) < 3 {
			ns[i-1].end = ns[tail].end
			ns = append(ns[:i], ns[i+count:]...)
			i--
		}
	}
	return ns
}

// initialisms joins the dots of A.D.H.D. into the word before.
func initialisms(src string, ns []node) []node {
	for i := 1; i < len(ns); i++ {
		if ns[i].text(src) != "." || ns[i-1].kind != kWord {
			continue
		}
		if initialism(ns[i-1].text(src)) {
			ns[i-1].end = ns[i].end
			ns = append(ns[:i], ns[i+1:]...)
			i--
		}
	}
	return ns
}

// initialism reports whether w alternates single letters and dots and does
// not consist of digits only.
func initialism(w string) bool {
	parts := strings.Split(w, ".")
	if len(parts) < 2 {
		return false
	}
	digits := true
	for _, p := range parts {
		if len([]rune(p)) != 1 {
			return false
		}
		if !unicode.IsNumber([]rune(p)[0]) {
			digits = false
		}
	}
	return !digits
}

// abbreviations joins the full stop of an abbreviation such as `Mr.` to the
// word, so that it does not end the sentence.
func abbreviations(src string, ns []node) []node {
	for i := 1; i < len(ns); i++ {
		if ns[i].kind != kOther || ns[i].text(src) != "." || ns[i-1].kind != kWord {
			continue
		}
		w := ns[i-1].text(src)
		if lw := strings.ToLower(w); abbrAny.MatchString(lw) || abbrPrefix.MatchString(lw) || abbrCase.MatchString(w) {
			ns[i-1].end = ns[i].end
			ns = append(ns[:i], ns[i+1:]...)
			i--
		}
	}
	return ns
}

// paragraphs groups nodes into paragraphs, and each paragraph into sentences.
func paragraphs(src string, ns []node) [][][]node {
	var sents [][]node
	var cur []node
	for _, n := range ns {
		cur = append(cur, n)
		if n.kind == kOther && terminal.MatchString(n.text(src)) {
			sents, cur = append(sents, cur), nil
		}
	}
	if len(cur) > 0 {
		sents = append(sents, cur)
	}
	sents = mergeSentences(src, sents)
	for i := range sents {
		sents[i] = remainingStops(src, sents[i])
	}
	var paras [][][]node
	var para [][]node
	flush := func() {
		if len(para) > 0 {
			paras, para = append(paras, para), nil
		}
	}
	for _, s := range sents {
		// Leading and trailing white space sits between sentences.
		lo, hi := 0, len(s)
		for lo < hi && s[lo].kind == kSpace {
			lo++
		}
		for hi > lo && s[hi-1].kind == kSpace {
			hi--
		}
		if lo > 0 && newLine.MatchString(s[0].text(src)) {
			flush()
		}
		for _, part := range splitBlank(src, s[lo:hi]) {
			if part == nil {
				flush()
			} else {
				para = append(para, part)
			}
		}
		if hi < len(s) && newLine.MatchString(s[hi].text(src)) {
			flush()
		}
	}
	flush()
	return paras
}

// mergeSentences applies the paragraph plugins of parse-latin: a sentence
// without words, a closing mark, or a start in lower case or with a digit
// joins its neighbour.
func mergeSentences(src string, sents [][]node) [][]node {
	for i := 0; i < len(sents); i++ {
		if hasWord(sents[i]) {
			continue
		}
		if i > 0 {
			sents[i-1] = append(sents[i-1], sents[i]...)
			sents = append(sents[:i], sents[i+1:]...)
			i--
		} else if len(sents) > 1 {
			sents[1] = append(sents[0], sents[1]...)
			sents = sents[1:]
			i--
		}
	}
	for i := 1; i < len(sents); i++ {
		for len(sents[i]) > 0 && sents[i][0].kind == kOther && isAffix(sents[i][0].text(src)) {
			sents[i-1] = append(sents[i-1], sents[i][0])
			sents[i] = sents[i][1:]
		}
	}
	for i := 1; i < len(sents); i++ {
		if startsLower(src, sents[i]) {
			sents[i-1] = append(sents[i-1], sents[i]...)
			sents = append(sents[:i], sents[i+1:]...)
			i--
		}
	}
	for i := 1; i < len(sents); i++ {
		if startsWithComma(src, sents[i]) {
			sents[i-1] = append(sents[i-1], sents[i]...)
			sents = append(sents[:i], sents[i+1:]...)
			i--
		}
	}
	return sents
}

// startsWithComma reports whether a sentence begins, before any word, with a
// comma or a semicolon. The sentence holds a word.
func startsWithComma(src string, s []node) bool {
	i := 0
	for s[i].kind == kSpace {
		i++
	}
	t := s[i].text(src)
	return s[i].kind == kOther && (t == "," || t == ";")
}

// isAffix reports whether s is one closing mark, repeated.
func isAffix(s string) bool {
	r := []rune(s)
	return strings.ContainsRune(affixSet, r[0]) && strings.Count(s, string(r[0])) == len(r)
}

func hasWord(s []node) bool {
	for _, n := range s {
		if n.kind == kWord {
			return true
		}
	}
	return false
}

// startsLower reports whether a sentence begins with a word that starts in
// lower case, or directly with a digit. The sentence holds a word.
func startsLower(src string, s []node) bool {
	i := 0
	for s[i].kind == kSpace {
		i++
	}
	if s[i].kind != kWord {
		return false
	}
	r := []rune(s[i].text(src))[0]
	// A digit only counts when no white space comes first.
	return unicode.IsLower(r) || (i == 0 && r >= '0' && r <= '9')
}

// splitBlank splits a sentence at white space with two or more line breaks.
// It returns nil entries where the paragraph breaks.
func splitBlank(src string, s []node) [][]node {
	var out [][]node
	from := 0
	for i := 1; i < len(s)-1; i++ {
		if s[i].kind == kSpace && len(lineBreak.Split(s[i].text(src), -1)) >= 3 {
			out = append(out, s[from:i], nil)
			from = i + 1
		}
	}
	return append(out, s[from:])
}

// remainingStops joins each full stop inside a sentence, other than its
// final mark, to a neighbouring word, such as the stop in `miss. today`.
func remainingStops(src string, s []node) []node {
	found := false
	for i := len(s) - 1; i >= 0; i-- {
		switch {
		case s[i].kind == kWord:
			found = true
			continue
		case s[i].kind != kOther || !terminal.MatchString(s[i].text(src)):
			continue
		case !found:
			found = true
			continue
		case s[i].text(src) != ".":
			continue
		}
		switch {
		case i > 0 && s[i-1].kind == kWord:
			if i+2 < len(s) && s[i+1].kind == kSpace && s[i+2].text(src) == "." {
				continue
			}
			s[i-1].end = s[i].end
			s = append(s[:i], s[i+1:]...)
			i--
		case i+1 < len(s) && s[i+1].kind == kWord:
			s[i+1].start = s[i].start
			s = append(s[:i], s[i+1:]...)
		}
	}
	return s
}
