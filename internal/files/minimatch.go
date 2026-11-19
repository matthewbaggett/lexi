package files

import (
	"regexp"
	"strings"

	"github.com/dlclark/regexp2"
)

// This file is a port of the pattern parser of minimatch 5.1.9, the library that glob 8.1.0
// uses. It builds the same JavaScript regular expressions, and runs them with regexp2 in its
// ECMAScript mode, so the quirks of minimatch stay. glob switches off negation and comments,
// and leaves dot, nocase, noext and noglobstar off: the port has none of these options.

// notEmpty is "(?=.)" of minimatch. In JavaScript "." does not match a line end: \n, \r, U+2028
// and U+2029. regexp2 leaves the last two out of its list, so the class says all four.
const notEmpty = `(?=[^\n\r\u2028\u2029])`

const (
	qmark = `[^/]`
	star  = qmark + `*?`
)

type partKind int

const (
	partLit partKind = iota // a literal name
	partRe                  // a name with wildcards
	partGlobstar
)

// A part is one segment of a pattern.
type part struct {
	kind partKind
	lit  string // for partLit
	glob string // for partRe: the segment, as written
	re   *regexp2.Regexp
}

type plType struct{ open, close string }

var plTypes = map[rune]plType{
	'!': {"(?:(?!(?:", "))[^/]*?)"},
	'?': {"(?:", ")?"},
	'+': {"(?:", ")+"},
	'*': {"(?:", ")*"},
	'@': {"(?:", ")"},
}

type plEntry struct {
	typ          rune
	start        int
	reStart      int
	reEnd        int
	open, closeS string
}

func reSpecial(c rune) bool { return strings.ContainsRune(`().*{}+?[]^$\!`, c) }

// A JavaScript string is a list of UTF-16 units, so `?` matches half of an emoji. regexp2 works
// on code points. js16 maps each half of a surrogate pair to a private-use code point, so that
// a pattern and a name keep the JavaScript view of both.
func js16(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r < 0x10000 {
			b.WriteRune(r)
			continue
		}
		r -= 0x10000
		b.WriteRune(0xF0000 + 0xD800 + (r >> 10))
		b.WriteRune(0xF0000 + 0xDC00 + (r & 0x3FF))
	}
	return b.String()
}

// splitSlashes is s.split(/\/+/).
func splitSlashes(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '/' {
			continue
		}
		out = append(out, s[start:i])
		for i+1 < len(s) && s[i+1] == '/' {
			i++
		}
		start = i + 1
	}
	return append(out, s[start:])
}

// parseSet expands the braces of a pattern, and parses each result into parts.
func parseSet(pattern string) [][]part {
	var set [][]part
	for _, p := range expandBraces(pattern) {
		var parts []part
		for _, seg := range splitSlashes(p) {
			parts = append(parts, parseSegment(seg))
		}
		set = append(set, parts)
	}
	return set
}

var matchNothing = regexp2.MustCompile(`$.`, regexp2.ECMAScript)

func parseSegment(seg string) part {
	if seg == "**" {
		return part{kind: partGlobstar}
	}
	if seg == "" {
		return part{kind: partLit}
	}
	pattern := []rune(js16(seg))
	re, magic := translate(pattern, false)
	if !magic {
		return part{kind: partLit, lit: globUnescape(seg)}
	}
	return part{kind: partRe, glob: seg, re: compileRe("^" + string(re) + "$")}
}

// compileRe compiles a JavaScript regular expression. One that does not compile matches
// nothing, as in minimatch.
func compileRe(src string) *regexp2.Regexp {
	rx, err := regexp2.Compile(src, regexp2.ECMAScript)
	if err != nil {
		return matchNothing
	}
	return rx
}

// globUnescape is s.replace(/\\(.)/g, "$1").
func globUnescape(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) && !isLineEnd(rs[i+1]) {
			i++
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

func isLineEnd(r rune) bool { return r == '\n' || r == '\r' || r == ' ' || r == ' ' }

// charUnescape is s.replace(/\\([^-\]])/g, "$1").
func charUnescape(cs []rune) []rune {
	var out []rune
	for i := 0; i < len(cs); i++ {
		if cs[i] == '\\' && i+1 < len(cs) && cs[i+1] != '-' && cs[i+1] != ']' {
			i++
		}
		out = append(out, cs[i])
	}
	return out
}

// validClass tells whether RegExp("[" + braExpEscape(charUnescape(cs)) + "]") compiles: it
// does not when a range runs backwards.
func validClass(cs []rune) bool {
	var toks []rune // the characters of the class, one for each atom
	for _, c := range charUnescape(cs) {
		if c == '\\' || c == '[' || c == ']' {
			toks = append(toks, '\\') // braExpEscape adds a backslash: a literal backslash
			if c == '\\' {
				continue
			}
		}
		toks = append(toks, c)
	}
	// Now an escaped backslash is two atoms of "\" and an escaped bracket is "\" and the
	// bracket. Read the atoms as JavaScript does.
	var atoms []rune
	for i := 0; i < len(toks); i++ {
		if toks[i] == '\\' && i+1 < len(toks) {
			i++
		}
		atoms = append(atoms, toks[i])
	}
	for i := 0; i < len(atoms); i++ {
		if i+2 < len(atoms) && atoms[i+1] == '-' && atoms[i] > atoms[i+2] {
			return false
		}
		if i+2 < len(atoms) && atoms[i+1] == '-' {
			i += 2
		}
	}
	return true
}

// translate is the loop of minimatch.parse. It returns the source of the regular expression
// and whether the segment has any magic.
func translate(pattern []rune, isSub bool) ([]rune, bool) {
	var re []rune
	hasMagic, escaping, inClass := false, false, false
	var stack, negs []*plEntry
	var stateChar rune
	reClassStart, classStart := -1, -1
	dotTrav := pattern[0] == '.'
	add := func(s string) { re = append(re, []rune(s)...) }
	subStart := func(rest []rune) string {
		if len(rest) > 0 && rest[0] == '.' {
			return ""
		}
		return `(?!\.)`
	}
	clearState := func() {
		switch stateChar {
		case 0:
			return
		case '*':
			add(star)
			hasMagic = true
		case '?':
			add(qmark)
			hasMagic = true
		default:
			add(`\` + string(stateChar))
		}
		stateChar = 0
	}
	top := func() *plEntry {
		if len(stack) == 0 {
			return nil
		}
		return stack[len(stack)-1]
	}

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if escaping {
			if reSpecial(c) {
				re = append(re, '\\')
			}
			re = append(re, c)
			escaping = false
			continue
		}
		switch c {
		case '\\':
			if inClass && i+1 < len(pattern) && pattern[i+1] == '-' {
				re = append(re, c)
				continue
			}
			clearState()
			escaping = true
		case '?', '*', '+', '@', '!':
			if inClass {
				if c == '!' && i == classStart+1 {
					c = '^'
				}
				re = append(re, c)
				continue
			}
			if c == '*' && stateChar == '*' {
				continue
			}
			clearState()
			stateChar = c
		case '(':
			if inClass {
				re = append(re, '(')
				continue
			}
			if stateChar == 0 {
				add(`\(`)
				continue
			}
			pl := &plEntry{typ: stateChar, start: i - 1, reStart: len(re), open: plTypes[stateChar].open, closeS: plTypes[stateChar].close}
			stack = append(stack, pl)
			add(pl.open)
			if pl.start == 0 && pl.typ != '!' {
				dotTrav = true
				add(subStart(pattern[i+1:]))
			}
			stateChar = 0
		case ')':
			pl := top()
			if inClass || pl == nil {
				add(`\)`)
				continue
			}
			stack = stack[:len(stack)-1]
			clearState()
			hasMagic = true
			add(pl.closeS)
			if pl.typ == '!' {
				pl.reEnd = len(re)
				negs = append(negs, pl)
			}
		case '|':
			pl := top()
			if inClass || pl == nil {
				add(`\|`)
				continue
			}
			clearState()
			re = append(re, '|')
			if pl.start == 0 && pl.typ != '!' {
				dotTrav = true
				add(subStart(pattern[i+1:]))
			}
		case '[':
			clearState()
			if inClass {
				add(`\[`)
				continue
			}
			inClass = true
			classStart = i
			reClassStart = len(re)
			re = append(re, c)
		case ']':
			if i == classStart+1 || !inClass {
				add(`\]`)
				continue
			}
			if validClass(pattern[classStart+1 : i]) {
				re = append(re, c)
			} else {
				re = append(re[:reClassStart:reClassStart], []rune(`(?:$.)`)...)
			}
			hasMagic = true
			inClass = false
		default:
			clearState()
			if reSpecial(c) && (c != '^' || !inClass) {
				re = append(re, '\\')
			}
			re = append(re, c)
		}
	}

	// An open class: "[abc" is "\[abc".
	if inClass {
		cs := pattern[classStart+1:]
		sub := "undefined" // minimatch reads sp[0] of "" and of the globstar: undefined
		if len(cs) > 0 && string(cs) != "**" {
			r, m := translate(cs, true)
			sub = string(r)
			hasMagic = hasMagic || m
		}
		re = append(re[:reClassStart:reClassStart], []rune(`\[`+sub)...)
	}

	// An open +( at the end: escape the bars.
	for pl := top(); pl != nil; pl = top() {
		stack = stack[:len(stack)-1]
		tail := escapeBars(string(jsSlice(re, pl.reStart+len([]rune(pl.open)), len(re))))
		t := `\` + string(pl.typ)
		switch pl.typ {
		case '*':
			t = star
		case '?':
			t = qmark
		}
		hasMagic = true
		re = append(append([]rune(nil), jsSlice(re, 0, pl.reStart)...), []rune(t+`\(`+tail)...)
	}

	clearState()
	if escaping {
		add(`\\`)
	}

	addPatternStart := len(re) > 0 && (re[0] == '[' || re[0] == '.' || re[0] == '(')

	// A negative list has to look ahead to the end of the pattern.
	for n := len(negs) - 1; n > -1; n-- {
		nl := negs[n]
		before := string(jsSlice(re, 0, nl.reStart))
		first := string(jsSlice(re, nl.reStart, nl.reEnd-8))
		after := string(jsSlice(re, nl.reEnd, len(re)))
		last := string(jsSlice(re, nl.reEnd-8, nl.reEnd)) + after
		openParens := strings.Count(before, "(") - strings.Count(before, ")")
		for i := 0; i < openParens; i++ {
			after = dropCloseParen(after)
		}
		dollar := ""
		if after == "" && !isSub {
			dollar = `(?:$|\/)`
		}
		re = []rune(before + first + after + dollar + last)
	}

	if len(re) > 0 && hasMagic {
		re = append([]rune(notEmpty), re...)
	}
	if addPatternStart && !dotTrav {
		re = append([]rune(`(?!\.)`), re...)
	}
	return re, hasMagic
}

// jsSlice is r.slice(a, b) of JavaScript for non-negative indexes: it never panics. The
// indexes of a negative list can be stale after an open list has changed the text.
func jsSlice(r []rune, a, b int) []rune {
	b = min(b, len(r))
	if a >= b {
		return nil
	}
	return r[a:b]
}

var barRe = regexp.MustCompile(`((?:\\\\){0,64})(\\?)\|`)

// escapeBars escapes each "|" that is not escaped yet, as minimatch does.
func escapeBars(tail string) string {
	return barRe.ReplaceAllStringFunc(tail, func(m string) string {
		sub := barRe.FindStringSubmatch(m)
		slash := sub[2]
		if slash == "" {
			slash = `\`
		}
		return sub[1] + sub[1] + slash + "|"
	})
}

// dropCloseParen is s.replace(/\)[+*?]?/, "").
func dropCloseParen(s string) string {
	i := strings.IndexByte(s, ')')
	if i < 0 {
		return s
	}
	end := i + 1
	if end < len(s) && strings.IndexByte("+*?", s[end]) >= 0 {
		end++
	}
	return s[:i] + s[end:]
}
