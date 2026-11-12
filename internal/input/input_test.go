package input

import (
	"fmt"
	"strings"
	"testing"

	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// found lists the findings of a text as "line:column rule".
func found(t *testing.T, kind Kind, text string, o rules.Options) []string {
	t.Helper()
	rs, err := rules.Load()
	if err != nil {
		t.Fatal(err)
	}
	fs, err := Extract(kind, text).Check(rs, o)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, f := range fs {
		out = append(out, fmt.Sprintf("%d:%d %s", f.Line, f.Column, f.RuleID))
	}
	return out
}

func eq(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPlainText(t *testing.T) {
	// Plain text has no comments and no markup: all of it is prose.
	eq(t, found(t, Text, "He said <!--alex ignore he-she--> **x**", rules.Options{}), "1:1 he-she", "1:25 heshe")
}

func TestAllowAndDeny(t *testing.T) {
	d := Extract(Markdown, "He said.")
	rs, _ := rules.Load()
	if _, err := d.Check(rs, rules.Options{Allow: []string{"a"}, Deny: []string{"b"}}); err != rules.ErrAllowDeny {
		t.Fatalf("err=%v", err)
	}
}

func TestControlWithDeny(t *testing.T) {
	text := "He said, his bike.\n\n<!--alex disable her-him-->\n\nHis.\n\n<!--alex enable her-him-->\n\nHis."
	eq(t, found(t, Markdown, text, rules.Options{Deny: []string{"he-she", "her-him"}}), "1:1 he-she", "1:10 her-him", "9:1 her-him")
	// enable re-enables a rule that allow switched off.
	eq(t, found(t, Markdown, "<!--alex enable he-she-->\n\nHe.", rules.Options{Allow: []string{"he-she"}}), "3:1 he-she")
}

func TestMarkerSyntax(t *testing.T) {
	// A comment for another tool, and one with bad parameters, do nothing.
	eq(t, found(t, Markdown, "<!--other ignore he-she-->\n\nHe.\n\n<!--alex ignore he,she-->\n\nHe.", rules.Options{}), "3:1 he-she", "7:1 he-she")
}

func TestUnknownKeyword(t *testing.T) {
	d := Extract(Markdown, "<!--alex frob a-->\n\n<!--alex ignore he-she-->\n\nHe.")
	e := d.Err()
	if e == nil || e.Line != 1 || e.EndColumn != 19 || !strings.Contains(e.Error(), "Unknown keyword `frob`") {
		t.Fatalf("err=%+v", e)
	}
	rs, _ := rules.Load()
	fs, err := d.Check(rs, rules.Options{})
	if err != nil || len(fs) != 1 {
		t.Fatalf("findings are not filtered: %v %v", fs, err)
	}
	// Only the first error counts.
	if e2 := Extract(Markdown, "<!--alex a-->\n\n<!--alex b-->").Err(); e2.Message != "Unknown keyword `a`: expected `'enable'`, `'disable'`, or `'ignore'`" {
		t.Fatalf("err=%v", e2)
	}
}

func TestProse(t *testing.T) {
	d := Extract(HTML, "<p>a</p><p>b</p>")
	if got := d.Prose(); got != "a\n--\nb" {
		t.Fatalf("prose %q", got)
	}
}

func TestBlockEnds(t *testing.T) {
	// The comment ignores the block after it, whatever that block is.
	for name, doc := range map[string]string{
		"fence at the end of the file": "<!--alex ignore he-she-->\n\n```\nx\n```",
		"break in a quote":             "> <!--alex ignore he-she-->\n>\n> ***\n\nHe.",
		"break with tabs":              "<!--alex ignore he-she-->\n\n-\t-\t-\n\nHe.",
		"html comment last":            "He.\n\n<!--alex ignore he-she-->",
	} {
		t.Run(name, func(t *testing.T) { found(t, Markdown, doc, rules.Options{}) })
	}
	eq(t, found(t, Markdown, "<!--alex ignore he-she-->\n\n-\t-\t-\n\nHe.", rules.Options{}), "5:1 he-she")
}

func TestMDXHelpers(t *testing.T) {
	for _, s := range []string{"a { b", "a <B c={d", "a <B c=\"d", "<B c={\"}\"} \\\" />", "a <", "text {a \"b\\\" c\"} d", "b {a `x` }"} {
		t.Run(s, func(t *testing.T) { maskMDX(s) })
	}
	in := "a {/* alex ignore x */} b {x}\n<B>\nc <I a='>' />"
	masked, marks := maskMDX(in)
	if len(masked) != len(in) || strings.ContainsAny(masked, "{}<>/*") || len(marks) != 1 || marks[0].value != "/* alex ignore x */" {
		t.Fatalf("%q %v", masked, marks)
	}
	if end := matchBrace("{a", 0); end != -1 {
		t.Fatal(end)
	}
	if isTagStart("") || !isTagStart("A") || isTagStart(" ") {
		t.Fatal("isTagStart")
	}
	if tailOf("a", 1) != -1 || tailOf("a\n\nb", 0) != 1 {
		t.Fatal("tailOf")
	}
}

func TestPositions(t *testing.T) {
	// Columns count UTF-16 units. A line break is \n, \r\n or \r.
	eq(t, found(t, Markdown, "😀 he\r\n😀 his\rhis", rules.Options{}), "1:4 he-she", "2:4 her-him", "3:1 her-him")
	// A character reference is shorter in the text than in the source. alex
	// counts from the start of the text node, so the column is 3, not 7.
	eq(t, found(t, Markdown, "&amp; he said", rules.Options{}), "1:3 he-she")
}

func TestHTMLShapes(t *testing.T) {
	eq(t, found(t, HTML, "<p>He<p>His", rules.Options{}), "1:4 he-she", "1:9 her-him")
	eq(t, found(t, HTML, "<body><p>He</p></body>", rules.Options{}), "1:10 he-she")
	// A global disable turns off every rule, in the whole file. alex does so.
	eq(t, found(t, HTML, "<body><p>He</p></body>\n<!--alex disable-->\n<p>He</p>", rules.Options{}))
}
