package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	{name: "glob plus", files: globFiles, args: []string{"+(a|b).md"}},
	{name: "glob at", files: globFiles, args: []string{"@(src|docs)/*.md"}},
	{name: "glob not", files: globFiles, args: []string{"!(a|b).md"}},
	{name: "glob not dir", files: globFiles, args: []string{"!(src)/*.md"}},
	{name: "glob star list", files: globFiles, args: []string{"*(a|b).md"}},
	{name: "glob optional", files: globFiles, args: []string{"?(a)b.md"}},
	{name: "glob nested", files: globFiles, args: []string{"+(a|@(b|ab)).md"}},
	{name: "glob posix class", files: globFiles, args: []string{"[[:alpha:]].md"}},
	{name: "glob class", files: globFiles, args: []string{"[!a-b].md"}},
	{name: "glob range", files: globFiles, args: []string{"n{1..3}.md"}},
	{name: "glob padded range", files: globFiles, args: []string{"p{08..10}.md"}},
	{name: "glob letter range", files: globFiles, args: []string{"{a..c}.md"}},
	{name: "glob dot", files: globFiles, args: []string{".*/*.md"}},
	{name: "glob dot file", files: globFiles, args: []string{".*.md"}},
	{name: "glob globstar skips dots", files: globFiles, args: []string{"**/*.md"}},
	{name: "glob globstar files", files: globFiles, args: []string{"src/**/*.md"}},
	{name: "mdx default", files: map[string]string{"g.mdx": "He", "docs/h.mdx": "He"}, args: []string{"--mdx"}},
	{name: "glob trailing slash", files: globFiles, args: []string{"*/"}},
	{name: "glob escape", files: globFiles, args: []string{`\*.md`}},
	{name: "glob escaped brace", files: globFiles, args: []string{`\{x,y}.md`}},
	{name: "glob two", files: globFiles, args: []string{"a.md", "+(src|docs)/*.md", "{b,c}.md"}},
	{name: "glob negated name", files: globFiles, args: []string{"!a.md"}},
	{name: "glob relative", files: globFiles, args: []string{"./src/../*.md"}},
	{name: "glob unclosed", files: globFiles, args: []string{"+(a|b.md", "[a.md"}},
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
	{name: "profanity", files: map[string]string{"a.md": "Don't be an *asshat* here.\n\n<!--alex ignore asshat-->\n\nasshat", "b.md": "A beaver and an addict."}, args: []string{"a.md", "b.md"}},
	{name: "sureness", files: map[string]string{".alexrc": `{"profanitySureness":1}`, "b.md": "A beaver and an addict."}, args: []string{"b.md"}},
	{name: "color", files: map[string]string{"a.md": "He"}, args: []string{"a.md"}},
}

// globFiles is the tree of the runs that check the expansion of patterns.
var globFiles = map[string]string{
	"a.md": "He", "b.md": "He", "ab.md": "He", "c.md": "He", "n1.md": "He", "n2.md": "He", "n9.md": "He",
	"p08.md": "He", "p09.md": "He", "p10.md": "He", "*.md": "He", "{x,y}.md": "He", ".dot.md": "He",
	"src/s.md": "He", "src/lib/l.md": "He", "src/.hid/h.md": "He", "docs/d.md": "He", "docs/a.md": "He",
	".cfg/c.md": "He", "other/o.md": "He", "other/p.txt": "He", "!a.md": "He",
}

// jsonCases are runs that only the JSON report needs: each kind of message
// that has no position, an escape in a string, and a note.
var jsonCases = []cliCase{
	{name: "allow and deny", files: map[string]string{".alexrc": `{"allow":["a"],"deny":["b"]}`, "a.md": "He"}, args: []string{"a.md"}},
	{name: "empty html", files: map[string]string{"e.html": ""}, args: []string{"--html", "e.html"}},
	{name: "escapes", files: map[string]string{"a.md": "He is learning\ndisabled and an \"invalid\" <b>man</b> \u2028 \\ \x01\n"}, args: []string{"a.md"}},
	{name: "tab", files: map[string]string{"a.txt": "learning\tdisabled"}, args: []string{"-t", "a.txt"}},
	{name: "stdin two", stdin: "He and she and an asshat", args: []string{"--stdin", "--why"}},
	{name: "quiet clean", files: map[string]string{"a.md": "Fine", "b.md": "Fine"}, args: []string{"-q", "."}},
	{name: "hard break", files: map[string]string{"a.md": "one he  \nand his\n\nand\\\nhis, he\n"}, args: []string{"a.md"}},
	{name: "late profanity", files: map[string]string{"a.md": "A whore and an asshat.\n\nA hell and an arse.\n"}, args: []string{"a.md"}},
	{name: "unicode", files: map[string]string{"é.md": "Ça va, il a dit: \"He\" 😀 He"}, args: []string{"é.md"}},
	{name: "not utf8", files: map[string]string{"a.md": "\xff He \xfe"}, args: []string{"a.md"}},
}

// alexRun runs the real alex, with extra arguments before the case's own.
func alexRun(t *testing.T, cli string, tc cliCase, extra ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("node", append(append([]string{cli}, extra...), tc.args...)...)
	cmd.Env = append(os.Environ(), "NO_UPDATE_NOTIFIER=1", "NODE_NO_WARNINGS=1")
	cmd.Stdin = strings.NewReader(tc.stdin)
	var out bytes.Buffer
	cmd.Stderr, cmd.Stdout = &out, &out
	code := 0
	var ee *exec.ExitError
	if err := cmd.Run(); errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, strings.TrimRight(out.String(), "\n")
}

func oracleDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("ALEX_ORACLE")
	if dir == "" {
		t.Skip("ALEX_ORACLE is not set")
	}
	abs, err := filepath.Abs(filepath.Join(dir, "node_modules"))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestCLIOracle(t *testing.T) {
	cli := filepath.Join(oracleDir(t), "alex", "cli.js")
	for _, tc := range cliCases {
		t.Run(tc.name, func(t *testing.T) {
			project(t, tc.files)
			if tc.name == "color" {
				t.Setenv("FORCE_COLOR", "1")
			}
			wantCode, want := alexRun(t, cli, tc)
			code, stdout, stderr := lexi(t, tc.stdin, tc.args...)
			got := strings.TrimRight(stdout+stderr, "\n")
			if got != want || code != wantCode {
				t.Errorf("exit %d (alex %d)\n lexi:\n%s\n alex:\n%s", code, wantCode, got, want)
			}
		})
	}
}

// jsonRuns are the runs that the JSON report is checked with: the runs of the
// text report, the runs that only the JSON report needs, and each fixture file
// of the input formats with the flag of its kind.
func jsonRuns(t testing.TB) []cliCase {
	t.Helper()
	var runs []cliCase
	for _, tc := range append(append([]cliCase{}, cliCases...), jsonCases...) {
		if tc.name != "color" {
			runs = append(runs, tc)
		}
	}
	flag := map[string]string{".html": "--html", ".mdx": "--mdx", ".txt": "--text"}
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "formats", "*.*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		ext := filepath.Ext(path)
		if ext != ".md" && flag[ext] == "" {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		args := []string{name}
		if flag[ext] != "" {
			args = []string{flag[ext], name}
		}
		runs = append(runs, cliCase{name: "fixture " + name, files: map[string]string{name: string(body)}, args: args})
	}
	return runs
}

// goldenFile records what alex prints for each of the runs, with the working
// directory of the run as <cwd>.
var goldenFile = filepath.Join("..", "..", "testdata", "reporter", "golden.json")

type jsonGolden struct {
	Code int    `json:"code"`
	Want string `json:"want"`
}

// physicalCwd is the working directory the way node reports it.
func physicalCwd(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(wd)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestCLIOracleJSON compares the JSON report with vfile-reporter-json 4.0.0,
// which alex loads. lexi gets the name json, and then the path of the entry
// file, the form that tools/alex/run-alex.sh passes. UPDATE_GOLDEN=1 records
// what alex prints in the golden file, which TestCLIJSONGolden reads.
func TestCLIOracleJSON(t *testing.T) {
	modules := oracleDir(t)
	cli := filepath.Join(modules, "alex", "cli.js")
	entry := filepath.Join(modules, "vfile-reporter-json", "index.js")
	golden := map[string]jsonGolden{}
	for _, tc := range jsonRuns(t) {
		for _, name := range []string{"json", entry} {
			t.Run(tc.name+" "+filepath.Base(name), func(t *testing.T) {
				project(t, tc.files)
				wantCode, want := alexRun(t, cli, tc, "--reporter="+entry)
				code, stdout, stderr := lexi(t, tc.stdin, append([]string{"--reporter=" + name}, tc.args...)...)
				if stdout != "" {
					t.Errorf("stdout is not empty: %q", stdout)
				}
				got := strings.TrimRight(stderr, "\n")
				if got != want || code != wantCode {
					t.Errorf("exit %d (alex %d)\n lexi:\n%s\n alex:\n%s", code, wantCode, got, want)
				}
				golden[tc.name] = jsonGolden{wantCode, strings.ReplaceAll(want, physicalCwd(t), "<cwd>")}
			})
		}
	}
	if os.Getenv("UPDATE_GOLDEN") == "" || t.Failed() {
		return
	}
	names := make([]string, 0, len(golden))
	for n := range golden {
		names = append(names, n)
	}
	sort.Strings(names)
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, n := range names {
		k, _ := json.Marshal(n)
		v, _ := json.Marshal(golden[n])
		b.WriteString("  " + string(k) + ": " + string(v))
		if i < len(names)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	if err := os.MkdirAll(filepath.Dir(goldenFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goldenFile, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
