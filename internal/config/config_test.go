package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindNone(t *testing.T) {
	c, err := Find(t.TempDir())
	if err != nil || !reflect.DeepEqual(c, Config{}) {
		t.Fatalf("c=%+v err=%v", c, err)
	}
}

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".alexrc.json"), `{"allow":["a"],"deny":["b"],"profanitySureness":1,"noBinary":true}`)
	sub := filepath.Join(root, "x", "y")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	c, path, err := FindFile(sub)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Allow: []string{"a"}, Deny: []string{"b"}, ProfanitySureness: 1, NoBinary: true}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v", c)
	}
	if path != filepath.Join(root, ".alexrc.json") {
		t.Fatalf("path %q", path)
	}
}

func TestFormats(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{".alexrc", `{"allow":["a"]}`},
		{".alexrc.yml", "allow:\n  - a\n"},
		{".alexrc.yaml", "allow: [a]\n"},
		{"package.json", `{"name":"x","alex":{"allow":["a"]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, tc.name), tc.body)
			c, err := Find(dir)
			if err != nil || !reflect.DeepEqual(c.Allow, []string{"a"}) {
				t.Fatalf("c=%+v err=%v", c, err)
			}
		})
	}
}

func TestPackageWithoutField(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".alexrc"), `{"noBinary":true}`)
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(sub, "package.json"), `{"name":"x"}`)
	c, err := Find(sub)
	if err != nil || !c.NoBinary {
		t.Fatalf("c=%+v err=%v", c, err)
	}
}

func TestOrder(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".alexrc.yml"), "allow: [y]\n")
	write(t, filepath.Join(dir, ".alexrc.json"), `{"allow":["j"]}`)
	c, err := Find(dir)
	if err != nil || !reflect.DeepEqual(c.Allow, []string{"j"}) {
		t.Fatalf("c=%+v err=%v", c, err)
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{".alexrc", `{bad`, "cannot parse file"},
		{".alexrc.yml", "a: [", "cannot parse file"},
		{".alexrc.yml", "{? [a]: b}", "cannot parse file"},
		{".alexrc.yml", "1: a\n", "cannot parse file"},
		{".alexrc.js", `module.exports = {}`, "JavaScript"},
		{"package.json", `{bad`, "cannot parse file"},
		{".alexrc", `{"allow":"x"}`, "cannot parse file"},
	} {
		t.Run(tc.name+tc.body, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, tc.name), tc.body)
			_, err := Find(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".alexrc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(dir); err == nil {
		t.Fatal("expected an error for a directory")
	}
}

func TestAbsError(t *testing.T) {
	old := abs
	t.Cleanup(func() { abs = old })
	abs = func(string) (string, error) { return "", errors.New("boom") }
	if _, err := Find("."); err == nil {
		t.Fatal("expected an error")
	}
}
