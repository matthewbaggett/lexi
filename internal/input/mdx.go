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

// maskMDX blanks the code of an MDX file. It returns the control comments and
// the byte ranges that it blanked.
func maskMDX(content string) (string, []jsMark, [][2]int) {
	out := []byte(content)
	var marks []jsMark
	var masks [][2]int
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
			masks = append(masks, [2]int{pos, next})
		case !isBlank:
			var found []jsMark
			var ranges [][2]int
			found, ranges, next = maskInline(content, out, pos, next)
			marks = append(marks, found...)
			masks = append(masks, ranges...)
			for _, r := range ranges {
				keepLine(out, r)
			}
		}
		blankBefore = isBlank
		pos = next
	}
	return string(out), marks, masks
}

// maskInline blanks the JSX and the expressions that start in a line. An
// expression may run past the end of the line. It returns the control
// comments, the blanked ranges, and the offset where the scan ended.
func maskInline(s string, out []byte, from, to int) ([]jsMark, [][2]int, int) {
	var marks []jsMark
	var masks [][2]int
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
				masks = append(masks, [2]int{i, end + 1})
				i = end
			}
		case c == '<' && isTagStart(s[i+1:]):
			if end := matchTag(s, i); end > 0 {
				copy(out[i:end+1], blank(s[i:end+1]))
				masks = append(masks, [2]int{i, end + 1})
				i = end
			}
		}
	}
	return marks, masks, nextLine(s, i-1)
}

// keepLine stops goldmark from reading two blank bytes at the end of a line as
// a hard line break. When code ends a line that has text, its last byte gets a
// letter. The walker cuts the code out of the text, so the letter never shows.
func keepLine(out []byte, r [2]int) {
	end := r[1]
	if end-r[0] < 2 || (end < len(out) && out[end] != '\n' && out[end] != '\r') {
		return
	}
	if strings.TrimSpace(string(out[strings.LastIndexByte(string(out[:end]), '\n')+1:end])) != "" {
		out[end-1] = 'x'
	}
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

var (
	// quotePrefix and listPrefix strip what a container adds to the start of
	// its lines.
	listPrefix = regexp.MustCompile(`^(?:[ \t]*(?:>|[-+*][ \t]|[0-9]{1,9}[.)][ \t]))*[ \t]*`)
)

// stripQuotes removes q block quote markers from the start of a line. It
// reports false when the line has fewer.
func stripQuotes(line string, q int) (string, bool) {
	for ; q > 0; q-- {
		t := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(t, ">") {
			return line, false
		}
		line = strings.TrimPrefix(t[1:], " ")
	}
	return line, true
}

// mdxTail returns the byte offset where the node after a control comment ends,
// as remark-message-control sees it, or -1 when the comment is the last node of
// its container. body is the file, masks are the ranges that hold code.
func mdxTail(body string, masks [][2]int, m jsMark) int {
	lineStart := strings.LastIndexByte(body[:m.start], '\n') + 1
	lineEnd := nextLine(body, m.end)
	prefix := body[lineStart:m.start]
	if strings.TrimSpace(listPrefix.ReplaceAllString(prefix, "")) != "" || strings.TrimSpace(body[m.end:lineEnd]) != "" {
		return inlineTail(body, masks, m.end)
	}
	return flowTail(body, lineEnd, strings.Count(prefix, ">"))
}

// inlineTail finds the end of the node that follows a comment in a paragraph.
func inlineTail(body string, masks [][2]int, from int) int {
	end := len(body)
	if loc := blankLine.FindStringIndex(body[from:]); loc != nil {
		end = from + loc[0]
	}
	end = len(strings.TrimRight(body[:end], " \t\r\n"))
	if end <= from || strings.TrimSpace(body[from:end]) == "" {
		return -1
	}
	for _, mk := range masks {
		if mk[0] >= from && mk[0] < end {
			if mk[0] == from {
				return mk[1]
			}
			return mk[0]
		}
	}
	return end
}

// flowTail finds the end of the block that follows a comment on its own line.
// The block must be in the same block quote.
func flowTail(body string, pos, quotes int) int {
	for ; pos < len(body); pos = nextLine(body, pos) {
		line, ok := stripQuotes(body[pos:nextLine(body, pos)], quotes)
		if !ok {
			return -1 // a line without the markers ends the quote
		}
		if strings.TrimSpace(line) != "" {
			break
		}
	}
	if pos >= len(body) {
		return -1
	}
	end := pos
	for end < len(body) {
		line, ok := stripQuotes(body[end:nextLine(body, end)], quotes)
		if !ok || strings.TrimSpace(line) == "" {
			break
		}
		end = nextLine(body, end)
	}
	return len(strings.TrimRight(body[:end], " \t\r\n"))
}
