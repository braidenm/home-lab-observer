#!/usr/bin/env bash
# Unprivileged candidate archive; never installs or activates a service.
set -euo pipefail
[[ $# == 3 ]] || exit 2
version=$1
revision=$2
output=$3
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
bash "$root/scripts/build-action-binaries-linux.sh" "$version" "$revision" "$output"
output=$(realpath -- "$output")
name="home-lab-observer-actions_${version}_linux_amd64.tar.gz"
for round in first second; do
  "$output/connectedpack" --mode pack --profile actions --version "$version" --commit "$revision" \
    --binaries "$output/$round" --output "$output/$round-packed"
done
members=("$name" connected-manifest.json SHA256SUMS)
for member in "${members[@]}"; do
  cmp -- "$output/first-packed/$member" "$output/second-packed/$member"
done
mkdir -m 700 -- "$output/candidate"
for member in "${members[@]}"; do
  install -m 644 -- "$output/first-packed/$member" "$output/candidate/$member"
done
echo 'ACTION_CANDIDATE_REPRODUCIBLE_NOT_INSTALLABLE'
