# The public mirror

The primary repo is private. A public repo on GitHub mirrors it. The mirror is read-only.
Nobody opens an issue or a pull request there, and nobody pushes to it by hand.

| Remote | Role |
|---|---|
| `origin` | The primary repo. All work, issues, and pull requests live here |
| `github` | The public mirror. A script pushes `main` and the tags to it |

## Set up once

```bash
git remote add github git@github.com:matthewbaggett/lexi.git
```

Create the empty public repo on GitHub first. Do not add a README or a licence there.

## Sync

Run the sync after each merge to `main`, and after each tag:

```bash
mise run sync
```

`scripts/sync-mirror.sh` does these steps:

1. It checks that you are on `main`, that `main` equals `origin/main`, and that the tree is clean.
2. It fetches the mirror. It stops when the mirror holds a commit that `main` lacks. It never
   forces a push.
3. It runs `scripts/check-mirror-safe.sh` on the tree and on the commits it will push.
4. It pushes `main`, then the tags.

Run `scripts/sync-mirror.sh --dry-run` to see what a push would send.

## Keep the private host out

Nothing in this repo may name the private host. A file, a commit message, or a diff that names
it would reach the public mirror. `scripts/check-mirror-safe.sh` checks the tracked files.
The pre-commit and pre-push hooks run it. The sync runs it on the history it pushes.

The submodule address in `.gitmodules` is relative, so it names no host. It resolves against
`origin`. On the mirror it resolves to a repo that may not exist. A public clone then has no
`.claude/agent-standards`. The Go code needs none of it.

If a commit names the host, do not push it. Fix it on a branch before it reaches `main`.
History that reached `main` can't be changed after a sync. Ask the owner.

## Hooks

```bash
mise install
mise run hooks
```

| Hook | Steps |
|---|---|
| `pre-commit` | gofmt, ShellCheck, markdownlint, the mirror check. Each runs on the staged files |
| `pre-push` | `go vet`, `go test ./...`, the mirror check on the whole tree |
