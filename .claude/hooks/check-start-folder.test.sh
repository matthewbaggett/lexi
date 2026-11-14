#!/usr/bin/env bash
# Table test for check-start-folder.sh. Each row builds a folder with an origin remote, feeds the
# hook a SessionStart event with that folder as `cwd`, and checks for "stop" or "pass".
# Run it from anywhere:  .claude/hooks/check-start-folder.test.sh
set -uo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
HOOK="$HERE/check-start-folder.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# The hook reads the expected repo from the checkout that holds it, so test a copy of it in a
# fake project whose origin is ssh://git@host.example/Org/proj.git.
proj="$tmp/proj"
mkdir -p "$proj/.claude/hooks"
cp "$HOOK" "$proj/.claude/hooks/"
git -C "$proj" init -q
git -C "$proj" remote add origin ssh://git@host.example/Org/proj.git
copy="$proj/.claude/hooks/check-start-folder.sh"

mkrepo() {  # $1 = folder, $2 = origin url or empty
  mkdir -p "$1"
  git -C "$1" init -q
  [ -z "$2" ] || git -C "$1" remote add origin "$2"
}

pass=0 fail=0 n=0
row() {  # $1 = name, $2 = want (stop|pass), $3 = cwd
  n=$((n + 1))
  local out got=pass
  out=$(printf '{"hook_event_name":"SessionStart","cwd":"%s"}' "$3" | bash "$copy"); rc=$?
  case "$out" in *'"continue":false'*) got=stop ;; esac
  [ "$rc" -eq 0 ] || got="exit$rc"
  if [ "$got" = "$2" ]; then pass=$((pass + 1)); else
    fail=$((fail + 1)); echo "FAIL row $n: $1 (want $2, got $got)"
  fi
}

mkrepo "$tmp/same" "ssh://git@host.example/Org/proj.git"
mkrepo "$tmp/https" "https://host.example/Org/proj"
mkrepo "$tmp/scp" "git@host.example:Org/proj.git"
mkrepo "$tmp/case" "ssh://git@HOST.example/org/PROJ.git"
mkrepo "$tmp/other" "ssh://git@host.example/Org/other.git"
mkrepo "$tmp/otherhost" "ssh://git@elsewhere.example/Org/proj.git"
mkrepo "$tmp/noorigin" ""
mkdir -p "$tmp/plain"
git -C "$proj" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
git -C "$proj" worktree add -q "$tmp/wt" -b wt
mkdir -p "$proj/sub"

row "same remote passes" pass "$tmp/same"
row "https without .git passes" pass "$tmp/https"
row "scp style passes" pass "$tmp/scp"
row "case and user do not count" pass "$tmp/case"
row "a linked worktree passes" pass "$tmp/wt"
row "a subfolder of the project passes" pass "$proj/sub"
row "another repo stops" stop "$tmp/other"
row "another host stops" stop "$tmp/otherhost"
row "no origin stops" stop "$tmp/noorigin"
row "not a git folder stops" stop "$tmp/plain"
row "a missing folder stops" stop "$tmp/missing"

# Without a cwd field, the hook uses the current folder.
n=$((n + 1))
out=$(cd "$tmp/other" && bash "$copy" </dev/null)
case "$out" in *'"continue":false'*) pass=$((pass + 1)) ;; *) fail=$((fail + 1)); echo "FAIL row $n: no cwd uses PWD" ;; esac

# The message names the folder and the project, and is valid JSON.
n=$((n + 1))
out=$(printf '{"cwd":"%s"}' "$tmp/other" | bash "$copy")
if printf '%s' "$out" | jq -e '.stopReason | contains("host.example/org/proj") and contains("other")' >/dev/null 2>&1; then
  pass=$((pass + 1))
else fail=$((fail + 1)); echo "FAIL row $n: the message is not clear"; fi

echo "check-start-folder.test: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
