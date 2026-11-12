// Package files finds the files that lexi checks. It follows unified-engine:
// globs, directories with default extensions, and .alexignore files.
package files

import (
	"regexp"
	"strings"
)

// HasMagic reports whether a path holds glob characters.
func HasMagic(p string) bool { return strings.ContainsAny(p, "*?[{") }

// expandBraces turns `a{b,c}d` into `abd` and `acd`.
func expandBraces(p string) []string {
	open := strings.IndexByte(p, '{')
	if open < 0 {
		return []string{p}
	}
	depth, last := 0, open+1
	var parts []string
	for i := open; i < len(p); i++ {
		switch p[i] {
		case '{':
			depth++
		case ',':
			if depth == 1 {
				parts = append(parts, p[last:i])
				last = i + 1
			}
		case '}':
			depth--
			if depth == 0 {
				parts = append(parts, p[last:i])
				var out []string
				for _, part := range parts {
					out = append(out, expandBraces(p[:open]+part+p[i+1:])...)
				}
				return out
			}
		}
	}
	return []string{p}
}

// compile turns a glob without braces into a regular expression over slash
// paths. Dot names are left to dotOK.
func compile(p string) *regexp.Regexp {
	segs := strings.Split(p, "/")
	var parts []string
	for i, seg := range segs {
		switch {
		case seg == "**" && i < len(segs)-1:
			parts = append(parts, `(?:[^/]+/)*`)
		case seg == "**": // matches the directory itself, and all below it
			parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], "/")
			parts = append(parts, `(?:/[^/]+)*`)
		default:
			parts = append(parts, segment(seg))
			if i < len(segs)-1 {
				parts = append(parts, "/")
			}
		}
	}
	return regexp.MustCompile("^" + strings.Join(parts, "") + "$")
}

func segment(seg string) string {
	var sb strings.Builder
	for i := 0; i < len(seg); i++ {
		switch c := seg[i]; c {
		case '*':
			sb.WriteString(`[^/]*`)
		case '?':
			sb.WriteString(`[^/]`)
		case '[':
			end := strings.IndexByte(seg[i+1:], ']')
			if end < 1 {
				sb.WriteString(`\[`)
				continue
			}
			class := seg[i+1 : i+1+end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			sb.WriteString("[" + strings.ReplaceAll(class, `\`, `\\`) + "]")
			i += end + 1
		case '\\':
			if i+1 < len(seg) {
				i++
				sb.WriteString(regexp.QuoteMeta(seg[i : i+1]))
			}
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return sb.String()
}

// literalRoot returns the directory that a glob starts from: the leading
// segments without glob characters.
func literalRoot(p string) string {
	segs := strings.Split(p, "/")
	var root []string
	for _, s := range segs[:len(segs)-1] {
		if HasMagic(s) {
			break
		}
		root = append(root, s)
	}
	if len(root) == 0 {
		return "."
	}
	return strings.Join(root, "/")
}

// hasDotSegment reports whether a glob names a dot directory on purpose.
func hasDotSegment(p string) bool {
	for _, s := range strings.Split(p, "/") {
		if strings.HasPrefix(s, ".") && s != "." && s != ".." {
			return true
		}
	}
	return false
}

// dotOK reports whether a path may match a glob. A wildcard does not match a
// name that starts with a dot, so such a path needs a dot in the glob itself.
func dotOK(path string, dots bool) bool {
	if dots {
		return true
	}
	for _, s := range strings.Split(path, "/") {
		if strings.HasPrefix(s, ".") {
			return false
		}
	}
	return true
}
