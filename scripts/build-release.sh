#!/usr/bin/env bash
# Builds the release binaries and a checksum file. The release workflow runs it.
#
# Usage: scripts/build-release.sh <tag> [<out-dir>]
#
# <tag> is a semantic version with a leading v, such as v0.1.0 or v0.2.0-rc.1.
# The out dir defaults to dist/release. The script writes one static binary for each of
# linux and darwin on amd64 and arm64, named lexi-<os>-<arch>, and checksums.txt.
set -euo pipefail

tag="${1:?usage: build-release.sh <tag> [<out-dir>]}"
out="${2:-dist/release}"

semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
[[ "$tag" =~ $semver ]] || {
  echo "build-release: '$tag' is not a semantic version tag such as v0.1.0" >&2
  exit 1
}
version="${tag#v}"

cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/.."
mkdir -p "$out"
rm -f "$out"/lexi-* "$out"/checksums.txt

for os in linux darwin; do
  for arch in amd64 arm64; do
    echo "build-release: lexi-$os-$arch"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
      -ldflags="-s -w -X main.version=$version" -o "$out/lexi-$os-$arch" ./cmd/lexi
  done
done

(
  cd "$out"
  if command -v sha256sum >/dev/null; then
    sha256sum lexi-* >checksums.txt
  else
    shasum -a 256 lexi-* >checksums.txt
  fi
)
echo "build-release: wrote $out/checksums.txt"
