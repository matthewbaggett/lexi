package input

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// This file ports rehype-parse (as far as the tree matters) and
// hast-util-to-nlcst.

type hkind int

const (
	hRoot hkind = iota
	hElement
	hText
	hComment
	hDoctype
)

type hnode struct {
	kind       hkind
	tag, value string
	attrs      map[string]string
	// positioned is false for an element that parse5 implies.
	positioned bool
	start, end int // UTF-16 offsets
	children   []*hnode
	parent     *hnode
}

var (
	voidTags = []string{"area", "base", "basefont", "bgsound", "br", "col", "embed", "frame", "hr", "img", "input", "keygen", "link", "meta", "param", "source", "track", "wbr"}
	headTags = []string{"base", "basefont", "bgsound", "link", "meta", "noscript", "script", "style", "template", "title"}
	// closeP lists the start tags that close an open paragraph.
	closeP  = []string{"address", "article", "aside", "blockquote", "center", "details", "dialog", "dir", "div", "dl", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "hr", "main", "menu", "nav", "ol", "p", "pre", "section", "table", "ul", "li", "dd", "dt", "listing", "xmp"}
	special = []string{"address", "applet", "area", "article", "aside", "base", "basefont", "bgsound", "blockquote", "body", "br", "button", "caption", "center", "col", "colgroup", "dd", "details", "dir", "div", "dl", "dt", "embed", "fieldset", "figcaption", "figure", "footer", "form", "frame", "frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head", "header", "hgroup", "hr", "html", "iframe", "img", "input", "keygen", "li", "link", "listing", "main", "marquee", "menu", "meta", "nav", "noembed", "noframes", "noscript", "object", "ol", "p", "param", "plaintext", "pre", "script", "section", "select", "source", "style", "summary", "table", "tbody", "td", "template", "textarea", "tfoot", "th", "thead", "title", "tr", "track", "ul", "wbr", "xmp"}
	// scope lists the elements that stop the search for an open paragraph.
	scope = []string{"applet", "caption", "html", "table", "td", "th", "marquee", "object", "template", "button"}
)

// treeBuilder builds the tree that parse5 builds: a root with an html element
// that holds a head and a body, whether or not the tags are in the source.
type treeBuilder struct {
	root             *hnode
	html, head, body *hnode
	headClosed       bool
	stack            []*hnode // open elements, root first
}

func (t *treeBuilder) top() *hnode { return t.stack[len(t.stack)-1] }

func (t *treeBuilder) push(n *hnode) {
	n.parent = t.top()
	n.parent.children = append(n.parent.children, n)
}

func (t *treeBuilder) open(n *hnode) {
	t.push(n)
	t.stack = append(t.stack, n)
}

func (t *treeBuilder) pop(to int, end int) {
	for len(t.stack) > to {
		n := t.top()
		if n.end < end {
			n.end = end
		}
		t.stack = t.stack[:len(t.stack)-1]
	}
}

// find returns the index of the nearest open element with a tag in tags,
// searching down to the first element in stop.
func (t *treeBuilder) find(tags, stop []string) int {
	for i := len(t.stack) - 1; i > 0; i-- {
		if slices.Contains(tags, t.stack[i].tag) {
			return i
		}
		if slices.Contains(stop, t.stack[i].tag) {
			return -1
		}
	}
	return -1
}

// element makes an element node. An element without a start tag has no
// position.
func element(tag string, attrs map[string]string, start, end int, positioned bool) *hnode {
	return &hnode{kind: hElement, tag: tag, attrs: attrs, positioned: positioned, start: start, end: end}
}

func (t *treeBuilder) ensureHTML(start, end int, positioned bool) {
	if t.html == nil {
		t.html = element("html", nil, start, end, positioned)
		t.open(t.html)
	}
}

func (t *treeBuilder) ensureHead(start, end int, positioned bool) {
	t.ensureHTML(0, 0, false)
	if t.head == nil {
		t.head = element("head", nil, start, end, positioned)
		t.open(t.head)
	}
}

// closeHead leaves the head.
func (t *treeBuilder) closeHead(at int) {
	t.ensureHead(0, 0, false)
	if !t.headClosed {
		t.headClosed = true
		if i := t.find([]string{"head"}, nil); i > 0 {
			t.pop(i, at)
		}
	}
}

func (t *treeBuilder) ensureBody(start, end int, positioned bool) {
	if t.body == nil {
		t.closeHead(start)
		t.body = element("body", nil, start, end, positioned)
		t.open(t.body)
	}
}

func (t *treeBuilder) startTag(tag string, attrs map[string]string, start, end int, selfClose bool) {
	switch tag {
	case "html":
		t.ensureHTML(start, end, true)
		return
	case "head":
		if t.body == nil && !t.headClosed {
			t.ensureHead(start, end, true)
		}
		return
	case "body":
		t.ensureBody(start, end, true)
		return
	}
	if t.body == nil {
		if slices.Contains(headTags, tag) && !t.headClosed {
			t.ensureHead(0, 0, false)
		} else {
			t.ensureBody(0, 0, false)
		}
	}
	if slices.Contains(closeP, tag) {
		if i := t.find([]string{"p"}, scope); i > 0 {
			t.pop(i, start)
		}
	}
	switch tag {
	case "li":
		t.closeItem([]string{"li"})
	case "dd", "dt":
		t.closeItem([]string{"dd", "dt"})
	case "tr":
		if i := t.find([]string{"tr"}, []string{"table"}); i > 0 {
			t.pop(i, start)
		}
	case "td", "th":
		if i := t.find([]string{"td", "th"}, []string{"table", "tr"}); i > 0 {
			t.pop(i, start)
		}
	case "option":
		if t.top().tag == "option" {
			t.pop(len(t.stack)-1, start)
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		if len(t.top().tag) == 2 && t.top().tag[0] == 'h' && t.top().tag[1] >= '1' && t.top().tag[1] <= '6' {
			t.pop(len(t.stack)-1, start)
		}
	}
	n := element(tag, attrs, start, end, true)
	if slices.Contains(voidTags, tag) || selfClose {
		t.push(n)
	} else {
		t.open(n)
	}
}

// closeItem closes an open list item, or definition, before a sibling opens.
func (t *treeBuilder) closeItem(tags []string) {
	for i := len(t.stack) - 1; i > 0; i-- {
		tag := t.stack[i].tag
		if slices.Contains(tags, tag) {
			t.pop(i, t.top().end)
			return
		}
		if slices.Contains(special, tag) && tag != "address" && tag != "div" && tag != "p" {
			return
		}
	}
}

func (t *treeBuilder) endTag(tag string, end int) {
	switch tag {
	case "html", "body", "br":
		return
	case "head":
		t.closeHead(end)
		return
	}
	if i := t.find([]string{tag}, nil); i > 0 {
		t.pop(i, end)
	}
}

func isSpace(s string) bool { return strings.Trim(s, " \t\n\f\r") == "" }

func (t *treeBuilder) text(value string, start, end int) {
	if lead := len(value) - len(strings.TrimLeft(value, " \t\n\f\r")); t.body == nil && lead > 0 && lead < len(value) {
		// parse5 reads the white space that starts the text on its own.
		t.text(value[:lead], start, start+lead)
		value, start = value[lead:], start+lead
	}
	if t.body == nil {
		switch {
		case !isSpace(value) && slices.Contains([]string{"title", "script", "style", "textarea", "noscript", "template"}, t.top().tag):
		case !isSpace(value):
			t.ensureBody(0, 0, false)
		case t.head != nil: // the text sits in the head, or in html after it
		default:
			return
		}
	}
	if kids := t.top().children; len(kids) > 0 && kids[len(kids)-1].kind == hText {
		kids[len(kids)-1].value += value
		kids[len(kids)-1].end = end
		return
	}
	t.push(&hnode{kind: hText, value: value, positioned: true, start: start, end: end})
}

func parseHTML(content string, src *source) *hnode {
	t := &treeBuilder{root: &hnode{kind: hRoot}}
	t.stack = []*hnode{t.root}
	tz := html.NewTokenizer(strings.NewReader(content))
	off := 0
	for {
		tt := tz.Next()
		if tt == html.ErrorToken {
			break
		}
		raw := tz.Raw()
		start, end := src.u16[off], src.u16[off+len(raw)]
		off += len(raw)
		if t.find([]string{"template"}, nil) > 0 && tt != html.EndTagToken {
			continue // template content is not part of the tree
		}
		switch tt {
		case html.TextToken:
			t.text(string(tz.Text()), start, end)
		case html.CommentToken:
			t.push(&hnode{kind: hComment, value: string(tz.Text()), positioned: true, start: start, end: end})
		case html.DoctypeToken:
			if t.html == nil {
				t.push(&hnode{kind: hDoctype, positioned: true, start: start, end: end})
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, more := tz.TagName()
			attrs := map[string]string{}
			for more {
				var k, v []byte
				k, v, more = tz.TagAttr()
				attrs[string(k)] = string(v)
			}
			t.startTag(string(name), attrs, start, end, tt == html.SelfClosingTagToken)
		case html.EndTagToken:
			name, _ := tz.TagName()
			t.endTag(string(name), end)
		}
	}
	t.ensureBody(0, 0, false)
	t.pop(1, src.u16[len(content)])
	return t.root
}

// gaps finds what remark-message-control calls gaps: the parts of the file
// that no node with a position covers. alex drops the findings there. They
// are the work of the elements that parse5 implies, which have no position.
func gaps(root *hnode) [][2]int {
	var out [][2]int
	offset, gap := 0, false
	update := func(at int, ok bool) {
		switch {
		case !ok:
			gap = true
		case offset < at:
			if gap {
				out = append(out, [2]int{offset, at})
				gap = false
			}
			offset = at
		}
	}
	var visit func(n *hnode)
	visit = func(n *hnode) {
		if n.kind == hRoot {
			update(0, true)
		} else {
			update(n.start, n.positioned)
		}
		if n.kind != hRoot && n.kind != hElement {
			update(n.end, n.positioned)
		}
		for _, k := range n.children {
			visit(k)
		}
	}
	visit(root)
	return out
}

func extractHTML(content string) *Doc {
	src := newSource(content)
	b := &builder{src: src}
	root := parseHTML(content, src)
	c := &converter{b: b}
	c.find(root)
	c.marks(root)
	b.gaps = gaps(root)
	return b.doc()
}

var (
	ignored  = []string{"script", "style", "svg", "math", "del"}
	explicit = []string{"p", "h1", "h2", "h3", "h4", "h5", "h6"}
	flow     = []string{"body", "article", "section", "blockquote", "nav", "aside", "header", "footer", "address", "li", "dt", "dd", "figure", "figcaption", "div", "main", "caption", "td", "th", "form", "fieldset", "details", "dialog"}
	basic    = []string{"a", "abbr", "area", "b", "bdi", "bdo", "br", "button", "cite", "code", "data", "datalist", "del", "dfn", "em", "i", "input", "ins", "kbd", "keygen", "label", "map", "mark", "meter", "noscript", "output", "progress", "q", "ruby", "s", "samp", "script", "select", "small", "span", "strong", "sub", "sup", "template", "textarea", "time", "u", "var", "wbr"}
	embedded = []string{"audio", "canvas", "embed", "iframe", "img", "math", "object", "picture", "svg", "video"}
)

type converter struct{ b *builder }

func (n *hnode) isIgnored() bool {
	return n.kind == hElement && (slices.Contains(ignored, n.tag) || n.attrs["data-nlcst"] == "ignore")
}

func (n *hnode) isEmbedded() bool { return n.kind == hElement && slices.Contains(embedded, n.tag) }

func (n *hnode) isPhrasing() bool {
	if n.kind == hText {
		return true
	}
	if n.kind != hElement {
		return false
	}
	if slices.Contains(basic, n.tag) || n.isEmbedded() {
		return true
	}
	_, itemprop := n.attrs["itemprop"]
	switch n.tag {
	case "meta":
		return itemprop
	case "link":
		if itemprop {
			return true
		}
		rel := strings.Fields(n.attrs["rel"])
		for _, r := range rel {
			if r != "pingback" && r != "prefetch" && r != "stylesheet" {
				return false
			}
		}
		return len(rel) > 0
	}
	return false
}

func (c *converter) find(n *hnode) {
	switch {
	case n.kind == hRoot:
		c.findAll(n.children)
	case n.kind == hElement && !n.isIgnored():
		switch {
		case slices.Contains(explicit, n.tag):
			c.add(n.children)
		case slices.Contains(flow, n.tag):
			c.implicit(flatten(n.children))
		default:
			c.findAll(n.children)
		}
	}
}

func (c *converter) findAll(ns []*hnode) {
	for _, n := range ns {
		c.find(n)
	}
}

func flatten(ns []*hnode) []*hnode {
	var out []*hnode
	for _, n := range ns {
		if n.kind == hElement && (n.tag == "a" || n.tag == "ins" || n.tag == "del" || n.tag == "map") {
			out = append(out, flatten(n.children)...)
		} else {
			out = append(out, n)
		}
	}
	return out
}

func (c *converter) implicit(ns []*hnode) {
	start := -1
	viable := false
	for i := 0; i <= len(ns); i++ {
		var n *hnode
		if i < len(ns) {
			n = ns[i]
		}
		switch {
		case n != nil && n.isPhrasing():
			if start == -1 {
				start = i
			}
			if !viable && !n.isEmbedded() && (n.kind != hText || !isSpace(n.value)) {
				viable = true
			}
		case n != nil && start == -1:
			c.find(n)
			start = i + 1
		case start != -1:
			if viable {
				c.add(ns[start:i])
			} else {
				c.findAll(ns[start:i])
			}
			if n != nil {
				c.find(n)
			}
			viable = false
			start = -1
		}
	}
}

// add emits one paragraph.
func (c *converter) add(ns []*hnode) {
	c.b.para()
	before := len(c.b.cur().segs)
	for _, n := range ns {
		c.one(n)
	}
	if len(c.b.cur().segs) == before {
		c.b.parts = c.b.parts[:len(c.b.parts)-1]
	}
}

func (c *converter) one(n *hnode) {
	switch {
	case n.kind == hText:
		c.b.text(n.value, n.start)
	case n.kind == hElement && !n.isIgnored():
		switch n.tag {
		case "wbr":
			c.b.space(" ")
		case "br":
			c.b.space("\n")
		case "code":
			c.b.code()
		default:
			for _, k := range n.children {
				c.one(k)
			}
		}
	}
}

// marks records the control comments, in document order.
func (c *converter) marks(n *hnode) {
	for i, k := range n.children {
		if k.kind == hComment {
			tail := -1
			if i+1 < len(n.children) {
				tail = n.children[i+1].end
			}
			c.b.addMark(textMarker, k.value, k.start, k.end, tail)
		}
		c.marks(k)
	}
}
