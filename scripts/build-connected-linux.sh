#!/usr/bin/env bash
# Ubuntu 24.04 amd64 build-only boundary. Candidate commands are never executed.
set -euo pipefail
if [[ $# != 3 ]]; then echo 'CONNECTED_BUILD_INPUT_FAILED' >&2; exit 2; fi
version=$1
commit=$2
[[ "$version" =~ ^0\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})-canary\.([1-9][0-9]{0,3})$ && ${#version} -le 40 && "$commit" =~ ^[a-f0-9]{40}$ ]] || { echo 'CONNECTED_BUILD_INPUT_FAILED' >&2; exit 1; }
[[ -n "$3" && "$3" != *$'\n'* && "$3" != *$'\r'* ]] || { echo 'CONNECTED_BUILD_INPUT_FAILED' >&2; exit 1; }
output=$(realpath -m -- "$3")
repo_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd -- "$repo_root"
export LANG=C LC_ALL=C TZ=UTC
umask 077
go_environment=(GOENV=off GOWORK=off GOFLAGS=-buildvcs=false GOEXPERIMENT= GOTOOLCHAIN=local GOAMD64=v1 CGO_ENABLED=0 GOOS=linux GOARCH=amd64)
fixed_go() { env "${go_environment[@]}" go "$@"; }
[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 && -r /etc/os-release ]] || { echo 'CONNECTED_BUILD_HOST_FAILED' >&2; exit 1; }
grep -Eq '^ID=("?ubuntu"?)$' /etc/os-release && grep -Eq '^VERSION_ID="?24\.04"?$' /etc/os-release || { echo 'CONNECTED_BUILD_HOST_FAILED' >&2; exit 1; }
[[ "$(fixed_go version)" == 'go version go1.27.3 linux/amd64' && "$(git rev-parse HEAD)" == "$commit" && -z "$(git status --porcelain --untracked-files=all)" ]] || { echo 'CONNECTED_BUILD_SOURCE_FAILED' >&2; exit 1; }
[[ ! -e "$output" && ! -L "$output" && -d "$(dirname -- "$output")" ]] || { echo 'CONNECTED_BUILD_OUTPUT_FAILED' >&2; exit 1; }
mkdir -m 700 -- "$output"
fixed_go build -trimpath -ldflags '-s -w -buildid=' -o "$output/connectedpack" ./cmd/connectedpack
for round in first second; do
  mkdir -m 700 -- "$output/$round" "$output/$round/binaries"
  for role in collector uploader install; do
    identity=$("$output/connectedpack" --mode identity --role "$role" --version "$version" --commit "$commit")
    fixed_go build -trimpath -ldflags "-s -w -buildid= -X main.releaseIdentity=$identity" \
      -o "$output/$round/binaries/observer-connected-$role" "./cmd/observer-connected-$role"
    chmod 755 -- "$output/$round/binaries/observer-connected-$role"
  done
  "$output/connectedpack" --mode pack --version "$version" --commit "$commit" \
    --binaries "$output/$round/binaries" --output "$output/$round/packed"
done
mkdir -m 700 -- "$output/payload"
for name in "home-lab-observer-connected_${version}_linux_amd64.tar.gz" connected-manifest.json install-connected-linux.sh SHA256SUMS; do
  cmp -- "$output/first/packed/$name" "$output/second/packed/$name" || { echo 'CONNECTED_BUILD_REPRODUCIBILITY_FAILED' >&2; exit 1; }
  install -m 644 -- "$output/first/packed/$name" "$output/payload/$name"
done
echo 'CONNECTED_BUILD_REPRODUCIBLE'
