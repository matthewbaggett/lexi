package input

import (
	"html"
	"regexp"
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

// extractMarkdown reads a Markdown or MDX file.
func extractMarkdown(content string, mdx bool) *Doc {
	src := newSource(content)
	b := &builder{src: src}
	var masked string
	var js []jsMark
	if mdx {
		masked, js = maskMDX(content)
	} else {
		masked = maskFrontmatter(content)
	}
	w := &walker{b: b, src: []byte(masked), s: src}
	w.blocks(newParser(mdx).Parse(text.NewReader(w.src)))
	for _, m := range js {
		tail := -1
		if t := tailOf(masked, m.end); t >= 0 {
			tail = src.u16[t]
		}
		b.addMark(jsMarker, m.value, src.u16[m.start], src.u16[m.end], tail)
	}
	return b.doc()
}

type walker struct {
	b   *builder
	src []byte
	s   *source
}

func (w *walker) u16(byteOff int) int { return w.s.u16[byteOff] }

// blocks walks the children of a container block.
func (w *walker) blocks(parent ast.Node) {
	for i, c := range w.children(parent) {
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
		if c := n.LastChild(); c != nil {
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
	if next := n.NextSibling(); next != nil {
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
			var val strings.Builder
			start := -1
			end := 0
			broken := false
			for ; c != nil; c = c.NextSibling() {
				t, ok := c.(*ast.Text)
				if !ok {
					break
				}
				if start < 0 {
					start = w.u16(t.Segment.Start)
				}
				val.WriteString(unescape(string(t.Segment.Value(w.src))))
				end = w.u16(t.Segment.Stop)
				if t.HardLineBreak() {
					broken = true
					c = c.NextSibling()
					break
				}
				if t.SoftLineBreak() {
					after := w.afterLine(t.Segment.Stop)
					val.WriteString(lineEnd.FindString(string(w.src[t.Segment.Stop:after])))
					end = w.u16(after)
				}
			}
			_ = n
			if start >= 0 {
				v := val.String()
				emit(start, end, func() { w.b.text(v, start) })
			}
			if broken {
				w.b.space("\n")
				if len(w.src) > 0 {
					prev = &inline{end, end}
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
		for !end.SoftLineBreak() && !end.HardLineBreak() {
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

// unescape resolves backslash escapes and character references, as the
// Markdown parser does for a text node.
func unescape(s string) string {
	return escapeRe.ReplaceAllStringFunc(s, func(m string) string {
		if m[0] == '\\' {
			return m[1:]
		}
		if d := html.UnescapeString(m); d != m {
			return d
		}
		return m
	})
}
