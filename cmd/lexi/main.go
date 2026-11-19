// Command lexi checks text for insensitive, inconsiderate writing. It reads
// plain text, Markdown, HTML and MDX, and takes the same flags as alex.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	"git.grey.ooo/grey.ooo/lexi/internal/config"
	"git.grey.ooo/grey.ooo/lexi/internal/files"
	"git.grey.ooo/grey.ooo/lexi/internal/input"
	"git.grey.ooo/grey.ooo/lexi/internal/report"
	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// version is set at build time with -ldflags "-X main.version=<version>".
var version = "dev"

// alexVersion is the alex release that lexi follows.
const alexVersion = "11.0.1"

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
        --reporter=REPORTER use a custom vfile-reporter
    --stdin                 read from stdin

  When no input files are given, searches for markdown and text
  files in the current directory, ` + "`doc`, and `docs`" + `.

  Examples
    $ echo "His network looks good" | lexi --stdin
    $ lexi *.md !example.md
    $ lexi

`

type flags struct {
	why, quiet, text, html, mdx, diff, stdin, help, version bool
	reporter                                                string
	reporters                                               int // how many times --reporter is given
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
			if name == "reporter" {
				if !hasVal && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					val = args[i]
				}
				f.reporter = val
				f.reporters++
				continue
			}
			negate := strings.HasPrefix(name, "no-")
			if p, ok := long[strings.TrimPrefix(name, "no-")]; ok {
				*p = !negate && (!hasVal || val != "false")
			} else if !hasVal {
				takesValue()
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			cluster := []rune(a[1:])
			for k, c := range cluster {
				if p, ok := short[c]; ok {
					*p = true
				} else if k == len(cluster)-1 {
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
	// meow reads --version and --help only when they are the one argument.
	alone := len(args) == 1 && len(inputs) == 0
	switch {
	case alone && f.version:
		_, _ = fmt.Fprintf(stdout, "lexi %s (alex %s compatible)\n", version, alexVersion)
		return 0
	case alone && f.help:
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	case f.reporters > 1:
		_, _ = fmt.Fprintln(stderr, "lexi: The flag --reporter can only be set once.")
		return 1
	case f.stdin && len(inputs) > 0:
		_, _ = fmt.Fprintln(stderr, "lexi: Do not pass globs with `--stdin`")
		return 1
	}
	json := false
	if f.reporter != "" {
		if !jsonReporter(f.reporter) {
			_, _ = fmt.Fprintf(stderr, "lexi: Could not find reporter `%s`. lexi has the reporter `json`\n", f.reporter)
			return 2
		}
		json = true
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
	c := &checker{f: f, json: json, rs: rs, ws: ws, cwd: cwd, cfgs: map[string]cfgResult{}}

	var results []report.File
	if f.stdin {
		b, err := io.ReadAll(stdin)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "lexi:", err)
			return 1
		}
		res := c.check("<stdin>", cwd, toUTF8(b))
		res.Stdin = true
		results = append(results, res)
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
	if json {
		_, _ = fmt.Fprintln(stderr, report.JSON(results, report.JSONOptions{Quiet: f.quiet, Cwd: physical(cwd)}))
		return exitCode(results)
	}
	text, errs, warns := report.Format(results, report.Options{
		Quiet: f.quiet, Verbose: f.why, Color: useColor(stderr, args),
	})
	if text != "" {
		_, _ = fmt.Fprintln(stderr, text)
	}
	if errs+warns > 0 {
		return 1
	}
	return 0
}

// exitCode is 1 when any file has a message, and 0 when none has.
func exitCode(files []report.File) int {
	for _, f := range files {
		if len(f.Messages) > 0 {
			return 1
		}
	}
	return 0
}

// physical resolves the symbolic links of the working directory. Node reports
// the physical path as the working directory, and Go reports $PWD.
func physical(dir string) string {
	if p, err := filepath.EvalSymlinks(dir); err == nil {
		return p
	}
	return dir
}

// jsonReporter tells if the value of --reporter names vfile-reporter-json. alex
// loads the name, or the path of the package, or the path of its entry file.
func jsonReporter(v string) bool {
	v = strings.ReplaceAll(v, `\`, "/")
	for _, tail := range []string{"/lib/index.js", "/index.js"} {
		v = strings.TrimSuffix(v, tail)
	}
	return v == "json" || v == "vfile-reporter-json" || strings.HasSuffix(v, "/vfile-reporter-json")
}

// useColor follows supports-color 9, the package alex asks, for stderr: the flags --color and
// --no-color, FORCE_COLOR, a terminal, TERM, CI and the other hints. NO_COLOR has no effect, as in alex.
func useColor(w io.Writer, args []string) bool {
	return supportsColor(args, isTerminal(w), os.LookupEnv, runtime.GOOS == "windows")
}

// hasFlag tells if argv holds the flag before a `--`, as supports-color reads its own argv.
func hasFlag(args []string, flag string) bool {
	prefix := "--"
	switch {
	case strings.HasPrefix(flag, "-"):
		prefix = ""
	case len(flag) == 1:
		prefix = "-"
	}
	pos := slices.Index(args, prefix+flag)
	end := slices.Index(args, "--")
	return pos != -1 && (end == -1 || pos < end)
}

// supportsColor is the decision of supports-color: true when its level is not 0.
func supportsColor(args []string, tty bool, lookup func(string) (string, bool), windows bool) bool {
	env := func(k string) string { v, _ := lookup(k); return v }
	has := func(k string) bool { _, ok := lookup(k); return ok }
	// force is nil when no flag or variable says anything. The value is the level; nan is a
	// FORCE_COLOR that is not a number: it is not 0, and it works as 0 below.
	var force *int
	nan := false
	set := func(v int) { force, nan = &v, false }
	switch {
	case hasFlag(args, "no-color") || hasFlag(args, "no-colors") || hasFlag(args, "color=false") || hasFlag(args, "color=never"):
		set(0)
	case hasFlag(args, "color") || hasFlag(args, "colors") || hasFlag(args, "color=true") || hasFlag(args, "color=always"):
		set(1)
	}
	if v, ok := lookup("FORCE_COLOR"); ok {
		switch n, err := parseLeadingInt(v); {
		case v == "true" || v == "":
			set(1)
		case v == "false":
			set(0)
		case err != nil:
			set(0)
			nan = true
		default:
			set(min(n, 3))
		}
	}
	if force != nil && *force == 0 && !nan {
		return false
	}
	if hasFlag(args, "color=16m") || hasFlag(args, "color=full") || hasFlag(args, "color=truecolor") || hasFlag(args, "color=256") {
		return true
	}
	if has("TF_BUILD") && has("AGENT_NAME") {
		return true
	}
	if !tty && force == nil {
		return false
	}
	atLeast := force != nil && *force != 0
	term := env("TERM")
	switch {
	case term == "dumb":
		return atLeast
	case windows:
		return true
	case has("CI"):
		ci := []string{"GITHUB_ACTIONS", "GITEA_ACTIONS", "TRAVIS", "CIRCLECI", "APPVEYOR", "GITLAB_CI", "BUILDKITE", "DRONE"}
		if slices.ContainsFunc(ci, has) || env("CI_NAME") == "codeship" {
			return true
		}
		return atLeast
	case has("TEAMCITY_VERSION"):
		return teamcity.MatchString(env("TEAMCITY_VERSION"))
	case env("COLORTERM") == "truecolor" || term == "xterm-kitty":
		return true
	case env("TERM_PROGRAM") == "iTerm.app" || env("TERM_PROGRAM") == "Apple_Terminal":
		return true
	case termColor.MatchString(term) || has("COLORTERM"):
		return true
	}
	return atLeast
}

var (
	teamcity  = regexp.MustCompile(`^(9\.(0*[1-9]\d*)\.|\d{2,}\.)`)
	termColor = regexp.MustCompile(`(?i)-256(color)?$|^screen|^xterm|^vt100|^vt220|^rxvt|color|ansi|cygwin|linux`)
	leadInt   = regexp.MustCompile(`^\s*[+-]?\d+`)
)

// parseLeadingInt reads the digits at the start of s, as parseInt does.
func parseLeadingInt(s string) (int, error) {
	m := leadInt.FindString(s)
	if m == "" {
		return 0, errors.New("not a number")
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(m, "+")))
	if err != nil {
		return 3, nil // a number too large for an int is large
	}
	return n, nil
}

type cfgResult struct {
	cfg config.Config
	err error
}

type checker struct {
	f    flags
	json bool
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
	return report.Message{Line: 1, Column: 1, Fatal: true, Reason: reason, NoPlace: true}
}

// failure is a fatal message for an error that alex prints with its stack.
func failure(reason string) report.Message {
	m := fatal(upperFirst(reason))
	m.Prefix = "Error: "
	return m
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
		out.Messages = []report.Message{failure(strings.ReplaceAll(cr.err.Error(), c.cwd+string(filepath.Separator), ""))}
		return out
	}
	cfg := cr.cfg
	doc := input.Extract(c.f.kind(), text)
	check := doc.Check
	if c.json {
		check = doc.CheckRaw
	}
	found, err := check(c.rs, c.ws, rules.Options{Allow: cfg.Allow, Deny: cfg.Deny, NoBinary: cfg.NoBinary, Sureness: cfg.ProfanitySureness})
	if err != nil {
		out.Messages = []report.Message{failure(err.Error())}
		return out
	}
	// alex adds the error of a control comment last. The text report sorts the messages.
	for _, m := range found {
		out.Messages = append(out.Messages, report.Message{
			Line: m.Line, Column: m.Column, EndLine: m.EndLine, EndColumn: m.EndColumn,
			Reason: m.Message, Note: m.Note, RuleID: m.RuleID, Source: m.Source,
			Actual: m.Actual, Expected: m.Expected, URL: m.URL,
		})
	}
	if e := doc.Err(); e != nil {
		out.Messages = append(out.Messages, report.Message{
			Line: max(e.Line, 1), Column: max(e.Column, 1), EndLine: e.EndLine, EndColumn: e.EndColumn,
			Fatal: true, Reason: e.Message, NoPlace: e.Line == 0,
		})
	}
	return out
}
