package input

import "testing"

// TestMDXErrors checks the messages of the syntax errors against what alex
// 11.0.1 reports. An empty message means that alex accepts the text, or that it
// stops with an acorn error that lexi cannot word.
func TestMDXErrors(t *testing.T) {
	const (
		attrName = "a character that can start an attribute name, such as a letter, `$`, or `_`"
		attrEnd  = "; whitespace before attributes; or the end of the tag"
		attrInit = "; `=` to initialize a value; or the end of the tag"
		inAttr   = "an attribute name character such as letters, digits, `$`, or `_`; `=` to initialize a value; whitespace before attributes; or the end of the tag"
	)
	for _, c := range []struct{ in, want string }{
		{"< a\n", ""},
		{"a <`b>", "Unexpected character `` ` `` (U+0060) before name, expected a character that can start a name, such as a letter, `$`, or `_`"},
		{"a </3>", "Unexpected character `3` (U+0033) before name, expected a character that can start a name, such as a letter, `$`, or `_`"},
		{"a </*>", "Unexpected character `*` (U+002A) before name, expected a character that can start a name, such as a letter, `$`, or `_` (note: JS comments in JSX tags are not supported in MDX)"},
		{"a <a.b 3>", "Unexpected character `3` (U+0033) after member name, expected " + attrName + attrEnd},
		{"a <a:b 3>", "Unexpected character `3` (U+0033) after local name, expected " + attrName + attrEnd},
		{"<A b=\"1\"3>", "Unexpected character `3` (U+0033) before attribute name, expected " + attrName + attrEnd},
		{"a <A b%>", "Unexpected character `%` (U+0025) in attribute name, expected " + inAttr},
		{"a <A b:3>", "Unexpected character `3` (U+0033) before local attribute name, expected " + attrName + attrInit},
		{"a <A b:c%>", "Unexpected character `%` (U+0025) in local attribute name, expected " + inAttr},
		{"a <A b:c 3>", "Unexpected character `3` (U+0033) after local attribute name, expected " + attrName + attrInit},
		{"a <A b 3>", "Unexpected character `3` (U+0033) after attribute name, expected " + attrName + attrInit},
		{"a <A {/* x */} />", "Unexpected empty expression"},
		{"<A b={\n// x\n} />", "Unexpected empty expression"},
		{"a <A b={/* x */ } />", "Unexpected empty expression"},
		{"a < b {} c", ""},
	} {
		e := mdxErrors(c.in, newSource(c.in))
		got := ""
		if e != nil {
			got = e.Message
		}
		if got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}
