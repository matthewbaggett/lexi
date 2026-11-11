# lexi

A static Go port of alex, the inclusive-language linter. Same configuration, one portable binary or a FROM scratch image.
## Development

```bash
mise install        # Go, hk, pkl, golangci-lint, shellcheck
mise run hooks      # pre-commit lints, pre-push tests
mise run ci         # lints, then tests
```

Unit tests must cover 100% of statements. `scripts/coverage.sh` enforces it, the pre-push hook
runs it, and CI fails a pull request that drops below it.
