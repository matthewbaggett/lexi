package files

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

// The expected values of these tests come from minimatch 5.1.9 and glob 8.1.0, the libraries of
// alex 11.0.1. The corpus tests cover the rest.

func TestExpandBracesEdges(t *testing.T) {
	for pattern, want := range map[string][]string{
		"{{{${,}}":     {"{{{${,}}"},
		"x{{${,}}y":    {"x{{${,}}y"},
		"a${b,c}":      {"a${b,c}"},
		"a${b,c}d":     {"a${b,c}d"},
		"{a},b}":       {"a}", "b"},
		"x{a\n},b}":    {"x{a\n},b}"},
		"{}":           {"{}"},
		"{},a}b":       {"{},a}b"},
		"a{{b}}c":      {"a{{b}}c"},
		"{a,b}{c,d}":   {"ac", "ad", "bc", "bd"},
		"{1..3}":       {"1", "2", "3"},
		"{c..a..2}":    {"c", "a"},
		"{01..03}":     {"01", "02", "03"},
		"{-2..1}":      {"-2", "-1", "0", "1"},
		"{3..1}":       {"3", "2", "1"},
		"{a..Z..x}":    {"{a..Z..x}"},
		"{1..2}{a..b}": {"1a", "1b", "2a", "2b"},
		"{Y..b}":       {"Y", "Z", "[", "", "]", "^", "_", "`", "a", "b"},
	} {
		if got := expandBraces(pattern); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %q, want %q", pattern, got, want)
		}
	}
	// The most results are 100,000, as in brace-expansion.
	for _, pattern := range []string{"{1..100000}{1..2}", "{{1..60000},{1..60000}}"} {
		if got := expandBraces(pattern); len(got) != braceMax {
			t.Errorf("%q: %d results", pattern, len(got))
		}
	}
	if commaSetFollows("a,b\n}") || !commaSetFollows("a,b}") || !commaSetFollows("a,,}") || commaSetFollows("a,x") {
		t.Error("commaSetFollows")
	}
}

func TestHasMagicEdges(t *testing.T) {
	// glob throws a TypeError for a pattern with no result. lexi reads it as a path.
	if HasMagic("{,}") || HasMagic("{,,}") {
		t.Error("an empty set is no pattern")
	}
	if !HasMagic("{a,a}") {
		t.Error("two results are a pattern")
	}
}

func TestCompileReFallback(t *testing.T) {
	if compileRe("(") != matchNothing {
		t.Error("a regular expression that does not compile matches nothing")
	}
	if m, _ := compileRe("^a$").MatchString("a"); !m {
		t.Error("a valid regular expression")
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"/a/b/../c": "/a/c", "/a/./b/": "/a/b/", "/../a": "/a", "/": "/", "//a//b": "/a/b", "/a/..": "/", "/a/../": "/",
	} {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeDirs lists directories from a map, so that a test can glob from "/".
func fakeDirs(m map[string][]string) func(string) ([]string, error) {
	return func(dir string) ([]string, error) {
		if names, ok := m[dir]; ok {
			return names, nil
		}
		return nil, os.ErrNotExist
	}
}

func TestGlobFromRoot(t *testing.T) {
	read := fakeDirs(map[string][]string{"/": {"tmp", "usr", ".hidden"}, "/tmp": {"a.md", "b.txt"}})
	if got := globWith("/*", "/work", read); !reflect.DeepEqual(got, []string{"/tmp", "/usr"}) {
		t.Errorf("/*: %q", got)
	}
	if got := globWith("/t?p/*.md", "/work", read); !reflect.DeepEqual(got, []string{"/tmp/a.md"}) {
		t.Errorf("/t?p/*.md: %q", got)
	}
}

func TestGlobRelativeCwd(t *testing.T) {
	root := tree(t, "d/a.md", "d/b.md")
	t.Chdir(root)
	if got := Glob("d/*.md", "."); !reflect.DeepEqual(got, []string{"d/a.md", "d/b.md"}) {
		t.Errorf("got %q", got)
	}
}

func TestGlobReadErrors(t *testing.T) {
	root := tree(t, "a.md", "d/b.md")
	denied := func(dir string) ([]string, error) { return nil, os.ErrPermission }
	if got := globWith("*", root, denied); len(got) != 0 {
		t.Errorf("an unreadable directory has no matches: %q", got)
	}
	notDir := func(dir string) ([]string, error) {
		return nil, &os.PathError{Op: "readdir", Path: dir, Err: syscall.ENOTDIR}
	}
	if got := globWith("*", root, notDir); len(got) != 0 {
		t.Errorf("a file has no matches: %q", got)
	}
}

// run calls stat and ends the queue.
func (g *globber) statNow(f string) cacheKind {
	var got cacheKind = -1
	g.stat(f, func(k cacheKind) { got = k })
	for len(g.queue) > 0 {
		next := g.queue[0]
		g.queue = g.queue[1:]
		next()
	}
	return got
}

func TestStatCaches(t *testing.T) {
	root := tree(t, "f.md", "d/")
	g := newGlobber(root, readNames)
	abs := func(n string) string { return filepath.Join(root, n) }

	// The cache of directory entries answers when it can.
	g.cache[abs("f.md")] = cacheVal{kind: cFile}
	g.cache[abs("d")] = cacheVal{kind: cList}
	g.cache[abs("gone")] = cacheVal{kind: cFalse}
	if g.statNow("f.md") != cFile || g.statNow("f.md/") != 0 || g.statNow("d/") != cDir || g.statNow("d") != cDir {
		t.Error("cache of files and directories")
	}
	if g.statNow("gone") != cFalse {
		t.Error("a path that is known to be gone")
	}

	// The stat cache answers next.
	g.statCache[abs("s.md")] = statVal{}
	g.statCache[abs("sd")] = statVal{dir: true}
	g.statCache[abs("sm")] = statVal{missing: true}
	if g.statNow("s.md") != cFile || g.statNow("s.md/") != 0 || g.statNow("sd") != cDir || g.statNow("sm") != cFalse {
		t.Error("stat cache")
	}

	// Else the path is read, and a link counts as its target.
	if err := os.Symlink("f.md", abs("link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nowhere", abs("dead")); err != nil {
		t.Fatal(err)
	}
	if g.statNow("link") != cFile || g.statNow("dead") != cFile || g.statNow("d/") != cDir || g.statNow("none") != 0 {
		t.Error("lstat and stat")
	}
	// A cache of "does not exist" is kept for a path with no slash, and checked for one with a slash.
	g2 := newGlobber(root, readNames)
	g2.cache[abs("f.md")] = cacheVal{kind: cFalse}
	if g2.statNow("f.md") != cFalse {
		t.Error("f.md")
	}
	if g2.statNow("f.md/") != 0 || g2.cache[abs("f.md")].kind != cFile {
		t.Error("f.md/")
	}
	// An error other than "not found" leaves a path that exists, as glob has it.
	g3 := newGlobber(root, readNames)
	g3.stat2("x", abs("x"), statResult{err: errors.New("other")}, func(k cacheKind) {
		if k != cTrue {
			t.Errorf("kind %v", k)
		}
	})
}

func TestWaitersRejoin(t *testing.T) {
	w := waiters[int]{}
	var got []string
	if !w.join("k", func(v int) {
		got = append(got, "a"+string(rune('0'+v)))
		// A callback that joins the key again is called with the same result, after the others.
		if v == 1 && len(got) == 1 {
			w.join("k", func(v int) { got = append(got, "late"+string(rune('0'+v))) })
		}
	}) {
		t.Fatal("the first caller starts the read")
	}
	if w.join("k", func(v int) { got = append(got, "b"+string(rune('0'+v))) }) {
		t.Fatal("the second caller waits")
	}
	w.done("k", 1)
	if want := "a1 b1 late1"; strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(w) != 0 {
		t.Error("the key is gone")
	}
}

// In JavaScript "." does not match a line end, so a name that starts with one matches no
// wildcard. The expected values come from glob 8.1.0.
func TestGlobLineEnds(t *testing.T) {
	root := tree(t, "\rx", " y", "a b", "ok")
	for pattern, want := range map[string][]string{
		"*":    {"a b", "ok"},
		"??":   {"ok"},
		"?y":   nil,
		"a?b":  {"a b"},
		"[^a]": nil,
	} {
		if got := Glob(pattern, root); !reflect.DeepEqual(orEmpty(got), orEmpty(want)) {
			t.Errorf("%q: %q, want %q", pattern, got, want)
		}
	}
}

func TestGlobMissingLiterals(t *testing.T) {
	root := tree(t, "a.md")
	if got := Glob("{nope,zz}", root); len(got) != 0 {
		t.Errorf("got %q", got)
	}
	if got := Glob("{a.md,nope}", root); !reflect.DeepEqual(got, []string{"a.md"}) {
		t.Errorf("got %q", got)
	}
}
