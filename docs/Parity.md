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
| Markdown (default) | done | goldmark with the table, strike-through, task list and footnote extensions, and YAML or TOML front matter. alex uses micromark. In a one-time check, 446 Markdown files from npm packages gave the same findings in both. |
| HTML (`--html`) | partial | lexi builds the tree that parse5 builds, with the implied `html`, `head` and `body`. It ports `hast-util-to-nlcst`, and the gaps that `remark-message-control` drops. It has no adoption agency algorithm and no foster parenting, and it does not turn CRLF into LF before it reads the file. In a one-time check, 300 HTML files gave the same findings in both. |
| MDX (`--mdx`) | partial | lexi blanks the ESM blocks, the JSX tags and the `{expressions}`, then reads the rest as Markdown. It does not parse JavaScript, so it never reports an MDX syntax error. alex stops on a file with such an error. |
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
| `-h`, `--help` | done | The text names `lexi`. |
| `-v`, `--version` | partial | Prints the version of lexi, not `11.0.1`. |
| `--reporter` | not planned | It loads a JavaScript module. |
| A flag that lexi does not know | done | It is ignored, and it takes the next argument as its value. |
| No arguments | done | lexi searches the current directory, `doc` and `docs`. It does not read stdin. |

## Files

| Feature | State | Notes |
|---|---|---|
| File arguments | done | A file that you name is checked, whatever its extension. A file that does not exist is an error. |
| Directory arguments | done | lexi searches below the directory, and skips `node_modules` and names that start with a dot. |
| Globs | partial | `*`, `?`, `[...]`, `**` and `{a,b}`. It has none of the extended patterns of node-glob, such as `+(a\|b)`. A `!pattern` is a file name, as in alex. |
| Default extensions | done | `txt`, `text`, `md`, `markdown`, `mkd`, `mkdn`, `mkdown` and `ron`. With `--html`: `htm` and `html`. With `--mdx`: `mdx`. |
| `.alexignore` | done | The syntax of `.gitignore`. The nearest file above the checked file wins. An ignored file that you name is an error. An ignored file that a search finds is skipped. |
| Order of the report | done | Sorted by path. |
| A file named twice | not planned | alex checks it twice, and the second report is wrong. lexi checks it once. |

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
| Colors | partial | On when stderr is a terminal, or `FORCE_COLOR` is set. `NO_COLOR` turns them off. |
| `no issues found` line | done | |
| Summary line | done | `⚠ 1 warning`, `✖ 1 error`, or `2 messages (✖ 1 error, ⚠ 1 warning)`. |
| Exit code | done | 1 when there is a warning or an error. 0 when there is none. |
| Text of a JavaScript error | not planned | alex prints a stack trace for some errors, such as a config file that does not parse. lexi prints the message. |
| Update notice | not planned | alex checks npm for a new version. |

## How we check

- `go test ./...` runs the unit tests and the golden files, with no network and no node.
- `ALEX_ORACLE=<dir> go test ./...` also runs the real alex. Install it with
  `npm ci` in `testdata/oracle`, which pins alex 11.0.1 and every dependency of it. Then set
  `ALEX_ORACLE=$PWD/testdata/oracle`. Three tests use it:
  - `internal/rules`: texts, with the rules and the profanity check.
  - `internal/input`: the fixture files in `testdata/formats`, as Markdown, HTML, MDX and
    plain text.
  - `cmd/lexi`: 30 runs of the command. It compares the report and the exit code.
- CI runs the oracle in `.forgejo/workflows/oracle.yml`. It runs on each pull request that
  touches `internal/`, `cmd/`, `testdata/`, or the Go module files, and every night at 03:17 UTC.
  A mismatch with alex fails the run. The normal Lint and Test jobs do not run it, so they stay fast.
- `UPDATE_GOLDEN=1` with `ALEX_ORACLE` refreshes `testdata/formats/golden.json`.
