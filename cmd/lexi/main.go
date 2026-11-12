// Command lexi checks text for insensitive, inconsiderate writing. It reads
// plain text, Markdown, HTML and MDX, and takes the same flags as alex.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"git.grey.ooo/grey.ooo/lexi/internal/config"
	"git.grey.ooo/grey.ooo/lexi/internal/files"
	"git.grey.ooo/grey.ooo/lexi/internal/input"
	"git.grey.ooo/grey.ooo/lexi/internal/report"
	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// version is set at build time.
var version = "dev"

// Replaced in tests.
var (
	exit       = os.Exit
	loadRules  = rules.Load
	loadWords  = rules.LoadProfanity
	getwd      = os.Getwd
	readFile   = os.ReadFile
	isTerminal = func(w io.Writer) bool {
		f, ok := w.(*os.File)
		if !ok {
			return false
		}
		st, err := f.Stat()
		return err == nil && st.Mode()&os.ModeCharDevice != 0
	}
)

const usage = `
  Catch insensitive, inconsiderate writing

  Usage: lexi [<glob> ...] [options ...]

  Options:

    -w, --why               output sources (when available)
    -q, --quiet             output only warnings and errors
    -t, --text              treat input as plain-text (not markdown)
    -l, --html              treat input as html (not markdown)
        --mdx               treat input as mdx (not markdown)
    -d, --diff              ignore unchanged lines (affects Travis only)
        --stdin             read from stdin
    -h, --help              output this help
    -v, --version           output the version

  When no input files are given, searches for markdown and text
  files in the current directory, ` + "`doc`, and `docs`" + `.

  Examples
    $ echo "His network looks good" | lexi --stdin
    $ lexi *.md
    $ lexi
`

type flags struct {
	why, quiet, text, html, mdx, diff, stdin, help, version bool
}

// parseArgs reads the flags and the inputs. Like alex, it ignores a flag that
// it does not know, and that flag takes the argument after it as its value.
func parseArgs(args []string) (flags, []string) {
	var f flags
	var in []string
	short := map[rune]*bool{'w': &f.why, 'q': &f.quiet, 't': &f.text, 'l': &f.html, 'd': &f.diff, 'h': &f.help, 'v': &f.version}
	long := map[string]*bool{"why": &f.why, "quiet": &f.quiet, "text": &f.text, "html": &f.html, "mdx": &f.mdx,
		"diff": &f.diff, "stdin": &f.stdin, "help": &f.help, "version": &f.version}
	for i := 0; i < len(args); i++ {
		a := args[i]
		takesValue := func() {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
		}
		switch {
		case a == "--":
			return f, append(in, args[i+1:]...)
		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a[2:], "=")
			negate := strings.HasPrefix(name, "no-")
			if p, ok := long[strings.TrimPrefix(name, "no-")]; ok {
				*p = !negate && (!hasVal || val != "false")
			} else if !hasVal {
				takesValue()
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for _, c := range a[1:] {
				if p, ok := short[c]; ok {
					*p = true
				} else if len(a) == 2 {
					takesValue()
				}
			}
		default:
			in = append(in, a)
		}
	}
	return f, in
}

func main() { exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func (f flags) kind() input.Kind {
	switch {
	case f.html:
		return input.HTML
	case f.mdx:
		return input.MDX
	case f.text:
		return input.Text
	}
	return input.Markdown
}

func (f flags) extensions() []string {
	switch {
	case f.html:
		return []string{".htm", ".html"}
	case f.mdx:
		return []string{".mdx"}
	}
	return []string{".txt", ".text", ".md", ".markdown", ".mkd", ".mkdn", ".mkdown", ".ron"}
}

// toUTF8 replaces each byte that is not UTF-8 with U+FFFD, as Node does.
func toUTF8(b []byte) string { return string([]rune(string(b))) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	f, inputs := parseArgs(args)
	switch {
	case f.help:
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	case f.version:
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	case f.stdin && len(inputs) > 0:
		_, _ = fmt.Fprintln(stderr, "lexi: Do not pass globs with `--stdin`")
		return 1
	}
	cwd, err := getwd()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "lexi:", err)
		return 1
	}
	rs, err := loadRules()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "lexi:", err)
		return 1
	}
	ws, err := loadWords()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "lexi:", err)
		return 1
	}
	c := &checker{f: f, rs: rs, ws: ws, cwd: cwd, cfgs: map[string]cfgResult{}}

	var results []report.File
	if f.stdin {
		b, err := io.ReadAll(stdin)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "lexi:", err)
			return 1
		}
		results = append(results, c.check("<stdin>", cwd, toUTF8(b)))
	} else {
		o := files.Options{Cwd: cwd, Extensions: f.extensions()}
		if len(inputs) == 0 {
			ext := strings.ReplaceAll(strings.Join(f.extensions(), ","), ".", "")
			inputs = []string{"{docs/**/,doc/**/,}*.{" + ext + "}"}
			o.SilentlyIgnore = true
		}
		for _, it := range files.Find(inputs, o) {
			results = append(results, c.checkFile(it))
		}
		sort.SliceStable(results, func(a, b int) bool { return results[a].Name < results[b].Name })
	}
	if f.diff {
		for i := range results {
			results[i].Messages = diffFilter(cwd, results[i].Name, results[i].Messages)
		}
	}
	text, errs, warns := report.Format(results, report.Options{
		Quiet: f.quiet, Verbose: f.why, Color: useColor(stderr),
	})
	if text != "" {
		_, _ = fmt.Fprintln(stderr, text)
	}
	if errs+warns > 0 {
		return 1
	}
	return 0
}

// useColor follows supports-color: a terminal gets color, FORCE_COLOR forces
// it and NO_COLOR turns it off.
func useColor(w io.Writer) bool {
	if v, ok := os.LookupEnv("FORCE_COLOR"); ok {
		return v != "0" && v != "false"
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return isTerminal(w)
}

type cfgResult struct {
	cfg config.Config
	err error
}

type checker struct {
	f    flags
	rs   []rules.Rule
	ws   []rules.Word
	cwd  string
	cfgs map[string]cfgResult
}

func (c *checker) config(dir string) cfgResult {
	if r, ok := c.cfgs[dir]; ok {
		return r
	}
	cfg, err := config.Find(dir)
	r := cfgResult{cfg, err}
	c.cfgs[dir] = r
	return r
}

func fatal(reason string) report.Message {
	return report.Message{Line: 1, Column: 1, Fatal: true, Reason: reason}
}

func (c *checker) checkFile(it files.Item) report.File {
	out := report.File{Name: it.Path}
	if it.Err != "" {
		out.Messages = []report.Message{fatal(it.Err)}
		return out
	}
	b, err := readFile(filepath.Join(c.cwd, it.Path))
	if err != nil {
		out.Messages = []report.Message{fatal(errReason(err))}
		return out
	}
	return c.check(it.Path, filepath.Dir(filepath.Join(c.cwd, it.Path)), toUTF8(b))
}

// errReason is the reason of a read error, without the path.
func errReason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return upperFirst(err.Error())
}

func upperFirst(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

func (c *checker) check(name, dir, text string) report.File {
	out := report.File{Name: name}
	cr := c.config(dir)
	if cr.err != nil {
		out.Messages = []report.Message{fatal("Error: " + upperFirst(strings.ReplaceAll(cr.err.Error(), c.cwd+string(filepath.Separator), "")))}
		return out
	}
	cfg := cr.cfg
	doc := input.Extract(c.f.kind(), text)
	found, err := doc.Check(c.rs, c.ws, rules.Options{Allow: cfg.Allow, Deny: cfg.Deny, NoBinary: cfg.NoBinary, Sureness: cfg.ProfanitySureness})
	if err != nil {
		out.Messages = []report.Message{fatal("Error: " + upperFirst(err.Error()))}
		return out
	}
	if e := doc.Err(); e != nil {
		out.Messages = append(out.Messages, report.Message{
			Line: e.Line, Column: e.Column, EndLine: e.EndLine, EndColumn: e.EndColumn, Fatal: true, Reason: e.Message,
		})
	}
	for _, m := range found {
		out.Messages = append(out.Messages, report.Message{
			Line: m.Line, Column: m.Column, EndLine: m.EndLine, EndColumn: m.EndColumn,
			Reason: m.Message, Note: m.Note, RuleID: m.RuleID, Source: m.Source,
		})
	}
	return out
}
