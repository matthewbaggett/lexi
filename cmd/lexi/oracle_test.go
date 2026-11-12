package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cliCase is one run of the command in a directory of files.
type cliCase struct {
	name  string
	files map[string]string
	args  []string
	stdin string
}

var cliCases = []cliCase{
	{name: "default search", files: map[string]string{"a.md": "He said", "docs/d.md": "His bike", "doc/c.txt": "Fine", "node_modules/x/n.md": "He", ".hid/h.md": "He", "src/e.md": "He"}},
	{name: "directory", files: map[string]string{"src/e.md": "He", "src/deep/f.markdown": "Her", "src/x.js": "He"}, args: []string{"src"}},
	{name: "dot", files: map[string]string{"a.md": "He", "src/e.md": "Fine", "r.ron": "His", "t.text": "Her"}, args: []string{"."}},
	{name: "quiet", files: map[string]string{"a.md": "He", "b.md": "Fine"}, args: []string{"-q", "."}},
	{name: "clean", files: map[string]string{"b.md": "Fine"}, args: []string{"b.md"}},
	{name: "no files", files: map[string]string{"b.js": "He"}},
	{name: "missing", files: map[string]string{"a.md": "He"}, args: []string{"nothere.md", "a.md"}},
	{name: "glob", files: map[string]string{"src/e.md": "He", "src/a/f.md": "Her", "o.md": "His"}, args: []string{"src/**/*.md"}},
	{name: "braces", files: map[string]string{"src/e.md": "He", "docs/f.md": "Her", "o.md": "His"}, args: []string{"{src,docs}/*.md"}},
	{name: "no match", files: map[string]string{"a.md": "He"}, args: []string{"zzz/*.md"}},
	{name: "explicit js", files: map[string]string{"x.js": "He said"}, args: []string{"x.js"}},
	{name: "sorted", files: map[string]string{"z.md": "He", "a.md": "He", "m/b.md": "He"}, args: []string{"z.md", "m", "a.md"}},
	{name: "html", files: map[string]string{"o.html": "<p>He</p>", "o.md": "He"}, args: []string{"--html", "."}},
	{name: "html flag", files: map[string]string{"o.md": "<p>He</p><!--alex ignore he-she--><p>He</p>"}, args: []string{"-l", "o.md"}},
	{name: "mdx", files: map[string]string{"g.mdx": "import X from 'x'\n\nHe <B he=\"1\">his</B> {he}\n", "g.md": "He"}, args: []string{"--mdx", "."}},
	{name: "text flag", files: map[string]string{"a.md": "He <b>said</b> **x**\n<!--alex ignore he-she-->\nHe"}, args: []string{"-t", "a.md"}},
	{name: "stdin", stdin: "He said\n\n<!--alex ignore he-she-->\n\nHe", args: []string{"--stdin"}},
	{name: "stdin empty", args: []string{"--stdin"}},
	{name: "stdin quiet", stdin: "Fine", args: []string{"--stdin", "-q"}},
	{name: "why", files: map[string]string{"a.md": "He and the hysterical"}, args: []string{"--why", "a.md"}},
	{name: "allow", files: map[string]string{".alexrc": `{"allow":["he-she"]}`, "a.md": "He and man"}, args: []string{"a.md"}},
	{name: "deny yaml", files: map[string]string{".alexrc.yml": "deny:\n  - gals-man\n", "a.md": "He and man"}, args: []string{"a.md"}},
	{name: "package field", files: map[string]string{"package.json": `{"alex":{"noBinary":true}}`, "a.md": "He and she"}, args: []string{"a.md"}},
	{name: "nearest config", files: map[string]string{".alexrc": `{"allow":["he-she"]}`, "s/.alexrc.json": `{}`, "s/a.md": "He", "a.md": "He"}, args: []string{"."}},
	{name: "ignore file", files: map[string]string{".alexignore": "src\n*.txt\n!keep.txt\n", "src/e.md": "He", "a.md": "He", "b.txt": "He", "keep.txt": "He"}, args: []string{"."}},
	{name: "ignored by name", files: map[string]string{".alexignore": "src\n", "src/e.md": "He", "a.md": "Fine"}, args: []string{"src/e.md", "a.md"}},
	{name: "unknown keyword", files: map[string]string{"a.md": "<!--alex frob-->\n\nHe said"}, args: []string{"a.md"}},
	{name: "unknown flag", files: map[string]string{"a.md": "He"}, args: []string{"--bogus", "a.md"}},
	{name: "color", files: map[string]string{"a.md": "He"}, args: []string{"a.md"}},
}

func TestCLIOracle(t *testing.T) {
	dir := os.Getenv("ALEX_ORACLE")
	if dir == "" {
		t.Skip("ALEX_ORACLE is not set")
	}
	cli, err := filepath.Abs(filepath.Join(dir, "node_modules", "alex", "cli.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cliCases {
		t.Run(tc.name, func(t *testing.T) {
			project(t, tc.files)
			if tc.name == "color" {
				t.Setenv("FORCE_COLOR", "1")
			}
			cmd := exec.Command("node", append([]string{cli}, tc.args...)...)
			cmd.Env = append(os.Environ(), "NO_UPDATE_NOTIFIER=1")
			cmd.Stdin = strings.NewReader(tc.stdin)
			var out bytes.Buffer
			cmd.Stderr, cmd.Stdout = &out, &out
			wantCode := 0
			var ee *exec.ExitError
			if err := cmd.Run(); errors.As(err, &ee) {
				wantCode = ee.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := lexi(t, tc.stdin, tc.args...)
			want := strings.TrimRight(out.String(), "\n")
			got := strings.TrimRight(stdout+stderr, "\n")
			if got != want || code != wantCode {
				t.Errorf("exit %d (alex %d)\n lexi:\n%s\n alex:\n%s", code, wantCode, got, want)
			}
		})
	}
}
