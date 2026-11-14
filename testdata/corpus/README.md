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
