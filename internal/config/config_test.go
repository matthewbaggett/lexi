package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	c, err := Find(sub)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Allow: []string{"a"}, Deny: []string{"b"}, ProfanitySureness: 1, NoBinary: true}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v", c)
	}
}

func TestFindPrefersAlexrc(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".alexrc"), `{"allow":["first"]}`)
	write(t, filepath.Join(root, ".alexrc.json"), `{"allow":["second"]}`)
	c, _ := Find(root)
	if len(c.Allow) != 1 || c.Allow[0] != "first" {
		t.Fatalf("got %+v", c)
	}
}

func TestFindBadJSON(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".alexrc"), "{")
	if _, err := Find(root); err == nil {
		t.Fatal("want error")
	}
}

func TestFindReadError(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".alexrc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(root); err == nil {
		t.Fatal("want error reading a directory")
	}
}

func TestFindAbsError(t *testing.T) {
	old := abs
	abs = func(string) (string, error) { return "", errors.New("no cwd") }
	t.Cleanup(func() { abs = old })
	if _, err := Find("."); err == nil {
		t.Fatal("want error")
	}
}
