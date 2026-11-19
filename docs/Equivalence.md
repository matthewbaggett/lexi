# Equivalence with alex

The equivalence suite runs the same case in the real alex 11.0.1 and in lexi, in scratch directories. It
compares stdout, stderr, the exit code, and the files that each tool creates, changes or removes, byte for
byte. One entry point runs every case and prints one report.

```bash
mise run equivalence                       # or: scripts/equivalence.sh
scripts/equivalence.sh --row diff          # one matrix row
scripts/equivalence.sh --filter 'globs/*'  # cases by id
EQUIV_SEED=7 EQUIV_FUZZ=2000 scripts/equivalence.sh
```

The script builds lexi to `dist/lexi`, installs alex from `testdata/oracle` (`npm ci`), and runs
[scripts/equivalence/run.mjs](../scripts/equivalence/run.mjs). It needs `go`, `node`, `npm` and `git`.
Cases that give the tool a terminal need `python3`; the report counts them as skipped when it is missing.

## What the suite changes before it compares

Two things only:

- the path of the scratch directory becomes `<cwd>`, and its parent becomes `<tmp>`;
- the program name `alex` or `lexi` becomes `<prog>`.

The run starts with a clean environment (`PATH`, a scratch `HOME`, `TERM=xterm`, and the variables of the
case), so a variable of the host, such as `GITHUB_SHA` on a CI runner, cannot change a result. A case sets
more variables in its `env` file.

alex loads a reporter from the working directory, and a scratch directory has no `node_modules`. So the suite
gives alex the entry file of `vfile-reporter-json` when a case names the reporter `json`,
`vfile-reporter-json`, or the path of the package. lexi gets the name as the case wrote it.

## The result

The report has a row for each matrix row below: the cases that ran, the matches, the mismatches that
[known-gaps.json](../testdata/equivalence/known-gaps.json) lists, and the others. The run fails (exit 1) on a
mismatch that no gap lists. For each mismatch the suite writes both outputs to `dist/equivalence/mismatches/`.

A known gap names the row of [Parity.md](Parity.md) it belongs to and gives the reason, and it can cover
chosen fields only (`stderr`, for example). The suite checks that the row exists. It warns when a gap matches
no mismatch, so remove the gap when lexi matches alex.

## Add a case

Make the directory `testdata/equivalence/cases/<row>/<name>/` with the files of
[the case format](../testdata/equivalence/README.md). No Go is needed.

## The matrix

Each row says what alex has, which tests covered it before the suite, and what the suite adds.
"Unit" is `go test ./...` with no node. "Oracle" is `ALEX_ORACLE=<dir> go test ./...`.

| Row | What alex does | Covered before | What the suite adds |
|---|---|---|---|
| `flags-single` | `-w -q -t -l -d`, `--why --quiet --text --html --mdx --diff --stdin`, `-h -v`, `--reporter`, unknown flags, `--no-x`, `--x=false` | Oracle: `-q`, `-w`, `-t`, `-l`, `--mdx`, `--stdin`, an unknown flag. Unit: the argument parser. | Each flag alone on one file, the forms of a value (`--reporter x`, `--why=false`), flags before and after the input, an unknown flag at the end. Found: `--help` and `--version` only act when they are the only argument. |
| `flags-combo` | Groups such as `-wq`, the order `--html` over `--mdx` over `--text`, `--`, `--stdin` with inputs | Oracle: none. Unit: some groups. | Twenty groups and precedence pairs, `--`, `--stdin` with every kind of input. Found: the last unknown flag of a group takes a value (`-qx a.md`). |
| `input-kinds` | Text, Markdown, HTML and MDX, by flag; extensions of the search; CRLF, BOM, NUL, empty files | Oracle: one run for each kind. Corpus: 28,600 documents through the library. | The command line on each kind and each extension, upper-case extensions, a file with no extension, an empty file, a very long line. |
| `formats` | The same, for the 38 fixtures | Oracle: `internal/input` compares the library. | Each fixture through the command, as a file argument and on stdin. |
| `rules` | The 425 equality rules | Oracle and corpus: the library, 17,205 cases. | One run of the command for each rule. |
| `profanity` | `retext-profanities` with `profanitySureness` 0, 1 and 2 | Oracle and corpus: the library, 1,345 cases. | A seeded sample of the corpus for each level through the command, and hand-made words, inflections, control comments, and each input kind. |
| `options` | `allow`, `deny`, `noBinary`, `profanitySureness`; both `allow` and `deny` | Oracle: `allow`, `deny`, `noBinary`, `profanitySureness`. | A value of each type for each option (empty, null, string, number, object), the options from YAML and `package.json`, and a seeded sample of the corpus cases that have options. Found: an empty list is a list that is given. |
| `config-forms` | `.alexrc`, `.alexrc.json`, `.yml`, `.yaml`, `.js`, `.cjs`, `.mjs`, the `alex` field; order between forms; nearest file; parse errors | Oracle: four forms and the nearest file. Unit: each form. | The order between forms, a broken file of each form, `null`, an array or a number as the whole config, a directory as the config, the config with `--stdin`, and a bad config in only one of two directories. Found: a config that is not an object, and `null`. |
| `files-args` | File and directory arguments; missing files; the skip of `node_modules` and dot names; links; sorting | Oracle: file, directory, missing file, sorting. | Trailing slashes, `./`, `..`, names with spaces and Unicode, links, a directory that is named like a file, a file named twice. |
| `globs` | Globs of node-glob 8: `*`, `**`, braces, classes, extended patterns, negation | Oracle: three globs. Another branch: a corpus of 20,000 glob cases. | Forty globs on one tree, with other flags. |
| `default-search` | `{docs/**/,doc/**/,}*.{ext}` when no input is given, for each input kind | Oracle: the default search. | Each kind, nested folders, hidden folders, upper-case extensions, `--stdin` text that is not read. |
| `alexignore` | `.alexignore` with the syntax of `.gitignore`; nested files; an ignored file that is named | Oracle: one file. Unit: the patterns. | Twenty-three pattern forms, nested files, negation of a parent, named files, globs, and `--stdin`. |
| `control-comments` | `ignore`, `disable`, `enable`, an unknown keyword, in Markdown, HTML and MDX | Oracle: one comment. Corpus: the comment kinds in all formats. | Twenty-one comment forms, the scope of `ignore`, nesting, text mode, and each reporter. Found: the order of an error among the findings. |
| `reporter-text` | The table of `vfile-reporter`, the summary line, `--why` notes, `--quiet` | Oracle: the reports of all runs. | Column widths, wide characters, many messages, errors with warnings, several files. |
| `reporter-json` | `vfile-reporter-json`, with the name or the path | Oracle: 40 runs, and golden files. | The forms of the reporter value, and each error kind in the JSON form. |
| `colors` | `supports-color` 9 on stderr: `FORCE_COLOR`, `--color` flags, a terminal, `TERM`, `CI`; `NO_COLOR` has no effect | Oracle: `FORCE_COLOR=1`. | Each value of `FORCE_COLOR`, `NO_COLOR`, `TERM=dumb`, `COLORTERM`, `CI`, and runs with a pseudo terminal on stderr. Found: the whole decision of `supports-color`. |
| `exit-codes` | 0 with no message, 1 with any, and for errors | Oracle: the code of each run. | The codes for each kind of run, including no files and the JSON form. |
| `stdin` | `--stdin` for each kind; `<stdin>` as the name; no globs with `--stdin` | Oracle: three runs. | Twenty-seven runs: each kind, encodings, line endings, large input, config and ignore files, and diff. |
| `errors` | A missing file, a bad config, an unknown keyword, an empty HTML file, an ignored file, MDX syntax errors, JavaScript config files | Oracle: three runs. | Twenty-nine runs of the cases that fail. Most differ in the text of a JavaScript error, so they are known gaps. |
| `diff` | `--diff` under Travis CI (`TRAVIS_COMMIT_RANGE`) and GitHub Actions (`GITHUB_SHA`, `GITHUB_BASE_REF`, `GITHUB_HEAD_REF`), in a git repository | Unit: `diff_test.go` with a scratch repository. Oracle: none. | Sixty-one runs in a scratch repository for each form of the range, and each kind of change: added, changed, deleted and renamed lines, new files, merges, bad revisions, no repository, and no CI. |
| `fuzz` | Everything above, together | none | A seeded generator of files, config, `.alexignore` and arguments. 400 cases a run, and 2,000 in the nightly run, with a seed from the date. |

## Running it in CI

`.forgejo/workflows/oracle.yml` runs the suite after the oracle tests, on each pull request that touches
`internal/`, `cmd/`, `testdata/` or the scripts, and every night at 03:17 UTC. The nightly run uses a fuzz seed
from the date. The log prints the seed, and `EQUIV_SEED=<seed> scripts/equivalence.sh` repeats it. A failed
run keeps `dist/equivalence` as an artifact.

## Known gaps

The gaps are in [known-gaps.json](../testdata/equivalence/known-gaps.json). Each names a row of
[Parity.md](Parity.md). The summary of the last full run is in the pull request that adds the suite.
