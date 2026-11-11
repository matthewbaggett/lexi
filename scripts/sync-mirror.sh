#!/usr/bin/env bash
# Pushes main and the tags to the public mirror. See docs/Mirror.md.
#
# Usage: scripts/sync-mirror.sh [--dry-run]
#
# `origin` is the primary remote. `github` is the mirror. The script refuses to push when:
#   - the work tree is not on main, or main differs from origin/main
#   - the commits to push name the private host (scripts/check-mirror-safe.sh)
#   - the mirror holds a commit that main lacks (a push would need a force)
# It never forces. It pushes main and the tags only, never another branch.
set -euo pipefail

dry=()
[ "${1:-}" = "--dry-run" ] && dry=(--dry-run)
mirror="${MIRROR_REMOTE:-github}"
here="$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")"
cd "$here/.."

die() {
  echo "sync-mirror: $*" >&2
  exit 1
}

git remote get-url "$mirror" >/dev/null 2>&1 ||
  die "no remote named $mirror. Add it: git remote add $mirror git@github.com:matthewbaggett/lexi.git"

[ "$(git branch --show-current)" = main ] || die "switch to main first"
git fetch -q origin main
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || die "main differs from origin/main. Pull or push it first"
[ -z "$(git status --porcelain)" ] || die "the work tree is not clean"

if git fetch -q "$mirror" main 2>/dev/null; then
  git merge-base --is-ancestor "$mirror/main" main ||
    die "$mirror/main holds a commit that main lacks. Do not force. Find out why"
  range="$mirror/main..main"
else
  range="main" # the mirror has no main yet: the first push sends the whole history
fi

"$here/check-mirror-safe.sh"
"$here/check-mirror-safe.sh" --history "$range"

git push "${dry[@]}" "$mirror" main
git push "${dry[@]}" "$mirror" --tags
echo "sync-mirror: $mirror is in step with main"
