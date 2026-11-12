#!/usr/bin/env bash
# SessionStart check: stop the session when its folder is not this project, or a worktree of it.
#
# The forgejo scripts of agent-standards read the repo from the `origin` remote of the current
# folder. A session that starts in another repo acts on that repo. See issue 3.
#
# The expected repo is the `origin` remote of the checkout that holds this script. The session
# folder is the `cwd` field of the hook input, or the current folder without it. The check passes
# when both remotes name the same repo. A linked worktree shares the remote of its main checkout.
# It compares the host and the path. The scheme, the user, and a trailing `.git` do not count.
#
# On a mismatch it prints a JSON verdict with `continue: false`, which stops the session, and the
# reason for the user. It always exits 0. When it cannot read the expected remote, it passes.
set -uo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# Print host/path of a remote URL, in lower case. It handles ssh://, https://, and scp style.
normalize() {
  local u=$1
  u=${u%/}
  u=${u%.git}
  u=${u#*://}
  u=${u#*@}
  u=${u/:/\/}
  printf '%s\n' "$u" | tr '[:upper:]' '[:lower:]'
}

expected_url=$(git -C "$here" remote get-url origin 2>/dev/null) || exit 0
[ -n "$expected_url" ] || exit 0

input=""
[ -t 0 ] || input=$(cat)
dir=""
if [ -n "$input" ]; then
  if command -v jq >/dev/null 2>&1; then
    dir=$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)
  else
    dir=$(printf '%s' "$input" | sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
  fi
fi
[ -n "$dir" ] || dir=$PWD

actual_url=""
if [ -d "$dir" ]; then
  actual_url=$(git -C "$dir" remote get-url origin 2>/dev/null || true)
fi

if [ -n "$actual_url" ] && [ "$(normalize "$actual_url")" = "$(normalize "$expected_url")" ]; then
  exit 0
fi

expected=$(normalize "$expected_url")
if [ -n "$actual_url" ]; then
  found="its origin remote is $(normalize "$actual_url")"
else
  found="it is not a git checkout with an origin remote"
fi
msg="Wrong start folder: $dir. This project is $expected, but $found. Forgejo scripts read the repo from origin, so they would act on the wrong repo. Close this session and start it in the project root or in one of its worktrees."

if command -v jq >/dev/null 2>&1; then
  jq -cn --arg m "$msg" '{continue:false, stopReason:$m, systemMessage:$m}'
else
  esc=${msg//\\/\\\\}
  esc=${esc//\"/\\\"}
  printf '{"continue":false,"stopReason":"%s","systemMessage":"%s"}\n' "$esc" "$esc"
fi
exit 0
