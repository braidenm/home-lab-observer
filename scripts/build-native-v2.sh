#!/usr/bin/env bash
# Fixed Ubuntu/GNU release builder. Cross-built outputs are never executed here.
set -euo pipefail
if [[ $# != 3 ]]; then echo 'usage: build-native-v2.sh VERSION COMMIT NEW_OUTPUT_DIRECTORY' >&2; exit 2; fi
version=$1
commit=$2
if [[ -z "$3" || "$3" == *$'\n'* || "$3" == *$'\r'* ]]; then echo 'build output must be a clean path' >&2; exit 1; fi
output=$(realpath -m -- "$3")
repo_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd -- "$repo_root"
export LANG=C LC_ALL=C TZ=UTC
umask 077

# Release bytes cannot depend on a developer's persistent go env, workspace,
# architecture tuning, experiment selection, or VCS stamping defaults. The
# authorized workflow installs the exact go.mod toolchain before calling us.
go_environment=(
  GOENV=off
  GOWORK=off
  GOFLAGS=-buildvcs=false
  GOEXPERIMENT=
  GOTOOLCHAIN=local
  GOAMD64=v1
  GOARM64=v8.0
)
host_os=$(env "${go_environment[@]}" go env GOHOSTOS)
host_arch=$(env "${go_environment[@]}" go env GOHOSTARCH)
if [[ "$(uname -s)" != Linux || ! -r /etc/os-release ]] || ! grep -Eq '^ID=("?ubuntu"?)$' /etc/os-release; then
  echo 'build-native-v2 requires an Ubuntu host' >&2
  exit 1
fi
if [[ "$host_os" != linux || ("$host_arch" != amd64 && "$host_arch" != arm64) ]]; then
  echo 'build-native-v2 requires a supported native Ubuntu Go toolchain' >&2
  exit 1
fi
host_go() {
  env "${go_environment[@]}" CGO_ENABLED=0 GOOS="$host_os" GOARCH="$host_arch" go "$@"
}
target_go() {
  local target_os=$1 target_arch=$2
  shift 2
  env "${go_environment[@]}" CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go "$@"
}
# Validate release grammar through the same formatter used by runtime/packaging.
host_go run ./cmd/releaseidentity --role journal-helper --version "$version" --commit "$commit" --os linux --arch amd64 >/dev/null
if [[ -e "$output" || -L "$output" || -z "$output" ]]; then echo 'build output must be new' >&2; exit 1; fi
if [[ ! -d "$(dirname -- "$output")" ]]; then echo 'build output parent must already exist' >&2; exit 1; fi
mkdir -m 700 -- "$output"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  platform=${target%/*}
  architecture=${target#*/}
  digest_args=()
  if [[ "$platform" == linux ]]; then
    record=$(host_go run ./cmd/releaseidentity --role journal-helper --version "$version" --commit "$commit" --os "$platform" --arch "$architecture")
    helper="$output/observer-journal-helper_linux_$architecture"
    target_go "$platform" "$architecture" build -buildvcs=false -trimpath \
      -ldflags "-s -w -buildid= -X main.releaseIdentity=$record" -o "$helper" ./cmd/observer-journal-helper
    digest=$(sha256sum -- "$helper")
    digest=${digest%% *}
    digest_args=(--helper-sha256 "$digest")
  fi
  record=$(host_go run ./cmd/releaseidentity --role observer --version "$version" --commit "$commit" --os "$platform" --arch "$architecture" "${digest_args[@]}")
  suffix=''
  if [[ "$platform" == windows ]]; then suffix='.exe'; fi
  target_go "$platform" "$architecture" build -buildvcs=false -trimpath \
    -ldflags "-s -w -buildid= -X main.releaseIdentity=$record" \
    -o "$output/observer_${platform}_${architecture}${suffix}" ./cmd/observer
done
