#!/usr/bin/env bash
# Build-only; never enrolls, installs, starts workers or publishes a release.
set -euo pipefail
fail() { printf '%s\n' 'CONNECTED_BUILD_FAILED' >&2; exit 1; }
[[ $# == 3 ]] || fail
version=$1
commit=$2
output=$3
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd -- "$repo"
[[ $(git rev-parse HEAD) == "$commit" ]] || fail
[[ -z $(git status --porcelain) ]] || fail
[[ ! -e "$output" && ! -L "$output" ]] || fail
scratch=$(mktemp -d -t observer-connected-build.XXXXXXXX) || fail
scratch=$(cd -- "$scratch" && pwd -P)
cleanup() {
  # Delete only the exact mktemp-created task directory, never an inferred root.
  [[ -n "$scratch" && "$scratch" != / && "${scratch##*/}" == observer-connected-build.* ]] || return
  rm -rf -- "$scratch"
}
trap cleanup EXIT
export GOTOOLCHAIN=local
host_os=$(go env GOHOSTOS)
host_arch=$(go env GOHOSTARCH)
CGO_ENABLED=0 GOOS="$host_os" GOARCH="$host_arch" go build -trimpath -o "$scratch/connectedpack" ./cmd/connectedpack >"$scratch/build-output" 2>&1 || fail
mkdir -- "$scratch/bin"
for role in collector uploader install; do
  identity=$("$scratch/connectedpack" --mode identity --role "$role" --version "$version" --commit "$commit") || fail
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-s -w -X main.releaseIdentity=$identity" \
    -o "$scratch/bin/observer-connected-$role" "./cmd/observer-connected-$role" >"$scratch/build-output" 2>&1 || fail
done
"$scratch/connectedpack" --mode pack --version "$version" --commit "$commit" --binaries "$scratch/bin" --output "$output" || fail
printf '%s\n' 'CONNECTED_BUILD_OK'
