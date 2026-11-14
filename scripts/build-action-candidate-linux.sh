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
members=(SHA256SUMS observer-action-broker observer-action-setup observer-action-worker)
pack() {
  tar --format=ustar --mtime=@0 --owner=0 --group=0 --numeric-owner \
    -C "$output/payload" -cf - -- "${members[@]}" | gzip -n > "$1"
}
pack "$output/first/$name"
pack "$output/second/$name"
cmp -- "$output/first/$name" "$output/second/$name"
mkdir -m 700 -- "$output/candidate"
install -m 644 -- "$output/first/$name" "$output/candidate/$name"
(
  cd -- "$output/candidate"
  sha256sum -- "$name" > SHA256SUMS
)
echo 'ACTION_CANDIDATE_REPRODUCIBLE_NOT_INSTALLABLE'
