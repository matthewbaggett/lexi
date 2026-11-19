package files

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// tree makes files under a temporary directory. A name that ends in a slash
// makes a directory.
func tree(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range names {
		p := filepath.Join(root, filepath.FromSlash(n))
		if strings.HasSuffix(n, "/") {
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func paths(items []Item) []string {
	out := []string{}
	for _, it := range items {
		s := it.Path
		if it.Err != "" {
			s += " !" + it.Err
		}
		out = append(out, s)
	}
	return out
}

var exts = []string{".md", ".txt"}

func find(t *testing.T, root string, silent bool, inputs ...string) []string {
	t.Helper()
	return paths(Find(inputs, Options{Cwd: root, Extensions: exts, SilentlyIgnore: silent}))
}

func eq(t *testing.T, got []string, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestExpandBraces(t *testing.T) {
	eq(t, expandBraces("a{b,c{d,e}}f"), "abf", "acdf", "acef")
	eq(t, expandBraces("x/{a/**/,}*.md"), "x/a/**/*.md", "x/*.md")
	eq(t, expandBraces("a{b"), "a{b")
	eq(t, expandBraces("plain"), "plain")
}

func TestHasMagic(t *testing.T) {
	for p, want := range map[string]bool{
		"a*": true, "a?": true, "[a]": true, "{a,b}": true, "+(a|b)": true, "abc": false, "": false,
		"{a}": false, `a\*`: false, "!a": false, "a/b.md": false, "[": false, "{a..c}": true,
	} {
		if got := HasMagic(p); got != want {
			t.Errorf("HasMagic(%q) = %v", p, got)
		}
	}
}

func TestIgnorePatterns(t *testing.T) {
	ig := ParseIgnore("# comment\n\n*.log\n!keep.log\n/root.md\nbuild/\ndocs/**/draft.md\n/\nsrc/*.txt\n**/deep\n  \nname\\ \na[bc]d\nq?\nopen[\nesc\\*\ngen/**\nz[!x]e\n")
	for path, want := range map[string]bool{
		"a.log":              true,
		"keep.log":           false,
		"x/a.log":            true,
		"root.md":            true,
		"sub/root.md":        false,
		"build/out.md":       true,
		"x/build/out.md":     true,
		"docs/a/b/draft.md":  true,
		"docs/draft.md":      true,
		"src/a.txt":          true,
		"src/x/a.txt":        false,
		"x/y/deep":           true,
		"name ":              true,
		"abd":                true,
		"aed":                false,
		"qx":                 true,
		"open[":              true,
		"esc*":               true,
		"escX":               false,
		"gen/a/b":            true,
		"zce":                true,
		"zxe":                false,
		"unrelated.md":       false,
		"build":              false, // a file called build is not a directory
		"a/b/c/unrelated.md": false,
	} {
		if got := ig.Ignores(path, false); got != want {
			t.Errorf("%q: %v, want %v", path, got, want)
		}
	}
	if !ig.Ignores("build", true) {
		t.Error("a directory named build is ignored")
	}
}

func TestFindDirectory(t *testing.T) {
	root := tree(t, "a.md", "b.js", "docs/d.txt", "docs/sub/s.md", "docs/sub/n.png", ".hid/h.md", "node_modules/x/n.md", "src/e.md", "empty/")
	eq(t, find(t, root, false, "."), "a.md", "docs/d.txt", "docs/sub/s.md", "src/e.md")
	eq(t, find(t, root, false, "docs", "src"), "docs/d.txt", "docs/sub/s.md", "src/e.md")
	// A file that is named is checked, whatever its extension.
	eq(t, find(t, root, false, "b.js", "./a.md"), "b.js", "a.md")
	eq(t, find(t, root, false, "empty"))
	// An explicit path in node_modules or a dot directory is fine.
	eq(t, find(t, root, false, "node_modules/x/n.md", ".hid/h.md"), "node_modules/x/n.md", ".hid/h.md")
	eq(t, find(t, root, false, "nothere.md", "a.md"), "nothere.md !"+ErrNotFound, "a.md")
	eq(t, find(t, root, false, "a.md", "a.md"), "a.md")
}

func TestFindGlob(t *testing.T) {
	root := tree(t, "a.md", "b.txt", "docs/d.md", "docs/sub/s.md", ".hid/h.md", "src/e.md", "src/deep/f.md")
	eq(t, find(t, root, false, "*.md"), "a.md")
	eq(t, find(t, root, false, "*/*.md"), "docs/d.md", "src/e.md")
	eq(t, find(t, root, false, "**/*.md"), "a.md", "docs/d.md", "docs/sub/s.md", "src/deep/f.md", "src/e.md")
	eq(t, find(t, root, false, "{docs,src}/*.md"), "docs/d.md", "src/e.md")
	eq(t, find(t, root, false, ".hid/*.md"), ".hid/h.md")
	eq(t, find(t, root, false, "zzz/*.md"))
	eq(t, find(t, root, false, "{docs/**/,doc/**/,}*.{md,txt}"), "a.md", "b.txt", "docs/d.md", "docs/sub/s.md")
	// A glob can name a directory, which is then searched.
	eq(t, find(t, root, false, "sr?"), "src/deep/f.md", "src/e.md")
	// A match of a glob stays as glob gives it, as in alex: absolute, or with its "./".
	eq(t, find(t, root, false, filepath.Join(root, "src", "*.md")), filepath.Join(root, "src", "e.md"))
	eq(t, find(t, root, false, "./src/*.md"), "./src/e.md")
	eq(t, find(t, root, false, "src/../*.md"), "src/../a.md")
	// A directory that a glob matches gives the joined paths of its files.
	eq(t, find(t, root, false, "./sr?/"), "src/deep/f.md", "src/e.md")
	eq(t, find(t, root, false, "./sr?"), "src/deep/f.md", "src/e.md")
}

func TestFindIgnore(t *testing.T) {
	root := tree(t, "a.md", "docs/d.md", "docs/sub/s.md", "src/e.md", "src/keep.md", "src/skip/x.md", "out/")
	write(t, filepath.Join(root, IgnoreName), "src/*.md\n!src/keep.md\nsrc/skip\ndocs/sub/\n")
	eq(t, find(t, root, false, "."), "a.md", "docs/d.md", "src/keep.md")
	eq(t, find(t, root, false, "src/e.md", "docs/sub", "a.md"), "src/e.md !"+ErrIgnored, "docs/sub !"+ErrIgnored, "a.md")
	eq(t, find(t, root, true, "src/e.md", "docs/sub", "a.md"), "a.md")
	// The nearest ignore file wins, and it reads patterns from its own directory.
	write(t, filepath.Join(root, "docs", IgnoreName), "d.md\n")
	eq(t, find(t, root, false, "."), "a.md", "docs/sub/s.md", "src/keep.md")
	// A directory that has no ignore file in or above it is not ignored.
	other := tree(t, "x.md")
	eq(t, find(t, other, false, "."), "x.md")
}

func TestFindEdges(t *testing.T) {
	root := tree(t, "d/a.md")
	old := readDir
	t.Cleanup(func() { readDir = old })
	readDir = func(string) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	eq(t, find(t, root, false, "d"), "d !Cannot read directory")
}

func TestFindAbsolute(t *testing.T) {
	root := tree(t, "a.md")
	other := tree(t, "x/b.md")
	// An absolute path inside the working directory, and one outside it.
	eq(t, find(t, root, false, filepath.Join(root, "a.md")), "a.md")
	got := find(t, root, false, filepath.Join(other, "x", "b.md"))
	if len(got) != 1 || strings.Contains(got[0], "!") {
		t.Errorf("got %q", got)
	}
}
