package input

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// This file builds the tree that parse5 builds. It follows the tree
// construction rules of the HTML standard: the insertion modes, the adoption
// agency algorithm for misnested formatting tags, and foster parenting for
// content that is not allowed in a table. golang.org/x/net/html tokenizes the
// source. It cannot give the tree, because its nodes have no positions.
//
// One shortcut keeps the code short: the content of a template is not part of
// the tree that hast-util-from-parse5 builds, so lexi skips it. rehype-parse
// turns the scripting flag off, so a noscript element holds HTML, not raw text.

// The insertion modes.
const (
	mInitial = iota
	mBeforeHTML
	mBeforeHead
	mInHead
	mInHeadNoscript
	mAfterHead
	mInBody
	mText
	mInTable
	mInTableText
	mInCaption
	mInColumnGroup
	mInTableBody
	mInRow
	mInCell
	mInSelect
	mInSelectInTable
	mAfterBody
	mInFrameset
	mAfterFrameset
	mAfterAfterBody
)

// The scopes of the standard.
const (
	scDefault = iota
	scButton
	scListItem
	scTable
)

// token is a token of the source with its UTF-16 span.
type token struct {
	typ        html.TokenType
	data       string
	attrs      map[string]string
	self       bool
	start, end int
}

// afeEntry is an entry of the list of active formatting elements. A nil el is
// a marker. The token is the start tag that made the element: when the parser
// makes the element again, the new element has the position of that tag.
type afeEntry struct {
	el  *hnode
	tok token
}

type treeParser struct {
	src        *source
	root       *hnode
	oe         []*hnode // the stack of open elements, html first
	afe        []afeEntry
	head, form *hnode
	mode, orig int
	tok        token
	quirks     bool
	framesetOK bool
	foster     bool
	pending    []token // character tokens of a table
	template   int     // how deep the parser is inside templates
	dropNL     bool    // a pre or a listing just opened
	tz         *html.Tokenizer
	size       int // UTF-16 length of the source
}

var (
	// special lists the special elements of the standard.
	special = []string{"address", "applet", "area", "article", "aside", "base", "basefont", "bgsound", "blockquote", "body", "br", "button", "caption", "center", "col", "colgroup", "dd", "details", "dir", "div", "dl", "dt", "embed", "fieldset", "figcaption", "figure", "footer", "form", "frame", "frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head", "header", "hgroup", "hr", "html", "iframe", "img", "input", "keygen", "li", "link", "listing", "main", "marquee", "menu", "meta", "nav", "noembed", "noframes", "noscript", "object", "ol", "p", "param", "plaintext", "pre", "script", "section", "select", "source", "style", "summary", "table", "tbody", "td", "template", "textarea", "tfoot", "th", "thead", "title", "tr", "track", "ul", "wbr", "xmp"}
	// scopeStops lists the elements that end a search in the default scope.
	scopeStops = []string{"applet", "caption", "html", "table", "td", "th", "marquee", "object", "template"}
	// breakout lists the start tags that leave foreign content.
	breakout = []string{"b", "big", "blockquote", "body", "br", "center", "code", "dd", "div", "dl", "dt", "em", "embed", "h1", "h2", "h3", "h4", "h5", "h6", "head", "hr", "i", "img", "li", "listing", "menu", "meta", "nobr", "ol", "p", "pre", "ruby", "s", "small", "span", "strong", "strike", "sub", "sup", "table", "tt", "u", "ul", "var"}
	// impliedEnd lists the elements that end by themselves.
	impliedEnd = []string{"dd", "dt", "li", "optgroup", "option", "p", "rb", "rp", "rt", "rtc"}
)

func (n *hnode) is(tags ...string) bool { return n.ns == "" && slices.Contains(tags, n.tag) }

func (n *hnode) isSpecial() bool {
	switch n.ns {
	case "":
		return slices.Contains(special, n.tag)
	case "math":
		return slices.Contains([]string{"mi", "mo", "mn", "ms", "mtext", "annotation-xml"}, n.tag)
	}
	return slices.Contains([]string{"foreignObject", "desc", "title"}, n.tag)
}

// textPoint reports whether n is a MathML text integration point.
func (n *hnode) textPoint() bool {
	return n.ns == "math" && slices.Contains([]string{"mi", "mo", "mn", "ms", "mtext"}, n.tag)
}

// htmlPoint reports whether n is an HTML integration point.
func (n *hnode) htmlPoint() bool {
	switch n.ns {
	case "svg":
		return slices.Contains([]string{"foreignObject", "desc", "title"}, n.tag)
	case "math":
		enc := strings.ToLower(n.attrs["encoding"])
		return n.tag == "annotation-xml" && (enc == "text/html" || enc == "application/xhtml+xml")
	}
	return false
}

func (p *treeParser) top() *hnode { return p.oe[len(p.oe)-1] }

// current returns the current node, or nil before the first element.
func (p *treeParser) current() *hnode {
	if len(p.oe) == 0 {
		return nil
	}
	return p.top()
}

// inScope reports whether an element with one of the tags is in the scope.
func (p *treeParser) inScope(scope int, tags ...string) bool {
	return p.scopeIndex(scope, tags...) >= 0
}

// scopeIndex returns the index in the stack of the nearest element with one of
// the tags in the scope, or -1. The html element ends every search, so the loop
// never runs past the bottom of the stack.
func (p *treeParser) scopeIndex(scope int, tags ...string) int {
	for i := len(p.oe) - 1; ; i-- {
		n := p.oe[i]
		if n.is(tags...) {
			return i
		}
		if scope == scTable {
			if n.is("html", "table", "template") {
				return -1
			}
			continue
		}
		if n.is(scopeStops...) || n.textPoint() || n.htmlPoint() || n.ns == "math" && n.tag == "annotation-xml" {
			return -1
		}
		if scope == scButton && n.is("button") || scope == scListItem && n.is("ol", "ul") {
			return -1
		}
	}
}

// popTop removes the current node. A node that closes by its own end tag ends
// where the tag ends. Any other node ends where the token that closes it starts.
func (p *treeParser) popTop() {
	n := p.top()
	p.oe = p.oe[:len(p.oe)-1]
	if p.tok.typ == html.EndTagToken && n.ns == "" && p.tok.data == n.tag {
		n.endTag, n.end = true, p.tok.end
	} else {
		n.end = p.tok.start
	}
}

// popUntil pops elements up to the first one with one of the tags, when such an
// element is in the scope. It reports whether it did.
func (p *treeParser) popUntil(scope int, tags ...string) bool {
	i := p.scopeIndex(scope, tags...)
	if i < 0 {
		return false
	}
	for len(p.oe) > i {
		p.popTop()
	}
	return true
}

// popSelect pops elements up to the select element. Only option elements and
// option groups can be above it.
func (p *treeParser) popSelect() {
	for !p.top().is("select") {
		p.popTop()
	}
	p.popTop()
}

// clearBack pops elements until the current node has one of the tags, or is
// the html element or a template.
func (p *treeParser) clearBack(tags ...string) {
	for !p.top().is(tags...) && !p.top().is("html", "template") {
		p.popTop()
	}
}

// impliedEndTags pops the elements that end by themselves, except those in keep.
func (p *treeParser) impliedEndTags(keep ...string) {
	for len(p.oe) > 0 && p.top().is(impliedEnd...) && !p.top().is(keep...) {
		p.popTop()
	}
}

// place returns where a new node goes: a parent and the node it goes before. A
// table that gets content that it does not allow passes the content to the
// parent of the table: that is foster parenting.
func (p *treeParser) place() (*hnode, *hnode) {
	if len(p.oe) == 0 {
		return p.root, nil
	}
	target := p.top()
	if !p.foster || !target.is("table", "tbody", "tfoot", "thead", "tr") {
		return target, nil
	}
	return p.fosterPlace()
}

// fosterPlace returns the parent of the last table in the stack, and the table.
func (p *treeParser) fosterPlace() (*hnode, *hnode) {
	for i := len(p.oe) - 1; ; i-- {
		if t := p.oe[i]; t.is("table") {
			return t.parent, t
		}
	}
}

// insertAt puts a node below a parent, before the node ref when there is one.
func insertAt(parent, ref, n *hnode) {
	n.parent = parent
	if ref == nil {
		parent.children = append(parent.children, n)
		return
	}
	i := slices.Index(parent.children, ref)
	parent.children = slices.Insert(parent.children, i, n)
}

// detach removes a node from its parent.
func detach(n *hnode) {
	if n.parent != nil {
		i := slices.Index(n.parent.children, n)
		n.parent.children = slices.Delete(n.parent.children, i, i+1)
		n.parent = nil
	}
}

func (p *treeParser) insert(n *hnode) {
	parent, ref := p.place()
	insertAt(parent, ref, n)
}

// addText adds characters. They join the text node that ends the place.
func (p *treeParser) addText(s string, start, end int) {
	parent, ref := p.place()
	var prev *hnode
	switch {
	case ref == nil && len(parent.children) > 0:
		prev = parent.children[len(parent.children)-1]
	case ref != nil:
		if i := slices.Index(parent.children, ref); i > 0 {
			prev = parent.children[i-1]
		}
	}
	if prev != nil && prev.kind == hText {
		prev.value += s
		prev.end = end
		return
	}
	insertAt(parent, ref, &hnode{kind: hText, value: s, positioned: true, start: start, end: end})
}

// addElement makes the element of a start tag and opens it.
func (p *treeParser) addElement(t token, ns string) *hnode {
	n := p.newElement(t, ns)
	p.insert(n)
	p.oe = append(p.oe, n)
	return n
}

func (p *treeParser) newElement(t token, ns string) *hnode {
	tag := t.data
	if ns == "svg" && tag == "foreignobject" {
		tag = "foreignObject"
	}
	return &hnode{kind: hElement, tag: tag, ns: ns, attrs: t.attrs, positioned: true, start: t.start, end: t.end}
}

// addFake opens an element that the source does not have. It has no position.
func (p *treeParser) addFake(tag string) *hnode {
	n := &hnode{kind: hElement, tag: tag, attrs: map[string]string{}}
	p.insert(n)
	p.oe = append(p.oe, n)
	return n
}

// addVoid adds an element that has no content.
func (p *treeParser) addVoid(t token) {
	p.addElement(t, "")
	p.popTop()
}

// raw reads the content of a raw text element or an RCDATA element.
func (p *treeParser) raw(t token) {
	p.addElement(t, "")
	p.orig = p.mode
	p.mode = mText
}

func (p *treeParser) comment(t token, parent *hnode) {
	n := &hnode{kind: hComment, value: t.data, positioned: true, start: t.start, end: t.end}
	if parent == nil {
		p.insert(n)
		return
	}
	insertAt(parent, nil, n)
}

// The list of active formatting elements.

func (p *treeParser) afeIndex(n *hnode) int {
	return slices.IndexFunc(p.afe, func(e afeEntry) bool { return e.el == n })
}

func (p *treeParser) afeRemove(n *hnode) {
	if i := p.afeIndex(n); i >= 0 {
		p.afe = slices.Delete(p.afe, i, i+1)
	}
}

func (p *treeParser) clearAFE() {
	for len(p.afe) > 0 {
		e := p.afe[len(p.afe)-1]
		p.afe = p.afe[:len(p.afe)-1]
		if e.el == nil {
			return
		}
	}
}

func sameAttrs(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// addFormatting opens a formatting element. Only three equal ones stay in the
// list after the last marker.
func (p *treeParser) addFormatting(t token) {
	n := p.addElement(t, "")
	count, first := 0, -1
	for i := len(p.afe) - 1; i >= 0 && p.afe[i].el != nil; i-- {
		if e := p.afe[i].el; e.tag == n.tag && sameAttrs(e.attrs, n.attrs) {
			count++
			first = i
		}
	}
	if count >= 3 {
		p.afe = slices.Delete(p.afe, first, first+1)
	}
	p.afe = append(p.afe, afeEntry{n, t})
}

// reconstruct opens the formatting elements again that a block closed.
func (p *treeParser) reconstruct() {
	if len(p.afe) == 0 {
		return
	}
	i := len(p.afe) - 1
	if e := p.afe[i]; e.el == nil || slices.Contains(p.oe, e.el) {
		return
	}
	for i > 0 {
		e := p.afe[i-1]
		if e.el == nil || slices.Contains(p.oe, e.el) {
			break
		}
		i--
	}
	for ; i < len(p.afe); i++ {
		e := p.afe[i]
		n := p.addElement(e.tok, "")
		p.afe[i].el = n
	}
}

// clone makes a copy of an element, with no position and no children.
func clone(n *hnode) *hnode {
	return &hnode{kind: hElement, tag: n.tag, ns: n.ns, attrs: n.attrs}
}

// adopt runs the adoption agency algorithm for the end tag of a formatting
// element.
func (p *treeParser) adopt(tag string) {
	if cur := p.top(); cur.is(tag) && p.afeIndex(cur) < 0 {
		p.popTop()
		return
	}
	for range 8 {
		var fe *hnode
		for i := len(p.afe) - 1; i >= 0 && p.afe[i].el != nil; i-- {
			if p.afe[i].el.tag == tag {
				fe = p.afe[i].el
				break
			}
		}
		if fe == nil {
			p.endOther(tag)
			return
		}
		feIdx := slices.Index(p.oe, fe)
		if feIdx < 0 {
			p.afeRemove(fe)
			return
		}
		if !p.inScope(scDefault, tag) {
			return
		}
		var furthest *hnode
		for _, n := range p.oe[feIdx:] {
			if n.isSpecial() {
				furthest = n
				break
			}
		}
		if furthest == nil {
			for p.top() != fe {
				p.popTop()
			}
			p.popTop()
			p.afeRemove(fe)
			return
		}
		common := p.oe[feIdx-1]
		bookmark := p.afeIndex(fe)
		last := furthest
		idx := slices.Index(p.oe, furthest)
		for inner := 1; ; inner++ {
			idx--
			node := p.oe[idx]
			if node == fe {
				break
			}
			ai := p.afeIndex(node)
			if inner > 3 && ai >= 0 {
				p.afe = slices.Delete(p.afe, ai, ai+1)
				if ai < bookmark {
					bookmark--
				}
				ai = -1
			}
			if ai < 0 {
				p.oe = slices.Delete(p.oe, idx, idx+1)
				continue
			}
			c := clone(node)
			p.afe[ai].el = c
			p.oe[idx] = c
			if last == furthest {
				bookmark = ai + 1
			}
			detach(last)
			insertAt(c, nil, last)
			last = c
		}
		detach(last)
		if p.foster && common.is("table", "tbody", "tfoot", "thead", "tr") {
			parent, ref := p.fosterPlace()
			insertAt(parent, ref, last)
		} else {
			insertAt(common, nil, last)
		}
		c := clone(fe)
		tok := p.afe[p.afeIndex(fe)].tok
		for _, k := range furthest.children {
			k.parent = c
		}
		c.children, furthest.children = furthest.children, nil
		insertAt(furthest, nil, c)
		if i := p.afeIndex(fe); i >= 0 {
			if i < bookmark {
				bookmark--
			}
			p.afe = slices.Delete(p.afe, i, i+1)
		}
		p.afe = slices.Insert(p.afe, bookmark, afeEntry{c, tok})
		p.oe = slices.Delete(p.oe, slices.Index(p.oe, fe), slices.Index(p.oe, fe)+1)
		p.oe = slices.Insert(p.oe, slices.Index(p.oe, furthest)+1, c)
	}
}

// endOther handles the end tag of an element with no rule of its own.
func (p *treeParser) endOther(tag string) {
	for i := len(p.oe) - 1; i > 0; i-- {
		n := p.oe[i]
		// parse5 compares the name only, so an SVG element ends by an HTML tag too.
		if n.tag == tag {
			for len(p.oe) > i {
				p.popTop()
			}
			return
		}
		if n.isSpecial() {
			return
		}
	}
}

// resetMode picks the insertion mode from the stack, after a table or a select
// closed.
func (p *treeParser) resetMode() {
	p.mode = mInBody
	for i := len(p.oe) - 1; i >= 0; i-- {
		switch n := p.oe[i]; {
		case n.is("td", "th"):
			p.mode = mInCell
		case n.is("tr"):
			p.mode = mInRow
		case n.is("tbody", "thead", "tfoot"):
			p.mode = mInTableBody
		case n.is("caption"):
			p.mode = mInCaption
		case n.is("table"):
			p.mode = mInTable
		case n.is("body"):
			return
		default:
			continue
		}
		return
	}
}

// process gives the current token to the insertion mode until a mode takes it.
func (p *treeParser) process() {
	for !p.dispatch() {
	}
}

// dispatch sends the token to the rules for HTML or to the rules for foreign
// content. It reports whether the token is done.
func (p *treeParser) dispatch() bool {
	n := p.current()
	if n == nil || n.ns == "" || p.tok.typ == html.ErrorToken ||
		n.textPoint() && (p.tok.typ == html.StartTagToken && p.tok.data != "mglyph" && p.tok.data != "malignmark" || p.tok.typ == html.TextToken) ||
		n.ns == "math" && n.tag == "annotation-xml" && p.tok.typ == html.StartTagToken && p.tok.data == "svg" ||
		n.htmlPoint() && (p.tok.typ == html.StartTagToken || p.tok.typ == html.TextToken) {
		return p.step()
	}
	return p.foreign()
}

// foreign handles a token in foreign content: SVG or MathML.
func (p *treeParser) foreign() bool {
	t := p.tok
	switch t.typ {
	case html.TextToken:
		p.addText(strings.ReplaceAll(t.data, "\x00", "�"), t.start, t.end)
		if strings.Trim(t.data, " \t\n\f\r") != "" {
			p.framesetOK = false
		}
	case html.CommentToken:
		p.comment(t, nil)
	case html.StartTagToken, html.SelfClosingTagToken:
		if slices.Contains(breakout, t.data) || t.data == "font" && (t.attrs["color"] != "" || t.attrs["face"] != "" || t.attrs["size"] != "") {
			for n := p.top(); n.ns != "" && !n.textPoint() && !n.htmlPoint(); n = p.top() {
				p.popTop()
			}
			return false
		}
		p.addElement(t, p.top().ns)
		if t.typ == html.SelfClosingTagToken {
			p.popTop()
		} else {
			p.tz.NextIsNotRawText()
		}
	case html.EndTagToken:
		for i := len(p.oe) - 1; i > 0; i-- {
			n := p.oe[i]
			if n.ns == "" {
				return p.step()
			}
			if strings.EqualFold(n.tag, t.data) {
				for len(p.oe) > i {
					p.popTop()
				}
				break
			}
		}
	}
	return true
}

var whitespace = " \t\n\f\r"

// splitText cuts the text of a token into runs of white space, runs of NUL and
// runs of other characters, as parse5 emits them.
func (p *treeParser) splitText(t token) []token {
	var out []token
	class := func(c byte) int {
		switch {
		case strings.IndexByte(whitespace, c) >= 0:
			return 0
		case c == 0:
			return 1
		}
		return 2
	}
	at, off := 0, t.start
	for at < len(t.data) {
		end := at
		for end < len(t.data) && class(t.data[end]) == class(t.data[at]) {
			end++
		}
		n := utf16Count(t.data[at:end])
		out = append(out, token{typ: html.TextToken, data: t.data[at:end], start: off, end: min(off+n, t.end)})
		at, off = end, off+n
	}
	if len(out) > 0 {
		out[len(out)-1].end = t.end
	}
	return out
}

func utf16Count(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Len(r)
	}
	return n
}

func isSpace(s string) bool { return strings.Trim(s, whitespace) == "" }

// feed runs the parser on the source.
func (p *treeParser) feed(content string) {
	p.tz = html.NewTokenizer(strings.NewReader(content))
	p.framesetOK = true
	off := 0
	for {
		p.tz.AllowCDATA(p.current() != nil && p.current().ns != "")
		tt := p.tz.Next()
		if tt == html.ErrorToken {
			break
		}
		raw := p.tz.Raw()
		if tt == html.CommentToken && string(raw) == "</>" {
			off += len(raw)
			continue // parse5 drops it, x/net makes an empty comment
		}
		start, end := p.src.u16[off], p.src.u16[off+len(raw)]
		off += len(raw)
		t := token{typ: tt, start: start, end: end}
		switch tt {
		case html.TextToken, html.CommentToken, html.DoctypeToken:
			t.data = string(p.tz.Text())
		case html.StartTagToken, html.SelfClosingTagToken, html.EndTagToken:
			name, more := p.tz.TagName()
			t.data = string(name)
			t.attrs = map[string]string{}
			for more {
				var k, v []byte
				k, v, more = p.tz.TagAttr()
				if _, ok := t.attrs[string(k)]; !ok {
					t.attrs[string(k)] = string(v)
				}
			}
			t.self = tt == html.SelfClosingTagToken
			if t.data == "noscript" && tt != html.EndTagToken {
				p.tz.NextIsNotRawText()
			}
		}
		if p.template > 0 {
			p.inTemplate(t)
			continue
		}
		if tt == html.TextToken {
			for _, part := range p.splitText(t) {
				p.tok = part
				p.process()
			}
			continue
		}
		p.tok = t
		p.process()
		if tt == html.StartTagToken && slices.Contains(rawTags, t.data) && !p.opensRaw(t.data) {
			// x/net reads the text after the tag as raw. The parser ignored the tag.
			p.tz.NextIsNotRawText()
		}
	}
	p.tok = token{typ: html.ErrorToken, start: p.size, end: p.size}
	p.process()
	for len(p.oe) > 0 {
		p.popTop()
	}
}

// rawTags lists the elements that x/net tokenizes as raw text, apart from noscript.
var rawTags = []string{"iframe", "noembed", "noframes", "plaintext", "script", "style", "textarea", "title", "xmp"}

// opensRaw reports whether the parser took the last start tag as a raw text
// element: it opened the element, and it reads text now.
func (p *treeParser) opensRaw(tag string) bool {
	n := p.current()
	return n != nil && n.ns == "" && n.tag == tag && (p.mode == mText || tag == "plaintext")
}

// inTemplate skips the content of a template. hast keeps it out of the tree.
func (p *treeParser) inTemplate(t token) {
	switch {
	case t.typ == html.StartTagToken && t.data == "template":
		p.template++
	case t.typ == html.EndTagToken && t.data == "template":
		p.template--
		if p.template == 0 {
			p.tok = t
			p.endOther("template")
		}
	}
}

func (p *treeParser) startTemplate(t token) {
	p.addElement(t, "")
	p.template++
}

func (p *treeParser) step() bool {
	switch p.mode {
	case mInitial:
		return p.initial()
	case mBeforeHTML:
		return p.beforeHTML()
	case mBeforeHead:
		return p.beforeHead()
	case mInHead:
		return p.inHead()
	case mInHeadNoscript:
		return p.inHeadNoscript()
	case mAfterHead:
		return p.afterHead()
	case mInBody:
		return p.inBody()
	case mText:
		return p.text()
	case mInTable:
		return p.inTable()
	case mInTableText:
		return p.inTableText()
	case mInCaption:
		return p.inCaption()
	case mInColumnGroup:
		return p.inColumnGroup()
	case mInTableBody:
		return p.inTableBody()
	case mInRow:
		return p.inRow()
	case mInCell:
		return p.inCell()
	case mInSelect:
		return p.inSelect()
	case mInSelectInTable:
		return p.inSelectInTable()
	case mAfterBody:
		return p.afterBody()
	case mInFrameset:
		return p.inFrameset()
	case mAfterFrameset:
		return p.afterFrameset()
	}
	return p.afterAfterBody()
}

func (p *treeParser) isStart() bool {
	return p.tok.typ == html.StartTagToken || p.tok.typ == html.SelfClosingTagToken
}

func (p *treeParser) isEnd() bool { return p.tok.typ == html.EndTagToken }

func (p *treeParser) initial() bool {
	switch p.tok.typ {
	case html.TextToken:
		if isSpace(p.tok.data) {
			return true
		}
	case html.CommentToken:
		p.comment(p.tok, p.root)
		return true
	case html.DoctypeToken:
		p.root.children = append(p.root.children, &hnode{kind: hDoctype, positioned: true, start: p.tok.start, end: p.tok.end, parent: p.root})
		p.quirks = isQuirks(p.tok.data)
		p.mode = mBeforeHTML
		return true
	}
	p.quirks = true
	p.mode = mBeforeHTML
	return false
}

func (p *treeParser) beforeHTML() bool {
	switch {
	case p.tok.typ == html.TextToken && isSpace(p.tok.data), p.tok.typ == html.DoctypeToken:
		return true
	case p.tok.typ == html.CommentToken:
		p.comment(p.tok, p.root)
		return true
	case p.isStart() && p.tok.data == "html":
		p.addElement(p.tok, "")
		p.mode = mBeforeHead
		return true
	case p.isEnd() && !slices.Contains([]string{"head", "body", "html", "br"}, p.tok.data):
		return true
	}
	p.addFake("html")
	p.mode = mBeforeHead
	return false
}

func (p *treeParser) beforeHead() bool {
	switch {
	case p.tok.typ == html.TextToken && isSpace(p.tok.data), p.tok.typ == html.DoctypeToken:
		return true
	case p.tok.typ == html.CommentToken:
		p.comment(p.tok, nil)
		return true
	case p.isStart() && p.tok.data == "html":
		return p.inBody()
	case p.isStart() && p.tok.data == "head":
		p.head = p.addElement(p.tok, "")
		p.mode = mInHead
		return true
	case p.isEnd() && !slices.Contains([]string{"head", "body", "html", "br"}, p.tok.data):
		return true
	}
	p.head = p.addFake("head")
	p.mode = mInHead
	return false
}

func (p *treeParser) inHead() bool {
	t := p.tok
	switch {
	case t.typ == html.TextToken:
		if isSpace(t.data) {
			p.addText(t.data, t.start, t.end)
			return true
		}
	case t.typ == html.CommentToken:
		p.comment(t, nil)
		return true
	case t.typ == html.DoctypeToken:
		return true
	case p.isStart():
		switch t.data {
		case "html":
			return p.inBody()
		case "base", "basefont", "bgsound", "link", "meta":
			p.addVoid(t)
			return true
		case "title", "noframes", "style", "script":
			p.raw(t)
			return true
		case "noscript":
			p.addElement(t, "")
			p.mode = mInHeadNoscript
			return true
		case "template":
			p.startTemplate(t)
			return true
		case "head":
			return true
		}
	case p.isEnd():
		switch t.data {
		case "head":
			p.popTop()
			p.mode = mAfterHead
			return true
		case "body", "html", "br":
		default:
			return true
		}
	}
	p.popTop()
	p.mode = mAfterHead
	return false
}

func (p *treeParser) inHeadNoscript() bool {
	t := p.tok
	switch {
	case t.typ == html.DoctypeToken:
		return true
	case p.isStart() && t.data == "html":
		return p.inBody()
	case p.isEnd() && t.data == "noscript":
		p.popTop()
		p.mode = mInHead
		return true
	case t.typ == html.TextToken && isSpace(t.data), t.typ == html.CommentToken,
		p.isStart() && slices.Contains([]string{"basefont", "bgsound", "link", "meta", "noframes", "style"}, t.data):
		return p.inHead()
	case p.isStart() && (t.data == "head" || t.data == "noscript"), p.isEnd() && t.data != "br":
		return true
	}
	p.popTop()
	p.mode = mInHead
	return false
}

func (p *treeParser) afterHead() bool {
	t := p.tok
	switch {
	case t.typ == html.TextToken:
		if isSpace(t.data) {
			p.addText(t.data, t.start, t.end)
			return true
		}
	case t.typ == html.CommentToken:
		p.comment(t, nil)
		return true
	case t.typ == html.DoctypeToken:
		return true
	case p.isStart():
		switch t.data {
		case "html":
			return p.inBody()
		case "body":
			p.addElement(t, "")
			p.framesetOK = false
			p.mode = mInBody
			return true
		case "frameset":
			p.addElement(t, "")
			p.mode = mInFrameset
			return true
		case "base", "basefont", "bgsound", "link", "meta", "noframes", "script", "style", "title", "template":
			// These belong in the head, which is closed. The standard opens it again.
			p.oe = append(p.oe, p.head)
			ok := p.inHead()
			if i := slices.Index(p.oe, p.head); i >= 0 {
				p.oe = slices.Delete(p.oe, i, i+1)
			}
			return ok
		case "head":
			return true
		}
	case p.isEnd():
		if !slices.Contains([]string{"body", "html", "br"}, t.data) {
			return true
		}
	}
	p.addFake("body")
	p.framesetOK = true
	p.mode = mInBody
	return false
}

func (p *treeParser) text() bool {
	t := p.tok
	switch t.typ {
	case html.TextToken:
		d := t.data
		if p.dropNL {
			p.dropNL = false
			if strings.HasPrefix(d, "\n") {
				d = d[1:]
				t.start++
			}
		}
		if d != "" {
			p.addText(d, t.start, t.end)
		}
		return true
	case html.EndTagToken:
		p.dropNL = false
		p.popTop()
		p.mode = p.orig
		return true
	}
	p.popTop()
	p.mode = p.orig
	return false
}

func (p *treeParser) inBody() bool {
	t := p.tok
	switch t.typ {
	case html.TextToken:
		d := strings.ReplaceAll(t.data, "\x00", "")
		if p.dropNL {
			p.dropNL = false
			if strings.HasPrefix(d, "\n") {
				d = d[1:]
				t.start++
			}
		}
		if d == "" {
			return true
		}
		p.reconstruct()
		p.addText(d, t.start, t.end)
		if !isSpace(d) {
			p.framesetOK = false
		}
	case html.CommentToken:
		p.comment(t, nil)
	case html.StartTagToken, html.SelfClosingTagToken:
		p.dropNL = false
		return p.bodyStart()
	case html.EndTagToken:
		p.dropNL = false
		return p.bodyEnd()
	case html.ErrorToken:
		return true
	}
	return true
}

func (p *treeParser) bodyStart() bool {
	t := p.tok
	switch t.data {
	case "html":
	case "base", "basefont", "bgsound", "link", "meta", "noframes", "script", "style", "title":
		return p.inHead()
	case "template":
		p.startTemplate(t)
	case "body":
	case "frameset":
		if len(p.oe) > 1 && p.oe[1].is("body") && p.framesetOK {
			detach(p.oe[1])
			p.oe = p.oe[:1]
			p.addElement(t, "")
			p.mode = mInFrameset
		}
	case "address", "article", "aside", "blockquote", "center", "details", "dialog", "dir", "div", "dl", "fieldset", "figcaption", "figure", "footer", "header", "hgroup", "main", "menu", "nav", "ol", "p", "section", "summary", "ul":
		p.popUntil(scButton, "p")
		p.addElement(t, "")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		p.popUntil(scButton, "p")
		if p.top().is("h1", "h2", "h3", "h4", "h5", "h6") {
			p.popTop()
		}
		p.addElement(t, "")
	case "pre", "listing":
		p.popUntil(scButton, "p")
		p.addElement(t, "")
		p.dropNL = true
		p.framesetOK = false
	case "form":
		if p.form != nil {
			return true
		}
		p.popUntil(scButton, "p")
		p.form = p.addElement(t, "")
	case "li":
		p.framesetOK = false
		p.closeItem("li")
		p.popUntil(scButton, "p")
		p.addElement(t, "")
	case "dd", "dt":
		p.framesetOK = false
		p.closeItem("dd", "dt")
		p.popUntil(scButton, "p")
		p.addElement(t, "")
	case "plaintext":
		p.popUntil(scButton, "p")
		p.addElement(t, "")
	case "button":
		p.popUntil(scDefault, "button")
		p.reconstruct()
		p.addElement(t, "")
		p.framesetOK = false
	case "a":
		for i := len(p.afe) - 1; i >= 0 && p.afe[i].el != nil; i-- {
			if n := p.afe[i].el; n.is("a") {
				p.adopt("a")
				p.afeRemove(n)
				if j := slices.Index(p.oe, n); j >= 0 {
					p.oe = slices.Delete(p.oe, j, j+1)
				}
				break
			}
		}
		p.reconstruct()
		p.addFormatting(t)
	case "b", "big", "code", "em", "font", "i", "s", "small", "strike", "strong", "tt", "u":
		p.reconstruct()
		p.addFormatting(t)
	case "nobr":
		p.reconstruct()
		if p.inScope(scDefault, "nobr") {
			p.adopt("nobr")
			p.reconstruct()
		}
		p.addFormatting(t)
	case "applet", "marquee", "object":
		p.reconstruct()
		p.addElement(t, "")
		p.afe = append(p.afe, afeEntry{})
		p.framesetOK = false
	case "table":
		if !p.quirks {
			p.popUntil(scButton, "p")
		}
		p.addElement(t, "")
		p.framesetOK = false
		p.mode = mInTable
	case "area", "br", "embed", "img", "keygen", "wbr":
		p.reconstruct()
		p.addVoid(t)
		p.framesetOK = false
	case "input":
		p.reconstruct()
		p.addVoid(t)
		if !strings.EqualFold(t.attrs["type"], "hidden") {
			p.framesetOK = false
		}
	case "param", "source", "track":
		p.addVoid(t)
	case "hr":
		p.popUntil(scButton, "p")
		p.addVoid(t)
		p.framesetOK = false
	case "image":
		p.tok.data = "img"
		return false
	case "textarea":
		p.raw(t)
		p.dropNL = true
		p.framesetOK = false
	case "xmp":
		p.popUntil(scButton, "p")
		p.reconstruct()
		p.framesetOK = false
		p.raw(t)
	case "iframe":
		p.framesetOK = false
		p.raw(t)
	case "noembed":
		p.raw(t)
	case "select":
		p.reconstruct()
		p.addElement(t, "")
		p.framesetOK = false
		if slices.Contains([]int{mInTable, mInCaption, mInTableBody, mInRow, mInCell}, p.mode) {
			p.mode = mInSelectInTable
		} else {
			p.mode = mInSelect
		}
	case "optgroup", "option":
		if p.top().is("option") {
			p.popTop()
		}
		p.reconstruct()
		p.addElement(t, "")
	case "rb", "rtc":
		if p.inScope(scDefault, "ruby") {
			p.impliedEndTags()
		}
		p.addElement(t, "")
	case "rp", "rt":
		if p.inScope(scDefault, "ruby") {
			p.impliedEndTags("rtc")
		}
		p.addElement(t, "")
	case "math", "svg":
		p.reconstruct()
		p.addElement(t, t.data)
		if t.typ == html.SelfClosingTagToken {
			p.popTop()
		} else {
			p.tz.NextIsNotRawText()
		}
	case "caption", "col", "colgroup", "frame", "head", "tbody", "td", "tfoot", "th", "thead", "tr":
	default:
		p.reconstruct()
		p.addElement(t, "")
	}
	return true
}

// closeItem closes the open list item or definition before a new one.
func (p *treeParser) closeItem(tags ...string) {
	for i := len(p.oe) - 1; i >= 0; i-- {
		n := p.oe[i]
		if n.is(tags...) {
			for len(p.oe) > i {
				p.popTop()
			}
			return
		}
		if n.isSpecial() && !n.is("address", "div", "p") {
			return
		}
	}
}

func (p *treeParser) bodyEnd() bool {
	t := p.tok
	switch t.data {
	case "body":
		if p.inScope(scDefault, "body") {
			p.markEnd("body")
			p.mode = mAfterBody
		}
	case "html":
		if p.inScope(scDefault, "body") {
			p.markEnd("html")
			p.mode = mAfterBody
			return false
		}
	// parse5 6 leaves button out of this list: </button> takes the rule for any other end tag.
	case "address", "article", "aside", "blockquote", "center", "details", "dialog", "dir", "div", "dl", "fieldset", "figcaption", "figure", "footer", "header", "hgroup", "listing", "main", "menu", "nav", "ol", "pre", "section", "summary", "ul":
		p.popUntil(scDefault, t.data)
	case "form":
		f := p.form
		p.form = nil
		if i := p.scopeIndex(scDefault, "form"); f != nil && i >= 0 && p.oe[i] == f {
			p.oe = slices.Delete(p.oe, i, i+1)
			f.end = t.end
			f.endTag = true
		}
	case "p":
		if !p.inScope(scButton, "p") {
			p.addFake("p")
		}
		p.popUntil(scButton, "p")
	case "li":
		p.popUntil(scListItem, "li")
	case "dd", "dt":
		p.popUntil(scDefault, t.data)
	case "h1", "h2", "h3", "h4", "h5", "h6":
		p.popUntil(scDefault, "h1", "h2", "h3", "h4", "h5", "h6")
	case "a", "b", "big", "code", "em", "font", "i", "nobr", "s", "small", "strike", "strong", "tt", "u":
		p.adopt(t.data)
	case "applet", "marquee", "object":
		if p.popUntil(scDefault, t.data) {
			p.clearAFE()
		}
	case "br":
		// It acts as a start tag, and parse5 inserts the element at once.
		p.reconstruct()
		p.addFake("br")
		p.popTop()
		p.framesetOK = false
	default:
		p.endOther(t.data)
	}
	return true
}

// markEnd records the end tag of the body or the html element. They stay open.
func (p *treeParser) markEnd(tag string) {
	for _, n := range p.oe {
		if n.is(tag) {
			n.endTag, n.end = true, p.tok.end
		}
	}
}

// The table modes.

func (p *treeParser) inTable() bool {
	t := p.tok
	switch {
	case t.typ == html.TextToken:
		if n := p.top(); n.is("table", "tbody", "tfoot", "thead", "tr", "template") {
			p.pending = nil
			p.orig = p.mode
			p.mode = mInTableText
			return false
		}
	case t.typ == html.CommentToken:
		p.comment(t, nil)
		return true
	case t.typ == html.DoctypeToken:
		return true
	case p.isStart():
		switch t.data {
		case "caption":
			p.clearBack("table")
			p.afe = append(p.afe, afeEntry{})
			p.addElement(t, "")
			p.mode = mInCaption
			return true
		case "colgroup":
			p.clearBack("table")
			p.addElement(t, "")
			p.mode = mInColumnGroup
			return true
		case "col":
			p.clearBack("table")
			p.addFake("colgroup")
			p.mode = mInColumnGroup
			return false
		case "tbody", "tfoot", "thead":
			p.clearBack("table")
			p.addElement(t, "")
			p.mode = mInTableBody
			return true
		case "td", "th", "tr":
			p.clearBack("table")
			p.addFake("tbody")
			p.mode = mInTableBody
			return false
		case "table":
			p.popUntil(scTable, "table")
			p.resetMode()
			return false
		case "style", "script":
			return p.inHead()
		case "template":
			p.startTemplate(t)
			return true
		case "input":
			if strings.EqualFold(t.attrs["type"], "hidden") {
				p.addVoid(t)
				return true
			}
		case "form":
			if p.form == nil {
				p.form = p.addElement(t, "")
				p.popTop()
			}
			return true
		}
	case p.isEnd():
		switch t.data {
		case "table":
			if p.popUntil(scTable, "table") {
				p.resetMode()
			}
			return true
		case "body", "caption", "col", "colgroup", "html", "tbody", "td", "tfoot", "th", "thead", "tr":
			return true
		}
	case t.typ == html.ErrorToken:
		return p.inBody()
	}
	p.foster = true
	done := p.inBody()
	p.foster = false
	return done
}

func (p *treeParser) inTableText() bool {
	t := p.tok
	if t.typ == html.TextToken {
		if strings.Trim(t.data, "\x00") != "" {
			p.pending = append(p.pending, t)
		}
		return true
	}
	loud := false
	for _, c := range p.pending {
		if !isSpace(c.data) {
			loud = true
		}
	}
	for _, c := range p.pending {
		if loud {
			p.foster = true
			p.tok = c
			p.inBody()
			p.foster = false
		} else {
			p.addText(c.data, c.start, c.end)
		}
	}
	p.tok = t
	p.mode = p.orig
	return false
}

func (p *treeParser) inCaption() bool {
	t := p.tok
	switch {
	case p.isEnd() && t.data == "caption":
		p.popUntil(scTable, "caption")
		p.clearAFE()
		p.mode = mInTable
		return true
	case p.isStart() && slices.Contains([]string{"caption", "col", "colgroup", "tbody", "td", "tfoot", "th", "thead", "tr"}, t.data),
		p.isEnd() && t.data == "table":
		p.popUntil(scTable, "caption")
		p.clearAFE()
		p.mode = mInTable
		return false
	case p.isEnd() && slices.Contains([]string{"body", "col", "colgroup", "html", "tbody", "td", "tfoot", "th", "thead", "tr"}, t.data):
		return true
	}
	return p.inBody()
}

func (p *treeParser) inColumnGroup() bool {
	t := p.tok
	switch {
	case t.typ == html.TextToken && isSpace(t.data):
		p.addText(t.data, t.start, t.end)
		return true
	case t.typ == html.CommentToken:
		p.comment(t, nil)
		return true
	case t.typ == html.DoctypeToken:
		return true
	case p.isStart() && t.data == "html":
		return p.inBody()
	case p.isStart() && t.data == "col":
		p.addVoid(t)
		return true
	case p.isEnd() && t.data == "colgroup":
		if p.top().is("colgroup") {
			p.popTop()
			p.mode = mInTable
		}
		return true
	case p.isEnd() && t.data == "col":
		return true
	case p.isStart() && t.data == "template":
		p.startTemplate(t)
		return true
	case t.typ == html.ErrorToken:
		return p.inBody()
	}
	p.popTop()
	p.mode = mInTable
	return false
}

func (p *treeParser) inTableBody() bool {
	t := p.tok
	switch {
	case p.isStart() && t.data == "tr":
		p.clearBack("tbody", "tfoot", "thead")
		p.addElement(t, "")
		p.mode = mInRow
		return true
	case p.isStart() && (t.data == "td" || t.data == "th"):
		p.clearBack("tbody", "tfoot", "thead")
		p.addFake("tr")
		p.mode = mInRow
		return false
	case p.isEnd() && slices.Contains([]string{"tbody", "tfoot", "thead"}, t.data):
		if p.inScope(scTable, t.data) {
			p.clearBack("tbody", "tfoot", "thead")
			p.popTop()
			p.mode = mInTable
		}
		return true
	case p.isStart() && slices.Contains([]string{"caption", "col", "colgroup", "tbody", "tfoot", "thead"}, t.data), p.isEnd() && t.data == "table":
		p.clearBack("tbody", "tfoot", "thead")
		p.popTop()
		p.mode = mInTable
		return false
	case p.isEnd() && slices.Contains([]string{"body", "caption", "col", "colgroup", "html", "td", "th", "tr"}, t.data):
		return true
	}
	return p.inTable()
}

func (p *treeParser) inRow() bool {
	t := p.tok
	switch {
	case p.isStart() && (t.data == "td" || t.data == "th"):
		p.clearBack("tr")
		p.addElement(t, "")
		p.mode = mInCell
		p.afe = append(p.afe, afeEntry{})
		return true
	case p.isEnd() && t.data == "tr":
		if p.inScope(scTable, "tr") {
			p.clearBack("tr")
			p.popTop()
			p.mode = mInTableBody
		}
		return true
	case p.isStart() && slices.Contains([]string{"caption", "col", "colgroup", "tbody", "tfoot", "thead", "tr"}, t.data), p.isEnd() && t.data == "table":
		p.clearBack("tr")
		p.popTop()
		p.mode = mInTableBody
		return false
	case p.isEnd() && slices.Contains([]string{"tbody", "tfoot", "thead"}, t.data):
		if p.inScope(scTable, t.data) && p.inScope(scTable, "tr") {
			p.clearBack("tr")
			p.popTop()
			p.mode = mInTableBody
			return false
		}
		return true
	case p.isEnd() && slices.Contains([]string{"body", "caption", "col", "colgroup", "html", "td", "th"}, t.data):
		return true
	}
	return p.inTable()
}

func (p *treeParser) closeCell() {
	p.impliedEndTags()
	p.popUntil(scTable, "td", "th")
	p.clearAFE()
	p.mode = mInRow
}

func (p *treeParser) inCell() bool {
	t := p.tok
	switch {
	case p.isEnd() && (t.data == "td" || t.data == "th"):
		if p.inScope(scTable, t.data) {
			p.closeCell()
		}
		return true
	case p.isStart() && slices.Contains([]string{"caption", "col", "colgroup", "tbody", "td", "tfoot", "th", "thead", "tr"}, t.data):
		p.closeCell()
		return false
	case p.isEnd() && slices.Contains([]string{"body", "caption", "col", "colgroup", "html"}, t.data):
		return true
	case p.isEnd() && slices.Contains([]string{"table", "tbody", "tfoot", "thead", "tr"}, t.data):
		if p.inScope(scTable, t.data) {
			p.closeCell()
			return false
		}
		return true
	}
	return p.inBody()
}

func (p *treeParser) inSelect() bool {
	t := p.tok
	switch {
	case t.typ == html.TextToken:
		if d := strings.ReplaceAll(t.data, "\x00", ""); d != "" {
			p.addText(d, t.start, t.end)
		}
	case t.typ == html.CommentToken:
		p.comment(t, nil)
	case p.isStart():
		switch t.data {
		case "html":
			return p.inBody()
		case "option":
			if p.top().is("option") {
				p.popTop()
			}
			p.addElement(t, "")
		case "optgroup":
			if p.top().is("option") {
				p.popTop()
			}
			if p.top().is("optgroup") {
				p.popTop()
			}
			p.addElement(t, "")
		case "select":
			p.popSelect()
			p.resetMode()
		case "input", "keygen", "textarea":
			p.popSelect()
			p.resetMode()
			return false
		case "script":
			return p.inHead()
		case "template":
			p.startTemplate(t)
		}
	case p.isEnd():
		switch t.data {
		case "optgroup":
			if p.top().is("option") && len(p.oe) > 1 && p.oe[len(p.oe)-2].is("optgroup") {
				p.popTop()
			}
			if p.top().is("optgroup") {
				p.popTop()
			}
		case "option":
			if p.top().is("option") {
				p.popTop()
			}
		case "select":
			p.popSelect()
			p.resetMode()
		}
	case t.typ == html.ErrorToken:
		return p.inBody()
	}
	return true
}

func (p *treeParser) inSelectInTable() bool {
	t := p.tok
	tables := []string{"caption", "table", "tbody", "tfoot", "thead", "tr", "td", "th"}
	switch {
	case p.isStart() && slices.Contains(tables, t.data):
		p.popSelect()
		p.resetMode()
		return false
	case p.isEnd() && slices.Contains(tables, t.data):
		if p.inScope(scTable, t.data) {
			p.popSelect()
			p.resetMode()
			return false
		}
		return true
	}
	return p.inSelect()
}

func (p *treeParser) afterBody() bool {
	t := p.tok
	switch {
	case t.typ == html.TextToken && isSpace(t.data):
		return p.inBody()
	case t.typ == html.CommentToken:
		p.comment(t, p.oe[0])
		return true
	case t.typ == html.DoctypeToken:
		return true
	case p.isStart() && t.data == "html":
		return p.inBody()
	case p.isEnd() && t.data == "html":
		p.mode = mAfterAfterBody
		return true
	case t.typ == html.ErrorToken:
		return true
	}
	p.mode = mInBody
	return false
}

func (p *treeParser) afterAfterBody() bool {
	t := p.tok
	switch {
	case t.typ == html.CommentToken:
		p.comment(t, p.root)
		return true
	case t.typ == html.DoctypeToken, t.typ == html.TextToken && isSpace(t.data), p.isStart() && t.data == "html":
		return p.inBody()
	case t.typ == html.ErrorToken:
		return true
	}
	p.mode = mInBody
	return false
}

func (p *treeParser) inFrameset() bool {
	t := p.tok
	switch {
	case t.typ == html.TextToken:
		if isSpace(t.data) {
			p.addText(t.data, t.start, t.end)
		}
	case t.typ == html.CommentToken:
		p.comment(t, nil)
	case p.isStart() && t.data == "frameset":
		p.addElement(t, "")
	case p.isEnd() && t.data == "frameset":
		if len(p.oe) > 1 {
			p.popTop()
			if !p.top().is("frameset") {
				p.mode = mAfterFrameset
			}
		}
	case p.isStart() && t.data == "frame":
		p.addVoid(t)
	case p.isStart() && t.data == "noframes":
		return p.inHead()
	}
	return true
}

func (p *treeParser) afterFrameset() bool {
	t := p.tok
	switch {
	case t.typ == html.TextToken:
		if isSpace(t.data) {
			p.addText(t.data, t.start, t.end)
		}
	case t.typ == html.CommentToken:
		p.comment(t, nil)
	case p.isEnd() && t.data == "html":
		p.mode = mAfterAfterBody
	case p.isStart() && t.data == "noframes":
		return p.inHead()
	}
	return true
}

// parseHTML builds the tree of a file.
func parseHTML(content string, src *source) *hnode {
	p := &treeParser{src: src, root: &hnode{kind: hRoot}, size: src.u16[len(content)]}
	p.feed(content)
	finish(p.root)
	return p.root
}

// finish gives an element with no end tag the end of its last child, as
// hast-util-from-parse5 does.
func finish(n *hnode) {
	for _, k := range n.children {
		finish(k)
	}
	if n.kind == hElement && n.positioned && !n.endTag && len(n.children) > 0 {
		if tail := n.children[len(n.children)-1]; tail.positioned {
			n.end = tail.end
		}
	}
}

// quirkyIDs lists the public identifiers of a doctype that turn on quirks mode.
var quirkyIDs = []string{
	"+//silmaril//dtd html pro v0r11 19970101//", "-//advasoft ltd//dtd html 3.0 aswedit + extensions//", "-//as//dtd html 3.0 aswedit + extensions//", "-//ietf//dtd html 2.0 level 1//", "-//ietf//dtd html 2.0 level 2//", "-//ietf//dtd html 2.0 strict level 1//", "-//ietf//dtd html 2.0 strict level 2//", "-//ietf//dtd html 2.0 strict//", "-//ietf//dtd html 2.0//", "-//ietf//dtd html 2.1e//", "-//ietf//dtd html 3.0//", "-//ietf//dtd html 3.2 final//", "-//ietf//dtd html 3.2//", "-//ietf//dtd html 3//", "-//ietf//dtd html level 0//", "-//ietf//dtd html level 1//", "-//ietf//dtd html level 2//", "-//ietf//dtd html level 3//", "-//ietf//dtd html strict level 0//", "-//ietf//dtd html strict level 1//", "-//ietf//dtd html strict level 2//", "-//ietf//dtd html strict level 3//", "-//ietf//dtd html strict//", "-//ietf//dtd html//", "-//metrius//dtd metrius presentational//", "-//microsoft//dtd internet explorer 2.0 html strict//", "-//microsoft//dtd internet explorer 2.0 html//", "-//microsoft//dtd internet explorer 2.0 tables//", "-//microsoft//dtd internet explorer 3.0 html strict//", "-//microsoft//dtd internet explorer 3.0 html//", "-//microsoft//dtd internet explorer 3.0 tables//", "-//netscape comm. corp.//dtd html//", "-//netscape comm. corp.//dtd strict html//", "-//o'reilly and associates//dtd html 2.0//", "-//o'reilly and associates//dtd html extended 1.0//", "-//o'reilly and associates//dtd html extended relaxed 1.0//", "-//softquad software//dtd hotmetal pro 6.0::19990601::extensions to html 4.0//", "-//softquad//dtd hotmetal pro 4.0::19971010::extensions to html 4.0//", "-//spyglass//dtd html 2.0 extended//", "-//sq//dtd html 2.0 hotmetal + extensions//", "-//sun microsystems corp.//dtd hotjava html//", "-//sun microsystems corp.//dtd hotjava strict html//", "-//w3c//dtd html 3 1995-03-24//", "-//w3c//dtd html 3.2 draft//", "-//w3c//dtd html 3.2 final//", "-//w3c//dtd html 3.2//", "-//w3c//dtd html 3.2s draft//", "-//w3c//dtd html 4.0 frameset//", "-//w3c//dtd html 4.0 transitional//", "-//w3c//dtd html experimental 19960712//", "-//w3c//dtd html experimental 970421//", "-//w3c//dtd w3 html//", "-//w3o//dtd w3 html 3.0//", "-//webtechs//dtd mozilla html 2.0//", "-//webtechs//dtd mozilla html//",
}

// isQuirks reads the content of a doctype and reports whether it turns on quirks
// mode. Only quirks mode matters: a table keeps an open paragraph there.
func isQuirks(s string) bool {
	space := strings.IndexAny(s, whitespace)
	if space < 0 {
		space = len(s)
	}
	quirks := !strings.EqualFold(s[:space], "html")
	s = strings.TrimSpace(s[space:])
	if s == "" {
		return quirks
	}
	key := strings.ToLower(s[:min(6, len(s))])
	if key != "public" && key != "system" {
		return true
	}
	s = s[6:]
	// quoted reads an identifier in quotes. It fails when no quote starts the text.
	quoted := func() (string, bool) {
		s = strings.TrimLeft(s, whitespace)
		if s == "" || s[0] != '"' && s[0] != '\'' {
			return "", false
		}
		quote := s[0]
		id := s[1:]
		s = ""
		if q := strings.IndexByte(id, quote); q >= 0 {
			id, s = id[:q], id[q+1:]
		}
		return id, true
	}
	first, ok := quoted()
	if !ok {
		return true
	}
	public, system := "", first
	if key == "public" {
		public, system = strings.ToLower(first), ""
		if strings.TrimSpace(s) != "" {
			if system, ok = quoted(); !ok {
				return true
			}
		}
	}
	if slices.Contains([]string{"-//w3o//dtd w3 html strict 3.0//en//", "-/w3c/dtd html 4.0 transitional/en", "html"}, public) {
		return true
	}
	for _, q := range quirkyIDs {
		if strings.HasPrefix(public, q) {
			return true
		}
	}
	// These two public identifiers turn on quirks mode when no system identifier follows.
	if system == "" && (strings.HasPrefix(public, "-//w3c//dtd html 4.01 frameset//") || strings.HasPrefix(public, "-//w3c//dtd html 4.01 transitional//")) {
		return true
	}
	return quirks || strings.EqualFold(system, "http://www.ibm.com/data/dtd/v11/ibmxhtml1-transitional.dtd")
}
