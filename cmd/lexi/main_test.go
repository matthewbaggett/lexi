package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

// inDir runs the test with dir as the working directory.
func inDir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// project makes a directory with files, and runs the test inside it.
func project(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inDir(t, dir)
	t.Setenv("FORCE_COLOR", "0")
}

func lexi(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestParseArgs(t *testing.T) {
	f, in := parseArgs([]string{"-wq", "a.md", "--html", "--no-why", "--mdx=false", "--stdin=true", "-x", "val", "--bogus", "v2", "--bogus=v", "-xy", "b.md", "-", "--", "--c.md"})
	want := flags{quiet: true, html: true, stdin: true}
	if f != want {
		t.Errorf("flags %+v", f)
	}
	if !reflect.DeepEqual(in, []string{"a.md", "-", "--c.md"}) {
		t.Errorf("inputs %q", in)
	}
	f, in = parseArgs([]string{"-qx", "a.md", "b.md", "-xq", "c.md"})
	if !f.quiet || !reflect.DeepEqual(in, []string{"b.md", "c.md"}) {
		t.Errorf("the last unknown flag of a group takes a value: %+v %q", f, in)
	}
	f, _ = parseArgs([]string{"-t", "-l", "-d", "-h", "-v", "--text", "--diff"})
	if !f.text || !f.html || !f.diff || !f.help || !f.version {
		t.Errorf("flags %+v", f)
	}
}

func TestReporterOnce(t *testing.T) {
	code, _, e := lexi(t, "", "--reporter=json", "--reporter", "json")
	if code != 1 || !strings.Contains(e, "can only be set once") {
		t.Errorf("%d %q", code, e)
	}
}

func TestKindAndExtensions(t *testing.T) {
	for _, tc := range []struct {
		f    flags
		ext  string
		kind int
	}{
		{flags{}, ".md", 0},
		{flags{text: true}, ".md", 1},
		{flags{html: true, mdx: true, text: true}, ".html", 2},
		{flags{mdx: true, text: true}, ".mdx", 3},
	} {
		if got := tc.f.extensions(); !strings.Contains(strings.Join(got, " "), tc.ext) {
			t.Errorf("%+v: %v", tc.f, got)
		}
		if int(tc.f.kind()) != tc.kind {
			t.Errorf("%+v: kind %v", tc.f, tc.f.kind())
		}
	}
}

func TestHelpAndVersion(t *testing.T) {
	code, out, errb := lexi(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: lexi") || errb != "" {
		t.Errorf("help: %d %q %q", code, out, errb)
	}
	code, out, _ = lexi(t, "", "-v")
	if code != 0 || out != "lexi dev (alex 11.0.1 compatible)\n" {
		t.Errorf("version: %d %q", code, out)
	}
	project(t, map[string]string{"a.md": "Fine"})
	for _, args := range [][]string{{"-v", "-h"}, {"--help", "a.md"}, {"--version", "a.md"}, {"-h", "-q"}} {
		if code, out, _ := lexi(t, "", args...); strings.Contains(out, "Usage") || strings.Contains(out, "compatible") || code != 0 {
			t.Errorf("%q is not alone: %d %q", args, code, out)
		}
	}
	old := version
	t.Cleanup(func() { version = old })
	version = "0.1.0"
	code, out, _ = lexi(t, "", "--version")
	if code != 0 || out != "lexi 0.1.0 (alex 11.0.1 compatible)\n" {
		t.Errorf("version with a set version: %d %q", code, out)
	}
}

func TestStdin(t *testing.T) {
	project(t, nil)
	code, out, errb := lexi(t, "He said", "--stdin")
	want := "<stdin>\n  1:1-1:3  warning  `He` may be insensitive, use `They`, `It` instead  he-she  retext-equality\n\n⚠ 1 warning\n"
	if code != 1 || out != "" || errb != want {
		t.Errorf("code=%d out=%q err=%q", code, out, errb)
	}
	// Plain text: markup is prose.
	code, _, errb = lexi(t, "<!--alex ignore he-she--> He", "--stdin", "--text")
	if code != 1 || !strings.Contains(errb, "1:27-1:29") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	code, _, errb = lexi(t, "", "--stdin")
	if code != 0 || errb != "<stdin>: no issues found\n" {
		t.Errorf("empty: code=%d err=%q", code, errb)
	}
	code, _, errb = lexi(t, "fine", "--stdin", "-q")
	if code != 0 || errb != "" {
		t.Errorf("quiet: code=%d err=%q", code, errb)
	}
	// Bytes that are not UTF-8 become U+FFFD.
	code, _, errb = lexi(t, "\xff\xfeHe", "--stdin")
	if code != 1 || !strings.Contains(errb, "1:3-1:5") {
		t.Errorf("bytes: code=%d err=%q", code, errb)
	}
}

func TestStdinErrors(t *testing.T) {
	project(t, nil)
	var errb bytes.Buffer
	if code := run([]string{"--stdin"}, errReader{}, io.Discard, &errb); code != 1 || !strings.Contains(errb.String(), "boom") {
		t.Errorf("read error: %d %q", code, errb.String())
	}
	if code, _, e := lexi(t, "", "--stdin", "a.md"); code != 1 || e != "lexi: Do not pass globs with `--stdin`\n" {
		t.Errorf("globs with stdin: %d %q", code, e)
	}
}

func TestFilesAndDefaults(t *testing.T) {
	project(t, map[string]string{
		"a.md":                "He said",
		"b.txt":               "Fine",
		"docs/d.md":           "His bike",
		"doc/c.markdown":      "Hers",
		"src/e.md":            "He",
		"node_modules/x/n.md": "He",
		"p.html":              "<p>He</p>",
		"q.mdx":               "He",
	})
	code, _, errb := lexi(t, "")
	if code != 1 || !strings.Contains(errb, "a.md\n") || !strings.Contains(errb, "docs/d.md") || !strings.Contains(errb, "b.txt: no issues found") || strings.Contains(errb, "src/") || strings.Contains(errb, "p.html") {
		t.Errorf("default: %d\n%s", code, errb)
	}
	if code, _, errb = lexi(t, "", "--html"); code != 1 || !strings.HasPrefix(errb, "p.html\n  1:4-1:6") {
		t.Errorf("html: %d\n%s", code, errb)
	}
	if code, _, errb = lexi(t, "", "--mdx"); code != 1 || !strings.HasPrefix(errb, "q.mdx\n  1:1-1:3") {
		t.Errorf("mdx: %d\n%s", code, errb)
	}
	if code, _, errb = lexi(t, "", "src", "b.txt", "-q"); code != 1 || !strings.HasPrefix(errb, "src/e.md\n") || strings.Contains(errb, "b.txt") {
		t.Errorf("dir: %d\n%s", code, errb)
	}
	if code, _, errb = lexi(t, "", "nothere.md"); code != 1 || !strings.Contains(errb, "1:1  error  No such file or directory") {
		t.Errorf("missing: %d\n%s", code, errb)
	}
	if code, _, errb = lexi(t, "", "b.txt"); code != 0 || errb != "b.txt: no issues found\n" {
		t.Errorf("clean: %d\n%s", code, errb)
	}
	if code, _, errb = lexi(t, "", "zzz/*.md"); code != 0 || errb != "" {
		t.Errorf("no match: %d\n%s", code, errb)
	}
	// A .alexignore file hides files from a search, and rejects them by name.
	if err := os.WriteFile(".alexignore", []byte("a.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errb = lexi(t, ""); strings.Contains(errb, "a.md") {
		t.Errorf("ignored by default search: %d\n%s", code, errb)
	}
	if code, _, errb = lexi(t, "", "a.md"); code != 1 || !strings.Contains(errb, "it’s ignored") {
		t.Errorf("ignored by name: %d\n%s", code, errb)
	}
}

func TestWhy(t *testing.T) {
	project(t, map[string]string{"a.md": "A gypsy."})
	_, _, plain := lexi(t, "", "a.md")
	_, _, why := lexi(t, "", "--why", "a.md")
	if plain == why || strings.Count(why, "\n") <= strings.Count(plain, "\n") {
		t.Errorf("--why adds notes:\n%s\n---\n%s", plain, why)
	}
}

func TestConfig(t *testing.T) {
	project(t, map[string]string{
		"a.md":               "He said",
		"allowed/.alexrc":    `{"allow":["he-she"]}`,
		"allowed/b.md":       "He said",
		"both/.alexrc.json":  `{"allow":["a"],"deny":["b"]}`,
		"both/c.md":          "He",
		"bad/.alexrc":        `{nope`,
		"bad/d.md":           "He",
		"script/.alexrc.js":  `module.exports = {}`,
		"script/e.md":        "He",
		"yaml/.alexrc.yml":   "deny:\n  - gals-man\n",
		"yaml/f.md":          "He and man",
		"pkg/package.json":   `{"alex":{"noBinary":true}}`,
		"pkg/g.md":           "He and she",
		"plain/g.md":         "He and she",
		"plain/.alexrc.json": `{}`,
	})
	for dir, want := range map[string]string{
		"allowed": "allowed/b.md: no issues found",
		"both":    "Error: Do not provide both allow and deny configuration parameters",
		"bad":     "Error: Cannot parse file `bad/.alexrc`",
		"script":  "Error: Cannot parse file `script/.alexrc.js`: lexi cannot run JavaScript configuration",
		"yaml":    "gals-man",
		"pkg":     "he-she",
		".":       "he-she",
	} {
		code, _, errb := lexi(t, "", dir)
		if !strings.Contains(errb, want) {
			t.Errorf("%s: %d\n%s", dir, code, errb)
		}
	}
	if _, _, errb := lexi(t, "", "yaml"); strings.Contains(errb, "he-she") {
		t.Errorf("deny leaves only the denied rule:\n%s", errb)
	}
	if _, _, errb := lexi(t, "", "plain"); strings.Contains(errb, "he-she") && !strings.Contains(errb, "plain/g.md") {
		t.Errorf("plain:\n%s", errb)
	}
	// The config of the working directory serves stdin.
	if err := os.WriteFile(".alexrc", []byte(`{"allow":["he-she"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errb := lexi(t, "He said", "--stdin"); code != 0 || !strings.Contains(errb, "no issues") {
		t.Errorf("stdin config: %d %q", code, errb)
	}
}

func TestUnknownKeyword(t *testing.T) {
	project(t, map[string]string{"a.md": "<!--alex frob-->\n\nHe said"})
	code, _, errb := lexi(t, "", "a.md")
	for _, want := range []string{"1:1-1:17  error  ", "Unknown keyword `frob`", "3:1-3:3  warning", "2 messages (✖ 1 error, ⚠ 1 warning)"} {
		if !strings.Contains(errb, want) {
			t.Errorf("missing %q in\n%s", want, errb)
		}
	}
	if code != 1 {
		t.Errorf("code %d", code)
	}
}

func TestRuntimeErrors(t *testing.T) {
	project(t, map[string]string{"a.md": "He"})
	old := getwd
	t.Cleanup(func() { getwd = old })
	getwd = func() (string, error) { return "", errors.New("no cwd") }
	if code, _, e := lexi(t, "", "a.md"); code != 1 || !strings.Contains(e, "no cwd") {
		t.Errorf("getwd: %d %q", code, e)
	}
	getwd = old
	oldRules := loadRules
	t.Cleanup(func() { loadRules = oldRules })
	loadRules = func() ([]rules.Rule, error) { return nil, errors.New("no rules") }
	if code, _, e := lexi(t, "", "a.md"); code != 1 || !strings.Contains(e, "no rules") {
		t.Errorf("rules: %d %q", code, e)
	}
	loadRules = oldRules
	oldWords := loadWords
	t.Cleanup(func() { loadWords = oldWords })
	loadWords = func() ([]rules.Word, error) { return nil, errors.New("no words") }
	if code, _, e := lexi(t, "", "a.md"); code != 1 || !strings.Contains(e, "no words") {
		t.Errorf("words: %d %q", code, e)
	}
}

func TestUnreadableFile(t *testing.T) {
	project(t, map[string]string{"a.md": "He"})
	old := readFile
	t.Cleanup(func() { readFile = old })
	readFile = func(name string) ([]byte, error) {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
	}
	if code, _, e := lexi(t, "", "a.md"); code != 1 || !strings.Contains(e, "error  Permission denied") {
		t.Errorf("%d %q", code, e)
	}
	if got := errReason(errors.New("plain")); got != "Plain" {
		t.Errorf("errReason %q", got)
	}
}

func TestMain(t *testing.T) {
	old, oldExit := os.Args, exit
	t.Cleanup(func() { os.Args, exit = old, oldExit })
	var code = -1
	exit = func(c int) { code = c }
	os.Args = []string{"lexi", "--version"}
	main()
	if code != 0 {
		t.Errorf("exit code %d", code)
	}
}

func TestColorChoice(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if isTerminal(f) {
		t.Error("a file is no terminal")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if isTerminal(f) {
		t.Error("a closed file is no terminal")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	if !isTerminal(null) {
		t.Error("the null device is a character device")
	}
	if err := null.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORCE_COLOR", "1")
	if !useColor(io.Discard, nil) {
		t.Error("FORCE_COLOR=1 forces color")
	}
	t.Setenv("FORCE_COLOR", "0")
	if useColor(io.Discard, nil) {
		t.Error("FORCE_COLOR=0 turns color off")
	}
}

// TestSupportsColor checks each branch of the port of supports-color.
func TestSupportsColor(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		tty  bool
		env  map[string]string
		win  bool
		want bool
	}{
		{name: "no terminal", args: nil, tty: false, env: nil, win: false, want: false},
		{name: "terminal no hint", args: nil, tty: true, env: nil, win: false, want: false},
		{name: "terminal xterm", args: nil, tty: true, env: map[string]string{"TERM": "xterm"}, win: false, want: true},
		{name: "terminal 256", args: nil, tty: true, env: map[string]string{"TERM": "foo-256color"}, win: false, want: true},
		{name: "terminal colorterm", args: nil, tty: true, env: map[string]string{"COLORTERM": "yes"}, win: false, want: true},
		{name: "colorterm truecolor", args: nil, tty: true, env: map[string]string{"COLORTERM": "truecolor"}, win: false, want: true},
		{name: "kitty", args: nil, tty: true, env: map[string]string{"TERM": "xterm-kitty"}, win: false, want: true},
		{name: "iterm", args: nil, tty: true, env: map[string]string{"TERM_PROGRAM": "iTerm.app"}, win: false, want: true},
		{name: "apple terminal", args: nil, tty: true, env: map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, win: false, want: true},
		{name: "other terminal program", args: nil, tty: true, env: map[string]string{"TERM_PROGRAM": "x"}, win: false, want: false},
		{name: "dumb", args: nil, tty: true, env: map[string]string{"TERM": "dumb"}, win: false, want: false},
		{name: "dumb forced", args: nil, tty: true, env: map[string]string{"TERM": "dumb", "FORCE_COLOR": "1"}, win: false, want: true},
		{name: "windows", args: nil, tty: true, env: nil, win: true, want: true},
		{name: "ci", args: nil, tty: true, env: map[string]string{"CI": "1", "TERM": "xterm"}, win: false, want: false},
		{name: "ci forced", args: nil, tty: true, env: map[string]string{"CI": "1", "FORCE_COLOR": "2"}, win: false, want: true},
		{name: "ci github", args: nil, tty: true, env: map[string]string{"CI": "1", "GITHUB_ACTIONS": "1"}, win: false, want: true},
		{name: "ci travis", args: nil, tty: true, env: map[string]string{"CI": "1", "TRAVIS": "1"}, win: false, want: true},
		{name: "ci codeship", args: nil, tty: true, env: map[string]string{"CI": "1", "CI_NAME": "codeship"}, win: false, want: true},
		{name: "teamcity new", args: nil, tty: true, env: map[string]string{"TEAMCITY_VERSION": "9.1.0"}, win: false, want: true},
		{name: "teamcity old", args: nil, tty: true, env: map[string]string{"TEAMCITY_VERSION": "8.0.0"}, win: false, want: false},
		{name: "azure agent", args: nil, tty: false, env: map[string]string{"TF_BUILD": "1", "AGENT_NAME": "a"}, win: false, want: true},
		{name: "azure no agent", args: nil, tty: false, env: map[string]string{"TF_BUILD": "1"}, win: false, want: false},
		{name: "force true", args: nil, tty: false, env: map[string]string{"FORCE_COLOR": "true"}, win: false, want: true},
		{name: "force empty", args: nil, tty: false, env: map[string]string{"FORCE_COLOR": ""}, win: false, want: true},
		{name: "force false", args: nil, tty: true, env: map[string]string{"FORCE_COLOR": "false", "TERM": "xterm"}, win: false, want: false},
		{name: "force zero", args: nil, tty: true, env: map[string]string{"FORCE_COLOR": "0", "TERM": "xterm"}, win: false, want: false},
		{name: "force big", args: nil, tty: false, env: map[string]string{"FORCE_COLOR": "9"}, win: false, want: true},
		{name: "force huge", args: nil, tty: false, env: map[string]string{"FORCE_COLOR": "99999999999999999999"}, win: false, want: true},
		{name: "force negative", args: nil, tty: false, env: map[string]string{"FORCE_COLOR": "-1"}, win: false, want: true},
		{name: "force plus", args: nil, tty: false, env: map[string]string{"FORCE_COLOR": "+2"}, win: false, want: true},
		{name: "force text", args: nil, tty: false, env: map[string]string{"FORCE_COLOR": "abc"}, win: false, want: false},
		{name: "force text on terminal", args: nil, tty: true, env: map[string]string{"FORCE_COLOR": "abc", "TERM": "xterm"}, win: false, want: true},
		{name: "no color variable is ignored", args: nil, tty: true, env: map[string]string{"NO_COLOR": "1", "TERM": "xterm"}, win: false, want: true},
		{name: "flag color", args: []string{"--color"}, tty: false, env: nil, win: false, want: true},
		{name: "flag colors", args: []string{"--colors"}, tty: false, env: nil, win: false, want: true},
		{name: "flag color=always", args: []string{"--color=always"}, tty: false, env: nil, win: false, want: true},
		{name: "flag color=true", args: []string{"--color=true"}, tty: false, env: nil, win: false, want: true},
		{name: "flag no-color", args: []string{"--no-color"}, tty: true, env: map[string]string{"TERM": "xterm"}, win: false, want: false},
		{name: "flag no-colors", args: []string{"--no-colors"}, tty: true, env: map[string]string{"TERM": "xterm"}, win: false, want: false},
		{name: "flag color=never", args: []string{"--color=never"}, tty: true, env: map[string]string{"TERM": "xterm"}, win: false, want: false},
		{name: "flag color=false", args: []string{"--color=false"}, tty: true, env: map[string]string{"TERM": "xterm"}, win: false, want: false},
		{name: "flag color=256", args: []string{"--color=256"}, tty: false, env: nil, win: false, want: true},
		{name: "flag color=16m", args: []string{"--color=16m"}, tty: false, env: nil, win: false, want: true},
		{name: "flag after terminator", args: []string{"--", "--color"}, tty: false, env: nil, win: false, want: false},
		{name: "flag before terminator", args: []string{"--color", "--"}, tty: false, env: nil, win: false, want: true},
		{name: "env beats flag", args: []string{"--color"}, tty: false, env: map[string]string{"FORCE_COLOR": "0"}, win: false, want: false},
		{name: "single dash is no flag", args: []string{"-color"}, tty: false, env: nil, win: false, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := tc.env[k]; return v, ok }
			if got := supportsColor(tc.args, tc.tty, lookup, tc.win); got != tc.want {
				t.Errorf("got %v", got)
			}
		})
	}
	if !hasFlag([]string{"-q"}, "q") {
		t.Error("a short flag has one dash")
	}
	if !hasFlag([]string{"-color"}, "-color") {
		t.Error("a flag that starts with a dash is as given")
	}
}

func TestColorOutput(t *testing.T) {
	project(t, map[string]string{"a.md": "He"})
	t.Setenv("FORCE_COLOR", "1")
	if _, _, e := lexi(t, "", "a.md"); !strings.Contains(e, "\x1b[33mwarning\x1b[39m") {
		t.Errorf("%q", e)
	}
}

func TestProfanity(t *testing.T) {
	project(t, map[string]string{"a.md": "Don't be an asshat.\n", ".alexrc": `{"profanitySureness":2}`})
	code, _, e := lexi(t, "", "a.md")
	if code != 1 || !strings.Contains(e, "retext-profanities") || !strings.Contains(e, "1:13-1:19") {
		t.Errorf("%d\n%s", code, e)
	}
	if err := os.WriteFile(".alexrc", []byte(`{"allow":["asshat"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, e = lexi(t, "", "a.md"); code != 0 {
		t.Errorf("allow: %d\n%s", code, e)
	}
}
