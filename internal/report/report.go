// Package report prints messages the way vfile-reporter does, which is the
// reporter of alex.
package report

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Message is one finding or error.
type Message struct {
	Line, Column       int
	EndLine, EndColumn int // zero when the message has no end
	Fatal              bool
	Reason             string
	Note               string
	RuleID, Source     string
}

// File is a checked file and its messages, in report order.
type File struct {
	Name     string
	Messages []Message
}

// Options controls the output.
type Options struct {
	Quiet   bool // leave out files without messages
	Verbose bool // add the note of each message
	Color   bool
}

type row struct {
	file                                 *File
	place, label, reason, ruleID, source string
}

func place(m Message) string {
	if m.EndLine > 0 && m.EndColumn > 0 {
		return fmt.Sprintf("%d:%d-%d:%d", m.Line, m.Column, m.EndLine, m.EndColumn)
	}
	return fmt.Sprintf("%d:%d", m.Line, m.Column)
}

func width(s string) int {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return runewidth.StringWidth(s)
}

// Format renders the report for files. It returns the text, with no final
// line break, and the number of errors and warnings.
func Format(files []File, o Options) (text string, errors, warnings int) {
	var rows []row
	var sizes [5]int
	for i := range files {
		f := &files[i]
		var mine []row
		for _, m := range f.Messages {
			r := row{file: f, place: place(m), label: "warning", reason: m.Reason, ruleID: m.RuleID, source: m.Source}
			if m.Fatal {
				r.label = "error"
				errors++
			} else {
				warnings++
			}
			if o.Verbose && m.Note != "" {
				r.reason += "\n" + m.Note
			}
			for j, v := range []string{r.place, r.label, r.reason, r.ruleID, r.source} {
				sizes[j] = max(sizes[j], width(v))
			}
			mine = append(mine, r)
		}
		if !o.Quiet || len(mine) > 0 {
			rows = append(rows, row{file: f, label: "file"})
			rows = append(rows, mine...)
		}
	}
	var lines []string
	for i, r := range rows {
		if r.label == "file" {
			lines = appendHeader(lines, rows, i, o)
			continue
		}
		reason, rest := r.reason, ""
		if k := strings.IndexAny(reason, "\r\n"); k >= 0 {
			reason, rest = reason[:k], reason[k:]
		}
		label := r.label
		if o.Color {
			c := "\x1b[33m"
			if r.label == "error" {
				c = "\x1b[31m"
			}
			label = c + r.label + "\x1b[39m"
		}
		line := "  " + strings.Repeat(" ", sizes[0]-width(r.place)) + r.place +
			"  " + label + strings.Repeat(" ", sizes[1]-width(r.label)) +
			"  " + reason + strings.Repeat(" ", sizes[2]-width(reason)) +
			"  " + r.ruleID + strings.Repeat(" ", sizes[3]-width(r.ruleID)) +
			"  " + r.source
		lines = append(lines, strings.TrimRight(line, " ")+rest)
	}
	if errors > 0 || warnings > 0 {
		lines = append(lines, "", summary(errors, warnings, o.Color))
	}
	return strings.Join(lines, "\n"), errors, warnings
}

func appendHeader(lines []string, rows []row, i int, o Options) []string {
	f := rows[i].file
	line := f.Name
	if o.Color {
		c := "\x1b[32m"
		switch {
		case anyFatal(f):
			c = "\x1b[31m"
		case len(f.Messages) > 0:
			c = "\x1b[33m"
		}
		line = "\x1b[4m" + c + f.Name + "\x1b[39m\x1b[24m"
	}
	if len(f.Messages) == 0 {
		line += ": no issues found"
	}
	if i > 0 && rows[i-1].label != "file" {
		lines = append(lines, "")
	}
	return append(lines, line)
}

func anyFatal(f *File) bool {
	for _, m := range f.Messages {
		if m.Fatal {
			return true
		}
	}
	return false
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func summary(errors, warnings int, color bool) string {
	paint := func(code, s string) string {
		if color {
			return code + s + "\x1b[39m"
		}
		return s
	}
	var line string
	if errors > 0 {
		line = paint("\x1b[31m", "✖") + " " + plural(errors, "error")
	}
	if warnings > 0 {
		if line != "" {
			line += ", "
		}
		line += paint("\x1b[33m", "⚠") + " " + plural(warnings, "warning")
	}
	if errors > 0 && warnings > 0 {
		line = fmt.Sprintf("%d messages (%s)", errors+warnings, line)
	}
	return line
}
