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
			next = esmEnd(content, pos, next)
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

// esmEnd returns the offset where the ESM block that starts at pos ends. next
// is the end of its first line. The block runs to the next blank line, unless a
// statement is still open there: acorn then fails at the end of the text, and
// micromark reads on.
func esmEnd(content string, pos, next int) int {
	for {
		for next < len(content) && strings.TrimSpace(content[next:nextLine(content, next)]) != "" {
			next = nextLine(content, next)
		}
		if next >= len(content) || !jsOpen(content[pos:next]) {
			return next
		}
		next = nextLine(content, next) // the blank line
	}
}

// maskInline blanks the JSX and the expressions that start in a line. An
// expression may run past the end of the line. It returns the control
// comments, the blanked ranges, and the offset where the scan ended.
func maskInline(s string, out []byte, from, to int) ([]jsMark, [][2]int, int) {
	var marks []jsMark
	var masks [][2]int
	i := from
	q := strings.Count(listPrefix.FindString(s[from:to]), ">")
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
			if end := matchBrace(s, i, q); end > 0 {
				if v := strings.TrimSpace(dropQuotes(s[i+1:end], q)); strings.HasPrefix(v, "/*") && strings.HasSuffix(v, "*/") {
					marks = append(marks, jsMark{i, end + 1, v})
				}
				copy(out[i:end+1], blankIn(s, i, end+1))
				masks = append(masks, [2]int{i, end + 1})
				i = end
			}
		case c == '<' && isTagStart(s[i+1:]):
			if end := matchTag(s, i, q); end > 0 {
				copy(out[i:end+1], blankIn(s, i, end+1))
				masks = append(masks, [2]int{i, end + 1})
				i = end
			}
		}
	}
	return marks, masks, nextLine(s, i-1)
}

// blankIn blanks s[from:to]. The code may run over several lines in a block
// quote. The lines after the first keep their quote markers, so goldmark still
// reads them as lines of the quote and gives the text the right columns.
func blankIn(s string, from, to int) string {
	b := []byte(blank(s[from:to]))
	lineStart := strings.LastIndexByte(s[:from], '\n') + 1
	quotes := strings.Count(listPrefix.FindString(s[lineStart:from]), ">")
	for pos := nextLine(s, from); quotes > 0 && pos < to; pos = nextLine(s, pos) {
		line := s[pos:min(nextLine(s, pos), to)]
		if rest, ok := stripQuotes(line, quotes); ok {
			copy(b[pos-from:], line[:len(line)-len(rest)])
		}
	}
	return string(b)
}

// keepLine stops goldmark from reading two blank bytes at the end of a line as
// a hard line break. When code ends a line that has text, its last byte gets a
// letter. The walker cuts the code out of the text, so the letter never shows.
// A line that holds only code, after the markers of its containers, is a flow
// node. It ends the paragraph, so it stays blank.
func keepLine(out []byte, r [2]int) {
	end := r[1]
	if end-r[0] < 2 || (end < len(out) && out[end] != '\n' && out[end] != '\r') {
		return
	}
	line := string(out[strings.LastIndexByte(string(out[:end]), '\n')+1 : end])
	if strings.TrimSpace(listPrefix.ReplaceAllString(line, "")) != "" {
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
// q is the number of block quote markers that start the line of the brace.
func matchBrace(s string, i, q int) int {
	sc := &jsScanner{s: unquoted(s, i, q), i: i + 1}
	sc.code('}')
	if sc.open {
		return -1
	}
	return sc.i - 1
}

// matchTag returns the offset of the `>` that closes the tag at i, or -1.
func matchTag(s string, i, q int) int {
	sc := &jsScanner{s: unquoted(s, i, q), i: i}
	sc.tag()
	if sc.open {
		return -1
	}
	return sc.i - 1
}

// dropQuotes removes the markers of q block quotes from the start of each line
// of s after the first.
func dropQuotes(s string, q int) string {
	if q == 0 || !strings.Contains(s, "\n") {
		return s
	}
	var b strings.Builder
	for pos := 0; pos < len(s); pos = nextLine(s, pos) {
		line := s[pos:nextLine(s, pos)]
		if pos > 0 {
			if rest, ok := stripQuotes(line, q); ok {
				line = rest
			}
		}
		b.WriteString(line)
	}
	return b.String()
}

// unquoted returns the text from i on, with the block quote markers of q
// quotes blanked at the start of each line, so a marker is not a `>`. The text
// ends where a line has fewer markers. The offsets stay the same.
func unquoted(s string, i, q int) string {
	if q == 0 {
		return s
	}
	b := []byte(s)
	for pos := nextLine(s, i); pos < len(s); pos = nextLine(s, pos) {
		line := s[pos:nextLine(s, pos)]
		rest, ok := stripQuotes(line, q)
		if !ok {
			return string(b[:pos])
		}
		for k := range len(line) - len(rest) {
			b[pos+k] = ' '
		}
	}
	return string(b)
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
	quotes := strings.Count(prefix, ">")
	// A comment that opens a list item has the item as its container. The
	// node after it must be indented as far as the item text.
	indent := 0
	if rest, ok := stripQuotes(prefix, quotes); ok && strings.TrimSpace(rest) != "" {
		indent = len(rest)
	}
	return flowTail(body, lineEnd, quotes, indent)
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
// The block must be in the same block quote, and in the same list item: when
// indent is not 0, its lines must be indented that far.
func flowTail(body string, pos, quotes, indent int) int {
	for ; pos < len(body); pos = nextLine(body, pos) {
		line, ok := stripQuotes(body[pos:nextLine(body, pos)], quotes)
		if !ok {
			return -1 // a line without the markers ends the quote
		}
		if strings.TrimSpace(line) != "" {
			if len(line)-len(strings.TrimLeft(line, " ")) < indent {
				return -1 // the item ends
			}
			break
		}
	}
	if pos >= len(body) {
		return -1
	}
	end := pos
	for end < len(body) {
		line, ok := stripQuotes(body[end:nextLine(body, end)], quotes)
		if !ok || strings.TrimSpace(line) == "" || len(line)-len(strings.TrimLeft(line, " ")) < indent {
			break
		}
		end = nextLine(body, end)
	}
	return len(strings.TrimRight(body[:end], " \t\r\n"))
}
