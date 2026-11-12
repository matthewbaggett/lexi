# Language-check corpus

Each line of `<category>/<kind>.jsonl` is one case. The generator `scripts/corpus/generate.mjs`
writes the files and records what the real alex 11.0.1 reports in the `want` field. The Go test
`TestCorpus` in `internal/rules` runs lexi over every case and compares the result. It needs no Node.

| Field | Meaning |
|---|---|
| `id` | Hash of the text and the options. Unique. |
| `category` | The check. `equality` now. Add `profanity`, `html` or `mdx` with their own handlers. |
| `kind` | The shape of the case. It matches the file name. |
| `label` | `positive`, `negative` or `edge`: what the case aims at. `want` is the truth. |
| `rule` | The rule id the case aims at, when it has one. |
| `text`, `allow`, `deny`, `noBinary` | The input and the alex options. |
| `want` | What alex reported: `line:col-line:col: message [rule-id]`. |

Refresh the files after you change the generator or the rules:

```bash
mkdir /tmp/alex-oracle && cd /tmp/alex-oracle && npm init -y && npm install alex@11.0.1
cd - && ALEX_ORACLE=/tmp/alex-oracle node scripts/corpus/generate.mjs
```

See [KNOWN_DIFFS.md](KNOWN_DIFFS.md) for the cases where lexi differs from alex on purpose.
