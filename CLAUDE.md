@.claude/agent-standards/CLAUDE.md
@.claude/agent-standards/docs/Topics/VCS.md
@.claude/agent-standards/docs/Topics/Tooling.md
@.claude/agent-standards/languages/go/CLAUDE.md

# lexi

lexi is a static Go port of [alex](https://github.com/get-alex/alex), the inclusive-language
linter. It reads the same configuration as alex. It ships as one portable binary and as a
`FROM scratch` image.

The rules come from `retext-equality` 6.6.0, the version alex 11.0.1 pins. The port
copies the upstream tests as fixtures. An oracle test compares its findings with the real alex.

## Quality gate

| Check | Command |
|---|---|
| Format | `gofmt -l .` prints nothing |
| Vet | `go vet ./...` |
| Test | `go test ./...` |
| Oracle | `ALEX_ORACLE=<dir with alex installed> go test ./... -run Oracle` |
| Static build | `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" ./cmd/lexi` |
| Mirror safety | `scripts/check-mirror-safe.sh` prints nothing |

## The public mirror

`origin` is the primary repo and is private. `github` is a public mirror. Sync it by hand with
`mise run sync`. See [docs/Mirror.md](docs/Mirror.md).

**Never write the name of the private host in a file, a commit message, or a pull request
body.** The mirror would publish it. `scripts/check-mirror-safe.sh` finds it.
