// Package rules holds the inclusive-language rule set and the matcher.
package rules

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

//go:embed data.json
var dataJSON []byte

// Rule flags a set of terms and suggests alternatives.
type Rule struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"` // "basic" or "or": see alex
	Terms       []string `json:"terms"`
	Considerate []string `json:"considerate"`
	Note        string   `json:"note"`
	Severity    int      `json:"severity"` // 1 = maybe, 2 = likely
	re          *regexp.Regexp
}

// Finding is one match in a text.
type Finding struct {
	Line    int
	Column  int
	Term    string
	RuleID  string
	Message string
	Fatal   bool
}

// Options controls a check.
type Options struct {
	Allow []string // rule IDs or terms to ignore
	Deny  []string // rule IDs or terms that are always fatal
	Maybe bool     // report severity 1 findings
}

// Load returns the embedded rule set, compiled.
func Load() ([]Rule, error) { return load(dataJSON) }

func load(data []byte) ([]Rule, error) {
	var rs []Rule
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, err
	}
	return compile(rs), nil
}

// compile builds each rule's matcher.
func compile(rs []Rule) []Rule {
	for i := range rs {
		parts := make([]string, len(rs[i].Terms))
		for j, t := range rs[i].Terms {
			parts[j] = regexp.QuoteMeta(strings.ToLower(t))
		}
		sort.Slice(parts, func(a, b int) bool { return len(parts[a]) > len(parts[b]) })
		rs[i].re = regexp.MustCompile(`(?i)\b(` + strings.Join(parts, "|") + `)\b`)
	}
	return rs
}

func in(list []string, vals ...string) bool {
	for _, l := range list {
		for _, v := range vals {
			if strings.EqualFold(l, v) {
				return true
			}
		}
	}
	return false
}

// Check scans text and returns findings ordered by position.
func Check(rs []Rule, text string, o Options) []Finding {
	var out []Finding
	for ln, line := range strings.Split(text, "\n") {
		for _, r := range rs {
			if r.Severity < 2 && !o.Maybe {
				continue
			}
			for _, m := range r.re.FindAllStringIndex(line, -1) {
				term := line[m[0]:m[1]]
				if in(o.Allow, r.ID, term) {
					continue
				}
				msg := "`" + term + "` may be insensitive"
				if len(r.Considerate) > 0 {
					msg += ", use " + joinOr(r.Considerate) + " instead"
				}
				out = append(out, Finding{
					Line: ln + 1, Column: m[0] + 1, Term: term, RuleID: r.ID,
					Message: msg, Fatal: in(o.Deny, r.ID, term) || r.Severity >= 2,
				})
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Line != out[b].Line {
			return out[a].Line < out[b].Line
		}
		return out[a].Column < out[b].Column
	})
	return out
}

func joinOr(s []string) string {
	q := make([]string, len(s))
	for i, v := range s {
		q[i] = "`" + v + "`"
	}
	if len(q) == 1 {
		return q[0]
	}
	return strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
}
