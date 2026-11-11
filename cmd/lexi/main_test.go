package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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

func exec(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunStdinFatal(t *testing.T) {
	inDir(t, t.TempDir())
	code, out, _ := exec(t, "add to whitelist", "-stdin")
	if code != 1 || !strings.Contains(out, "<stdin>:1:8") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRunNoArgsReadsStdin(t *testing.T) {
	inDir(t, t.TempDir())
	code, out, _ := exec(t, "all fine here")
	if code != 0 || out != "" {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRunMaybeIsNotFatal(t *testing.T) {
	inDir(t, t.TempDir())
	if code, out, _ := exec(t, "hi guys"); code != 0 || out != "" {
		t.Fatalf("hidden by default: code=%d out=%q", code, out)
	}
	code, out, _ := exec(t, "hi guys", "-diff")
	if code != 0 || !strings.Contains(out, "[guys]") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRunFiles(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte("ok\nblacklist it"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := exec(t, "", p)
	if code != 1 || !strings.Contains(out, "a.md:2:1") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRunConfigAllowAndDeny(t *testing.T) {
	dir := t.TempDir()
	inDir(t, dir)
	if err := os.WriteFile(".alexrc", []byte(`{"allow":["whitelist"],"deny":["guys"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ := exec(t, "whitelist and guys", "-diff")
	if code != 1 || strings.Contains(out, "[whitelist]") || !strings.Contains(out, "[guys]") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRunBadFlag(t *testing.T) {
	if code, _, errs := exec(t, "", "-nope"); code != 2 || errs == "" {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestRunBadConfig(t *testing.T) {
	inDir(t, t.TempDir())
	if err := os.WriteFile(".alexrc", []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errs := exec(t, "")
	if code != 2 || !strings.Contains(errs, "config") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestRunRuleLoadError(t *testing.T) {
	inDir(t, t.TempDir())
	old := loadRules
	loadRules = func() ([]rules.Rule, error) { return nil, errors.New("bad rules") }
	t.Cleanup(func() { loadRules = old })
	code, _, errs := exec(t, "")
	if code != 2 || !strings.Contains(errs, "bad rules") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestRunStdinReadError(t *testing.T) {
	inDir(t, t.TempDir())
	var out, errb bytes.Buffer
	if code := run(nil, errReader{}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "boom") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func TestRunMissingFile(t *testing.T) {
	inDir(t, t.TempDir())
	code, _, errs := exec(t, "", "nope.md")
	if code != 2 || !strings.Contains(errs, "nope.md") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestMain_(t *testing.T) {
	inDir(t, t.TempDir())
	var got int
	oldExit, oldArgs, oldStdin := exit, os.Args, os.Stdin
	t.Cleanup(func() { exit, os.Args, os.Stdin = oldExit, oldArgs, oldStdin })
	exit = func(c int) { got = c }
	os.Args = []string{"lexi", "-stdin"}
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	os.Stdin = f
	main()
	if got != 0 {
		t.Fatalf("exit=%d", got)
	}
}
