# Upstream rules

lexi gets its rules and its matching logic from `retext-equality` 6.6.0. This is the version that
alex 11.0.1 pins.

## Source and licence

| Item | Value |
|---|---|
| Package | `retext-equality@6.6.0` on npm |
| Repository | <https://github.com/retextjs/retext-equality>, tag `6.6.0` |
| Licence | MIT, Copyright (c) 2015 Titus Wormer. The text is in `internal/rules/LICENSE-retext-equality`. |
| Rule data | `lib/en.js` of the package. It holds 425 patterns, built from the YAML files in `data/en`. |
| Tests | `test.js` of the tag. `testdata/upstream.json` holds the 60 cases that call `process`. |

## Files

| File | Content |
|---|---|
| `internal/rules/data.json` | The 425 patterns as JSON. Each phrase keeps its category, in upstream order. |
| `internal/rules/parse.go` | A port of the parts of `parse-latin` and `parse-english` that decide what a word is. |
| `internal/rules/match.go` | A port of `nlcst-search`, `nlcst-is-literal`, and the `or` logic of `retext-equality`. |
| `testdata/upstream.json` | The upstream test cases. `TestUpstreamFixtures` runs them. |
| `testdata/oracle.json` | Edge cases for the tokenizer, such as quotes, labels, and abbreviations. |
| `testdata/golden.json` | What alex printed for `oracle.json` and 700 random texts. `TestGolden` replays it. |
| `scripts/upstream/import.mjs` | Rebuilds `data.json` and `upstream.json` from the package and the tests. |

## What the port does

- Pairs: an `or` rule does not report two phrases of different categories that sit next to each
  other, joined by white space, `and`, `or`, or a slash. `noBinary` turns this off.
- Case: a phrase with a capital letter matches only with that exact case.
- Suggestions: the first letter is a capital when the match starts with a capital.
- Literals: a word in quotes or brackets, or before or after `:`, `;` or a dash, is skipped.
- Apostrophes and dashes: normalised away, except for the one pattern that sets `apostrophe`.
- `allow` skips rules by id. `deny` reports only the rules it lists. Both together is an error, as in
  alex.
- Columns count UTF-16 units, as alex does.

## Left out

- HTML, MDX, and Markdown input. lexi reads plain text, like `alex.text()`.
- The `ignore` option of `retext-equality` is not in the config. `rules.Options.Ignore` has it, for
  the upstream tests. alex does not pass it either.
- `<!--alex ignore-->` comments. They only work in Markdown and HTML.
- The `--diff` flag of the alex CLI and its `--why` output.
- Sorting of findings that share a position uses a byte compare of the rule id. alex uses
  `localeCompare`. No known text shows a difference.
- Sentence rules of `parse-latin` are a close port, not an exact one. A rare mark next to a full stop
  could differ. See the oracle.

## The profanity check

lexi ports `retext-profanities` 7.2.2, the version alex 11.0.1 pins, with `cuss` 2.2.0 for the words and
their ratings and `pluralize` 8.0.0 for the inflections. All three packages use the MIT licence,
Copyright (c) 2016 Titus Wormer (`pluralize`: Blake Embrey, see `internal/rules/LICENSE-retext-profanities`). alex passes `profanitySureness` as the
`sureness` option, and the default is 0.

| Item | Value |
|---|---|
| Word list | `internal/rules/profanity.json`: 3083 words, each with a rating. It holds every `cuss` word with its singular and its plural, as `retext-profanities` builds it. |
| Rating | 0 is "profane in some cases", 1 is "may be profane", 2 is "profane". A word with a rating below `profanitySureness` is skipped. |
| Rule id | The word, with each run of other characters turned into a dash. `alligator bait` is `alligator-bait`. `allow` and `deny` use it. |
| Code | `internal/rules/profanity.go`. It shares the tokenizer and the matcher with the equality check. |
| Corpus | `testdata/corpus/profanity`: 1345 texts at all three ratings, with what alex printed. `TestCorpus` replays it. |
| Rebuild | `scripts/upstream/import-profanity.mjs`. Run it from the oracle directory. |

- A word is a match when its normalised form equals the normalised phrase. Case, dashes and apostrophes do
  not count. A quoted or labelled word is skipped, as in the equality check.
- `hell` and `whore` are the two upstream "regular" words. They are searched with apostrophes kept.
  The upstream code passes `true` to `nlcst-search`, which reads it as `allowApostrophes`. Quoted
  copies stay skipped.
- A match covers all words of the phrase. The equality check reports the first word only.
- A finding sorts by line, column, source (`retext-equality` first), rule id, then message.

## The oracle

`TestOracle` compares lexi with the real alex. It skips when `ALEX_ORACLE` is not set. It checks the
upstream cases, `oracle.json`, one text for each phrase of each rule, and 1500 random texts. It reads
only the `retext-equality` messages. `TestProfanityOracle` does the same for the profanity check.

```bash
mkdir /tmp/alex-oracle && cd /tmp/alex-oracle && npm init -y && npm install alex@11.0.1
cd - && ALEX_ORACLE=/tmp/alex-oracle go test ./internal/rules -run Oracle -v
```

At the time of the port, lexi and alex agreed on all of 7934 texts. Run the test with
`UPDATE_GOLDEN=1` to record `golden.json` again after you change `oracle.json`.

## The corpus

`testdata/corpus/` holds more than 17000 labelled texts with the alex result for each. `TestCorpus` runs
lexi over them without Node. See `testdata/corpus/README.md` to refresh them and
`testdata/corpus/KNOWN_DIFFS.md` for expected differences.

## Update the rules

For the profanity list, run `scripts/upstream/import-profanity.mjs` and then the oracle.

1. Extract the new npm package, and get its `test.js` from the tag.
2. Run `scripts/upstream/import.mjs` as its header says.
3. Run the oracle with the new alex version. Fix `parse.go` or `match.go` for each difference.
4. Run `scripts/coverage.sh`.
