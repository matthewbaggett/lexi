# lexi

A static Go port of [alex](https://github.com/get-alex/alex), the inclusive-language linter.
lexi reads the same configuration as alex. It ships as one portable binary and as a
`FROM scratch` image.

Status: the port is in progress.

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

## Develop

```bash
mise install        # Go, hk, pkl, golangci-lint, shellcheck
mise run hooks      # pre-commit lints, pre-push tests
mise run ci         # lints, then tests
```

Unit tests must cover 100% of statements. `scripts/coverage.sh` enforces it, the pre-push hook
runs it, and CI fails a pull request that drops below it.

See [docs/Mirror.md](docs/Mirror.md) for the public mirror and the hooks.
