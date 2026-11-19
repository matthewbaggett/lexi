#!/usr/bin/env bash
# Run the equivalence suite: build lexi, install the pinned alex, and compare the two tools on every case.
# See docs/Equivalence.md. Every option goes to scripts/equivalence/run.mjs, for example:
#   scripts/equivalence.sh --row diff
#   scripts/equivalence.sh --filter 'globs/*'
#   EQUIV_SEED=7 EQUIV_FUZZ=2000 scripts/equivalence.sh
# Exit: 0 when every case matches or is a known gap, 1 on any other mismatch, 2 when the run cannot start.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

command -v node > /dev/null || { echo "equivalence: node is not installed" >&2; exit 2; }

# Install alex 11.0.1 from the lock file, unless an install is there. ALEX_ORACLE can name another one.
oracle="${ALEX_ORACLE:-$PWD/testdata/oracle}"
if [ ! -f "$oracle/node_modules/alex/cli.js" ]; then
  npm ci --prefix "$oracle" --no-audit --no-fund > /dev/null
fi

mkdir -p dist
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/lexi ./cmd/lexi

exec node scripts/equivalence/run.mjs --lexi dist/lexi --alex "$oracle" "$@"
