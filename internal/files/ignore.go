package files

import (
	"regexp"
	"strings"
)

// Ignore holds the patterns of one .alexignore file. The format is the one of
// .gitignore.
type Ignore struct {
	rules []rule
}

type rule struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
}

// ParseIgnore reads the patterns of an ignore file.
func ParseIgnore(text string) *Ignore {
	ig := &Ignore{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		for strings.HasSuffix(line, " ") && !strings.HasSuffix(line, `\ `) {
			line = line[:len(line)-1]
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := rule{}
		if strings.HasPrefix(line, "!") {
			r.negate = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			r.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		if line == "" {
			continue
		}
		r.re = patternRegexp(line)
		ig.rules = append(ig.rules, r)
	}
	return ig
}

// patternRegexp compiles a gitignore pattern. A pattern with a slash inside
// or at the start matches from the root; any other matches at any depth.
func patternRegexp(p string) *regexp.Regexp {
	anchored := strings.Contains(p, "/")
	p = strings.TrimPrefix(p, "/")
	var sb strings.Builder
	sb.WriteString("^")
	if !anchored {
		sb.WriteString("(?:.*/)?")
	}
	segs := strings.Split(p, "/")
	for i, seg := range segs {
		last := i == len(segs)-1
		if seg == "**" {
			if last {
				sb.WriteString(".*")
			} else {
				sb.WriteString("(?:.*/)?")
			}
			continue
		}
		var body strings.Builder
		for j := 0; j < len(seg); j++ {
			switch c := seg[j]; c {
			case '*':
				body.WriteString(`[^/]*`)
			case '?':
				body.WriteString(`[^/]`)
			case '[':
				end := strings.IndexByte(seg[j+1:], ']')
				if end < 1 {
					body.WriteString(`\[`)
					continue
				}
				class := seg[j+1 : j+1+end]
				if strings.HasPrefix(class, "!") {
					class = "^" + class[1:]
				}
				body.WriteString("[" + class + "]")
				j += end + 1
			case '\\':
				if j+1 < len(seg) {
					j++
					body.WriteString(regexp.QuoteMeta(seg[j : j+1]))
				}
			default:
				body.WriteString(regexp.QuoteMeta(string(c)))
			}
		}
		sb.WriteString(body.String())
		if !last {
			sb.WriteString("/")
		}
	}
	sb.WriteString("$")
	return regexp.MustCompile(sb.String())
}

// Ignores reports whether a slash path, relative to the ignore file, is
// ignored. isDir tells whether the path is a directory. A path in an ignored
// directory is ignored.
func (ig *Ignore) Ignores(path string, isDir bool) bool {
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		if ig.match(strings.Join(parts[:i], "/"), true) {
			return true
		}
	}
	return ig.match(path, isDir)
}

func (ig *Ignore) match(path string, isDir bool) bool {
	ignored := false
	for _, r := range ig.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.re.MatchString(path) {
			ignored = !r.negate
		}
	}
	return ignored
}
