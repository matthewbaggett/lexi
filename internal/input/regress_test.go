package input

import (
	"testing"

	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// TestFormatRegressions keeps one small document for each difference that the
// format corpus found. alex 11.0.1 gave the findings. The corpus in
// testdata/corpus has the large number of documents that prove the rest.
func TestFormatRegressions(t *testing.T) {
	for _, c := range []struct {
		name string
		kind Kind
		text string
		want []string
	}{
		// Markdown.
		{"footnote with an HTML block", Markdown, "[^1]: <!--alex ignore he-she--> He\n\nText[^1]", nil},
		{"a footnote that nothing refers to", Markdown, "Text[^missing] and handicapped.\n\n[^unused]: unused handicapped\n", []string{"3:19 handicapped"}},
		{"a comment before a footnote", Markdown, "Text[^1] He\n\n<!--alex ignore he-she-->\n\n[^1]: note He\n\nHe said.", []string{"1:10 he-she", "7:1 he-she"}},
		{"a comment in a paragraph ignores the text after it", Markdown, "Text<!--alex ignore he-she-->\nHe said.", nil},
		{"HTML and trailing white space before a line break", Markdown, "a <b> \n postman", []string{"2:1 postman-postwoman"}},
		{"an empty list item with CRLF", Markdown, "-\r\n- whitelist\r\n-\r\n", []string{"2:3 whitelist"}},
		{"a byte order mark is not in the columns", Markdown, "\xef\xbb\xbfHe said.", []string{"1:1 he-she"}},
		{"a numeric reference above U+FFFF is one unit", Markdown, "&#128512; He said.", []string{"1:3 he-she"}},
		{"a name that is not a character reference", Markdown, "&notanentity; He said.", []string{"1:15 he-she"}},
		{"numeric references that are not allowed", Markdown, "&#x80;&#0; He said.", []string{"1:4 he-she"}},
		// MDX.
		{"an expression is the next node of a comment", MDX, "{/* alex ignore whitelist */}\n\n{'whitelist'}\n\nThe whitelist was late.", []string{"5:5 whitelist"}},
		{"code between words joins them", MDX, "{x}{y}{z}policeman{w}crazy", nil},
		{"a comment that ends its block quote ignores the rest", MDX, "> {/* alex ignore dumb */}\n\n> The dumb was late.\n\nThe dumb was late.", nil},
		{"a comment that ends its paragraph ignores the rest", MDX, "{/* alex ignore he-she */} {/* alex ignore he-she */}\n\nHe.\n\nHe.", nil},
		{"JSX at the end of a quoted line", MDX, "> <Foo>blacklist</Foo>\n> whitelist", []string{"1:8 blacklist", "2:1 whitelist"}},
		{"an astral character in an expression", MDX, "Café {'\U0001F600'} crazy", []string{"1:13 nuts"}},
		// HTML: foster parenting and the adoption agency algorithm.
		{"text in a table goes before it", HTML, "<table>He<tr><td>His</td></tr></table>", []string{"1:8 he-she", "1:18 her-him"}},
		{"a misnested formatting tag", HTML, "<p>1<b>2<i>3</b>He</i>His</p>", nil},
		{"deep formatting around a block", HTML, "<b><i><u><em><s><div>He</b>His</div>", nil},
		{"formatting in a table", HTML, "<table><b><div>He</b>His</div></table>", nil},
		{"a table in a paragraph, in quirks mode", HTML, "<p>He<table><tr><td>His</table>He", nil},
		{"a table in a paragraph, with a doctype", HTML, "<!DOCTYPE html><p>He<table><tr><td>His</table>He", []string{"1:19 he-she", "1:36 her-him", "1:47 he-she"}},
		{"noscript holds HTML", HTML, "<noscript>He</noscript><p>His</p>", []string{"1:11 he-she", "1:27 her-him"}},
		{"an end tag names an SVG element", HTML, "<svg><desc><b>He</desc></svg>His</b>", []string{"1:30 her-him"}},
		{"a comment before an implied tbody", HTML, "<table><!--alex ignore he-she--><tr><td>He</td></tr></table><p>He</p>", nil},
		{"</button> is blocked by a special element", HTML, "<b>1<button>x<dt>He</button>His", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			eq(t, found(t, c.kind, c.text, rules.Options{}), c.want...)
		})
	}
}

// TestGapsWithControlError: a control comment with an unknown keyword stops the filter of alex,
// so the findings in a gap come back too.
func TestGapsWithControlError(t *testing.T) {
	text := "x\n\nHe went\n"
	eq(t, found(t, HTML, text, rules.Options{}))
	eq(t, found(t, HTML, text+"\n<!--alex frob-->\n", rules.Options{}), "3:1 he-she")
}

func TestEmptyHTML(t *testing.T) {
	// parse5 gives no node a position, and alex throws.
	for _, s := range []string{"", "\n\n", "<div class=\"x", "<a href=\"x>x</a>"} {
		e := Extract(HTML, s).Err()
		if e == nil || e.Message != "hast-util-to-nlcst expected position on nodes" {
			t.Errorf("%q: err=%v", s, e)
		}
	}
}

func TestDoctypes(t *testing.T) {
	for doctype, quirks := range map[string]bool{
		"html":                                false,
		"HTML":                                false,
		"":                                    true,
		"svg":                                 true,
		"html foo":                            true,
		"html SYSTEM \"about:legacy-compat\"": false,
		"html PUBLIC":                         true,
		"html PUBLIC x":                       true,
		"html PUBLIC \"x\" y":                 true,
		"html PUBLIC \"-//W3C//DTD HTML 4.01//EN\"":                                      false,
		"html PUBLIC \"-//W3C//DTD HTML 4.01 Transitional//EN\"":                         true,
		"html PUBLIC \"-//W3C//DTD HTML 4.01 Transitional//EN\" \"http://w3.org/x.dtd\"": false,
		"html PUBLIC \"-//IETF//DTD HTML 2.0//EN\"":                                      true,
		"html PUBLIC \"-/W3C/DTD HTML 4.0 Transitional/EN\"":                             true,
		"html PUBLIC \"html\"":                                                           true,
		"html SYSTEM \"http://www.ibm.com/data/dtd/v11/ibmxhtml1-transitional.dtd\"":     true,
	} {
		if got := isQuirks(doctype); got != quirks {
			t.Errorf("%q: quirks=%v, want %v", doctype, got, quirks)
		}
	}
}
