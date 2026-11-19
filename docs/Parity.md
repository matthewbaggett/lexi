# Parity with alex 11.0.1

This page lists each feature of the alex 11.0.1 command line, and says if lexi has it.

- **done**: lexi does the same as alex. The oracle tests check it against the real alex.
- **partial**: lexi does most of it. The notes say what differs.
- **not planned**: lexi does not do it. The notes say why.

The profanity check of alex (`retext-profanities`) runs on the same prose as the rules, with the
same positions, in all input kinds.

## Input kinds

| Feature | State | Notes |
|---|---|---|
| Plain text (`--text`) | done | The whole file is prose. Control comments are text, as in alex. |
| Markdown (default) | done | goldmark with the table, strike-through, task list and footnote extensions, and YAML or TOML front matter. alex uses micromark. The corpus has 2,700 documents that give the same findings in both tools, in every construct of the format. The rare differences are in the [known differences](../testdata/corpus/KNOWN_DIFFS.md). |
| HTML (`--html`) | done | lexi builds the tree that parse5 builds: the insertion modes of the HTML standard, the adoption agency algorithm for misnested formatting tags, and foster parenting for content that does not belong in a table. It ports `hast-util-to-nlcst`, and the gaps that `remark-message-control` drops. A file with no node that has a position (an empty file, or only a tag that the end of the file cuts off) is an error, as in alex. CRLF line endings need no extra step: the tokenizer turns them into LF, as parse5 does, and the corpus documents with CRLF give the same findings. The corpus has 5,700 documents, of which 1,400 are random tag soup, and all give the same findings. The content of a `template` is not read, as in alex. Not planned: a self-closing tag such as `<title/>`, which parse5 reads as the start of raw text. |
| MDX (`--mdx`) | partial | lexi blanks the ESM blocks, the JSX tags and the `{expressions}`, then reads the rest as Markdown. The corpus has 3,369 MDX documents. On valid MDX, lexi agrees with alex on all of them. lexi stops with the alex text for 253 of the 443 syntax errors of the adversarial corpus (a bad character in a tag, an expression or tag that is not closed, an empty expression, a closing tag that does not match), and it reports no finding for such a file. **Not planned:** lexi does not parse JavaScript, so it does not report an error of acorn, or an error that needs a syntax tree. For such a file it reports the findings. A parser would add up to 5.9 MB, and the build of the MDX site reports a broken file anyway. See `mdx-syntax-error` in the [known differences](../testdata/corpus/KNOWN_DIFFS.md). The [MDX investigation](Investigations/MDX.md) lists the options. |
| Input kind from the file extension | not planned | alex does not do it. Only the flags choose the kind, so `alex x.html` reads the file as Markdown. |

The position of a finding follows alex, quirks included. A character reference such as `&amp;` is
one character in the text and five in the source, so the columns after it are off by four in
both tools.

## Flags

| Flag | State | Notes |
|---|---|---|
| `-t`, `--text` | done | |
| `-l`, `--html` | done | |
| `--mdx` | done | `--html` wins over `--mdx`, and `--mdx` over `--text`, as in alex. |
| `--stdin` | done | The file name in the report is `<stdin>`. `--stdin` with a glob is an error. |
| `-q`, `--quiet` | done | |
| `-w`, `--why` | done | Prints the note of a rule below its message. |
| `-d`, `--diff` | partial | Works as in alex, for Travis CI and GitHub Actions only. It runs the `git` program, so `git` must be on the path. |
| `-h`, `--help` | done | The text is the text of alex, with `lexi` as the name. As in alex (meow), `--help` and `--version` act only when they are the only argument. |
| `-v`, `--version` | done | Prints `lexi <version> (alex 11.0.1 compatible)`, not the alex version alone. |
| `--reporter` | partial | lexi knows the JSON reporter only: `--reporter=json`, `--reporter=vfile-reporter-json`, and the path of the package or of its entry file, such as `node_modules/vfile-reporter-json/index.js`. lexi reads the name and loads nothing. Another value is an error: lexi prints `Could not find reporter` and exits with 2. alex prints the same words and exits with 0. **Not planned:** a reporter that is a JavaScript module. See [Output and exit code](#output-and-exit-code). |
| A flag that lexi does not know | done | It is ignored, and it takes the next argument as its value. |
| No arguments | done | lexi searches the current directory, `doc` and `docs`. It does not read stdin. |

## Files

| Feature | State | Notes |
|---|---|---|
| File arguments | done | A file that you name is checked, whatever its extension. A file that does not exist is an error. |
| Directory arguments | done | lexi searches below the directory, and skips `node_modules` and names that start with a dot. |
| Globs | done | The same syntax and the same matches as glob 8.1.0 with minimatch 5.1.9. These are the libraries that alex 11.0.1 reaches through unified-engine 10.1.0. The syntax: `*`, `?`, `[...]` with `[!a]` and `[^a]`, `**`, braces `{a,b}`, ranges `{1..5}`, `{05..10}` and `{a..e..2}`, and the escape `\`. The extended patterns `?(a\|b)`, `*(a\|b)`, `+(a\|b)`, `@(a\|b)` and `!(a\|b)` work, also nested. A wildcard does not match a name that starts with a dot. The POSIX class `[[:alpha:]]` is not a class in minimatch 5.1.9, and not in lexi. It is the class `[[:alpha:]` and then `]`. A `!pattern` is a file name, as in alex. The report shows a match as glob gives it, `./src/a.md` too. See [Globs](#globs) below. |
| Default extensions | done | `txt`, `text`, `md`, `markdown`, `mkd`, `mkdn`, `mkdown` and `ron`. With `--html`: `htm` and `html`. With `--mdx`: `mdx`, but the default search of alex then finds no file: its glob ends in `*.{mdx}`, and glob does not expand a brace with one name in it. lexi does the same. Name the files to check them: `lexi --mdx docs/*.mdx`. |
| `.alexignore` | done | The syntax of `.gitignore`. The nearest file above the checked file wins. An ignored file that you name is an error. An ignored file that a search finds is skipped. |
| Order of the report | done | Sorted by path. |
| A file named twice | not planned | alex checks it twice, and the second report is wrong. lexi checks it once. |

### Globs

The corpus in `testdata/corpus/glob` has 20,169 cases. Each case is a file tree, a pattern, and what
the real alex 11.0.1 gives for it. The test `TestGlobCorpus` replays them with no Node. In all cases,
lexi gives the same matches and the same list of files. The 23 `glob ...` runs of
`TestCLIOracle` compare the text report with the real alex, too.

lexi ports the pattern parser of minimatch and the walk of glob, quirks included. It builds the
same regular expressions. It runs them with `regexp2` in its ECMAScript mode. glob reads the
file system with callbacks, and its answer can depend on the order in which the reads end. So lexi
puts the reads in a queue, and ends them in the order they start. The table shows where lexi differs.

| Difference | State | Notes |
|---|---|---|
| `{,}` and `{,,}` | not planned | These have no result. alex prints `Cannot read properties of undefined (reading 'length')` and exits 0. lexi reads them as a path, and reports that no such file exists. |
| A pattern of more than 65,536 characters | not planned | minimatch throws `pattern is too long`, and alex stops. lexi expands it. |
| Limits of the brace expansion | not planned | brace-expansion 2.1.8 stops at 100,000 results, at 1,000 levels, and at a budget of steps, and keeps the rest of the pattern as it is. lexi has the limit of the results only. A hostile pattern reaches the others. |
| A directory that cannot be read | not planned | glob stops with the error of the operating system, and alex prints it. lexi skips the directory. |
| Order of the matches | not planned | glob sorts them with ICU (`localeCompare` for `en`). lexi sorts the bytes, and then the report sorts by path, so the order of the matches does not show. |
| A class range with a character above U+FFFF | not planned | JavaScript compares the halves of a surrogate pair. lexi maps each half to a code point of the private-use plane and keeps the order of the halves, so `?` matches half of an emoji, as in alex. A range such as `[😀-ｱ]` compares a half with a character above U+E000, and the order differs. |
| A tree that changes during the search | not planned | The queue gives the order of the reads, and the real order of a thread pool can differ. The corpus ran twice with the real alex, and the two runs agree in all cases. |
| A symbolic link to a parent directory | done | A glob does not follow a link below `**`, and a directory argument follows it until the system stops it, in alex and in lexi. The corpus leaves the file search out for these trees. |

## Configuration

| Feature | State | Notes |
|---|---|---|
| `.alexrc` and `.alexrc.json` | done | JSON. |
| `.alexrc.yml` and `.alexrc.yaml` | done | |
| `alex` field of `package.json` | done | A `package.json` without the field is skipped. |
| `.alexrc.js`, `.alexrc.cjs`, `.alexrc.mjs` | not planned | They run JavaScript, and lexi is one static binary. lexi reports an error that tells you to use JSON or YAML. |
| Search for the nearest file | done | From the directory of each checked file upward. For stdin, from the working directory. |
| `allow` | done | |
| `deny` | done | `allow` with `deny` is an error, as in alex. |
| `noBinary` | done | |
| `profanitySureness` | done | |

## Control comments

| Feature | State | Notes |
|---|---|---|
| `<!--alex ignore rule-->` | done | In Markdown and HTML. It turns the rule off for the next node. |
| `<!--alex disable rule-->` and `<!--alex enable rule-->` | done | A comment with no rule applies to all rules. |
| `{/* alex ... */}` in MDX | done | |
| An unknown keyword | done | An error with the position of the comment. alex then reports every finding of the file, unfiltered, and so does lexi. |
| Control comments in plain text | done | There are none, as in alex. |

## Output and exit code

| Feature | State | Notes |
|---|---|---|
| Report format | done | The table of `vfile-reporter`, on stderr. |
| JSON report (`--reporter=json`) | done | The output of `vfile-reporter-json` 4.0.0, on stderr, as one line of compact JSON. It has the same fields in the same order: `path`, `cwd`, `history` and `messages` for a file, and `column`, `fatal`, `line`, `reason`, `ruleId`, `source`, `actual`, `expected`, `note` and `url` for a message. A field with no value is left out, as `JSON.stringify` does. A message with no place has `null` for `line` and `column`, and so has an error that has no `ruleId` or `source`. The input from `--stdin` has no `path` and an empty `history`. `cwd` is the real path of the working directory. `--quiet` leaves out the files without messages, and the report is `[]` when there are none. `--why` changes nothing. The messages are in the order that alex adds them, not sorted by place: the findings of the profanity check, then the findings of the rules, and the error of a control comment last. The tests compare each run with alex. |
| JSON report, order in misnested HTML | partial | In a document where the adoption agency algorithm rebuilds a formatting element after a block, such as `<b>x<p>y<i>x`, the order of the messages can differ from alex. The set of messages is the same. This is 6 documents of the 5,764 in the HTML corpus. |
| Colors | done | The decision of `supports-color` 9: the `--color` and `--no-color` flags, `FORCE_COLOR`, a terminal on stderr, `TERM`, `CI` and the other hints. `NO_COLOR` has no effect, as in alex. |
| `no issues found` line | done | |
| Summary line | done | `⚠ 1 warning`, `✖ 1 error`, or `2 messages (✖ 1 error, ⚠ 1 warning)`. |
| Exit code | done | 1 when there is a warning or an error. 0 when there is none. The JSON report gives the same codes. |
| Text of a JavaScript error | not planned | alex prints a stack trace for some errors, such as a config file that does not parse. lexi prints the message. |
| Update notice | not planned | alex checks npm for a new version. |

## How we check

- `go test ./...` runs the unit tests and the golden files, with no network and no node. The golden files
  include `testdata/corpus/{equality,profanity,markdown,html,mdx}`: 28,600 cases that alex recorded.
- `ALEX_ORACLE=<dir> go test ./...` also runs the real alex. Install it with
  `npm ci` in `testdata/oracle`, which pins alex 11.0.1, `vfile-reporter-json` 4.0.0 and every
  dependency of them. Then set `ALEX_ORACLE=$PWD/testdata/oracle`. Three tests use it:
  - `internal/rules`: texts, with the rules and the profanity check.
  - `internal/input`: the fixture files in `testdata/formats`, as Markdown, HTML, MDX and
    plain text.
  - `cmd/lexi`: 30 runs of the command. It compares the report and the exit code. A second test
    runs the same files, and the fixture files of `testdata/formats`, with the JSON reporter. It
    compares the JSON text and the exit code, with `--reporter=json` and with the path of the
    entry file of `vfile-reporter-json`.
- `scripts/equivalence.sh` (`mise run equivalence`) runs the equivalence suite: 1,700 cases in both tools, in scratch
  directories, compared on stdout, stderr, exit code and files. See [Equivalence.md](Equivalence.md). Each difference
  that is on purpose is in `testdata/equivalence/known-gaps.json`, with the row of this page that it belongs to.
- CI runs the oracle in `.forgejo/workflows/oracle.yml`. It runs on each pull request that
  touches `internal/`, `cmd/`, `testdata/`, or the Go module files, and every night at 03:17 UTC.
  A mismatch with alex fails the run. The normal Lint and Test jobs do not run it, so they stay fast.
- `UPDATE_GOLDEN=1` with `ALEX_ORACLE` refreshes `testdata/formats/golden.json`, and with the
  JSON test, `testdata/reporter/golden.json`. That file holds what alex prints with the JSON
  reporter. `go test ./...` compares lexi with it, with no node.
- `node scripts/corpus/generate-formats.mjs` (with `ALEX_ORACLE`) records the Markdown, HTML and MDX
  corpus. See [testdata/corpus/README.md](../testdata/corpus/README.md).
