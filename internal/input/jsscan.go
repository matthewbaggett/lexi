package input

// This file holds a small JavaScript scanner. It does not parse. It reads the
// text of an ESM block and tells if a statement is still open at its end. An
// open statement makes micromark go on past a blank line, because acorn fails
// at the end of the text and more text may fix it.
//
// Open means one of these: a `{`, `(` or `[` with no closer, a template
// literal, a block comment, or a JSX element with no closing tag.

type jsScanner struct {
	s    string
	i    int
	open bool
}

// jsOpen reports whether the statement in code is still open at its end.
func jsOpen(code string) bool {
	sc := &jsScanner{s: code}
	sc.code(0)
	return sc.open
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0x80
}

// code scans JavaScript. With closer `}` it returns after the brace that
// closes the caller's `{`. With 0 it scans to the end.
func (sc *jsScanner) code(closer byte) {
	var stack []byte
	value := false // the last token ends an expression, so `/` divides and `<` compares
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		switch {
		case c == '/' && sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '/':
			for sc.i < len(sc.s) && sc.s[sc.i] != '\n' {
				sc.i++
			}
		case c == '/' && sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '*':
			end := indexFrom(sc.s, "*/", sc.i+2)
			if end < 0 {
				sc.i = len(sc.s)
				sc.open = true
				return
			}
			sc.i = end + 2
		case c == '"' || c == '\'':
			sc.quoted(c)
			value = true
		case c == '`':
			sc.template()
			value = true
		case c == '/' && !value:
			sc.regex()
			value = true
		case c == '<' && !value && sc.jsxStart():
			sc.jsx()
			value = true
		case c == '{' || c == '(' || c == '[':
			stack = append(stack, c)
			sc.i++
			value = false
		case c == '}' || c == ')' || c == ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			} else if c == '}' && closer == '}' {
				sc.i++
				return
			}
			sc.i++
			value = true
		case isIdentByte(c):
			start := sc.i
			for sc.i < len(sc.s) && isIdentByte(sc.s[sc.i]) {
				sc.i++
			}
			value = !jsKeywordBeforeExpr[sc.s[start:sc.i]]
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			sc.i++
		default:
			sc.i++
			value = false
		}
	}
	if len(stack) > 0 || closer != 0 {
		sc.open = true
	}
}

// jsKeywordBeforeExpr lists the words after which a `/` starts a regex and a
// `<` starts JSX.
var jsKeywordBeforeExpr = map[string]bool{
	"return": true, "typeof": true, "instanceof": true, "in": true, "of": true, "new": true,
	"delete": true, "void": true, "throw": true, "case": true, "do": true, "else": true,
	"yield": true, "await": true, "default": true,
}

func indexFrom(s, sub string, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// quoted skips a string. It ends at the closing quote or at the end of the line.
func (sc *jsScanner) quoted(q byte) {
	for sc.i++; sc.i < len(sc.s); sc.i++ {
		switch sc.s[sc.i] {
		case '\\':
			sc.i++
		case q:
			sc.i++
			return
		case '\n':
			return
		}
	}
}

// template skips a template literal, and the code of each `${}` in it.
func (sc *jsScanner) template() {
	for sc.i++; sc.i < len(sc.s); {
		switch sc.s[sc.i] {
		case '\\':
			sc.i += 2
		case '`':
			sc.i++
			return
		case '$':
			if sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '{' {
				sc.i += 2
				sc.code('}')
				if sc.open {
					return
				}
				continue
			}
			sc.i++
		default:
			sc.i++
		}
	}
	sc.open = true
}

// regex skips a regular expression literal.
func (sc *jsScanner) regex() {
	inClass := false
	for sc.i++; sc.i < len(sc.s); sc.i++ {
		switch sc.s[sc.i] {
		case '\\':
			sc.i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				sc.i++
				return
			}
		case '\n':
			return
		}
	}
}

// jsxStart reports whether the `<` at the cursor starts a JSX element.
func (sc *jsScanner) jsxStart() bool {
	if sc.i+1 >= len(sc.s) {
		return false
	}
	c := sc.s[sc.i+1]
	return c == '>' || c == '_' || c == '$' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// jsx skips a JSX element, with its children.
func (sc *jsScanner) jsx() {
	closing, selfClosing := sc.tag()
	if sc.open || closing || selfClosing {
		return
	}
	for sc.i < len(sc.s) {
		switch sc.s[sc.i] {
		case '{':
			sc.i++
			sc.code('}')
			if sc.open {
				return
			}
		case '<':
			if sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '/' {
				sc.tag()
				return
			}
			sc.jsx()
			if sc.open {
				return
			}
		default:
			sc.i++
		}
	}
	sc.open = true
}

// tag skips one tag, from its `<` to its `>`.
func (sc *jsScanner) tag() (closing, selfClosing bool) {
	sc.i++ // the `<`
	if sc.i < len(sc.s) && sc.s[sc.i] == '/' {
		closing = true
	}
	for sc.i < len(sc.s) {
		switch c := sc.s[sc.i]; c {
		case '"', '\'':
			sc.i++
			for sc.i < len(sc.s) && sc.s[sc.i] != c {
				sc.i++
			}
			sc.i++
		case '{':
			sc.i++
			sc.code('}')
			if sc.open {
				return
			}
		case '/':
			if sc.i+1 < len(sc.s) && sc.s[sc.i+1] == '>' {
				sc.i += 2
				return closing, true
			}
			sc.i++
		case '>':
			sc.i++
			return closing, false
		default:
			sc.i++
		}
	}
	sc.open = true
	return closing, false
}
