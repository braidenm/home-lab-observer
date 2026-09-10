#!/usr/bin/env bash
# Ubuntu-only, opt-in reproducibility proof. It builds but never executes target outputs.
set -euo pipefail
if [[ $# != 0 ]]; then echo 'usage: OBSERVER_TEST_REPRODUCIBLE_BUILDS=1 test-build-native-v2.sh' >&2; exit 2; fi
if [[ "${OBSERVER_TEST_REPRODUCIBLE_BUILDS:-}" != 1 ]]; then
  echo 'native v2 reproducibility build skipped (explicit opt-in required)'
  exit 0
fi

repo_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd -- "$repo_root"
export LANG=C LC_ALL=C TZ=UTC
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
host_go() {
  env "${go_environment[@]}" CGO_ENABLED=0 GOOS="$host_os" GOARCH="$host_arch" go "$@"
}

version=0.1.0-preview.999
commit=0123456789abcdef0123456789abcdef01234567
temporary=$(mktemp -d "${TMPDIR:-/tmp}/observer-build-v2.XXXXXXXX")
first="$temporary/first"
second="$temporary/second"
cleanup() {
  chmod -R u+w -- "$temporary" 2>/dev/null || true
  rm -rf -- "$temporary"
}
trap cleanup EXIT

GOCACHE="$temporary/cache-first" bash scripts/build-native-v2.sh "$version" "$commit" "$first"
# Poison every normalized input with a valid but different ambient value. The
# resulting bytes must remain identical to the clean invocation.
env \
  GOOS=windows GOARCH=arm64 CGO_ENABLED=1 \
  GOENV="$temporary/missing-goenv" GOWORK="$temporary/missing.work" \
  GOFLAGS='-tags=synthetic_poison -buildvcs=true' GOEXPERIMENT=none GOTOOLCHAIN=auto \
  GOAMD64=v4 GOARM64=v9.5 GOCACHE="$temporary/cache-second" \
  bash scripts/build-native-v2.sh "$version" "$commit" "$second"

expected_files=(
  observer-journal-helper_linux_amd64
  observer-journal-helper_linux_arm64
  observer_darwin_amd64
  observer_darwin_arm64
  observer_linux_amd64
  observer_linux_arm64
  observer_windows_amd64.exe
  observer_windows_arm64.exe
)
expected_entries=$(printf 'f %s\n' "${expected_files[@]}" | sort)
for directory in "$first" "$second"; do
  actual_entries=$(find "$directory" -mindepth 1 -maxdepth 1 -printf '%y %f\n' | sort)
  if [[ "$actual_entries" != "$expected_entries" ]]; then
    echo 'native v2 build did not contain the exact eight regular files' >&2
    exit 1
  fi
done

for filename in "${expected_files[@]}"; do
  if ! cmp -s -- "$first/$filename" "$second/$filename"; then
    echo "native v2 build is not reproducible: $filename" >&2
    exit 1
  fi
done

for architecture in amd64 arm64; do
  helper="$first/observer-journal-helper_linux_$architecture"
  digest=$(sha256sum -- "$helper")
  digest=${digest%% *}
  expected_helper=$(host_go run ./cmd/releaseidentity --role journal-helper --version "$version" --commit "$commit" --os linux --arch "$architecture")
  actual_helper=$(host_go run ./cmd/releaseidentity --scan "$helper")
  if [[ "$actual_helper" != "$expected_helper" ]]; then
    echo "Linux $architecture helper identity mismatch" >&2
    exit 1
  fi
  expected_main=$(host_go run ./cmd/releaseidentity --role observer --version "$version" --commit "$commit" --os linux --arch "$architecture" --helper-sha256 "$digest")
  actual_main=$(host_go run ./cmd/releaseidentity --scan "$first/observer_linux_$architecture")
  if [[ "$actual_main" != "$expected_main" ]]; then
    echo "Linux $architecture main/helper identity mismatch" >&2
    exit 1
  fi
  build_info=$(host_go version -m "$first/observer_linux_$architecture")
  if ! grep -Eq $'build[[:space:]]+CGO_ENABLED=0' <<<"$build_info"; then
    echo "Linux $architecture main is not recorded as a static CGO-disabled build" >&2
    exit 1
  fi
done

for target in darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  platform=${target%/*}
  architecture=${target#*/}
  suffix=''
  if [[ "$platform" == windows ]]; then suffix='.exe'; fi
  expected=$(host_go run ./cmd/releaseidentity --role observer --version "$version" --commit "$commit" --os "$platform" --arch "$architecture")
  actual=$(host_go run ./cmd/releaseidentity --scan "$first/observer_${platform}_${architecture}${suffix}")
  if [[ "$actual" != "$expected" ]]; then
    echo "$platform $architecture main identity mismatch" >&2
    exit 1
  fi
done

for architecture in amd64 arm64; do
  dependencies=$(env "${go_environment[@]}" CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" go list -deps ./cmd/observer)
  for forbidden in \
    github.com/braidenm/home-lab-observer/internal/journalnative \
    github.com/braidenm/home-lab-observer/internal/journalreader \
    github.com/braidenm/home-lab-observer/internal/journalruntime \
    github.com/ebitengine/purego; do
    if grep -Fxq "$forbidden" <<<"$dependencies"; then
      echo "Linux $architecture main includes helper-only dependency $forbidden" >&2
      exit 1
    fi
  done
done

echo 'native v2 reproducibility and identity checks passed'
