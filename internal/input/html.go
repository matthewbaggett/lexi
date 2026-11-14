package input

import (
	"slices"
	"strings"
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
	// ns is "svg" or "math" for an element of foreign content, and "" for HTML.
	ns    string
	attrs map[string]string
	// positioned is false for an element that parse5 implies.
	positioned bool
	// endTag is true when an end tag in the source closed the element.
	endTag     bool
	start, end int // UTF-16 offsets
	children   []*hnode
	parent     *hnode
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
	if !hasPosition(root) {
		// hast-util-from-parse5 gives the root no position, and hast-util-to-nlcst throws.
		b.err = &Error{Message: "hast-util-to-nlcst expected position on nodes"}
	}
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
			if i+1 < len(n.children) && n.children[i+1].positioned {
				tail = n.children[i+1].end
			}
			c.b.addMark(textMarker, k.value, k.start, k.end, tail)
		}
		c.marks(k)
	}
}

// hasPosition reports whether any node of the tree has a position.
func hasPosition(n *hnode) bool {
	return n.positioned || slices.ContainsFunc(n.children, hasPosition)
}
