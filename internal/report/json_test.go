package report

import "testing"

func TestJSON(t *testing.T) {
	files := []File{
		{Name: "a.md", Messages: []Message{
			{Line: 2, Column: 3, Reason: "`He` may be insensitive", RuleID: "he-she", Source: "retext-equality",
				Actual: "He", Expected: []string{"They", "It"}, Note: "A note.", URL: "https://example.com"},
			{Line: 4, Column: 5, Reason: "profane", RuleID: "x", Source: "retext-profanities", Actual: "x", Expected: []string{}},
			{Line: 1, Column: 1, Fatal: true, Reason: "Unknown keyword", NoPlace: false},
		}},
		{Name: "b.md"},
		{Name: "c.md", Messages: []Message{{Line: 1, Column: 1, Fatal: true, Reason: "No such file", NoPlace: true}}},
		{Name: "<stdin>", Stdin: true, Messages: []Message{{Line: 1, Column: 1, Reason: "r", RuleID: "y", Source: "s"}}},
	}
	want := `[{"path":"a.md","cwd":"/w","history":["a.md"],"messages":[` +
		`{"column":3,"fatal":false,"line":2,"reason":"` + "`He` may be insensitive" + `","ruleId":"he-she","source":"retext-equality","actual":"He","expected":["They","It"],"note":"A note.","url":"https://example.com"},` +
		`{"column":5,"fatal":false,"line":4,"reason":"profane","ruleId":"x","source":"retext-profanities","actual":"x","expected":[]},` +
		`{"column":1,"fatal":true,"line":1,"reason":"Unknown keyword","ruleId":null,"source":null}]},` +
		`{"path":"b.md","cwd":"/w","history":["b.md"],"messages":[]},` +
		`{"path":"c.md","cwd":"/w","history":["c.md"],"messages":[{"column":null,"fatal":true,"line":null,"reason":"No such file","ruleId":null,"source":null}]},` +
		`{"cwd":"/w","history":[],"messages":[{"column":1,"fatal":false,"line":1,"reason":"r","ruleId":"y","source":"s"}]}]`
	if got := JSON(files, JSONOptions{Cwd: "/w"}); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	quiet := `[{"path":"a.md","cwd":"/w","history":["a.md"],"messages":[`
	if got := JSON(files[:2], JSONOptions{Cwd: "/w", Quiet: true}); got[:len(quiet)] != quiet || got[len(got)-3:] != "]}]" {
		t.Errorf("quiet: %s", got)
	}
	if got := JSON(files[1:2], JSONOptions{Quiet: true}); got != "[]" {
		t.Errorf("empty: %s", got)
	}
	if got := JSON(nil, JSONOptions{}); got != "[]" {
		t.Errorf("none: %s", got)
	}
}

func TestJSONStrings(t *testing.T) {
	// JSON.stringify escapes the quote, the backslash and the control characters,
	// and leaves <, >, & and non-ASCII text, even U+2028, as it is.
	got := quote("a\"b\\c\b\f\n\r\t\x01\x1f<>&é 😀\x7f")
	want := `"a\"b\\c\b\f\n\r\t\u0001\u001f<>&é` + " 😀\x7f" + `"`
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}
