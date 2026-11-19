package input

import (
	"regexp"
	"strconv"
	"strings"
)

// This file finds the syntax errors that alex reports for an MDX file, and
// that need no JavaScript parser. micromark finds them in two stages. The first
// stage is the tokenizer: it reads every tag and expression of the file, and
// the first error in file order wins. The second stage builds the tree, and it
// matches the tags. An error of the first stage always beats one of the second.
//
// lexi does not build the tree of micromark. It reads the lines of the file,
// finds where each block starts and ends, and runs the tag tokenizer of
// mdxjsx.go on the text of each block. See docs/Investigations/MDX.md.

var (
	headingLine  = regexp.MustCompile(`^#{1,6}(?:[ \t]|\r?\n?$)`)
	thematicLine = regexp.MustCompile(`^(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})\r?\n?$`)
	listMarker   = regexp.MustCompile(`^(?:[-+*]|[0-9]{1,9}[.)])`)
	// A definition holds its destination, so a `<` in it is no tag.
	definitionLine = regexp.MustCompile(`^\[[^\]\n]*\]:[ \t]*<`)
)

// mdxLine is what the scan knows about a line before it reads the next one.
type mdxLine struct {
	start   int   // byte offset of the line
	q       int   // block quote markers
	lead    int   // column of the first character after the quote markers and the spaces
	cs      int   // offset in the line where the content starts, after list markers too
	items   []int // the width of each list marker in the line, as a column
	seq     []int // the markers in order: -1 for a block quote marker, else the width of a list marker
	blank   bool  // nothing follows the markers
	block   bool  // the content is a heading, a thematic break, a fence or a definition
	heading bool
	fence   string // the fence that the line opens
}

type mdxEvent struct {
	tag   *mdxTag // a tag, or
	exit  string  // the end of a container of this type
	start int     // and where that container started
}

type mdxCont struct {
	typ   string
	start int
	width int
}

type mdxScan struct {
	body    string
	nb      string // body, with the quote markers blanked
	lines   []mdxLine
	events  []mdxEvent
	pending []mdxEvent // the tags of a flow construct, until it is known to be one
}

// mdxErrors returns the syntax error of an MDX file, or nil. It also returns
// nil when alex stops with an error that lexi cannot word.
func mdxErrors(body string, src *source) *Error {
	c := &mdxScan{body: body}
	c.prepare()
	if prob := c.tokenize(); prob != nil {
		return c.fail(prob.msg)
	}
	return c.fail(c.tree(src))
}

func (c *mdxScan) fail(msg string) *Error {
	if msg == "" {
		return nil
	}
	// The CLI of alex gives no position for these errors. It prints 1:1.
	return &Error{Line: 1, Column: 1, Message: msg}
}

// prepare reads the lines. It blanks the block quote markers in a copy of the
// file, so that a marker is not a `>` of a tag.
func (c *mdxScan) prepare() {
	nb := []byte(c.body)
	for pos := 0; pos < len(c.body); {
		next := nextLine(c.body, pos)
		line := c.body[pos:next]
		l := mdxLine{start: pos, lead: -1}
		i := 0
		for {
			j := i
			for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
			if j < len(line) && line[j] == '>' {
				l.q++
				l.seq = append(l.seq, -1)
				for k := i; k <= j; k++ {
					nb[pos+k] = ' '
				}
				i = j + 1
				if i < len(line) && line[i] == ' ' {
					i++
				}
				continue
			}
			if l.lead < 0 {
				l.lead = j
			}
			if n := markerLen(line[j:]); n > 0 && !thematicLine.MatchString(strings.TrimRight(line[j:], "\r\n")) {
				i = j + n
				l.items = append(l.items, i)
				l.seq = append(l.seq, i)
				continue
			}
			i = j
			break
		}
		l.cs = i
		content := line[i:]
		l.blank = strings.TrimSpace(content) == ""
		if !l.blank {
			if m := fenceOpen.FindStringSubmatch(content); m != nil {
				l.fence = m[1]
			}
			l.heading = headingLine.MatchString(content)
			l.block = l.heading || l.fence != "" || definitionLine.MatchString(content) ||
				thematicLine.MatchString(strings.TrimRight(content, "\r\n"))
		}
		c.lines = append(c.lines, l)
		pos = next
	}
	c.nb = string(nb)
}

// markerLen returns the length of a list marker and the spaces after it at the
// start of s, or 0 when s does not start with one.
func markerLen(s string) int {
	m := listMarker.FindString(s)
	if m == "" || len(s) > len(m) && s[len(m)] != ' ' && s[len(m)] != '\t' && s[len(m)] != '\r' && s[len(m)] != '\n' {
		return 0
	}
	n := len(m)
	for n < len(s) && (s[n] == ' ' || s[n] == '\t') {
		n++
	}
	return n
}

// lineAt returns the index of the line that holds offset off.
func (c *mdxScan) lineAt(off int) int {
	lo, hi := 0, len(c.lines)
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if c.lines[mid].start <= off {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

func (c *mdxScan) lineEnd(i int) int {
	if i+1 < len(c.lines) {
		return c.lines[i+1].start
	}
	return len(c.body)
}

// trimEOL removes the line ending at the end of s[:end].
func (c *mdxScan) trimEOL(end int) int {
	for end > 0 && (c.body[end-1] == '\n' || c.body[end-1] == '\r') {
		end--
	}
	return end
}

// containerEnd returns where the block that starts in line i must end: the
// start of the first line that leaves the block quote or the list item.
func (c *mdxScan) containerEnd(i, q, width int) int {
	if q == 0 && width == 0 {
		return len(c.body)
	}
	for j := i + 1; j < len(c.lines); j++ {
		l := c.lines[j]
		if l.q < q || (!l.blank && width > 0 && l.lead < width) {
			return l.start
		}
	}
	return len(c.body)
}

// paragraphEnd returns where the paragraph that starts in line i ends. The
// paragraph runs to the next blank line, or to the line that starts a block. A
// line that holds only a tag or an expression is a flow construct: it ends the
// paragraph too.
func (c *mdxScan) paragraphEnd(i, width int) int {
	q := c.lines[i].q
	j := i + 1
	for ; j < len(c.lines); j++ {
		l := c.lines[j]
		if l.blank || l.block || len(l.items) > 0 || l.fence != "" || l.q > q {
			break
		}
		if ch := c.nb[l.start+l.cs]; ch == '<' || ch == '{' {
			if _, ok, prob := c.flow(l.start+l.cs, c.containerEnd(j, l.q, width)); ok || prob != nil {
				break
			}
		}
	}
	return c.trimEOL(c.lineEnd(j - 1))
}

// tokenize reads the whole file. It returns the first error of micromark.
func (c *mdxScan) tokenize() *mdxProblem {
	var stack []mdxCont
	blankBefore := true
	fence := ""
	quotes := func() int {
		n := 0
		for _, k := range stack {
			if k.typ == "blockQuote" {
				n++
			}
		}
		return n
	}
	pop := func() {
		k := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		c.events = append(c.events, mdxEvent{exit: k.typ, start: k.start})
	}
	for i := 0; i < len(c.lines); i++ {
		l := c.lines[i]
		rawLine := c.body[l.start:c.lineEnd(i)]
		isBlank := strings.TrimSpace(rawLine) == ""
		switch {
		case fence != "":
			if strings.HasPrefix(strings.TrimLeft(rawLine[min(l.cs, len(rawLine)):], " "), fence) {
				fence = ""
			}
			blankBefore = isBlank
			continue
		case l.fence != "":
			fence = l.fence
			blankBefore = false
			continue
		case blankBefore && esmStart.MatchString(rawLine):
			end := esmEnd(c.body, l.start, c.lineEnd(i))
			i = c.lineAt(end) - 1
			if end >= len(c.body) {
				i = len(c.lines)
			}
			blankBefore = false
			continue
		}
		blankBefore = isBlank
		for quotes() > l.q {
			pop()
		}
		if l.blank {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].typ == "listItem" && stack[len(stack)-1].width > l.lead {
			pop()
		}
		seen := 0
		for _, w := range l.seq {
			switch {
			case w < 0:
				if seen++; seen > quotes() {
					stack = append(stack, mdxCont{typ: "blockQuote", start: l.start})
				}
			default:
				stack = append(stack, mdxCont{typ: "listItem", start: l.start, width: w})
			}
		}
		cs := l.start + l.cs
		switch {
		case l.heading:
			if prob := c.text(cs, c.trimEOL(c.lineEnd(i))); prob != nil {
				return prob
			}
			c.events = append(c.events, mdxEvent{exit: "heading", start: cs})
			continue
		case l.block:
			continue
		}
		width := 0
		if len(stack) > 0 && stack[len(stack)-1].typ == "listItem" {
			width = stack[len(stack)-1].width
		}
		if ch := c.nb[cs]; ch == '<' || ch == '{' {
			end, ok, prob := c.flow(cs, c.containerEnd(i, l.q, width))
			if prob != nil {
				return prob
			}
			if ok {
				c.events = append(c.events, c.pending...)
				i = c.lineAt(max(end-1, cs))
				continue
			}
		}
		end := c.paragraphEnd(i, width)
		if prob := c.text(cs, end); prob != nil {
			return prob
		}
		c.events = append(c.events, mdxEvent{exit: "paragraph", start: cs})
		i = c.lineAt(max(end-1, cs))
	}
	for len(stack) > 0 {
		pop()
	}
	return nil
}

// flow reads a flow construct that starts at i: a line that holds only tags,
// or only an expression. It reads on past blank lines, up to lim. It reports
// false when the text after the construct is more text. The text tokenizer then
// reads the line. The events of the tags wait in c.pending.
func (c *mdxScan) flow(i, lim int) (int, bool, *mdxProblem) {
	b := c.nb[:lim]
	c.pending = nil
	for {
		last := byte('>')
		if b[i] == '{' {
			end, prob := readExpr(b, i, lim, true)
			if prob != nil {
				return 0, false, prob
			}
			i, last = end, '}'
		} else {
			t, prob := parseTag(b, i, lim)
			if prob != nil || t == nil {
				return 0, false, prob
			}
			c.pending = append(c.pending, mdxEvent{tag: t})
			i = t.end
		}
		j := i
		for j < lim && (b[j] == ' ' || b[j] == '\t') {
			j++
		}
		switch {
		case j < lim && b[j] == '<' && last == '>':
			i = j
		case j >= lim || b[j] == '\n' || b[j] == '\r':
			return j, true, nil
		default:
			return 0, false, nil
		}
	}
}

// text reads the tags and the expressions in the text of a paragraph, from
// from to lim.
func (c *mdxScan) text(from, lim int) *mdxProblem {
	b := c.nb[:lim]
	for i := from; i < lim; i++ {
		switch b[i] {
		case '\\':
			i++
		case '`':
			n := 0
			for i+n < lim && b[i+n] == '`' {
				n++
			}
			i += n - 1
			for j := i + 1; j < lim; j++ {
				if b[j] != '`' {
					continue
				}
				m := 0
				for j+m < lim && b[j+m] == '`' {
					m++
				}
				if m == n {
					i = j + m - 1
					break
				}
				j += m - 1
			}
		case '(':
			// The destination of a link may be in angle brackets.
			if i > 0 && b[i-1] == ']' && i+1 < lim && b[i+1] == '<' {
				if end := strings.IndexAny(b[i+2:lim], ">\n<"); end >= 0 && b[i+2+end] == '>' {
					i += end + 2
				}
			}
		case '{':
			end, prob := readExpr(b, i, lim, true)
			if prob != nil {
				return prob
			}
			i = end - 1
		case '<':
			t, prob := parseTag(b, i, lim)
			if prob != nil {
				return prob
			}
			if t != nil {
				c.events = append(c.events, mdxEvent{tag: t})
				i = t.end - 1
			}
		}
	}
	return nil
}

// tree matches the tags, as mdast-util-mdx-jsx does. It returns the first
// error.
func (c *mdxScan) tree(src *source) string {
	var stack []*mdxTag
	var last *mdxTag
	pos := func(from, to int) string {
		l1, c1 := src.point(src.u16[from])
		l2, c2 := src.point(src.u16[to])
		return strconv.Itoa(l1) + ":" + strconv.Itoa(c1) + "-" + strconv.Itoa(l2) + ":" + strconv.Itoa(c2)
	}
	abbr := func(t *mdxTag) string {
		s := "<"
		if t.close {
			s += "/"
		}
		return s + t.name + ">"
	}
	unclosed := func(open *mdxTag, container string) string {
		msg := "Expected a closing tag for `" + abbr(last) + "` (" + pos(open.start, open.end) + ")"
		if container != "" {
			msg += " before the end of `" + container + "`"
		}
		return msg
	}
	for _, ev := range c.events {
		if ev.tag == nil {
			if n := len(stack); n > 0 && stack[n-1].start >= ev.start {
				return unclosed(stack[n-1], ev.exit)
			}
			continue
		}
		t := ev.tag
		last = t
		if !t.close {
			if !t.self {
				stack = append(stack, t)
			}
			continue
		}
		if len(stack) == 0 {
			return "Unexpected closing slash `/` in tag, expected an open tag first"
		}
		switch {
		case t.attrAt >= 0 && (t.selfAt < 0 || t.attrAt < t.selfAt):
			return "Unexpected attribute in closing tag, expected the end of the tag"
		case t.selfAt >= 0:
			return "Unexpected self-closing slash `/` in closing tag, expected the end of the tag"
		}
		tail := stack[len(stack)-1]
		if tail.name != t.name {
			return "Unexpected closing tag `" + abbr(t) + "`, expected corresponding closing tag for `" + abbr(tail) + "` (" + pos(tail.start, tail.end) + ")"
		}
		stack = stack[:len(stack)-1]
	}
	if n := len(stack); n > 0 {
		return unclosed(stack[n-1], "")
	}
	return ""
}
