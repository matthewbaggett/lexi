// Package files finds the files that lexi checks. It follows unified-engine:
// globs, directories with default extensions, and .alexignore files.
package files

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// This file is a port of glob 8.1.0, the library that unified-engine 10.1.0 (and so alex
// 11.0.1) calls for each argument with a wildcard. The pattern parser is in minimatch.go. glob
// runs with its default options and a cwd: no dot, no nocase, no follow, no mark.

// HasMagic reports whether glob treats a path as a pattern: it has more than one result
// after the braces, or a part with a wildcard. An empty path is no pattern.
func HasMagic(p string) bool {
	if p == "" {
		return false
	}
	set := parseSet(p)
	if len(set) == 0 { // "{,}" has no result. glob throws a TypeError here: lexi reads a path.
		return false
	}
	if len(set) > 1 {
		return true
	}
	for _, pt := range set[0] {
		if pt.kind != partLit {
			return true
		}
	}
	return false
}

// Glob returns the paths that match a pattern, as glob does for a cwd: relative to the cwd
// unless the pattern is absolute. glob sorts them as String.localeCompare("en") does, and lexi
// sorts the bytes: the report of lexi has its own order, so the order here does not show.
//
// glob reads the file system with callbacks, and its answer can depend on the order in which
// the reads end: a failed read of "a.md/a" marks that path as a file, and a later check of it
// finds it. alex runs the callback version, so this port does too: each read goes in a queue
// and ends in the order it started, which is what a thread pool of small reads does in practice.
func Glob(pattern, cwd string) []string {
	return globWith(pattern, cwd, readNames)
}

func newGlobber(cwd string, read func(string) ([]string, error)) *globber {
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	return &globber{
		cwd:       cwd,
		cache:     map[string]cacheVal{},
		statCache: map[string]statVal{},
		symlinks:  map[string]bool{},
		found:     map[string]bool{},
		reads:     waiters[dirResult]{},
		stats:     waiters[statResult]{},
		readNames: read,
	}
}

// globWith is Glob with another way to list a directory.
func globWith(pattern, cwd string, read func(string) ([]string, error)) []string {
	g := newGlobber(cwd, read)
	for _, parts := range parseSet(pattern) {
		g.process(parts, false)
	}
	for len(g.queue) > 0 {
		next := g.queue[0]
		g.queue = g.queue[1:]
		next()
	}
	sort.Strings(g.order)
	return g.order
}

type cacheKind int

const (
	cTrue cacheKind = iota + 1 // exists
	cDir
	cFile
	cFalse // does not exist
	cList  // a directory, with its entries
)

type cacheVal struct {
	kind    cacheKind
	entries []string
}

// statVal is an entry of the stat cache: a path that does not exist, or what it is.
type statVal struct{ missing, dir bool }

type dirResult struct {
	entries []string
	ok      bool
}

type statResult struct {
	info fs.FileInfo
	err  error
}

// waiters is the inflight package: the callbacks that wait for one read, by key.
type waiters[T any] map[string][]func(T)

// join adds a callback, and tells whether it is the first for the key: only the first starts
// the read.
func (w waiters[T]) join(key string, f func(T)) bool {
	first := len(w[key]) == 0
	w[key] = append(w[key], f)
	return first
}

// done calls the callbacks of a key. A callback that joins the key again is called with the
// same result, after the others.
func (w waiters[T]) done(key string, v T) {
	for {
		list := w[key]
		for _, f := range list {
			f(v)
		}
		if len(w[key]) == len(list) {
			delete(w, key)
			return
		}
		w[key] = w[key][len(list):]
	}
}

type globber struct {
	cwd       string
	cache     map[string]cacheVal
	statCache map[string]statVal
	symlinks  map[string]bool
	found     map[string]bool
	order     []string
	queue     []func()
	reads     waiters[dirResult]
	stats     waiters[statResult]
	// readNames lists a directory. Tests replace it.
	readNames func(string) ([]string, error)
}

// later queues the end of a read.
func (g *globber) later(f func()) { g.queue = append(g.queue, f) }

func readNames(dir string) ([]string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return f.Readdirnames(-1)
}

func isAbs(p string) bool { return strings.HasPrefix(p, "/") }

// normalize is path.normalize of Node for an absolute path: it resolves "." and "..", and
// keeps a slash at the end.
func normalize(p string) string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		switch s {
		case "", ".":
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, s)
		}
	}
	r := "/" + strings.Join(out, "/")
	if strings.HasSuffix(p, "/") && r != "/" {
		r += "/"
	}
	return r
}

func (g *globber) makeAbs(f string) string {
	switch {
	case isAbs(f):
		return normalize(f)
	case f == "":
		return ""
	}
	return strings.TrimSuffix(normalize(g.cwd+"/"+f), "/")
}

func joinLits(pattern []part) string {
	s := make([]string, len(pattern))
	for i, p := range pattern {
		s[i] = p.lit
	}
	return strings.Join(s, "/")
}

func (g *globber) process(pattern []part, inGlobStar bool) {
	n := 0
	for n < len(pattern) && pattern[n].kind == partLit {
		n++
	}
	if n == len(pattern) {
		g.simple(joinLits(pattern))
		return
	}
	prefix, hasPrefix := "", n > 0
	if hasPrefix {
		prefix = joinLits(pattern[:n])
	}
	remain := pattern[n:]
	read := "."
	if hasPrefix {
		if prefix == "" { // the pattern starts with a slash: "/*"
			prefix = "/"
		}
		read = prefix
	}
	abs := g.makeAbs(read)
	if remain[0].kind == partGlobstar {
		g.globstar(prefix, abs, remain, inGlobStar)
		return
	}
	g.readdirMatch(prefix, abs, remain, inGlobStar)
}

func joinPrefix(prefix, e string) string {
	if prefix == "" {
		return e
	}
	if prefix == "/" {
		return prefix + e
	}
	return prefix + "/" + e
}

func (g *globber) readdirMatch(prefix, abs string, remain []part, inGlobStar bool) {
	g.readdir(abs, inGlobStar, func(r dirResult) {
		if !r.ok {
			return
		}
		pn := remain[0]
		dotOK := strings.HasPrefix(pn.glob, ".")
		var matched []string
		for _, e := range r.entries {
			if e[0] == '.' && !dotOK {
				continue
			}
			if m, _ := pn.re.MatchString(js16(e)); m {
				matched = append(matched, e)
			}
		}
		for _, e := range matched {
			e = joinPrefix(prefix, e)
			if len(remain) > 1 {
				g.process(append([]part{{kind: partLit, lit: e}}, remain[1:]...), inGlobStar)
				continue
			}
			if isAbs(e) {
				e = normalize(e)
			}
			g.emit(e)
		}
	})
}

func (g *globber) emit(e string) {
	if !g.found[e] {
		g.found[e] = true
		g.order = append(g.order, e)
	}
}

func (g *globber) globstar(prefix, abs string, remain []part, inGlobStar bool) {
	g.readdir(abs, inGlobStar, func(r dirResult) {
		if !r.ok {
			return
		}
		var pref []part
		if prefix != "" {
			pref = []part{{kind: partLit, lit: prefix}}
		}
		rest := remain[1:]
		g.process(concat(pref, rest), false)
		if g.symlinks[abs] && inGlobStar {
			return
		}
		for _, e := range r.entries {
			if e[0] == '.' {
				continue
			}
			lit := part{kind: partLit, lit: e}
			g.process(concat(pref, []part{lit}, rest), true)
			g.process(concat(pref, []part{lit}, remain), true)
		}
	})
}

func concat(lists ...[]part) []part {
	var out []part
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

func (g *globber) simple(prefix string) {
	g.stat(prefix, func(k cacheKind) {
		if k == 0 || k == cFalse {
			return
		}
		if isAbs(prefix) {
			prefix = normalize(prefix)
		}
		g.emit(prefix)
	})
}

// stat is glob's _stat: it tells what a path is, or 0 when it does not exist.
func (g *globber) stat(f string, cb func(cacheKind)) {
	abs := g.makeAbs(f)
	needDir := strings.HasSuffix(f, "/")
	if c, ok := g.cache[abs]; ok {
		k := c.kind
		if k == cList {
			k = cDir
		}
		if !needDir || k == cDir {
			cb(k)
			return
		}
		if k == cFile {
			cb(0)
			return
		}
	}
	if s, ok := g.statCache[abs]; ok {
		switch {
		case s.missing:
			cb(cFalse)
		case s.dir:
			cb(cDir)
		case needDir:
			cb(0)
		default:
			cb(cFile)
		}
		return
	}
	key := abs
	if !g.stats.join(key, func(r statResult) { g.lstatDone(f, abs, r, cb) }) {
		return
	}
	g.later(func() {
		info, err := os.Lstat(abs)
		g.stats.done(key, statResult{info, err})
	})
}

func (g *globber) lstatDone(f, abs string, r statResult, cb func(cacheKind)) {
	if r.info == nil || r.info.Mode()&fs.ModeSymlink == 0 {
		g.stat2(f, abs, r, cb)
		return
	}
	// A link counts as its target. When the target is gone, the link is a file.
	g.later(func() {
		if st, err := os.Stat(abs); err == nil {
			g.stat2(f, abs, statResult{info: st}, cb)
			return
		}
		g.stat2(f, abs, statResult{info: r.info}, cb)
	})
}

func (g *globber) stat2(f, abs string, r statResult, cb func(cacheKind)) {
	if r.err != nil && (errors.Is(r.err, fs.ErrNotExist) || errors.Is(r.err, syscall.ENOTDIR)) {
		g.statCache[abs] = statVal{missing: true}
		cb(0)
		return
	}
	if r.info != nil {
		g.statCache[abs] = statVal{dir: r.info.IsDir()}
	}
	c := cTrue
	if r.info != nil {
		c = cFile
		if r.info.IsDir() {
			c = cDir
		}
	}
	if old, ok := g.cache[abs]; !ok || old.kind == cFalse {
		g.cache[abs] = cacheVal{kind: c}
	}
	if strings.HasSuffix(f, "/") && c == cFile {
		cb(0)
		return
	}
	cb(c)
}

// readdir is glob's _readdir.
func (g *globber) readdir(abs string, inGlobStar bool, cb func(dirResult)) {
	key := abs + "\x00" + strconv.FormatBool(inGlobStar)
	if !g.reads.join(key, cb) {
		return
	}
	done := func(r dirResult) { g.reads.done(key, r) }
	if _, known := g.symlinks[abs]; inGlobStar && !known {
		g.readdirInGlobStar(abs, done)
		return
	}
	if c, ok := g.cache[abs]; ok {
		switch c.kind {
		case cFalse, cFile:
			done(dirResult{})
			return
		case cList:
			done(dirResult{c.entries, true})
			return
		}
	}
	g.later(func() {
		names, err := g.readNames(abs)
		if err != nil {
			g.cache[abs] = cacheVal{kind: cFalse}
			if errors.Is(err, syscall.ENOTDIR) {
				g.cache[abs] = cacheVal{kind: cFile}
			}
			done(dirResult{})
			return
		}
		for _, e := range names {
			if abs == "/" {
				g.cache["/"+e] = cacheVal{kind: cTrue}
			} else {
				g.cache[abs+"/"+e] = cacheVal{kind: cTrue}
			}
		}
		g.cache[abs] = cacheVal{kind: cList, entries: names}
		done(dirResult{names, true})
	})
}

// readdirInGlobStar does not follow a symbolic link below a **.
func (g *globber) readdirInGlobStar(abs string, cb func(dirResult)) {
	g.later(func() {
		info, err := os.Lstat(abs)
		if errors.Is(err, fs.ErrNotExist) {
			cb(dirResult{})
			return
		}
		isSym := info != nil && info.Mode()&fs.ModeSymlink != 0
		g.symlinks[abs] = isSym
		if !isSym && info != nil && !info.IsDir() {
			g.cache[abs] = cacheVal{kind: cFile}
			cb(dirResult{})
			return
		}
		g.readdir(abs, false, cb)
	})
}
