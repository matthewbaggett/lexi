package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestCLIJSONGolden checks the JSON report against what alex printed, which
// the golden file holds. It needs no node. TestCLIOracleJSON records the file.
func TestCLIJSONGolden(t *testing.T) {
	b, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]jsonGolden
	if err := json.Unmarshal(b, &golden); err != nil {
		t.Fatal(err)
	}
	runs := jsonRuns(t)
	if len(golden) != len(runs) {
		t.Fatalf("the golden file has %d runs, and there are %d", len(golden), len(runs))
	}
	for _, tc := range runs {
		want, ok := golden[tc.name]
		if !ok {
			t.Fatalf("the golden file has no run %q", tc.name)
		}
		for _, name := range []string{"json", "/opt/alex/node_modules/vfile-reporter-json/index.js", "vfile-reporter-json"} {
			t.Run(tc.name+" "+name, func(t *testing.T) {
				project(t, tc.files)
				code, stdout, stderr := lexi(t, tc.stdin, append([]string{"--reporter=" + name}, tc.args...)...)
				got := strings.ReplaceAll(strings.TrimRight(stderr, "\n"), physicalCwd(t), "<cwd>")
				if stdout != "" || got != want.Want || code != want.Code {
					t.Errorf("exit %d (alex %d), stdout %q\n lexi:\n%s\n alex:\n%s", code, want.Code, stdout, got, want.Want)
				}
			})
		}
	}
}

func TestReporterFlag(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		in   []string
	}{
		{[]string{"--reporter=json", "a.md"}, "json", []string{"a.md"}},
		{[]string{"--reporter", "json", "a.md"}, "json", []string{"a.md"}},
		{[]string{"--reporter=", "a.md"}, "", []string{"a.md"}},
		{[]string{"a.md", "--reporter"}, "", []string{"a.md"}},
		{[]string{"--reporter", "--stdin"}, "", nil},
	} {
		f, in := parseArgs(tc.args)
		if f.reporter != tc.want || !reflect.DeepEqual(in, tc.in) {
			t.Errorf("%q: reporter %q inputs %q", tc.args, f.reporter, in)
		}
	}
}

func TestJSONReporterNames(t *testing.T) {
	for v, want := range map[string]bool{
		"json":                                                true,
		"vfile-reporter-json":                                 true,
		"vfile-reporter-json/index.js":                        true,
		"vfile-reporter-json/lib/index.js":                    true,
		"/opt/alex/node_modules/vfile-reporter-json":          true,
		"/opt/alex/node_modules/vfile-reporter-json/index.js": true,
		"./node_modules/vfile-reporter-json/lib/index.js":     true,
		`C:\alex\node_modules\vfile-reporter-json\index.js`:   true,
		"pretty":                          false,
		"vfile-reporter":                  false,
		"/x/not-vfile-reporter-json":      false,
		"/x/vfile-reporter-json/other.js": false,
	} {
		if got := jsonReporter(v); got != want {
			t.Errorf("%q: %v", v, got)
		}
	}
}

func TestUnknownReporter(t *testing.T) {
	project(t, map[string]string{"a.md": "He"})
	code, out, e := lexi(t, "", "--reporter=pretty", "a.md")
	if code != 2 || out != "" || e != "lexi: Could not find reporter `pretty`. lexi has the reporter `json`\n" {
		t.Errorf("%d %q %q", code, out, e)
	}
}

func TestJSONReport(t *testing.T) {
	project(t, map[string]string{"a.md": "He said.", "b.md": "Fine.", "c.md": "<!--alex frob-->\n\nFine."})
	cwd := physicalCwd(t)
	code, out, e := lexi(t, "", "--reporter=json", "a.md", "b.md")
	want := `[{"path":"a.md","cwd":"` + cwd + `","history":["a.md"],"messages":[{"column":1,"fatal":false,"line":1,` +
		`"reason":"` + "`He` may be insensitive, use `They`, `It` instead" + `","ruleId":"he-she","source":"retext-equality","actual":"He",` +
		`"expected":["They","It"],"url":"https://github.com/retextjs/retext-equality#readme"}]},` +
		`{"path":"b.md","cwd":"` + cwd + `","history":["b.md"],"messages":[]}]` + "\n"
	if code != 1 || out != "" || e != want {
		t.Errorf("%d %q\n%s\nwant\n%s", code, out, e, want)
	}
	// No message, no failure. The report is a list, even for no files.
	if code, _, e := lexi(t, "", "--reporter=json", "-q", "b.md"); code != 0 || e != "[]\n" {
		t.Errorf("quiet: %d %q", code, e)
	}
	// An error of the control comments makes the run fail, and comes last.
	if code, _, e := lexi(t, "", "--reporter", "json", "c.md"); code != 1 || !strings.Contains(e, `"fatal":true,"line":1,"reason":"Unknown keyword`) {
		t.Errorf("control: %d %q", code, e)
	}
	// The standard input has no path.
	if code, _, e := lexi(t, "He", "--reporter=json", "--stdin"); code != 1 || !strings.HasPrefix(e, `[{"cwd":"`+cwd+`","history":[],`) {
		t.Errorf("stdin: %d %q", code, e)
	}
}

func TestPhysicalCwd(t *testing.T) {
	if got := physical("/nowhere/at/all"); got != "/nowhere/at/all" {
		t.Errorf("got %q", got)
	}
	dir := t.TempDir()
	link := dir + "/link"
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if got := physical(link); got != real {
		t.Errorf("got %q want %q", got, real)
	}
}
