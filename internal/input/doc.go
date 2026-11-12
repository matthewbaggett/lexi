// Package input turns Markdown, HTML, MDX and plain text into the prose that
// the rules check. It gives each finding the position it has in the source
// file, and it applies the `<!--alex ignore-->` family of comments.
package input

import (
	"sort"
	"strings"
	"unicode/utf16"

	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// Kind is the format of an input.
type Kind int

// The input formats that alex reads.
const (
	Markdown Kind = iota
	Text
	HTML
	MDX
)

// placeholder stands for a node that the rules must see but not read, such as
// inline code. It is a symbol, so it never joins a word.
const placeholder = "￼"

// source indexes a file by UTF-16 offset, as JavaScript does.
type source struct {
	text  string
	u16   []int // u16[i] is the UTF-16 offset of byte i
	lines []int // the UTF-16 offset of the start of each line
}

func newSource(s string) *source {
	src := &source{text: s, u16: make([]int, len(s)+1), lines: []int{0}}
	n := 0
	for i := 0; i < len(s); {
		r, size := decodeRune(s[i:])
		for j := 0; j < size; j++ {
			src.u16[i+j] = n
		}
		n += utf16Len(r)
		i += size
		switch {
		case r == '\n', r == '\r' && !strings.HasPrefix(s[i:], "\n"):
			src.lines = append(src.lines, n)
		}
	}
	src.u16[len(s)] = n
	return src
}

func utf16Len(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// point returns the 1-based line and column of a UTF-16 offset.
func (s *source) point(off int) (line, col int) {
	i := sort.SearchInts(s.lines, off+1) - 1
	return i + 1, off - s.lines[i] + 1
}

// seg maps a part of the prose to the source.
type seg struct {
	synth, n int // byte range in the part
	src      int // UTF-16 offset of the start in the source, or -1
}

// A part is a run of prose that the rules read on its own, like a paragraph
// of HTML. Markdown and plain text are one part.
type part struct {
	text strings.Builder
	segs []seg
}

// A mark is an `alex` control comment.
type mark struct {
	line, col int // where the comment starts
	text      string
	tail      bool // whether the node after the comment is known
	tl, tc    int  // where that node ends
}

// Doc is the prose of one input.
type Doc struct {
	src   *source
	parts []*part
	marks []mark
	gaps  [][2]int // UTF-16 ranges where findings are dropped
	err   *Error
}

// Error is a fatal problem in a file, with a position.
type Error struct {
	Line, Column       int
	EndLine, EndColumn int
	Message            string
}

func (e *Error) Error() string { return e.Message }

type builder struct {
	src   *source
	parts []*part
	marks []mark
	gaps  [][2]int
	err   *Error
}

func (b *builder) cur() *part {
	if len(b.parts) == 0 {
		b.parts = append(b.parts, &part{})
	}
	return b.parts[len(b.parts)-1]
}

// para starts a new part.
func (b *builder) para() { b.parts = append(b.parts, &part{}) }

func (b *builder) add(s string, srcOff int) {
	p := b.cur()
	p.segs = append(p.segs, seg{synth: p.text.Len(), n: len(s), src: srcOff})
	p.text.WriteString(s)
}

// text adds the value of a text node that starts at UTF-16 offset start.
func (b *builder) text(value string, start int) { b.add(value, start) }

// code adds a node that holds no prose.
func (b *builder) code() { b.add(placeholder, -1) }

// space adds white space between nodes.
func (b *builder) space(s string) { b.add(s, -1) }

func (b *builder) doc() *Doc {
	return &Doc{src: b.src, parts: b.parts, marks: b.marks, gaps: b.gaps, err: b.err}
}

// Prose returns the text that the rules read, one part per line.
func (d *Doc) Prose() string {
	var out []string
	for _, p := range d.parts {
		out = append(out, p.text.String())
	}
	return strings.Join(out, "\n--\n")
}

// srcOffset maps a byte offset of a part to a UTF-16 offset of the source.
func (p *part) srcOffset(off int, end bool) int {
	probe := off
	if end {
		probe--
	}
	i := sort.Search(len(p.segs), func(i int) bool { return p.segs[i].synth+p.segs[i].n > probe })
	s := p.segs[i]
	return s.src + len(utf16.Encode([]rune(p.text.String()[s.synth:off])))
}

func (d *Doc) inGap(off int) bool {
	for _, g := range d.gaps {
		if g[0] <= off && g[1] > off {
			return true
		}
	}
	return false
}

// Err returns the fatal error of the file, such as an unknown keyword in a
// control comment. alex reports every finding of such a file, unfiltered.
func (d *Doc) Err() *Error { return d.err }

// Check finds the insensitive phrases in the prose. Positions refer to the
// source file. It applies allow, deny and the control comments.
func (d *Doc) Check(rs []rules.Rule, ws []rules.Word, o rules.Options) ([]rules.Finding, error) {
	if len(o.Allow) > 0 && len(o.Deny) > 0 {
		return nil, rules.ErrAllowDeny
	}
	marks := d.marks
	if d.err != nil {
		marks = nil
	}
	c := newControl(marks, o)
	var out []rules.Finding
	for _, p := range d.parts {
		// It cannot fail: allow and deny are not set.
		found, _ := rules.CheckAll(rs, ws, p.text.String(), rules.Options{NoBinary: o.NoBinary, Ignore: o.Ignore, Sureness: o.Sureness})
		for _, f := range found {
			at := p.srcOffset(f.Start, false)
			if d.inGap(at) {
				continue
			}
			f.Line, f.Column = d.src.point(at)
			f.EndLine, f.EndColumn = d.src.point(p.srcOffset(f.End, true))
			if c.allowed(f) {
				out = append(out, f)
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Line != out[b].Line {
			return out[a].Line < out[b].Line
		}
		return out[a].Column < out[b].Column
	})
	return out, nil
}

// Extract reads the prose of a file of the given kind.
func Extract(kind Kind, content string) *Doc {
	switch kind {
	case Markdown:
		return extractMarkdown(content, false)
	case MDX:
		return extractMarkdown(content, true)
	case HTML:
		return extractHTML(content)
	}
	b := &builder{src: newSource(content)}
	b.text(content, 0)
	return b.doc()
}
