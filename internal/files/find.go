package files

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
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

type finder struct {
	o       Options
	ignores map[string]*Ignore // by directory; nil when none
}

// Find expands the inputs: globs, directories and files.
func Find(inputs []string, o Options) []Item {
	f := &finder{o: o, ignores: map[string]*Ignore{}}
	var paths, globbed []string
	for _, in := range inputs {
		if HasMagic(in) {
			globbed = append(globbed, f.glob(in)...)
			continue
		}
		rel, _ := filepath.Rel(o.Cwd, filepath.Join(o.Cwd, in))
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

// glob lists the files and directories that match a pattern.
func (f *finder) glob(pattern string) []string {
	var out []string
	for _, p := range expandBraces(pattern) {
		p = filepath.ToSlash(p)
		root := literalRoot(p)
		base := filepath.Join(f.o.Cwd, filepath.FromSlash(root))
		if strings.HasPrefix(p, "/") {
			base = filepath.FromSlash(root)
		}
		re := compile(strings.TrimPrefix(p, "./"))
		deep := strings.Contains(p, "**")
		dots := hasDotSegment(p)
		maxDepth := strings.Count(p, "/") + 1
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			rel, relErr := filepath.Rel(f.o.Cwd, path)
			if err != nil || relErr != nil || path == base {
				return nil
			}
			slash := filepath.ToSlash(rel)
			if strings.HasPrefix(p, "/") {
				slash = filepath.ToSlash(path)
			}
			if m := strings.TrimPrefix(slash, "./"); re.MatchString(m) && dotOK(m, dots) {
				out = append(out, rel)
			}
			if d.IsDir() && ((!dots && strings.HasPrefix(d.Name(), ".")) || (!deep && strings.Count(slash, "/")+1 >= maxDepth)) {
				return filepath.SkipDir
			}
			return nil
		})
	}
	sort.Strings(out)
	return out
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

func (f *finder) ignored(rel string, isDir bool) bool {
	abs := filepath.Join(f.o.Cwd, rel)
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
		st, statErr := os.Stat(filepath.Join(f.o.Cwd, p))
		isDir := statErr == nil && st.IsDir()
		ignored := f.ignored(p, isDir)
		if ignored && (nested || f.o.SilentlyIgnore) {
			continue
		}
		if !ignored && isDir {
			entries, err := os.ReadDir(filepath.Join(f.o.Cwd, p))
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
