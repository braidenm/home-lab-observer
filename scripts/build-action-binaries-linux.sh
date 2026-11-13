#!/usr/bin/env bash
# Build-only prerequisite for the stopped Linux action package.
set -euo pipefail
[[ $# == 3 ]] || exit 2
version=$1
revision=$2
[[ "$version" =~ ^0\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})-canary\.([1-9][0-9]{0,3})$ && "$revision" =~ ^[a-f0-9]{40}$ ]] || exit 2
[[ -n "$3" && "$3" != *$'\n'* && "$3" != *$'\r'* ]] || exit 2
output=$(realpath -m -- "$3")
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
export LANG=C LC_ALL=C TZ=UTC
umask 077
fixed_go() {
  env GOENV=off GOWORK=off 'GOFLAGS=-mod=readonly -buildvcs=false' GOEXPERIMENT= GOTOOLCHAIN=local \
    GOAMD64=v1 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go "$@"
}
[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || exit 2
[[ "$(fixed_go version)" == 'go version go1.27.3 linux/amd64' && "$(git rev-parse HEAD)" == "$revision" ]] || exit 2
status=$(git status --porcelain --untracked-files=all)
[[ -z "$status" ]] || exit 2
mkdir -m 700 -- "$output"
fixed_go build -trimpath -ldflags '-s -w -buildid=' -o "$output/connectedpack" ./cmd/connectedpack
roles=(action-worker action-broker action-setup)
for round in first second; do
  mkdir -m 700 -- "$output/$round"
  for role in "${roles[@]}"; do
    identity=$("$output/connectedpack" --mode identity --profile actions --role "$role" --version "$version" --commit "$revision")
    fixed_go build -trimpath -ldflags "-s -w -buildid= -X main.releaseIdentity=$identity" \
      -o "$output/$round/observer-$role" "./cmd/observer-$role"
    grep -aFq -- "$identity" "$output/$round/observer-$role"
  done
done
for role in "${roles[@]}"; do
  cmp -- "$output/first/observer-$role" "$output/second/observer-$role"
done
mkdir -m 700 -- "$output/payload"
for role in "${roles[@]}"; do
  install -m 755 -- "$output/first/observer-$role" "$output/payload/observer-$role"
done
(
  cd -- "$output/payload"
  sha256sum -- observer-action-worker observer-action-broker observer-action-setup > SHA256SUMS
)
echo 'ACTION_BINARIES_REPRODUCIBLE'
