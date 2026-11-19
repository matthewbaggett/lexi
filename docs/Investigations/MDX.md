# Investigation: real MDX support

This page answers one question: should lexi read MDX with a real parser? It gives the facts,
four options, and a recommendation. Nothing in `internal/` changed.

## Status

Option A is done. The three causes below are fixed, and the 47 `known` marks are gone. A flow-only
line stays blank. `jsscan.go` reads an ESM block past a blank line while a statement is open. A
JSX tag or expression in a block quote skips the quote markers on its later lines. A control comment
reads its value without those markers, and its scope ends with its list item. The text below is the
state before the fix.

Option B is done for the errors that need no JavaScript parser. `internal/input/mdxjsx.go` ports the
tag tokenizer of `micromark-extension-mdx-jsx` 1.0.5, with its licence notice. `mdxcheck.go` reads the
lines of the file, runs the tokenizer on each block, reads the expressions with the scanner of
`jsscan.go`, and matches the tags as `mdast-util-mdx-jsx` 2.1.4 does. It stops at the first error, as
alex does, and it reports it through `Doc.Err`. The CLI prints it at `1:1`. A file with an error
gets no findings. lexi gives the alex text for 253 of the 443 errors of the adversarial corpus. Its
other 190 documents stay as the known difference `mdx-syntax-error`: 181 are errors of acorn or errors
that need a tree, and the rest are shapes that lexi does not read like micromark (a blank line in
an expression that does not parse, a lone `\r` line ending, a tag open at the end of an emphasis).
The 1,336 valid documents give no false error. Option C stays open.

## Summary

- The blanker of `internal/input/mdx.go` is good on valid MDX. In a new corpus of 1,336 valid
  documents, lexi differs from alex on 47 (3.5%). Three causes explain all 47. Each cause is small.
- alex stops with an error on 443 of the 1,779 documents of the new corpus. lexi reports findings
  on all of them. This is the known gap `mdx-syntax-error`.
- 260 of the 443 errors (59%) come from the state machines of micromark and mdast. They need no
  JavaScript parser. 168 (38%) come from acorn, and their text is the acorn text. The other 15 need
  a syntax tree.
- No Go parser gives the acorn text. No Go library parses JSX and ESM and stays small, except
  esbuild, and esbuild adds 5.9 MB.
- Recommendation: fix the three causes, then port the JSX and expression state machines to Go.
  Do not embed a JavaScript parser. See [Recommendation](#recommendation).

## How alex reads MDX

`alex.mdx` runs `remark-parse` with `remark-mdx`. The versions come from `testdata/oracle`:

| Package | Version |
|---|---|
| `remark-mdx` | 2.0.0 |
| `micromark-extension-mdxjs` | 1.0.1 |
| `micromark-extension-mdx-jsx` | 1.0.5 |
| `micromark-extension-mdx-expression` and its factory | 1.0.8 and 1.0.9 |
| `micromark-extension-mdxjs-esm` | 1.0.5 |
| `mdast-util-mdx-jsx` | 2.1.4 |
| `acorn` and `acorn-jsx` | 8.19.0 and 5.3.2 |

Two facts decide the design.

1. **micromark does not scan JavaScript. It asks acorn.** The expression factory reads to a `}`,
   then asks acorn if the text so far is an expression. If acorn fails, the factory keeps the
   error as `lastCrash`, reads to the next `}`, and asks again. At the end of the file it throws
   `lastCrash`. So `{"}"}` works because the text up to the first `}` does not parse. The ESM
   block works the same way. It ends at a blank line, unless acorn fails at the end of the text.
   Then the block goes on past the blank line.
2. **There are two stages, and the first error wins.** Tokenizing (micromark) runs first and
   throws the first error in file order. Tree building (`mdast-util-mdx-jsx`) runs after it. It
   reports a tag that is not closed, or a closing tag that does not match. A file with a bad
   expression in line 9 and an unclosed tag in line 1 reports the expression.

## Question 1: where the blanker differs from alex

### Method

`scripts/corpus/generate-mdx-adversarial.mjs` builds `testdata/corpus/mdx/adversarial.jsonl`. It
records what alex 11.0.1 reports. The documents are:

| Group | What it holds |
|---|---|
| `inline-*` | 68 inline shapes, each in 6 places: alone, after prose, in a list, in a quote, in a heading, in a table cell. The shapes are nested JSX, expressions with strings, braces, regexes and templates, attribute expressions, spreads, fragments, and entities |
| `flow-*` | 149 block shapes, each in 5 places: ESM with comments and templates, JSX in lists, quotes and tables, control comments, front matter, CRLF, tabs, a BOM, Unicode |
| `code-*` | 53 shapes with a term inside code, in 4 places each. Some have a blank line inside the code. alex stays silent about the term |
| `err-*` | 138 syntax errors and near misses, each in 3 places. Some are valid, as a check |

The corpus has 1,779 documents: 1,153 positive, 212 negative and 414 edge. Run it with:

```bash
cd testdata/oracle && npm ci && cd -
ALEX_ORACLE=$PWD/testdata/oracle node scripts/corpus/generate-mdx-adversarial.mjs
```

A document where lexi differs from alex has a `known` mark. `TestFormatCorpus` checks the mark.
To refresh the marks, write the list of differences with `CORPUS_DIFF_OUT=<file>`, then run the
generator with `DIFF_FILE=<file>`.

### Result

| Class | Documents | Differ |
|---|---|---|
| Valid MDX (positive and negative) | 1,336 | 47 |
| Syntax error in alex | 443 | 443 |

The 47 documents come from three causes.

**Cause 1: a flow line that stands alone joins the paragraph (33 documents).** Take `> {a}\n> mankind`.
In MDX, a line that holds only a JSX tag or an expression is a flow node. It ends the paragraph.
`mankind` then starts a new paragraph, and its column counts the `> `: alex reports `2:3`. lexi
blanks the line to `x`, and goldmark keeps `x` and `mankind` in one paragraph. A continuation line
in a block quote has no marker in its position. lexi reports `2:1`. The same happens in a list item
and after `<A>`, `<>`, `<Foo/>` and `{a}`. At the top level the column is the same, so only
containers show it. First place to look: `keepLine` and `split` in `internal/input/markdown.go`.
The fix is to blank a flow-only line to nothing, not to `x`.

**Cause 2: a blank line inside an ESM statement (12 documents).** lexi ends an ESM block at the
first blank line. alex goes on when the statement is open: an unclosed `{` or `(`, a template
literal, or a block comment. Then lexi reads the rest of the statement as prose. A term in a
comment after the blank line is reported by lexi. alex stays silent, and it reports the term that
follows the block, which lexi never reaches. The fix needs the depth of brackets, and it needs to
skip strings, comments, templates and regexes. This is a small JavaScript scanner.

**Cause 3: a control comment that spans lines, or sits in a list (2 documents).** alex ignores
all later findings in `- {/* alex ignore */}\n- hell\n- dumb`. lexi ends the scope too early. The
cause is not found yet. Start in `mdxTail`.

The old corpus has 1,590 documents. They pass, so the three causes need shapes that the old
templates did not have.

## Question 2: the errors that alex reports

### In the CLI

For the three files tried, `alex --mdx` prints one line per file and ignores the position:

```text
c.mdx
  1:1  error  Could not parse expression with acorn: Unexpected token
```

The error is at line 3 in the file. The position is `1:1` in the report, and the exit code is 1.
The file gets no findings. A file with an error stops there, even when it has terms. The
position and the message are the only data. So for the CLI, the **message** is what lexi must
match, and the position does not matter. The API gives the real position, and the table shows it.

### By source

The columns count UTF-16 code units, and a BOM does not count. Tabs count as one.

| Source and id | Documents | Position | Message |
|---|---|---|---|
| `micromark-extension-mdx-jsx:unexpected-character` | 93 | The bad character | `Unexpected character `c` (U+0063) before attribute value, expected a character that can start an attribute value, such as `"`, `'`, or `{``. The state names the place: before name, in name, after name, before local name, before member name, before attribute value, after self-closing slash. Some add a note: `(note: to create a comment in MDX, use `{/* text */}`)` for `<!--`, and notes for links and JS comments in tags |
| `micromark-extension-mdx-jsx:unexpected-eof` | 22 | The end of the file | `Unexpected end of file in name, expected a name character such as letters, digits, `$`, or `_`; whitespace before attributes; or the end of the tag`. The place changes: before attribute value, in attribute value |
| `micromark-extension-mdx-expression:unexpected-eof` | 54 | The end of the file | `Unexpected end of file in expression, expected a corresponding closing brace for `{``. Also when a blank line ends a paragraph inside `{` |
| `micromark-extension-mdx-expression:acorn` | 120 | Inside the expression | `Could not parse expression with acorn: <acorn text>` |
| `micromark-extension-mdx-expression:unexpected-empty-expression` | 12 | The `{` | `Unexpected empty expression`. For `<A {/* c */} />` and `<A {} />` |
| `micromark-extension-mdx-expression:non-spread` and `spread-extra` | 3 and 3 | The expression | `Unexpected `Property` in code: only spread elements are supported`. `Unexpected extra content in spread: only a single spread is supported` |
| `micromark-extension-mdxjs-esm:acorn` | 48 | Inside the block | `Could not parse import/exports with acorn: SyntaxError: <acorn text>` |
| `micromark-extension-mdxjs-esm:non-esm` | 9 | The statement | `Unexpected `ExpressionStatement` in code: only import/exports are supported` |
| `mdast-util-mdx-jsx:end-tag-mismatch` | 58 | See below | `Expected a closing tag for `<Foo>` (1:1-1:6)`, with `before the end of `paragraph``, `listItem` or `blockQuote` when a container ends first. Or `Unexpected closing tag `</B>`, expected corresponding closing tag for `<A>` (1:1-1:4)` |
| `mdast-util-mdx-jsx:unexpected-closing-slash` | 15 | The slash | `Unexpected closing slash `/` in tag, expected an open tag first` |
| `mdast-util-mdx-jsx:unexpected-attribute` and `unexpected-self-closing-slash` | 3 and 3 | In the closing tag | `Unexpected attribute in closing tag, expected the end of the tag`. `Unexpected self-closing slash `/` in closing tag, expected the end of the tag` |

An unclosed tag at the top level has no line and no column in the API (`null`). The place is only in
the message. Inside a container, the position is the start of the container.

The 168 acorn errors have 21 different texts. `Unexpected token` is 73 of them, and `Unexpected
content after expression` is 28. Then come `Cannot use keyword 'await' outside an async function`
(9), `Unterminated string constant` (8), `Expecting Unicode escape sequence \uXXXX` (5),
`Unterminated template` (5), and 15 more texts with 2 to 4 documents each.

### Things that bite

- `<!-- c -->`, `<!doctype>`, `<?php ?>`, `<3`, `<http://x.y>` and `<a@b.c>` are all errors.
  An autolink is an error, and so is a comparison such as `a <3`. A space after the `<` is fine.
- `{a; b}`, `{var a = 1}`, `{return a}` and `{await a}` are errors. `{a, b}`, `{import('a')}` and
  `{import.meta.url}` are valid.
- `export const await = 1`, a top-level `await`, `010`, `with`, a duplicate `export default` and a
  duplicate `const` are errors. ESM is strict-mode module code.
- `{a // }\n}` is valid. A `}` in a comment or a string does not close the expression.
- `<A b="1" b="2" />`, `<A b={1,\n 2} />` and a code span with `{` are valid.
- A duplicate attribute is not an error, and JS comments in a tag are an error.

## Question 3: the options

### Size of each library

The test builds a program that imports only the parser. It uses `CGO_ENABLED=0`, `-trimpath` and
`-ldflags="-s -w"` on darwin/arm64 with Go 1.26. An empty program is 1.64 MB. lexi is 4.12 MB.

| Library | Licence | Added size | JSX | `import` and `export` | Matches acorn text |
|---|---|---|---|---|---|
| `github.com/tdewolff/parse/v2/js` v2.25.0 | MIT | +0.35 MB (+9%) | No | Yes | No |
| `github.com/dop251/goja/parser` (2026-11 head) | MIT | +0.93 MB (+23%) | No | No | No |
| `github.com/robertkrimen/otto/parser` | MIT | +1.0 MB (+25%) | No | No | No |
| `github.com/evanw/esbuild/pkg/api` v0.28.2 | MIT | +5.9 MB (+143%) | Yes | Yes | No |

All four build with `CGO_ENABLED=0`. A single static binary stays possible with every option.
The `FROM scratch` image stays possible too. Notes:

- esbuild has no parser API. Its `internal/js_parser` cannot be imported. `api.Transform` is the
  only entry, and it pulls the printer, the bundler and the minifier into the binary.
- esbuild is more lenient than acorn: `await x` at the top level is accepted. Its messages are its
  own (`Unexpected end of file`, `Expected "{" but found "c"`).
- tdewolff reads ESM, but it fails on every JSX element. A `{<b>x</b>}` expression needs JSX.
  `await x` is accepted too.
- goja and otto fail on `import` and `export`. They fail on JSX.
- No library has the acorn text. A library tells **if** the code is bad. It does not tell the
  message. The CLI shows the message.
- The claim in `docs/Parity.md` that a parser "would double the size of the binary" is true for
  esbuild only.

### Option A: harden the blanker

Fix the three causes of Question 1. It removes the 47 known diffs. It reports no error.

| | |
|---|---|
| Cause 1 | Blank a flow-only line to nothing, and check the `keepLine` trick again. Small |
| Cause 2 | A scanner for ESM that tracks `{[(`, strings, templates with `${}`, comments and regexes. About 150 lines |
| Cause 3 | Find the cause in `mdxTail`. Unknown, probably small |
| Size | None. The binary stays the same |
| Cost | 1 to 2 days, with the corpus as the test |

### Option B: a Go tokenizer for JSX and expressions

Port the tokenizers `micromark-extension-mdx-jsx` and `micromark-extension-mdx-expression`, the
tag matching of `mdast-util-mdx-jsx`, and a JavaScript scanner for expressions and ESM. The scanner
must know strings, templates, comments, regexes and JSX inside an expression. It does not build
a tree. It reports the 260 errors that need no parser, with the alex message.

| | |
|---|---|
| Gain | 260 of 443 errors (59%), with the exact message. Replaces `matchTag`, `matchBrace` and `skipString` |
| Not covered | 168 acorn errors. lexi stays silent on them, as now. The 15 errors that need a tree (`non-esm`, `non-spread`, `spread-extra`) stay open |
| Risk | The "expression ends at the first `}` that parses" rule. The scanner decides "parses" by structure only. A `}` in a string whose text is not valid JavaScript can give the eof message where alex gives the acorn message. The 260 is an upper bound |
| Size | About 0 to +0.1 MB |
| Cost | 1,500 to 2,000 lines of Go with tests. 5 to 8 days |

### Option C: a tokenizer, plus a library for detection

Option B, plus esbuild `Transform` to decide if an expression or an ESM block parses. When it does
not parse, lexi prints a fixed message.

| | |
|---|---|
| Gain | Detection of the 168 acorn errors. A fixed text `Unexpected token` gives the exact message for 73, and `Unexpected content after expression` for 28 more, if the detection agrees. About 60% of the 168 |
| Risk | esbuild accepts what acorn rejects (top-level `await`, some strict-mode cases). It rejects some valid text. Each library update can change the result, with no oracle test to catch it unless the corpus runs |
| Size | +5.9 MB. lexi grows from 4.1 MB to about 10 MB |
| Cost | Option B, plus 2 to 3 days |

### Option D: embed a parser as the only reader

Replace the blanker with a full parse: micromark rules in Go, and a JavaScript parser for each
expression and each ESM block.

| | |
|---|---|
| Gain | The same as option C, in one design |
| Risk | There is no Go port of micromark. goldmark is the Markdown parser, and it does not have the MDX extension points. The MDX rules for flow and text and lazy lines would be a second Markdown parser. This is the most work, and it breaks the working Markdown path |
| Cost | 3 to 5 weeks |

## Recommendation

**Do A now, then B. Do not embed a JavaScript parser (C and D).**

1. **Option A (1 to 2 days).** It fixes valid documents that give wrong positions or wrong findings
   today. This matters more than errors: a wrong column is a wrong report. The 47 `known` marks
   go away. `docs/Parity.md` says `partial`, and it will say what is left.
2. **Option B (5 to 8 days).** It gives the exact alex message on 59% of the error documents. Wire it
   through `Doc.Err`: the type `Error` already exists for HTML. The CLI reports `1:1`, as alex does.
   lexi stops on the first error and reports no findings for the file.
3. **Stop there.** The last 38% (and 3% that need a tree) are acorn texts. A library detects them with a wrong text, and the
   price is +5.9 MB for esbuild, or no JSX for the small libraries. The site build reports these
   errors anyway. Keep `mdx-syntax-error` in `KNOWN_DIFFS.md` for the acorn errors, with the narrower
   description.

Open a new decision before step 2: the earlier decision was "not planned". The cost is 5 to 8
days, and the gain is the match of one kind of broken file. Step 1 does not depend on it.

### What to change when you decide

| Step | Files |
|---|---|
| Option A, cause 1 | `internal/input/mdx.go` (`keepLine`, `maskMDX`), `internal/input/markdown.go` |
| Option A, cause 2 | `internal/input/mdx.go`: a new scanner file `jsscan.go` |
| Option A, cause 3 | `internal/input/mdx.go` (`mdxTail`, `flowTail`) |
| Option B | New files `internal/input/mdxjsx.go`, `mdxexpr.go`; the `Err` path in `internal/input/doc.go` |
| After each step | `ALEX_ORACLE=... node scripts/corpus/generate-mdx-adversarial.mjs`, then drop the fixed `known` marks |

## Limits of this investigation

- The corpus has 1,779 documents from 408 shapes. It is not real-world MDX. The causes of
  Question 1 may be a part of a larger set.
- The sizes are for darwin/arm64 and a program that only parses. A real integration adds a little.
- The estimates in days are guesses from the line counts, not measured work.
- The CLI position was checked on 6 files. The result is always `1:1`.
