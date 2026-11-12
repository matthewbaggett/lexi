package input

import (
	"regexp"
	"strings"
)

// This file stands in for micromark-extension-mdxjs. It blanks the parts of an
// MDX file that hold code: ESM blocks, JSX tags and expressions. The Markdown
// walker then reads what is left. The blanks keep every offset in place.

var (
	esmStart  = regexp.MustCompile(`^(import|export)[ \t{*]`)
	fenceOpen = regexp.MustCompile("^ {0,3}(```+|~~~+)")
	blankLine = regexp.MustCompile(`\r?\n[ \t]*\r?\n`)
)

// jsMark is a `{/* alex ... */}` comment.
type jsMark struct {
	start, end int // byte offsets of the braces
	value      string
}

// nextLine returns the offset after the line that starts at pos.
func nextLine(s string, pos int) int {
	if i := strings.IndexByte(s[pos:], '\n'); i >= 0 {
		return pos + i + 1
	}
	return len(s)
}

// maskMDX blanks the code of an MDX file. It returns the control comments.
func maskMDX(content string) (string, []jsMark) {
	out := []byte(content)
	var marks []jsMark
	fence := ""
	blankBefore := true
	for pos := 0; pos < len(content); {
		next := nextLine(content, pos)
		line := content[pos:next]
		isBlank := strings.TrimSpace(line) == ""
		switch {
		case fence != "":
			if strings.HasPrefix(strings.TrimLeft(line, " "), fence) {
				fence = ""
			}
		case fenceOpen.MatchString(line):
			fence = fenceOpen.FindStringSubmatch(line)[1]
		case blankBefore && esmStart.MatchString(line):
			// An ESM block runs to the next blank line.
			for next < len(content) && strings.TrimSpace(content[next:nextLine(content, next)]) != "" {
				next = nextLine(content, next)
			}
			copy(out[pos:next], blank(content[pos:next]))
		case !isBlank:
			var found []jsMark
			found, next = maskInline(content, out, pos, next)
			marks = append(marks, found...)
		}
		blankBefore = isBlank
		pos = next
	}
	return string(out), marks
}

// maskInline blanks the JSX and the expressions that start in a line. An
// expression may run past the end of the line. It returns the offset where
// the scan ended.
func maskInline(s string, out []byte, from, to int) ([]jsMark, int) {
	var marks []jsMark
	i := from
	for ; i < to; i++ {
		switch c := s[i]; {
		case c == '\\':
			i++
		case c == '`':
			n := 0
			for i+n < to && s[i+n] == '`' {
				n++
			}
			i += n - 1
			if end := strings.Index(s[i+1:], strings.Repeat("`", n)); end >= 0 {
				i += end + n
			}
		case c == '{':
			if end := matchBrace(s, i); end > 0 {
				if v := strings.TrimSpace(s[i+1 : end]); strings.HasPrefix(v, "/*") && strings.HasSuffix(v, "*/") {
					marks = append(marks, jsMark{i, end + 1, v})
				}
				copy(out[i:end+1], blank(s[i:end+1]))
				i = end
			}
		case c == '<' && isTagStart(s[i+1:]):
			if end := matchTag(s, i); end > 0 {
				copy(out[i:end+1], blank(s[i:end+1]))
				i = end
			}
		}
	}
	return marks, nextLine(s, i-1)
}

// isTagStart reports whether the text after a `<` starts JSX.
func isTagStart(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c == '/' || c == '>' || c == '_' || c == '$' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// matchBrace returns the offset of the brace that closes the one at i, or -1.
func matchBrace(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		case '"', '\'', '`':
			j = skipString(s, j)
		}
	}
	return -1
}

// skipString returns the offset of the quote that closes the string at i.
func skipString(s string, i int) int {
	q := s[i]
	for i++; i < len(s) && s[i] != q; i++ {
		if s[i] == '\\' {
			i++
		}
	}
	return i
}

// matchTag returns the offset of the `>` that closes the tag at i, or -1.
func matchTag(s string, i int) int {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '>':
			return j
		case '{':
			if j = matchBrace(s, j); j < 0 {
				return -1
			}
		case '"', '\'':
			j = skipString(s, j)
		}
	}
	return -1
}

// tailOf returns the byte offset where the block after offset from ends: the
// next blank line, or the end of the file. It returns -1 when no block follows.
func tailOf(masked string, from int) int {
	rest := strings.TrimLeft(masked[from:], " \t\r\n")
	if rest == "" {
		return -1
	}
	begin := len(masked) - len(rest)
	end := len(masked)
	if loc := blankLine.FindStringIndex(masked[begin:]); loc != nil {
		end = begin + loc[0]
	}
	return len(strings.TrimRight(masked[:end], " \t\r\n"))
}
