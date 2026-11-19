# Language-check corpus

Each line of `<category>/<kind>.jsonl` is one case. The generator `scripts/corpus/generate.mjs`
writes the files and records what the real alex 11.0.1 reports in the `want` field. The Go test
`TestCorpus` in `internal/rules` runs lexi over every case and compares the result. It needs no Node.

| Field | Meaning |
|---|---|
| `id` | Hash of the text and the options. Unique. |
| `category` | `equality` or `profanity` (text), or `markdown`, `html` or `mdx` (the format of the document). |
| `kind` | The shape of the case. It matches the file name. |
| `label` | `positive`, `negative` or `edge`: what the case aims at. `want` is the truth. |
| `rule` | The rule id the case aims at, when it has one. |
| `text`, `allow`, `deny`, `noBinary` | The input and the alex options. |
| `sureness` | The `profanitySureness` option, 0, 1 or 2. Only the `profanity` category has it. |
| `want` | What alex reported: `line:col-line:col: message [rule-id]`. The format categories end it with `[source:rule-id]`, and they use `ERROR <message>` when alex stops with an error. |
| `known` | Only in the format categories. The name of a section of [KNOWN_DIFFS.md](KNOWN_DIFFS.md): lexi differs from alex on purpose. |

The `profanity` files come from `TestProfanityOracle`, not from the generator. Its `want` lines end with
`[source:rule-id]`. Record them again with `ALEX_ORACLE=<dir> UPDATE_GOLDEN=1 go test ./internal/rules -run ProfanityOracle`.

Refresh the `equality` files after you change the generator or the rules:

```bash
mkdir /tmp/alex-oracle && cd /tmp/alex-oracle && npm init -y && npm install alex@11.0.1
cd - && ALEX_ORACLE=/tmp/alex-oracle node scripts/corpus/generate.mjs
```

See [KNOWN_DIFFS.md](KNOWN_DIFFS.md) for the cases where lexi differs from alex on purpose.

## The format corpus

`markdown/`, `html/` and `mdx/` hold documents, not phrases. They check how lexi reads each format: which
text it takes as prose, and where in the file a finding is (line, column, end line, end column). Each
document has a term from the equality or the profanity check at a place that tests one construct: a
nested list, a table, a footnote, front matter, a code fence, a character reference, a control comment,
CRLF line endings, tabs, Unicode, a long line, misnested HTML, a table with stray content, and more.
The `soup` documents of `html/` are random sequences of tags and terms, with a fixed seed.

`TestFormatCorpus` in `internal/input` runs lexi over the files and compares with `want`. It needs no
Node. Refresh the files after you change the generator or the input code:

```bash
ALEX_ORACLE=/tmp/alex-oracle node scripts/corpus/generate-formats.mjs
```

Two more settings explore without a commit: `SOUP_COUNT` and `SOUP_SEED` make more HTML soup, and
`MD_SOUP_COUNT` makes random Markdown. Set `CORPUS_DIFF_OUT=<file>` when you run the test to list
every difference, not the first twenty.

## The glob corpus

`glob/` holds the cases of the file arguments: a file tree, a pattern, and what the real alex 11.0.1
expands for it. alex hands each argument with a wildcard to glob 8.1.0 (minimatch 5.1.9) through
unified-engine 10.1.0, and then searches the matches. The generator `scripts/corpus/generate-glob.mjs`
builds the trees in a temporary directory and records both steps. `TestGlobCorpus` in `internal/files`
builds the same trees, and compares the matches of `Glob` and the files of `Find`. It needs no Node.

| File | Content |
|---|---|
| `trees.json` | The ten trees. A name is a file, a name with a slash at the end is a directory, and `{"link": "x", "to": "y"}` is a symbolic link. The trees hold dot files, names with spaces, brackets, braces and Unicode, links that make a cycle, and names that differ in case (`case`, `unicode`). |
| `<kind>.jsonl` | One case on each line. `star`, `qmark`, `class`, `brace`, `range`, `extglob`, `globstar`, `dot`, `escape`, `slash`, `magicedge`, `mixed`, `dirmatch` and `caseless` are written by hand and run on every tree. `literal`, `near` (a pattern that is a little off a real path), `fuzz` (random tokens, with a fixed seed) and `absolute` are made from the trees. |

| Field | Meaning |
|---|---|
| `id` | Hash of the tree and the pattern. Unique. |
| `tree` | The tree. The test runs the pattern in `<tree>/root`. |
| `pattern` | The argument. `${ROOT}` is the absolute path of the root. |
| `magic` | What `glob.hasMagic` says. When it is false, alex does not call glob. |
| `want` | What glob returns. The test compares it as a set, because lexi does not sort as ICU does. |
| `files` | What the finder gives for the argument, with the paths as alex prints them. A sorted set: alex reads in the order of the file system. A file with a message adds ` !<message>`. |
| `crash`, `nofiles` | alex throws in the finder, or the finder would not end (a link to a parent). There is no list of files. |

The patterns have to leave out three kinds of case, so that the generator ends: a pattern from the root
of the real file system, a pattern that goes two levels above a tree, and `*/` in a chain on the tree with
links. `{,}` and `{,,}` are not cases: `glob.hasMagic` throws for them.

Record the files on Linux, where the file system tells `a` from `A`. On macOS, use Docker:

```bash
docker run --rm -v "$PWD":/w -w /w -e ALEX_ORACLE=/w/testdata/oracle node:25-slim \
  node scripts/corpus/generate-glob.mjs
```

On a file system that ignores case, the test skips the trees `case` and `unicode`, and the cases with a
capital letter in the pattern. A Linux run (`docker run golang:1.26`) checks all of them. Set
`CORPUS_DIFF_OUT=<file>` to list every difference, not the first 25.
