package input

import (
	"regexp"
	"slices"
	"strings"

	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// This file ports unified-message-control as remark-message-control 7 uses it
// for alex.

type point struct {
	line, col int
	ok        bool
}

type state struct {
	on bool
	at point
}

type control struct {
	reset, initial  bool
	enable, disable []string
	scope           map[string][]state
	globals         []state
}

var spaceSplit = regexp.MustCompile(`\s`)

func newControl(marks []mark, o rules.Options) *control {
	c := &control{reset: o.Deny != nil, enable: o.Deny, disable: o.Allow, scope: map[string][]state{}}
	c.initial = !c.reset
	for _, m := range marks {
		ids := spaceSplit.Split(m.text, -1)
		verb := ids[0]
		ids = ids[1:]
		at := point{m.line, m.col, true}
		tail := point{m.tl, m.tc, m.tail}
		switch {
		case len(ids) > 0:
			for _, id := range ids {
				c.toggle(at, verb == "enable", id)
				if verb == "ignore" {
					c.toggle(tail, true, id)
				}
			}
		case verb == "ignore":
			c.toggle(at, false, "")
			c.toggle(tail, true, "")
		default:
			c.toggle(at, verb == "enable", "")
			c.reset = verb != "enable"
		}
	}
	return c
}

func (c *control) ranges(id string) []state {
	if id == "" {
		return c.globals
	}
	return c.scope[id]
}

func (c *control) getState(id string) bool {
	if r := c.ranges(id); len(r) > 0 {
		return r[len(r)-1].on
	}
	if id == "" {
		return !c.reset
	}
	if c.reset {
		return slices.Contains(c.enable, id)
	}
	return !slices.Contains(c.disable, id)
}

func (c *control) toggle(at point, on bool, id string) {
	if on != c.getState(id) {
		if id == "" {
			c.globals = append(c.globals, state{on, at})
		} else {
			c.scope[id] = append(c.scope[id], state{on, at})
		}
	}
	if id == "" {
		for k := range c.scope {
			c.toggle(at, on, k)
		}
	}
}

func (c *control) check(f rules.Finding, ranges []state, id string) bool {
	for i := len(ranges) - 1; i >= 0; i-- {
		p := ranges[i].at
		if p.ok && (p.line < f.Line || (p.line == f.Line && p.col <= f.Column)) {
			return ranges[i].on
		}
	}
	if id == "" {
		return c.initial || c.reset
	}
	if c.reset {
		return slices.Contains(c.enable, id)
	}
	return !slices.Contains(c.disable, id)
}

func (c *control) allowed(f rules.Finding) bool {
	return c.check(f, c.scope[f.RuleID], f.RuleID) && c.check(f, c.globals, "")
}

var (
	htmlMarker = regexp.MustCompile(`^\s*<!--\s*([a-zA-Z\d-]+)(\s+([\s\S]*))?\s*-->\s*$`)
	jsMarker   = regexp.MustCompile(`^\s*/\*\s*([a-zA-Z\d-]+)(\s+([\s\S]*))?\s*\*/\s*$`)
	textMarker = regexp.MustCompile(`^\s*([a-zA-Z\d-]+)(\s+([\s\S]*))?\s*$`)
	// params checks the attribute syntax of mdast-comment-marker.
	params = regexp.MustCompile(`\s+([-\w]+)(?:=(?:"(?:\\[\s\S]|[^"])*"|'(?:\\[\s\S]|[^'])*'|(?:\\[\s\S]|[^"'\s])+))?`)
)

// marker returns the control text of a comment, or false when the comment is
// not for alex. re picks the comment syntax.
func marker(re *regexp.Regexp, value string) (string, bool) {
	m := re.FindStringSubmatch(value)
	if m == nil || m[1] != "alex" {
		return "", false
	}
	if strings.TrimSpace(params.ReplaceAllString(m[2], "")) != "" {
		return "", false
	}
	return strings.TrimSpace(m[3]), true
}

// addMark records a control comment. start, end and tail are UTF-16 offsets
// of the comment and the end of the node after it; tail is -1 when no node
// follows the comment.
func (b *builder) addMark(re *regexp.Regexp, value string, start, end, tail int) {
	text, ok := marker(re, value)
	if !ok {
		return
	}
	verb := spaceSplit.Split(text, 2)[0]
	line, col := b.src.point(start)
	if verb != "enable" && verb != "disable" && verb != "ignore" {
		if b.err == nil {
			el, ec := b.src.point(end)
			b.err = &Error{line, col, el, ec, "Unknown keyword `" + verb + "`: expected `'enable'`, `'disable'`, or `'ignore'`"}
		}
		return
	}
	m := mark{line: line, col: col, text: text}
	if tail >= 0 {
		m.tail = true
		m.tl, m.tc = b.src.point(tail)
	}
	b.marks = append(b.marks, m)
}
