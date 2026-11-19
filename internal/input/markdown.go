package input

import (
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func decodeRune(s string) (rune, int) { return utf8.DecodeRuneInString(s) }

// This file ports remark-parse, remark-gfm, remark-frontmatter and
// mdast-util-to-nlcst: it keeps the text nodes and turns the rest of the
// document into white space or placeholders.

func newParser(mdx bool) parser.Parser {
	if !mdx {
		return goldmark.New(goldmark.WithExtensions(
			extension.Table, extension.Strikethrough, extension.TaskList, extension.Footnote,
		)).Parser()
	}
	return parser.NewParser(
		parser.WithBlockParsers(
			util.Prioritized(parser.NewSetextHeadingParser(), 100),
			util.Prioritized(parser.NewThematicBreakParser(), 200),
			util.Prioritized(parser.NewListParser(), 300),
			util.Prioritized(parser.NewListItemParser(), 400),
			util.Prioritized(parser.NewATXHeadingParser(), 600),
			util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
			util.Prioritized(parser.NewBlockquoteParser(), 800),
			util.Prioritized(indentedParagraph{parser.NewParagraphParser()}, 1000),
		),
		parser.WithInlineParsers(
			util.Prioritized(parser.NewCodeSpanParser(), 100),
			util.Prioritized(parser.NewLinkParser(), 200),
			util.Prioritized(parser.NewEmphasisParser(), 500),
		),
	)
}

// indentedParagraph reads a line with four spaces or more as a paragraph. MDX
// has no indented code.
type indentedParagraph struct{ parser.BlockParser }

func (indentedParagraph) CanAcceptIndentedLine() bool { return true }

// maskFrontmatter blanks a YAML or TOML block at the start of a file.
func maskFrontmatter(s string) string {
	var fence string
	switch {
	case strings.HasPrefix(s, "---"):
		fence = "---"
	case strings.HasPrefix(s, "+++"):
		fence = "+++"
	default:
		return s
	}
	lines := strings.SplitAfter(s, "\n")
	if strings.TrimRight(strings.TrimSpace(lines[0]), "\r") != fence {
		return s
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == fence {
			n := 0
			for _, l := range lines[:i+1] {
				n += len(l)
			}
			return blank(s[:n]) + s[n:]
		}
	}
	return s
}

// blank replaces every byte of s but the line breaks with a space.
func blank(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c != '\n' && c != '\r' {
			b[i] = ' '
		}
	}
	return string(b)
}

var (
	// goldmark does not see an empty list item that a CRLF ends.
	emptyItemCRLF = regexp.MustCompile(`(?m)^([ \t]*(?:>[ \t]*)*(?:[-+*]|[0-9]{1,9}[.)]))\r\n`)
	footnoteDef   = regexp.MustCompile(`(?m)^ {0,3}\[\^([^\]\s]+)\]:`)
)

// withFootnoteRefs adds a reference to each footnote definition at the end of
// the source. goldmark drops a definition that nothing refers to, and mdast
// keeps it. The walker ignores the added paragraph.
func withFootnoteRefs(src string) string {
	var refs strings.Builder
	for _, m := range footnoteDef.FindAllStringSubmatch(src, -1) {
		refs.WriteString("[^" + m[1] + "]")
	}
	if refs.Len() == 0 {
		return src
	}
	return src + "\n\n" + refs.String()
}

// extractMarkdown reads a Markdown or MDX file.
func extractMarkdown(content string, mdx bool) *Doc {
	// micromark drops a byte order mark, but alex still counts it in the
	// columns: its offsets are one short.
	body := strings.TrimPrefix(content, "\ufeff")
	src := newSource(body)
	b := &builder{src: src}
	var masked string
	var js []jsMark
	var masks [][2]int
	if mdx {
		// alex stops at the first syntax error, and it reports no finding.
		if e := mdxErrors(body, src); e != nil {
			b.err = e
			return b.doc()
		}
		masked, js, masks = maskMDX(body)
	} else {
		masked = maskFrontmatter(body)
	}
	masked = emptyItemCRLF.ReplaceAllString(masked, "$1 \n")
	w := &walker{b: b, src: []byte(masked), s: src, n: len(masked), masks: masks}
	if !mdx {
		w.src = []byte(withFootnoteRefs(masked))
	}
	w.blocks(newParser(mdx).Parse(text.NewReader(w.src)))
	for _, m := range js {
		tail := -1
		if t := mdxTail(body, masks, m); t >= 0 {
			tail = src.u16[t]
		}
		b.addMark(jsMarker, m.value, src.u16[m.start], src.u16[m.end], tail)
	}
	if body != content {
		b.src = newSource(content)
	}
	return b.doc()
}

type walker struct {
	b   *builder
	src []byte
	s   *source
	n   int // the length of the file: what follows in src is added text
	// masks are the byte ranges of MDX code. They hold no text.
	masks [][2]int
}

func (w *walker) u16(byteOff int) int { return w.s.u16[byteOff] }

// blocks walks the children of a container block.
func (w *walker) blocks(parent ast.Node) {
	for i, c := range w.children(parent) {
		if s, ok := w.blockStart(c); ok && s >= w.n {
			continue // the references that withFootnoteRefs added
		}
		if i > 0 {
			w.b.space("\n\n")
		}
		switch n := c.(type) {
		case *ast.Paragraph, *ast.TextBlock, *ast.Heading:
			w.inlines(n)
		case *ast.Blockquote, *ast.List, *ast.ListItem, *east.Footnote, *east.FootnoteList:
			w.blocks(n)
		case *ast.HTMLBlock:
			w.htmlBlock(n)
		}
	}
}

// children lists the blocks of a container. goldmark moves footnote
// definitions to the end of the document; mdast keeps them in place.
func (w *walker) children(parent ast.Node) []ast.Node {
	var out, notes []ast.Node
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if list, ok := c.(*east.FootnoteList); ok {
			for f := list.FirstChild(); f != nil; f = f.NextSibling() {
				notes = append(notes, f)
			}
			continue
		}
		out = append(out, c)
	}
	for _, f := range notes {
		at := len(out)
		for i, c := range out {
			if s, ok := w.blockStart(c); ok && s > w.mustStart(f) {
				at = i
				break
			}
		}
		out = append(out[:at], append([]ast.Node{f}, out[at:]...)...)
	}
	return out
}

func (w *walker) mustStart(n ast.Node) int {
	s, _ := w.blockStart(n)
	return s
}

// blockStart returns the byte offset of the first line below a block.
func (w *walker) blockStart(n ast.Node) (int, bool) {
	if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
		return n.Lines().At(0).Start, true
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s, ok := w.blockStart(c); ok {
			return s, true
		}
	}
	return 0, false
}

var (
	fenceLine = regexp.MustCompile(`^ {0,3}(` + "```+|~~~+" + `)`)
	lineEnd   = regexp.MustCompile(`\r\n|\n|\r`)
)

// blockEnd returns the byte offset where a block ends, or -1.
func (w *walker) blockEnd(n ast.Node) int {
	if n.Lines().Len() == 0 {
		if c := lastBlock(n); c != nil {
			return w.blockEnd(c)
		}
		return w.breakEnd(n) // the only block without lines or children
	}
	last := n.Lines().At(n.Lines().Len() - 1)
	end := last.Stop
	for end > last.Start && (w.src[end-1] == '\n' || w.src[end-1] == '\r') {
		end--
	}
	if _, ok := n.(*ast.FencedCodeBlock); ok {
		rest := string(w.src[last.Stop:])
		if fenceLine.MatchString(rest) {
			if loc := lineEnd.FindStringIndex(rest); loc != nil {
				return last.Stop + loc[0]
			}
			return len(w.src)
		}
	}
	return end
}

// lastBlock returns the last child of n that is a block. goldmark ends a
// footnote with an inline back link, which has no lines.
func lastBlock(n ast.Node) ast.Node {
	c := n.LastChild()
	for c != nil && c.Type() != ast.TypeBlock {
		c = c.PreviousSibling()
	}
	return c
}

// nextBlock returns the block that follows n in the source. goldmark moves
// footnote definitions to the end of the document, so its next sibling is not
// always the next block.
func (w *walker) nextBlock(n ast.Node) ast.Node {
	list := w.children(n.Parent())
	i := slices.Index(list, n)
	for i++; i < len(list); i++ {
		if list[i].Type() == ast.TypeBlock {
			return list[i]
		}
	}
	return nil
}

var thematic = regexp.MustCompile(`(?m)^ {0,3}(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)

// breakEnd finds the end of a thematic break, which goldmark gives no position:
// it is the first break in the source after the block before it.
func (w *walker) breakEnd(n ast.Node) int {
	from := 0
	if p := n.PreviousSibling(); p != nil {
		from = max(w.blockEnd(p), 0)
	}
	loc := thematic.FindIndex(w.src[from:])
	if loc == nil {
		return -1
	}
	return from + loc[1]
}

func (w *walker) htmlBlock(n *ast.HTMLBlock) {
	var sb strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		l := n.Lines().At(i)
		sb.Write(l.Value(w.src))
	}
	if n.HasClosure() {
		cl := n.ClosureLine
		sb.Write(cl.Value(w.src))
	}
	start := n.Lines().At(0).Start
	for start < len(w.src) && (w.src[start] == ' ' || w.src[start] == '\t') {
		start++
	}
	tail := -1
	if next := w.nextBlock(n); next != nil && w.mustStart(next) < w.n {
		if e := w.blockEnd(next); e >= 0 {
			tail = w.u16(e)
		}
	}
	w.b.addMark(htmlMarker, strings.TrimRight(sb.String(), "\r\n"), w.u16(start), w.u16(w.htmlEnd(n)), tail)
}

// htmlEnd returns the byte offset where an HTML block ends.
func (w *walker) htmlEnd(n *ast.HTMLBlock) int {
	last := n.Lines().At(n.Lines().Len() - 1)
	if n.HasClosure() {
		last = n.ClosureLine
	}
	end := last.Stop
	for end > last.Start && (w.src[end-1] == '\n' || w.src[end-1] == '\r') {
		end--
	}
	return end
}

// maskIn reports whether MDX code overlaps the byte range [from, to).
func (w *walker) maskIn(from, to int) bool {
	for _, m := range w.masks {
		if m[0] < to && m[1] > from {
			return true
		}
	}
	return false
}

// piece is a run of text that one text node of mdast holds.
type piece struct {
	v  string
	at int // UTF-16 offset of the start in the source
}

// split gives the text of a segment to add, without the MDX code in it. The code
// ends the run, and the text after it starts a new one, as in mdast: the code is
// a node of its own, and it adds no white space.
func (w *walker) split(gap, from, to int, add func(string, int), cut func()) {
	if w.maskIn(gap, from) {
		cut() // code sits between the segments
	}
	pos := from
	for _, m := range w.masks {
		if m[1] <= pos || m[0] >= to {
			continue
		}
		if m[0] > pos {
			add(unescape(string(w.src[pos:m[0]])), pos)
		}
		cut()
		pos = m[1]
	}
	if pos < to {
		add(unescape(string(w.src[pos:to])), pos)
	}
}

// inline is one node of a paragraph, the way mdast sees it.
type inline struct {
	start, end int // UTF-16 offsets
}

// inlines emits the prose of a node that holds inline children.
func (w *walker) inlines(parent ast.Node) {
	var prev *inline
	emit := func(start, end int, f func()) {
		if prev != nil && start >= 0 {
			pl, _ := w.s.point(prev.end)
			if sl, _ := w.s.point(start); sl != pl {
				w.b.space(strings.Repeat("\n", max(2, sl-pl)))
			}
		}
		f()
		if start >= 0 {
			prev = &inline{start, end}
		}
	}
	for c := parent.FirstChild(); c != nil; {
		switch n := c.(type) {
		case *ast.Text, *ast.String:
			var pieces []piece
			open := false
			add := func(v string, at int) {
				if !open {
					pieces = append(pieces, piece{at: w.u16(at)})
					open = true
				}
				pieces[len(pieces)-1].v += v
			}
			start := -1
			end := 0
			broken := false
			lineAfter := 0 // where the line after the hard break starts
			gap := 0       // where the previous segment stopped
			for ; c != nil; c = c.NextSibling() {
				t, ok := c.(*ast.Text)
				if !ok {
					break
				}
				if start < 0 {
					start = w.u16(t.Segment.Start)
					gap = t.Segment.Start
				}
				w.split(gap, t.Segment.Start, t.Segment.Stop, add, func() { open = false })
				end = w.u16(t.Segment.Stop)
				if t.HardLineBreak() {
					broken = true
					lineAfter = w.u16(w.afterLine(t.Segment.Stop))
					c = c.NextSibling()
					break
				}
				if t.SoftLineBreak() {
					after := w.afterLine(t.Segment.Stop)
					nl := lineEnd.FindString(string(w.src[t.Segment.Stop:after]))
					add(nl, after-len(nl))
					end = w.u16(after)
					gap = after
				} else {
					gap = t.Segment.Stop
				}
			}
			_ = n
			if start >= 0 {
				emit(start, end, func() {
					for _, p := range pieces {
						w.b.text(p.v, p.at)
					}
				})
			}
			if broken {
				w.b.space("\n")
				if len(w.src) > 0 {
					prev = &inline{lineAfter, lineAfter}
				}
			}
			continue
		case *ast.CodeSpan:
			s, e := w.span(n)
			emit(s, e, w.b.code)
		case *ast.RawHTML:
			s, e := w.span(n)
			emit(s, e, func() {})
			w.rawHTMLMark(n)
		case *ast.Image:
			s, e := w.span(n)
			emit(s-1, e+1, func() { w.b.text(plainText(w.src, n), s+1) })
		case *ast.AutoLink:
			label := string(n.Label(w.src))
			s, e := w.autoLink(n, label)
			emit(s, e, func() { w.b.text(label, s) })
		case *ast.Link, *ast.Emphasis, *east.Strikethrough:
			s, e := w.span(n)
			emit(s, e, func() { w.inlines(n) })
		}
		c = c.NextSibling()
	}
}

func (w *walker) rawHTMLMark(n *ast.RawHTML) {
	var sb strings.Builder
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		sb.Write(seg.Value(w.src))
	}
	tail := -1
	if next := n.NextSibling(); next != nil {
		_, e := w.runSpan(next)
		tail = e
	}
	w.b.addMark(htmlMarker, sb.String(), w.u16(n.Segments.At(0).Start), w.u16(n.Segments.At(n.Segments.Len()-1).Stop), tail)
}

// runSpan returns the UTF-16 span of a node, with text runs joined.
func (w *walker) runSpan(n ast.Node) (int, int) {
	if t, ok := n.(*ast.Text); ok {
		end := t
		for !end.HardLineBreak() {
			nx, ok := end.NextSibling().(*ast.Text)
			if !ok {
				break
			}
			end = nx
		}
		return w.u16(t.Segment.Start), w.u16(end.Segment.Stop)
	}
	return w.span(n)
}

// afterLine returns the byte offset after the line break that follows off.
func (w *walker) afterLine(off int) int {
	return off + lineEnd.FindIndex(w.src[off:])[1]
}

// span returns the UTF-16 range of the text below a node, widened by one
// delimiter on each side.
func (w *walker) span(n ast.Node) (int, int) {
	first, last := -1, 0
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		switch t := c.(type) {
		case *ast.Text:
			if entering {
				if first < 0 {
					first = t.Segment.Start
				}
				last = t.Segment.Stop
			}
		case *ast.RawHTML:
			if entering && t.Segments.Len() > 0 {
				if first < 0 {
					first = t.Segments.At(0).Start
				}
				last = t.Segments.At(t.Segments.Len() - 1).Stop
			}
		}
		return ast.WalkContinue, nil
	})
	if first < 0 {
		return -1, -1
	}
	return w.u16(max(first-1, 0)), w.u16(min(last+1, len(w.src)))
}

func (w *walker) autoLink(n *ast.AutoLink, label string) (int, int) {
	from := 0
	if p := n.PreviousSibling(); p != nil {
		if t, ok := p.(*ast.Text); ok {
			from = t.Segment.Stop
		}
	}
	s := from + strings.Index(string(w.src[from:]), "<"+label+">") + 1
	return w.u16(s), w.u16(s + len(label))
}

// plainText is the text of a node without any markup, as mdast-util-to-string
// gives it.
func plainText(src []byte, n ast.Node) string {
	var sb strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			sb.WriteString(unescape(string(t.Segment.Value(src))))
			if t.SoftLineBreak() || t.HardLineBreak() {
				sb.WriteString("\n")
			}
		}
		return ast.WalkContinue, nil
	})
	return sb.String()
}

var (
	escapeRe = regexp.MustCompile("\\\\([!-/:-@\\[-`{-~])|&(?:#[xX][0-9a-fA-F]{1,6}|#[0-9]{1,7}|[a-zA-Z][a-zA-Z0-9]{1,31});")
)

// numericRef decodes a numeric character reference the way micromark does: a
// code that is not allowed gives U+FFFD, and a code above U+FFFF keeps only its
// low 16 bits, because micromark calls String.fromCharCode.
func numericRef(m string) string {
	digits, base := m[2:len(m)-1], 10
	if digits[0] == 'x' || digits[0] == 'X' {
		digits, base = digits[1:], 16
	}
	// The pattern limits the digits, so the number fits.
	code, _ := strconv.ParseInt(digits, base, 64)
	if code < 9 || code == 11 || (code > 13 && code < 32) || (code > 126 && code < 160) ||
		(code > 55295 && code < 57344) || (code > 64975 && code < 65008) ||
		code&0xFFFF == 0xFFFF || code&0xFFFF == 0xFFFE || code > 1114111 {
		return "\uFFFD"
	}
	return string(rune(code & 0xFFFF))
}

// unescape resolves backslash escapes and character references, as the
// Markdown parser does for a text node.
func unescape(s string) string {
	return escapeRe.ReplaceAllStringFunc(s, func(m string) string {
		switch {
		case m[0] == '\\':
			return m[1:]
		case m[1] == '#':
			return numericRef(m)
		}
		// html.UnescapeString also decodes the old entities that need no
		// semicolon, such as &not, at the start of a longer name. micromark
		// decodes a whole name only. A known entity gives one or two characters.
		if d := html.UnescapeString(m); d != m && utf8.RuneCountInString(d) <= 2 {
			return d
		}
		return m
	})
}
