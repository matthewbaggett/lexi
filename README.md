# lexi

A static Go port of [alex](https://github.com/get-alex/alex), the inclusive-language linter.
lexi reads the same configuration as alex. It ships as one portable binary and as a
`FROM scratch` image.

Status: the port is in progress.

- Rules: `retext-equality` 6.6.0, the version that alex 11.0.1 pins.
- Not ported: the profanity check, and HTML and MDX input.

## Develop

```bash
mise install        # Go, hk, pkl, golangci-lint, shellcheck
mise run hooks      # pre-commit lints, pre-push tests
mise run ci         # lints, then tests
```

Unit tests must cover 100% of statements. `scripts/coverage.sh` enforces it, the pre-push hook
runs it, and CI fails a pull request that drops below it.

See [docs/Mirror.md](docs/Mirror.md) for the public mirror and the hooks.
