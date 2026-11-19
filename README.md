# lexi

A static Go port of [alex](https://github.com/get-alex/alex), the inclusive-language linter.
lexi reads the same configuration as alex. It ships as one portable binary and as a
`FROM scratch` image.

Status: lexi 0.1.0 matches alex 11.0.1 on every feature except the gaps below. The oracle test
and a corpus of 28,600 cases recorded from alex check it.

Gaps, with the reasons in [docs/Parity.md](docs/Parity.md):

- MDX: lexi does not parse JavaScript, so it does not report an MDX syntax error.
- `.alexrc.js`, `.alexrc.cjs`, and `.alexrc.mjs`: they run JavaScript. Use JSON or YAML.
- `--reporter`: only the JSON reporter works.
- Globs: no extended patterns of node-glob, such as `+(a|b)`.
- `--diff`: it needs `git` on the path, and works in Travis CI and GitHub Actions only.

- Rules: `retext-equality` 6.6.0, the version that alex 11.0.1 pins. 425 patterns, with pairs,
  plurals, and inflections. See [docs/Upstream.md](docs/Upstream.md).
- Tested against the real alex with an oracle test.
- Input: plain text, Markdown, HTML and MDX, with the flags and the configuration of alex.
  See [docs/Parity.md](docs/Parity.md) for what is done and what differs.
- Profanity check: `retext-profanities` 7.2.2 with `cuss` 2.2.0, and `profanitySureness` from `.alexrc`.

## Install

Download one binary from the
[GitHub releases](https://github.com/matthewbaggett/lexi/releases). Pick the one for your system:
`lexi-linux-amd64`, `lexi-linux-arm64`, `lexi-darwin-amd64`, or `lexi-darwin-arm64`.

```bash
os=linux arch=amd64   # or darwin, arm64
curl -fLO "https://github.com/matthewbaggett/lexi/releases/latest/download/lexi-$os-$arch"
curl -fLO "https://github.com/matthewbaggett/lexi/releases/latest/download/checksums.txt"
grep "lexi-$os-$arch\$" checksums.txt | sha256sum -c -   # on macOS: shasum -a 256 -c -
chmod +x "lexi-$os-$arch" && mv "lexi-$os-$arch" /usr/local/bin/lexi
lexi --version
```

`lexi --version` prints the lexi version and the alex version that lexi follows, for example
`lexi 0.1.0 (alex 11.0.1 compatible)`.

The `FROM scratch` image is on the GitHub container registry:

```bash
echo "His network looks good" | docker run --rm -i ghcr.io/matthewbaggett/lexi:latest --stdin
```

Pin a version with a tag such as `ghcr.io/matthewbaggett/lexi:0.1.0`.

## Use lexi in CI instead of alex

lexi takes the flags, the configuration and the exit codes of alex. A CI job that runs alex can
run lexi with no other change. This holds for a job that reads the JSON report of alex, too:
`--reporter=json` prints the same JSON as `vfile-reporter-json` 4.0.0, with the same fields in
the same order, on stderr.

```bash
lexi --reporter=json docs/*.md 2> report.json
```

Take these points into account:

- The exit code is 0 when there is no message, and 1 when there is a warning or an error. The
  JSON report does not change it.
- `--stdin`, `--quiet`, `--text`, `--html`, `--mdx` and file, directory and glob arguments work as
  in alex. The globs have all the syntax of glob 8.1.0, the library of alex: `**`, braces and
  ranges, the extended patterns `+(a|b)`, `@(a|b)`, `!(a|b)`, `?(a|b)` and `*(a|b)`, and classes.
  Quote them, so that the shell leaves them alone. `--quiet` leaves the files without messages out of the JSON, and the report is `[]` when
  there are none.
- `--reporter` accepts `json`, `vfile-reporter-json`, and the path of that package or of its
  `index.js`. It loads nothing. Another value is an error with exit code 2.
- The report goes to stderr, and stdout stays empty.

A script that calls alex with `--reporter=<path>/vfile-reporter-json/index.js` needs a directory
with the layout of `node_modules`. Put lexi where the script looks for `alex`, and make an empty
file for the reporter. For `tools/alex/run-alex.sh` of agent-standards, this is `ALEX_ROOT`:

```bash
mkdir -p /opt/lexi/node_modules/.bin /opt/lexi/node_modules/vfile-reporter-json
ln -s "$(command -v lexi)" /opt/lexi/node_modules/.bin/alex
touch /opt/lexi/node_modules/vfile-reporter-json/index.js
ALEX_ROOT=/opt/lexi tools/alex/run-alex.sh docs/Guide.md
```

The script then runs lexi and reads the JSON from stderr. Exit 0 and exit 1 both mean that the
check ran. A tool such as `tools/vale/lint-prose.sh` filters the report with `tools/alex/filter.json`,
and that works with no change, because the fields and the rule ids are the same. A Docker
image can do the same with `COPY --from=ghcr.io/matthewbaggett/lexi:latest /lexi /opt/lexi/node_modules/.bin/alex`.

## Develop

```bash
mise install        # Go, hk, pkl, golangci-lint, shellcheck
mise run hooks      # pre-commit lints, pre-push tests
mise run ci         # lints, then tests
```

Unit tests must cover 100% of statements. `scripts/coverage.sh` enforces it, the pre-push hook
runs it, and CI fails a pull request that drops below it.

See [docs/Mirror.md](docs/Mirror.md) for the public mirror and the hooks.
