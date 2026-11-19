package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"git.grey.ooo/grey.ooo/lexi/internal/report"
)

func env(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{"TRAVIS_COMMIT_RANGE", "GITHUB_SHA", "GITHUB_BASE_REF", "GITHUB_HEAD_REF"} {
		t.Setenv(k, kv[k])
	}
}

func TestCommitRange(t *testing.T) {
	for name, tc := range map[string]struct {
		env  map[string]string
		want []string
	}{
		"none":   {nil, nil},
		"travis": {map[string]string{"TRAVIS_COMMIT_RANGE": "a...b"}, []string{"a", "b"}},
		"push":   {map[string]string{"GITHUB_SHA": "abc"}, []string{"abc^1", "abc"}},
		"pr":     {map[string]string{"GITHUB_SHA": "abc", "GITHUB_BASE_REF": "refs/heads/main", "GITHUB_HEAD_REF": "feat/x"}, []string{"main", "x"}},
		"half":   {map[string]string{"GITHUB_SHA": "abc", "GITHUB_BASE_REF": "main"}, []string{"abc^1", "abc"}},
	} {
		t.Run(name, func(t *testing.T) {
			env(t, tc.env)
			if got := commitRange(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

const patch = `diff --git a/a.md b/a.md
index 111..222 100644
--- a/a.md
+++ b/a.md
@@ -1,4 +1,6 @@
 one
+two
+three
 four
-five
 six
@@ -20 +22,2 @@
+x
+y
diff --git a/gone.md b/gone.md
deleted file mode 100644
--- a/gone.md
+++ /dev/null
@@ -1 +0,0 @@
-bye
diff --git a/b.md b/b.md
--- a/b.md
+++ b/b.md
@@ -5,0 +6 @@
+new
\ No newline at end of file
`

func TestAddedLines(t *testing.T) {
	got := addedLines([]byte(patch), "/r")
	// Like unified-diff, every line of a hunk counts, so ranges follow the
	// hunk line index, not the line number of the new file.
	want := map[string][][2]int{
		"/r/a.md": {{2, 3}, {22, 23}},
		"/r/b.md": {{6, 6}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func repoWith(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestGitRoot(t *testing.T) {
	dir := repoWith(t)
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if gitRoot(sub) != dir {
		t.Error("finds the root")
	}
	if got := gitRoot(t.TempDir()); got != "" {
		t.Errorf("no repository above a temporary directory: %q", got)
	}
}

func fakeGit(t *testing.T, out string, err error) {
	t.Helper()
	old := gitDiff
	t.Cleanup(func() { gitDiff = old })
	gitDiff = func(root, from, to string) ([]byte, error) {
		if from != "a" || to != "b" {
			t.Errorf("range %s %s", from, to)
		}
		return []byte(out), err
	}
}

func TestDiffFilter(t *testing.T) {
	dir := repoWith(t)
	msgs := []report.Message{{Line: 1}, {Line: 2}, {Line: 3}, {Line: 4}, {Line: 22}}
	env(t, map[string]string{"TRAVIS_COMMIT_RANGE": "a...b"})
	if err := os.Mkdir(filepath.Join(dir, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	fakeGit(t, strings.ReplaceAll(patch, "b/a.md", "b/x/a.md"), nil)
	got := diffFilter(dir, "x/a.md", msgs)
	if !reflect.DeepEqual(got, []report.Message{{Line: 2}, {Line: 3}, {Line: 22}}) {
		t.Errorf("got %v", got)
	}
	// A file that the commit range did not touch has no messages.
	if got := diffFilter(dir, "x/other.md", msgs); len(got) != 0 {
		t.Errorf("got %v", got)
	}
	// Outside CI, or for stdin, nothing changes.
	if got := diffFilter(dir, "<stdin>", msgs); len(got) != len(msgs) {
		t.Errorf("stdin: %v", got)
	}
	env(t, nil)
	if got := diffFilter(dir, "x/a.md", msgs); len(got) != len(msgs) {
		t.Errorf("no range: %v", got)
	}
}

func TestDiffErrors(t *testing.T) {
	env(t, map[string]string{"TRAVIS_COMMIT_RANGE": "a...b"})
	kept := []report.Message{{Line: 2, Column: 1, Reason: "kept"}}
	if got := diffFilter(t.TempDir(), "a.md", kept); len(got) != 2 || got[1].Reason != "kept" {
		t.Errorf("an error keeps the other messages: %+v", got)
	}
	if got := diffFilter(t.TempDir(), "a.md", nil); len(got) != 1 || got[0].Reason != "Not in a git repository" {
		t.Errorf("no repository: %v", got)
	}
	fakeGit(t, "", errors.New("bad revision"))
	if got := diffFilter(repoWith(t), "a.md", nil); len(got) != 1 || got[0].Reason != "bad revision" || !got[0].Fatal {
		t.Errorf("git error: %v", got)
	}
}

func TestDiffThroughRun(t *testing.T) {
	project(t, map[string]string{"a.md": "He\n\nHis\n"})
	if err := os.Mkdir(".git", 0o700); err != nil {
		t.Fatal(err)
	}
	env(t, map[string]string{"TRAVIS_COMMIT_RANGE": "a...b"})
	fakeGit(t, "+++ b/a.md\n@@ -3,0 +3 @@\n+His\n", nil)
	code, _, errb := lexi(t, "", "--diff", "a.md")
	if code != 1 || strings.Contains(errb, "1:1-1:3") || !strings.Contains(errb, "3:1-3:4") {
		t.Errorf("code %d\n%s", code, errb)
	}
}

func TestRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("fine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "one")
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("fine\nHe said\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("commit", "-q", "-am", "two")
	out, err := gitDiff(dir, "HEAD~1", "HEAD")
	if err != nil || !strings.Contains(string(out), "+++ b/a.md") {
		t.Fatalf("%v\n%s", err, out)
	}
}
