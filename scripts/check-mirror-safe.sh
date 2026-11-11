#!/usr/bin/env bash
# Fails when a file of this repo names the private host. The repo is mirrored to a public host, so
# a mention would leak. The pattern comes from two pieces so that this file doesn't match itself.
#
# Usage: scripts/check-mirror-safe.sh            check every tracked file
#        scripts/check-mirror-safe.sh <file>...  check these files only
#        scripts/check-mirror-safe.sh --history <rev-range>
#                                                check the messages and the diffs of the commits
#
# The submodule under .claude/agent-standards is not read. It holds a commit pointer only.
# Exit: 0 clean, 1 a mention was found.
set -euo pipefail

pattern="grey""\\.ooo"
cd "$(git rev-parse --show-toplevel)"

if [ "${1:-}" = "--history" ]; then
  range="${2:?usage: --history <rev-range>}"
  if git log -p --format='%H%n%an%n%ae%n%B' "$range" -- . ':(exclude).claude/agent-standards' |
    grep -n -i -E "$pattern" >&2; then
    echo "check-mirror-safe: a commit in $range names the private host" >&2
    exit 1
  fi
  exit 0
fi

if [ $# -gt 0 ]; then
  files=("$@")
else
  mapfile -t files < <(git ls-files -- . ':(exclude).claude/agent-standards')
fi

status=0
for f in "${files[@]}"; do
  # A symlink commits its target path, not the content of the file it points at.
  [ -L "$f" ] && continue
  [ -f "$f" ] || continue
  case "$f" in
    .claude/agent-standards | .claude/agent-standards/*) continue ;;
  esac
  if grep -n -I -i -E "$pattern" -- "$f" >&2; then
    echo "check-mirror-safe: $f names the private host" >&2
    status=1
  fi
done
exit "$status"
