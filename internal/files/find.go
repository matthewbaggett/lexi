package files

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// IgnoreName is the name of the ignore file.
const IgnoreName = ".alexignore"

// Item is a file to check. Err is set for a file that cannot be checked.
type Item struct {
	Path string // relative to the working directory
	Err  string
}

// Options controls a search.
type Options struct {
	Cwd string
	// Extensions are the extensions, with a dot, of the files that a
	// directory search finds.
	Extensions []string
	// SilentlyIgnore skips ignored files without an error.
	SilentlyIgnore bool
}

// Messages of files that cannot be checked.
const (
	ErrNotFound = "No such file or directory"
	ErrIgnored  = "Cannot process specified file: it’s ignored"
)

// readDir is replaced in tests.
var readDir = os.ReadDir

type finder struct {
	o       Options
	ignores map[string]*Ignore // by directory; nil when none
}

// Find expands the inputs: globs, directories and files.
//
// A path in a report is what alex prints. An argument that is no glob is made relative to the
// working directory. A match of a glob stays as the glob gives it, so "./src/*.md" gives
// "./src/a.md", and a directory gives the joined paths of the files below it.
func Find(inputs []string, o Options) []Item {
	f := &finder{o: o, ignores: map[string]*Ignore{}}
	var paths, globbed []string
	for _, in := range inputs {
		if HasMagic(in) {
			globbed = append(globbed, Glob(in, o.Cwd)...)
			continue
		}
		if !filepath.IsAbs(in) {
			in = filepath.Join(o.Cwd, in)
		}
		rel, _ := filepath.Rel(o.Cwd, in)
		paths = append(paths, rel)
	}
	paths = append(paths, globbed...)
	var out []Item
	seen := map[string]bool{}
	for _, it := range f.search(paths, false) {
		if !seen[it.Path] {
			seen[it.Path] = true
			out = append(out, it)
		}
	}
	return out
}

// abs makes a path of a report absolute.
func (f *finder) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(f.o.Cwd, p)
}

// ignoreFor finds the .alexignore file nearest to dir, going up.
func (f *finder) ignoreFor(dir string) (*Ignore, string) {
	for {
		if ig, ok := f.ignores[dir]; ok {
			if ig != nil {
				return ig, dir
			}
		} else {
			b, err := os.ReadFile(filepath.Join(dir, IgnoreName))
			if err == nil {
				ig = ParseIgnore(string(b))
				f.ignores[dir] = ig
				return ig, dir
			}
			f.ignores[dir] = nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ""
		}
		dir = parent
	}
}

func (f *finder) ignored(p string, isDir bool) bool {
	abs := f.abs(p)
	ig, dir := f.ignoreFor(filepath.Dir(abs))
	if ig == nil {
		return false
	}
	r, _ := filepath.Rel(dir, abs)
	return ig.Ignores(filepath.ToSlash(r), isDir)
}

// search stats each path. A directory gives its files, one level at a time.
func (f *finder) search(paths []string, nested bool) []Item {
	var out []Item
	for _, p := range paths {
		name := filepath.Base(p)
		if nested && (strings.HasPrefix(name, ".") || name == "node_modules") {
			continue
		}
		st, statErr := os.Stat(f.abs(p))
		isDir := statErr == nil && st.IsDir()
		ignored := f.ignored(p, isDir)
		if ignored && (nested || f.o.SilentlyIgnore) {
			continue
		}
		if !ignored && isDir {
			entries, err := readDir(f.abs(p))
			if err != nil {
				out = append(out, Item{p, "Cannot read directory"})
				continue
			}
			var kids []string
			for _, e := range entries {
				kids = append(kids, filepath.Join(p, e.Name()))
			}
			out = append(out, f.search(kids, true)...)
			continue
		}
		if !isDir && nested && !slices.Contains(f.o.Extensions, filepath.Ext(p)) {
			continue
		}
		it := Item{Path: p}
		switch {
		case ignored:
			it.Err = ErrIgnored
		case errors.Is(statErr, fs.ErrNotExist):
			it.Err = ErrNotFound
		}
		out = append(out, it)
	}
	return out
}
