// Command lexi checks text for insensitive, inconsiderate writing.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"git.grey.ooo/grey.ooo/lexi/internal/config"
	"git.grey.ooo/grey.ooo/lexi/internal/rules"
)

// Replaced in tests.
var (
	exit      = os.Exit
	loadRules = rules.Load
)

func main() { exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lexi", flag.ContinueOnError)
	fs.SetOutput(stderr)
	useStdin := fs.Bool("stdin", false, "read text from stdin")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Find(".")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "lexi: config:", err)
		return 2
	}
	rs, err := loadRules()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "lexi:", err)
		return 2
	}
	opts := rules.Options{Allow: cfg.Allow, Deny: cfg.Deny, NoBinary: cfg.NoBinary}

	type src struct{ name, text string }
	var srcs []src
	if *useStdin || fs.NArg() == 0 {
		b, err := io.ReadAll(stdin)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "lexi:", err)
			return 2
		}
		srcs = append(srcs, src{"<stdin>", string(b)})
	}
	for _, p := range fs.Args() {
		b, err := os.ReadFile(p)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "lexi:", err)
			return 2
		}
		srcs = append(srcs, src{p, string(b)})
	}

	code := 0
	for _, s := range srcs {
		found, err := rules.Check(rs, s.text, opts)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "lexi:", err)
			return 2
		}
		for _, f := range found {
			_, _ = fmt.Fprintf(stdout, "%s:%d:%d-%d:%d  %s  [%s]\n", s.name, f.Line, f.Column, f.EndLine, f.EndColumn, f.Message, f.RuleID)
			code = 1
		}
	}
	return code
}
