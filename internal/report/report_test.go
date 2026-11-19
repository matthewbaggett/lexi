package report

import (
	"strings"
	"testing"
)

func TestFormatPlain(t *testing.T) {
	files := []File{
		{Name: "a.md", Messages: []Message{
			{Line: 1, Column: 1, EndLine: 1, EndColumn: 3, Reason: "`He` may be insensitive", RuleID: "he-she", Source: "retext-equality"},
			{Line: 10, Column: 12, EndLine: 10, EndColumn: 20, Reason: "short", RuleID: "x", Source: "retext-equality", Note: "A note."},
		}},
		{Name: "b.md"},
		{Name: "c.md", Messages: []Message{{Line: 1, Column: 1, Fatal: true, Reason: "No such file or directory"}}},
	}
	got, errs, warns := Format(files, Options{})
	// Like vfile-reporter, the columns line up across all files, and only
	// a message row is followed by a blank line.
	want := strings.Join([]string{
		"a.md",
		"      1:1-1:3  warning  `He` may be insensitive    he-she  retext-equality",
		"  10:12-10:20  warning  short                      x       retext-equality",
		"",
		"b.md: no issues found",
		"c.md",
		"          1:1  error    No such file or directory",
		"",
		"3 messages (✖ 1 error, ⚠ 2 warnings)",
	}, "\n")
	if got != want || errs != 1 || warns != 2 {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestQuietAndNotes(t *testing.T) {
	files := []File{
		{Name: "a.md"},
		{Name: "b.md", Messages: []Message{{Line: 1, Column: 1, EndLine: 1, EndColumn: 2, Reason: "r", Note: "A note.", RuleID: "x"}}},
	}
	got, _, _ := Format(files, Options{Quiet: true, Verbose: true})
	if want := "b.md\n  1:1-1:2  warning  r  x\nA note.\n\n⚠ 1 warning"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got, e, w := Format([]File{{Name: "a.md"}}, Options{Quiet: true}); got != "" || e+w != 0 {
		t.Errorf("quiet with nothing: %q", got)
	}
	if got, _, _ := Format(files[:1], Options{}); got != "a.md: no issues found" {
		t.Errorf("got %q", got)
	}
}

func TestSummary(t *testing.T) {
	for _, tc := range []struct {
		files []File
		want  string
	}{
		{[]File{{Name: "a", Messages: []Message{{Line: 1, Column: 1}}}}, "⚠ 1 warning"},
		{[]File{{Name: "a", Messages: []Message{{Line: 1, Column: 1, Fatal: true}, {Line: 2, Column: 1, Fatal: true}}}}, "✖ 2 errors"},
	} {
		got, _, _ := Format(tc.files, Options{})
		if !strings.HasSuffix(got, "\n\n"+tc.want) {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestColor(t *testing.T) {
	files := []File{
		{Name: "ok.md"},
		{Name: "warn.md", Messages: []Message{{Line: 1, Column: 1, EndLine: 1, EndColumn: 2, Reason: "r"}}},
		{Name: "err.md", Messages: []Message{{Line: 1, Column: 1, Fatal: true, Reason: "e"}}},
	}
	got, _, _ := Format(files, Options{Color: true})
	for _, want := range []string{
		"\x1b[4m\x1b[32mok.md\x1b[39m\x1b[24m: no issues found",
		"\x1b[4m\x1b[33mwarn.md\x1b[39m\x1b[24m",
		"\x1b[4m\x1b[31merr.md\x1b[39m\x1b[24m",
		"\x1b[33mwarning\x1b[39m",
		"\x1b[31merror\x1b[39m",
		"\x1b[31m✖\x1b[39m 1 error, \x1b[33m⚠\x1b[39m 1 warning",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestWidth(t *testing.T) {
	// A wide character takes two columns.
	files := []File{{Name: "a", Messages: []Message{
		{Line: 1, Column: 1, Reason: "日本", RuleID: "x"},
		{Line: 2, Column: 1, Reason: "ab", RuleID: "y"},
	}}}
	got, _, _ := Format(files, Options{})
	if !strings.Contains(got, "日本  x\n") || !strings.Contains(got, "ab    y\n") {
		t.Errorf("got %q", got)
	}
}

func TestSortedLikeVfileSort(t *testing.T) {
	msgs := []Message{
		{Line: 5, Column: 1, Reason: "z", RuleID: "r", Source: "s"},
		{Line: 3, Column: 9, Reason: "b", RuleID: "r", Source: "s"},
		{Line: 3, Column: 2, Reason: "warning first? no", RuleID: "r", Source: "s"},
		{Line: 3, Column: 2, Fatal: true, Reason: "error", RuleID: "r", Source: "s"},
		{Line: 3, Column: 2, Reason: "a", RuleID: "r", Source: "a"},
		{Line: 3, Column: 2, Reason: "a", RuleID: "q", Source: "s"},
		{Line: 3, Column: 2, Reason: "a", RuleID: "r", Source: "s"},
		{Line: 3, Column: 2, Reason: "a", RuleID: "r", Source: "s"},
		{Line: 1, Column: 1, Fatal: true, Reason: "no place", NoPlace: true},
	}
	text, _, _ := Format([]File{{Name: "a.md", Messages: msgs}}, Options{})
	var reasons []string
	for _, l := range strings.Split(text, "\n")[1:] {
		if l == "" {
			break
		}
		reasons = append(reasons, strings.Fields(l)[2])
	}
	if got, want := strings.Join(reasons, ","), "no,error,a,a,a,a,warning,b,z"; got != want {
		t.Errorf("got %s, want %s\n%s", got, want, text)
	}
}
