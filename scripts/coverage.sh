#!/usr/bin/env bash
# Run the unit tests and fail unless statement coverage is exactly 100%.
set -euo pipefail

profile=$(mktemp)
trap 'rm -f "$profile"' EXIT

go test -race -covermode=atomic -coverprofile="$profile" ./...

total=$(go tool cover -func="$profile" | awk '/^total:/ {print $3}')
echo "total coverage: $total"
if [ "$total" != "100.0%" ]; then
  echo "coverage is below 100%. Uncovered:" >&2
  go tool cover -func="$profile" | grep -v '100.0%' >&2 || true
  exit 1
fi
