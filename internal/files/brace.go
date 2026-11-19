package files

import (
	"strconv"
	"strings"
)

// This file is a port of brace-expansion 2.1.8, the library that minimatch 5.1.9
// uses for {a,b} and {1..5}. It has no step budget: patterns are short.

const (
	braceMax = 100000 // EXPANSION_MAX: the most results of one expansion

	escSlash  = "\x00SLASH\x00"
	escOpen   = "\x00OPEN\x00"
	escClose  = "\x00CLOSE\x00"
	escComma  = "\x00COMMA\x00"
	escPeriod = "\x00PERIOD\x00"
)

// expandBraces expands the braces of a pattern, as minimatch does.
func expandBraces(p string) []string {
	if !hasBraceSet(p) {
		return []string{p}
	}
	return expandTop(p)
}

// hasBraceSet is the test /\{(?:(?!\{).)*\}/ of minimatch: a "{" and then a "}" with no
// "{" or line end between them.
func hasBraceSet(p string) bool {
	open := false
	for _, c := range p {
		switch c {
		case '{':
			open = true
		case '}':
			if open {
				return true
			}
		case '\n', '\r', ' ', ' ':
			open = false
		}
	}
	return false
}

func escapeBraces(s string) string {
	return strings.NewReplacer(`\\`, escSlash, `\{`, escOpen, `\}`, escClose, `\,`, escComma, `\.`, escPeriod).Replace(s)
}

func unescapeBraces(s string) string {
	return strings.NewReplacer(escSlash, `\`, escOpen, "{", escClose, "}", escComma, ",", escPeriod, ".").Replace(s)
}

func expandTop(s string) []string {
	// Bash keeps the first two bytes of a leading {}.
	if strings.HasPrefix(s, "{}") {
		s = `\{\}` + s[2:]
	}
	out := expand(escapeBraces(s), true)
	for i, v := range out {
		out[i] = unescapeBraces(v)
	}
	return out
}

// balancedRange finds the first balanced {...} of a string, as balanced-match does.
func balancedRange(s string) (start, end int, ok bool) {
	ai := strings.IndexByte(s, '{')
	bi := indexFrom(s, '}', ai+1)
	if ai < 0 || bi <= 0 {
		return 0, 0, false
	}
	var begs []int
	left, right := len(s), 0
	i := ai
	for i >= 0 {
		switch {
		case i == ai:
			begs = append(begs, i)
			ai = indexFrom(s, '{', i+1)
		case len(begs) == 1:
			return begs[0], bi, true
		default:
			beg := begs[len(begs)-1]
			begs = begs[:len(begs)-1]
			if beg < left {
				left, right = beg, bi
			}
			bi = indexFrom(s, '}', i+1)
		}
		if ai >= 0 && (ai < bi || bi < 0) {
			i = ai
		} else {
			i = bi
		}
	}
	// The loop starts with one open brace, and never closes the last: some are left.
	return left, right, true
}

// indexFrom is strings.IndexByte from a start index, at most len(s); -1 when not found.
func indexFrom(s string, c byte, from int) int {
	i := strings.IndexByte(s[from:], c)
	if i < 0 {
		return -1
	}
	return i + from
}

// commaSetFollows is /,(?!,).*\}/.test(s): a comma that no comma follows, and a "}" after
// it on the same line.
func commaSetFollows(s string) bool {
	closed := false
	for i := len(s) - 1; i >= 0; i-- {
		switch c := s[i]; {
		case c == '}':
			closed = true
		case c == '\n' || c == '\r':
			closed = false
		case c == ',' && closed && (i+1 >= len(s) || s[i+1] != ','):
			return true
		}
	}
	return false
}

// parseCommaParts is split(",") that keeps nested braces whole.
func parseCommaParts(s string) []string {
	var parts []string
	carry := ""
	for {
		start, end, ok := balancedRange(s)
		if !ok {
			tail := strings.Split(s, ",")
			tail[0] = carry + tail[0]
			return append(parts, tail...)
		}
		p := strings.Split(s[:start], ",")
		p[0] = carry + p[0]
		p[len(p)-1] += "{" + s[start+1:end] + "}"
		post := s[end+1:]
		if post == "" {
			return append(parts, p...)
		}
		carry = p[len(p)-1]
		parts = append(parts, p[:len(p)-1]...)
		s = post
	}
}

// combine builds acc[a] + pre + values[v] for every pair, at most braceMax results.
func combine(acc []string, pre string, values []string, dropEmpties bool) []string {
	var out []string
	var nonEmpty []string
	for _, a := range acc {
		prefix := a + pre
		vs := values
		if dropEmpties && prefix == "" {
			if nonEmpty == nil {
				for _, v := range values {
					if v != "" {
						nonEmpty = append(nonEmpty, v)
					}
				}
			}
			vs = nonEmpty
		}
		for _, v := range vs {
			if len(out) >= braceMax {
				return out
			}
			out = append(out, prefix+v)
		}
	}
	return out
}

// numeric is parseInt(s) when that equals s, and the code of the first character otherwise.
func numeric(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return int(s[0])
}

func isNumericSequence(b string) bool {
	parts := strings.Split(b, "..")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, p := range parts {
		if !isInt(p) {
			return false
		}
	}
	return true
}

func isInt(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isAlphaSequence(b string) bool {
	parts := strings.Split(b, "..")
	if len(parts) < 2 || len(parts) > 3 || !isLetter(parts[0]) || !isLetter(parts[1]) {
		return false
	}
	return len(parts) == 2 || isInt(parts[2])
}

func isLetter(s string) bool {
	return len(s) == 1 && (s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z')
}

// isPadded is /^-?0\d/.
func isPadded(s string) bool {
	s = strings.TrimPrefix(s, "-")
	return len(s) >= 2 && s[0] == '0' && s[1] >= '0' && s[1] <= '9'
}

func expandSequence(body string, alpha bool) []string {
	n := strings.Split(body, "..")
	x, y := numeric(n[0]), numeric(n[1])
	width := max(len(n[0]), len(n[1]))
	incr := 1
	if len(n) == 3 {
		incr = max(abs(numeric(n[2])), 1)
	}
	reverse := y < x
	if reverse {
		incr = -incr
	}
	pad := false
	for _, p := range n {
		pad = pad || isPadded(p)
	}
	var out []string
	for i := x; (i <= y && !reverse || i >= y && reverse) && len(out) < braceMax; i += incr {
		var c string
		if alpha {
			c = string(rune(i))
			if c == `\` {
				c = ""
			}
		} else {
			c = strconv.Itoa(i)
			if need := width - len(c); pad && need > 0 {
				z := strings.Repeat("0", need)
				if i < 0 {
					c = "-" + z + c[1:]
				} else {
					c = z + c
				}
			}
		}
		out = append(out, c)
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func expand(str string, isTop bool) []string {
	acc := []string{""}
	dropEmpties, firstGroup := false, true
	for {
		start, end, ok := balancedRange(str)
		if !ok {
			return combine(acc, str, []string{""}, dropEmpties)
		}
		pre, body, post := str[:start], str[start+1:end], str[end+1:]
		if strings.HasSuffix(pre, "$") {
			acc = combine(acc, pre+"{"+body+"}", []string{""}, dropEmpties && post == "")
			firstGroup = false
			if post == "" {
				break
			}
			str = post
			continue
		}
		numSeq, alphaSeq := isNumericSequence(body), isAlphaSequence(body)
		isSeq := numSeq || alphaSeq
		isOptions := strings.Contains(body, ",")
		if !isSeq && !isOptions {
			// {a},b}
			if commaSetFollows(post) {
				str = pre + "{" + body + escClose + post
				isTop = true
				continue
			}
			return combine(acc, pre+"{"+body+"}"+post, []string{""}, dropEmpties)
		}
		if firstGroup {
			dropEmpties = isTop && !isSeq
			firstGroup = false
		}
		var values []string
		if isSeq {
			values = expandSequence(body, alphaSeq)
		} else {
			n := parseCommaParts(body)
			if len(n) == 1 {
				// x{{a,b}}y ==> x{a}y x{b}y
				inner := expand(n[0], false)
				for i, v := range inner {
					inner[i] = "{" + v + "}"
				}
				n = inner
				if len(n) == 1 {
					acc = combine(acc, pre+n[0], []string{""}, dropEmpties && post == "")
					if post == "" {
						break
					}
					str = post
					continue
				}
			}
			dropsEmpties := dropEmpties && post == "" && pre == ""
			for _, a := range acc {
				dropsEmpties = dropsEmpties && a == ""
			}
			for _, part := range n {
				for _, v := range expand(part, false) {
					if dropsEmpties && v == "" {
						continue
					}
					if len(values) >= braceMax {
						break
					}
					values = append(values, v)
				}
			}
		}
		acc = combine(acc, pre, values, dropEmpties && post == "")
		if post == "" {
			break
		}
		str = post
	}
	return acc
}
