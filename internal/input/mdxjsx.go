package input

// This file ports the tag tokenizer of micromark-extension-mdx-jsx 1.0.5 (the
// file lib/factory-tag.js). It reads one JSX tag, and it reports the first
// error with the exact message of alex. It has no JavaScript parser.
//
// Parts of this file follow the source of that package. Its licence is below.
//
// The MIT License (MIT)
//
// Copyright (c) 2020 Titus Wormer <tituswormer@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// mdxProblem is an error that alex reports. An empty message means that alex
// stops with an error whose text lexi does not know: an acorn error.
type mdxProblem struct{ msg string }

// mdxTag is a JSX tag that the tokenizer read.
type mdxTag struct {
	name       string
	close      bool
	self       bool
	start, end int // byte offsets of the `<` and after the `>`
	closeMark  int // byte offset of the closing slash
	attrAt     int // byte offset of the first attribute, or -1
	selfAt     int // byte offset of the self-closing slash, or -1
}

const (
	nameExpect = "a name character such as letters, digits, `$`, or `_`; whitespace before attributes; or the end of the tag"
	attrExpect = "a character that can start an attribute name, such as a letter, `$`, or `_`; whitespace before attributes; or the end of the tag"
	attrAfter  = "a character that can start an attribute name, such as a letter, `$`, or `_`; `=` to initialize a value; or the end of the tag"
	nameStart  = "a character that can start a name, such as a letter, `$`, or `_`"
)

func idStart(r rune) bool {
	return r == '$' || r == '_' || unicode.IsLetter(r) || unicode.Is(unicode.Nl, r)
}

func idCont(r rune) bool {
	return idStart(r) || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc) || r == 0x200c || r == 0x200d
}

func lineEnding(r rune) bool { return r == '\n' || r == '\r' }
func mdSpace(r rune) bool    { return r == ' ' || r == '\t' }
func uniSpace(r rune) bool   { return r > 0 && unicode.IsSpace(r) }

// endsName reports whether r is white space or one of the characters of set.
func endsName(r rune, set string) bool {
	return r >= 0 && (strings.ContainsRune(set, r) || lineEnding(r) || mdSpace(r) || uniSpace(r))
}

// startsAttr reports whether r can follow a name: the next attribute, or the end of the tag.
func startsAttr(r rune) bool {
	return r == '/' || r == '>' || r == '{' || r >= 0 && idStart(r)
}

// tagParser reads a tag from a buffer. A character at lim or after is the end
// of the file.
type tagParser struct {
	s   string
	i   int
	lim int
	tag *mdxTag
}

// peek returns the next character, or -1 at the end.
func (p *tagParser) peek() rune {
	if p.i >= p.lim {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(p.s[p.i:p.lim])
	return r
}

func (p *tagParser) next() {
	_, n := utf8.DecodeRuneInString(p.s[p.i:p.lim])
	p.i += n
}

// ws skips white space, with line endings.
func (p *tagParser) ws() {
	for r := p.peek(); r >= 0 && (lineEnding(r) || mdSpace(r) || uniSpace(r)); r = p.peek() {
		p.next()
	}
}

func (p *tagParser) crash(r rune, at, expect string) *mdxProblem {
	got := "end of file"
	if r >= 0 {
		ch := string(r)
		if r == '`' {
			ch = "` ` `" // micromark prints a backtick in a way that shows it
		}
		got = fmt.Sprintf("character `%s` (U+%04X)", ch, r)
	}
	return &mdxProblem{"Unexpected " + got + " " + at + ", expected " + expect}
}

// name reads the characters of a name after its first character.
func (p *tagParser) name() {
	for r := p.peek(); r == '-' || r >= 0 && idCont(r); r = p.peek() {
		p.next()
	}
}

// parseTag reads the tag whose `<` is at i. It returns a nil tag and no error
// when the text is not a tag: a space or a line ending follows the `<`.
func parseTag(s string, i, lim int) (*mdxTag, *mdxProblem) {
	p := &tagParser{s: s, i: i + 1, lim: lim, tag: &mdxTag{start: i, closeMark: -1, attrAt: -1, selfAt: -1}}
	if r := p.peek(); lineEnding(r) || mdSpace(r) {
		return nil, nil
	}
	p.ws()
	t := p.tag
	r := p.peek()
	switch {
	case r == '/':
		t.close = true
		t.closeMark = p.i
		p.next()
		p.ws()
		r = p.peek()
		switch {
		case r == '>':
			return p.end()
		case r >= 0 && idStart(r):
		default:
			note := ""
			if r == '*' || r == '/' {
				note = " (note: JS comments in JSX tags are not supported in MDX)"
			}
			return nil, p.crash(r, "before name", nameStart+note)
		}
	case r == '>':
		return p.end()
	case r >= 0 && idStart(r):
	default:
		note := ""
		if r == '!' {
			note = " (note: to create a comment in MDX, use `{/* text */}`)"
		}
		return nil, p.crash(r, "before name", nameStart+note)
	}
	if prob := p.tagName(); prob != nil {
		return nil, prob
	}
	return p.attributes()
}

func (p *tagParser) end() (*mdxTag, *mdxProblem) {
	p.next() // the `>`
	p.tag.end = p.i
	return p.tag, nil
}

// tagName reads the name, with its members and its prefix.
func (p *tagParser) tagName() *mdxProblem {
	start := p.i
	p.next()
	p.name()
	// primary name, then whitespace
	r := p.peek()
	if !endsName(r, "./:>{") {
		note := ""
		if r == '@' {
			note = " (note: to create a link in MDX, use `[text](url)`)"
		}
		return p.crash(r, "in name", nameExpect+note)
	}
	p.ws()
	r = p.peek()
	switch r {
	case '.':
		for {
			p.next()
			p.ws()
			r = p.peek()
			if r < 0 || !idStart(r) {
				return p.crash(r, "before member name", attrExpect)
			}
			p.next()
			p.name()
			r = p.peek()
			if !endsName(r, "./>{") {
				note := ""
				if r == '@' {
					note = " (note: to create a link in MDX, use `[text](url)`)"
				}
				return p.crash(r, "in member name", nameExpect+note)
			}
			p.ws()
			r = p.peek()
			if r != '.' {
				break
			}
		}
		if !startsAttr(r) {
			return p.crash(r, "after member name", attrExpect)
		}
	case ':':
		p.next()
		p.ws()
		r = p.peek()
		if r < 0 || !idStart(r) {
			note := ""
			if r == '+' || r > '.' && r < ':' {
				note = " (note: to create a link in MDX, use `[text](url)`)"
			}
			return p.crash(r, "before local name", nameStart+note)
		}
		p.next()
		p.name()
		r = p.peek()
		if !endsName(r, "/>{") {
			return p.crash(r, "in local name", nameExpect)
		}
		p.ws()
		r = p.peek()
		if !startsAttr(r) {
			return p.crash(r, "after local name", attrExpect)
		}
	default:
		if !startsAttr(r) {
			return p.crash(r, "after name", attrExpect)
		}
	}
	p.tag.name = p.s[start:p.i]
	p.tag.name = strings.Join(strings.Fields(p.tag.name), "")
	return nil
}

// attributes reads what follows the name, up to the end of the tag.
func (p *tagParser) attributes() (*mdxTag, *mdxProblem) {
	t := p.tag
	for {
		r := p.peek()
		switch {
		case r == '/':
			if t.selfAt < 0 {
				t.selfAt = p.i
			}
			t.self = true
			p.next()
			p.ws()
			r = p.peek()
			if r == '>' {
				return p.end()
			}
			note := ""
			if r == '*' || r == '/' {
				note = " (note: JS comments in JSX tags are not supported in MDX)"
			}
			return nil, p.crash(r, "after self-closing slash", "`>` to end the tag"+note)
		case r == '>':
			return p.end()
		case r == '{':
			if t.attrAt < 0 {
				t.attrAt = p.i
			}
			if prob := p.expression(); prob != nil {
				return nil, prob
			}
			p.ws()
		case r >= 0 && idStart(r):
			if t.attrAt < 0 {
				t.attrAt = p.i
			}
			if prob := p.attribute(); prob != nil {
				return nil, prob
			}
		default:
			return nil, p.crash(r, "before attribute name", attrExpect)
		}
	}
}

// expression reads an attribute expression. An empty one is an error.
func (p *tagParser) expression() *mdxProblem {
	end, prob := readExpr(p.s, p.i, p.lim, false)
	if prob != nil {
		return prob
	}
	p.i = end
	return nil
}

// attribute reads one attribute, with its value.
func (p *tagParser) attribute() *mdxProblem {
	p.next()
	p.name()
	r := p.peek()
	if !endsName(r, "/:=>{") {
		return p.crash(r, "in attribute name", "an attribute name character such as letters, digits, `$`, or `_`; `=` to initialize a value; whitespace before attributes; or the end of the tag")
	}
	p.ws()
	r = p.peek()
	switch {
	case r == ':':
		p.next()
		p.ws()
		r = p.peek()
		if r < 0 || !idStart(r) {
			return p.crash(r, "before local attribute name", attrAfter)
		}
		p.next()
		p.name()
		r = p.peek()
		if !endsName(r, "/=>{") {
			return p.crash(r, "in local attribute name", "an attribute name character such as letters, digits, `$`, or `_`; `=` to initialize a value; whitespace before attributes; or the end of the tag")
		}
		p.ws()
		r = p.peek()
		if r == '=' {
			return p.value()
		}
		if !startsAttr(r) {
			return p.crash(r, "after local attribute name", attrAfter)
		}
	case r == '=':
		return p.value()
	case startsAttr(r):
	default:
		return p.crash(r, "after attribute name", attrAfter)
	}
	return nil
}

// value reads `=` and the value.
func (p *tagParser) value() *mdxProblem {
	p.next() // the `=`
	p.ws()
	r := p.peek()
	switch r {
	case '"', '\'':
		q := r
		p.next()
		for {
			r = p.peek()
			if r < 0 {
				return p.crash(r, "in attribute value", "a corresponding closing quote `"+string(q)+"`")
			}
			p.next()
			if r == q {
				break
			}
		}
		p.ws()
		return nil
	case '{':
		end, prob := readExpr(p.s, p.i, p.lim, false)
		if prob != nil {
			return prob
		}
		p.i = end
		p.ws()
		return nil
	}
	note := ""
	if r == '<' {
		note = " (note: to use an element or fragment as a prop value in MDX, use `{<element />}`)"
	}
	return p.crash(r, "before attribute value", "a character that can start an attribute value, such as `\"`, `'`, or `{`"+note)
}

// readExpr reads the expression whose `{` is at i, as factory-mdx-expression
// does. It returns the offset after the `}`. micromark asks acorn for each `}`
// if the text so far is an expression. lexi has no parser: it takes the first
// `}` that closes the braces, and it does not check the code.
func readExpr(s string, i, lim int, emptyOK bool) (int, *mdxProblem) {
	sc := &jsScanner{s: s[:lim], i: i + 1}
	sc.code('}')
	if sc.open {
		if strings.Contains(s[i+1:lim], "}") {
			return 0, &mdxProblem{} // acorn fails, and its text is not known
		}
		return 0, &mdxProblem{"Unexpected end of file in expression, expected a corresponding closing brace for `{`"}
	}
	if !emptyOK && jsEmpty(s[i+1:sc.i-1]) {
		return 0, &mdxProblem{"Unexpected empty expression"}
	}
	return sc.i, nil
}

// jsEmpty reports whether code holds only white space and comments. The
// expression is closed, so each `//` comment has a line ending after it.
func jsEmpty(code string) bool {
	for code != "" {
		switch {
		case strings.HasPrefix(code, "/*"):
			_, code, _ = strings.Cut(code[2:], "*/")
		case strings.HasPrefix(code, "//"):
			_, code, _ = strings.Cut(code, "\n")
		case strings.TrimSpace(code[:1]) == "":
			code = code[1:]
		default:
			return false
		}
	}
	return true
}
