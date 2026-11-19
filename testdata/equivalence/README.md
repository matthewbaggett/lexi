# Equivalence cases

Each case is a directory: `cases/<row>/<case>/`. The suite runs the case in the real alex and in
lexi, and compares the results. See [docs/Equivalence.md](../../docs/Equivalence.md). You add a case
with files, and you write no Go.

`<row>` is the `id` of a row in [matrix.json](matrix.json). `<case>` is a short name with
lowercase letters, digits and `-`. Use no two names that differ only in case.

| Path in the case directory | Content |
|---|---|
| `files/` | The files of the working directory, before the run. Dot files are fine. |
| `args` | The arguments after the program name, one on each line. A missing file means no arguments. |
| `env` | Extra environment variables, `NAME=value` on each line. The run starts with a clean environment, so a CI variable of the host cannot change a result. |
| `stdin` | The bytes on stdin. A missing file means an empty stdin. |
| `setup.sh` | A shell script that runs in the working directory before the program, with `sh -e`. Use it for `git init`, a commit, or a link. |
| `case.json` | `{"tty": true}` gives the program a terminal on stderr, as when a person runs it. It needs `python3`. |

## What the suite compares

For each case, in a scratch directory for each tool:

- stdout, stderr and the exit code, byte for byte;
- the files that the run created, changed or removed, compared by content (the `.git` directory is not
  compared).

Only two things are changed before the compare: the path of the scratch directory becomes `<cwd>`
(`<tmp>` for its parent), and the program name `alex` or `lexi` becomes `<prog>`.

`--reporter=json` goes to alex as the entry file of `vfile-reporter-json`, because alex loads a reporter
from the working directory, and the scratch directory has no `node_modules`.

## Known gaps

A mismatch that lexi has on purpose goes in [known-gaps.json](known-gaps.json):

```json
{
  "gaps": [
    {
      "id": "version-text",
      "parity": "`-v`, `--version`",
      "reason": "lexi prints its own version, and names the alex release it follows.",
      "cases": ["flags-single/info-v", "flags-single/info-version"]
    }
  ]
}
```

- `parity` is the text of the first cell of a row in [docs/Parity.md](../../docs/Parity.md). The suite
  fails if no row has it.
- `cases` are case ids. `*` matches any text, so `fuzz/*` is every fuzz case.
- A gap that matches no mismatch, or a case it names that now matches alex, fails the run. Remove the entry, so the
  baseline only shrinks. Add `"volatile": true` for a gap that other work is changing: it then only warns.
- `when` limits a gap to outputs of one shape: `{"alex": "regex", "lexi": "regex", "args": "regex"}`.

## The generated cases

The suite also makes cases without directories:

| Row | Source |
|---|---|
| `formats` | Each file of [testdata/formats](../formats), as a file argument and on stdin, with the flag for its kind. |
| `rules` | One case for each equality rule: the first positive case of the corpus. |
| `profanity` | A seeded sample of the profanity corpus for each `profanitySureness`. |
| `options` | A seeded sample of the corpus cases that have `allow`, `deny` or `noBinary`. |
| `fuzz` | Random files, configuration and arguments. |

`EQUIV_SEED` (default 1) sets the seed of the samples and of the fuzz cases. `EQUIV_FUZZ` (default 400) sets
the number of fuzz cases. `EQUIV_CORPUS` (default 40) sets the size of each corpus sample. A fuzz case
that fails has the id `fuzz/<seed>-<number>`. Run it again with
`EQUIV_SEED=<seed> scripts/equivalence.sh --filter 'fuzz/<seed>-<number>'`.
