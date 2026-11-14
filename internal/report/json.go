package report

import (
	"strconv"
	"strings"
)

// JSONOptions controls the JSON output.
type JSONOptions struct {
	Quiet bool   // leave out files without messages
	Cwd   string // the working directory, as the `cwd` of each file
}

// JSON renders files the way vfile-reporter-json 4.0.0 does: one line of
// compact JSON, with the fields of each file and each message in the order of
// the package. A field that the package leaves out, because its value is
// undefined, is not in the output. A file that has no path, such as the
// standard input, has no `path` field and an empty `history`.
func JSON(files []File, o JSONOptions) string {
	var b strings.Builder
	b.WriteByte('[')
	n := 0
	for i := range files {
		f := &files[i]
		if o.Quiet && len(f.Messages) == 0 {
			continue
		}
		if n > 0 {
			b.WriteByte(',')
		}
		n++
		writeFile(&b, f, o.Cwd)
	}
	b.WriteByte(']')
	return b.String()
}

func writeFile(b *strings.Builder, f *File, cwd string) {
	b.WriteByte('{')
	history := "[]"
	if !f.Stdin {
		b.WriteString(`"path":`)
		writeString(b, f.Name)
		b.WriteByte(',')
		history = "[" + quote(f.Name) + "]"
	}
	b.WriteString(`"cwd":`)
	writeString(b, cwd)
	b.WriteString(`,"history":` + history + `,"messages":[`)
	for i, m := range f.Messages {
		if i > 0 {
			b.WriteByte(',')
		}
		writeMessage(b, m)
	}
	b.WriteString("]}")
}

func writeMessage(b *strings.Builder, m Message) {
	b.WriteString(`{"column":` + number(m.Column, m.NoPlace))
	b.WriteString(`,"fatal":` + strconv.FormatBool(m.Fatal))
	b.WriteString(`,"line":` + number(m.Line, m.NoPlace))
	b.WriteString(`,"reason":` + quote(m.Reason))
	b.WriteString(`,"ruleId":` + nullable(m.RuleID))
	b.WriteString(`,"source":` + nullable(m.Source))
	if m.Actual != "" {
		b.WriteString(`,"actual":` + quote(m.Actual))
	}
	if m.Expected != nil {
		b.WriteString(`,"expected":[`)
		for i, e := range m.Expected {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, e)
		}
		b.WriteByte(']')
	}
	if m.Note != "" {
		b.WriteString(`,"note":` + quote(m.Note))
	}
	if m.URL != "" {
		b.WriteString(`,"url":` + quote(m.URL))
	}
	b.WriteByte('}')
}

func number(n int, null bool) string {
	if null {
		return "null"
	}
	return strconv.Itoa(n)
}

func nullable(s string) string {
	if s == "" {
		return "null"
	}
	return quote(s)
}

func quote(s string) string {
	var b strings.Builder
	writeString(&b, s)
	return b.String()
}

// writeString writes s as JSON.stringify does: it escapes the quote, the
// backslash and the control characters, and nothing else.
func writeString(b *strings.Builder, s string) {
	const hex = "0123456789abcdef"
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			b.WriteString(`\u00` + string(hex[r>>4]) + string(hex[r&15]))
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
