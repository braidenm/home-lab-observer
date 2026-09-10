#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
installer="$repo_root/packaging/install.sh"
case "$(uname -s)" in
  Linux) observer_os=linux; archive_helper=run-observer.sh ;;
  Darwin) observer_os=darwin; archive_helper=Run-Observer.command ;;
  *) echo "SKIP: Bash installer tests require Linux or macOS"; exit 0 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) observer_arch=amd64 ;;
  arm64|aarch64) observer_arch=arm64 ;;
  *) echo "SKIP: unsupported test architecture"; exit 0 ;;
esac

temporary=$(mktemp -d "${TMPDIR:-/tmp}/observer installer test.XXXXXX")
cleanup() {
  trap - EXIT
  rm -rf -- "$temporary"
}
trap cleanup EXIT

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

make_archive() {
  version=$1
  identity_version=${2:-$version}
  unsafe=${3:-safe}
  root="home-lab-observer_${version}_${observer_os}_${observer_arch}"
  build="$temporary/build-$version-$identity_version-$unsafe"
  mkdir -p "$build/$root"
  cat > "$build/$root/observer" <<EOF
#!/bin/sh
if [ "\${1:-}" = version ]; then
  if [ "\${2:-}" = --release-schema ] && { [ "$unsafe" = v2 ] || [ "$unsafe" = v2-missing ]; }; then
    printf '%s\n' observer-release/v2
    exit 0
  fi
  if [ "\${2:-}" = --release-manifest ] && [ "$unsafe" = v2 ]; then
    printf '%s\n' RELEASE_MANIFEST_VERIFIED
    exit 0
  fi
  printf '%s\n' 'Home Lab Observer $identity_version ($observer_os/$observer_arch, commit 0123456789abcdef0123456789abcdef01234567)'
  exit 0
fi
printf '%s\n' '$identity_version:'"\$*"
EOF
  chmod 700 "$build/$root/observer"
  cp "$repo_root/packaging/resources/LICENSE" "$build/$root/LICENSE"
  cp "$repo_root/packaging/resources/START-HERE.md" "$build/$root/START-HERE.md"
  cp "$repo_root/packaging/resources/$archive_helper" "$build/$root/$archive_helper"
  chmod 700 "$build/$root/$archive_helper"
  if [ "$unsafe" = link ]; then
    rm "$build/$root/START-HERE.md"
    ln -s LICENSE "$build/$root/START-HERE.md"
  elif [ "$unsafe" = extra ]; then
    printf 'unexpected\n' > "$build/$root/secret.txt"
  elif [ "$unsafe" = v2 ]; then
    printf 'synthetic-never-executed-helper\n' > "$build/$root/observer-journal-helper"
  fi
  archive="$temporary/$root-$identity_version-$unsafe.tar.gz"
  tar -czf "$archive" -C "$build" "$root"
  printf '%s\n' "$archive"
}

expect_failure() {
  if "$@" > "$temporary/failure.out" 2> "$temporary/failure.err"; then
    echo "expected failure: $*" >&2
    exit 1
  fi
}

install_root="$temporary/install root"
state_root="$temporary/state kept outside programs"
mkdir "$state_root"
printf 'keep-token-and-history\n' > "$state_root/sentinel"

v1=0.1.0-preview.1
archive1=$(make_archive "$v1")
checksum1=$(sha256_file "$archive1")
bash "$installer" --version "$v1" --archive "$archive1" --checksum "$checksum1" --install-root "$install_root"
test "$(cat "$install_root/current")" = "$v1"
"$install_root/bin/observer" version | grep -F "Home Lab Observer $v1 ($observer_os/$observer_arch, commit " >/dev/null

expect_failure bash "$installer" --version 0.1.0 --archive "$archive1" --checksum "$checksum1" --install-root "$temporary/unsafe version"
expect_failure bash "$installer" --version 01.1.0-preview.1 --archive "$archive1" --checksum "$checksum1" --install-root "$temporary/unsafe numeric version"
expect_failure bash "$installer" --version 0.1.0-preview.01 --archive "$archive1" --checksum "$checksum1" --install-root "$temporary/unsafe prerelease"
expect_failure bash "$installer" --version "$v1" --archive "$archive1" --checksum 0000000000000000000000000000000000000000000000000000000000000000 --install-root "$temporary/bad checksum"
test ! -e "$temporary/bad checksum"

unsafe_archive=$(make_archive "$v1" "$v1" link)
expect_failure bash "$installer" --version "$v1" --archive "$unsafe_archive" --checksum "$(sha256_file "$unsafe_archive")" --install-root "$temporary/unsafe archive"
extra_archive=$(make_archive "$v1" "$v1" extra)
expect_failure bash "$installer" --version "$v1" --archive "$extra_archive" --checksum "$(sha256_file "$extra_archive")" --install-root "$temporary/extra archive"

v2=0.1.0-preview.2
archive2=$(make_archive "$v2")
checksum2=$(sha256_file "$archive2")
bash "$installer" --version "$v2" --archive "$archive2" --checksum "$checksum2" --install-root "$install_root"
test "$(cat "$install_root/current")" = "$v2"
test "$(cat "$install_root/previous")" = "$v1"
"$install_root/bin/observer" version | grep -F "Home Lab Observer $v2" >/dev/null

v3=0.1.0-preview.3
mismatch_archive=$(make_archive "$v3" "$v2")
expect_failure bash "$installer" --version "$v3" --archive "$mismatch_archive" --checksum "$(sha256_file "$mismatch_archive")" --install-root "$install_root"
test "$(cat "$install_root/current")" = "$v2"
test ! -e "$install_root/versions/$v3"

mkdir "$install_root/.install-lock"
printf 'owned-by-other-process\n' > "$install_root/.install-lock/sentinel"
archive3=$(make_archive "$v3")
expect_failure bash "$installer" --version "$v3" --archive "$archive3" --checksum "$(sha256_file "$archive3")" --install-root "$install_root"
test -f "$install_root/.install-lock/sentinel"
rm "$install_root/.install-lock/sentinel"
rmdir "$install_root/.install-lock"

recovered="$install_root/versions/$v3"
mkdir "$recovered"
cp "$temporary/build-$v3-$v3-safe/home-lab-observer_${v3}_${observer_os}_${observer_arch}/observer" "$recovered/observer"
cp "$repo_root/packaging/resources/LICENSE" "$recovered/LICENSE"
cp "$repo_root/packaging/resources/START-HERE.md" "$recovered/START-HERE.md"
cp "$repo_root/packaging/resources/$archive_helper" "$recovered/run-observer.sh"
bash "$installer" --version "$v3" --archive "$archive3" --checksum "$(sha256_file "$archive3")" --install-root "$install_root"
test "$(cat "$install_root/current")" = "$v3"

bash "$installer" --rollback "$v1" --install-root "$install_root"
test "$(cat "$install_root/current")" = "$v1"
test "$(cat "$install_root/previous")" = "$v3"
"$install_root/bin/observer" version | grep -F "Home Lab Observer $v1" >/dev/null

mkdir "$install_root/background"
printf '%s\n' home-lab-observer-background-v1 > "$install_root/background/.managed"
printf '%s\n' '{}' > "$install_root/background/settings.json"
printf '%s\n' '[Service]' > "$install_root/background/home-lab-observer.service"
: > "$install_root/background/.operation-lock"
test "$(bash "$installer" --rollback "$v3" --install-root "$install_root" >/dev/null; cat "$install_root/current")" = "$v3"
test "$(bash "$installer" --rollback "$v1" --install-root "$install_root" >/dev/null; cat "$install_root/current")" = "$v1"
test -f "$install_root/background/.managed"
expect_failure bash "$installer" --uninstall --install-root "$install_root"
test -f "$install_root/current"
rm "$install_root/background/.managed" "$install_root/background/settings.json" "$install_root/background/home-lab-observer.service"
mkdir "$install_root/.install-lock"
expect_failure bash "$installer" --uninstall --install-root "$install_root"
test -f "$install_root/current"
test -f "$install_root/background/.operation-lock"
rmdir "$install_root/.install-lock"
printf 'owner-data\n' > "$install_root/background/unknown"
expect_failure bash "$installer" --uninstall --install-root "$install_root"
test -f "$install_root/background/unknown"
rm "$install_root/background/unknown"

printf 'unknown\n' > "$install_root/versions/.unknown"
expect_failure bash "$installer" --uninstall --install-root "$install_root"
test -f "$install_root/versions/.unknown"
rm "$install_root/versions/.unknown"

unmanaged="$temporary/unmanaged root"
mkdir "$unmanaged"
printf 'do-not-delete\n' > "$unmanaged/user-file"
expect_failure bash "$installer" --uninstall --install-root "$unmanaged"
test -f "$unmanaged/user-file"

partial="$temporary/partial managed root"
mkdir "$partial"
printf '%s\n' home-lab-observer-managed-v1 > "$partial/.home-lab-observer-managed"
bash "$installer" --uninstall --install-root "$partial"
test ! -e "$partial"
expect_failure bash "$installer" --rollback "$v1" --archive "$archive1" --checksum "$checksum1" --install-root "$install_root"

bash "$installer" --uninstall --install-root "$install_root"
test ! -e "$install_root"
test "$(cat "$state_root/sentinel")" = keep-token-and-history
# These synthetic fixtures exercise installer dispatch/staging only. The Go
# tests separately verify real embedded records and manifest content.
if [ "$observer_os" = linux ]; then
  pair_root="$temporary/pair installation"
  pair_archive=$(make_archive 0.1.0-preview.4 0.1.0-preview.4 v2)
  pair_hash=$(sha256_file "$pair_archive")
  printf '%s\n' '{"synthetic":"manifest dispatch fixture"}' > "$temporary/release-manifest.json"
  printf '%s  release-manifest.json\n' "$(sha256_file "$temporary/release-manifest.json")" > "$temporary/pair-checksums"
  expect_failure bash "$installer" --version 0.1.0-preview.4 --archive "$pair_archive" --checksum "$pair_hash" --install-root "$pair_root"
  test ! -e "$pair_root"
  missing_archive=$(make_archive 0.1.0-preview.5 0.1.0-preview.5 v2-missing)
  expect_failure bash "$installer" --version 0.1.0-preview.5 --archive "$missing_archive" --checksum "$(sha256_file "$missing_archive")" --install-root "$pair_root"
  test ! -e "$pair_root"
  bash "$installer" --version "$v1" --archive "$archive1" --checksum "$checksum1" --install-root "$pair_root"
  mkdir "$temporary/interrupted-copy-bin"
  real_cp=$(command -v cp)
  cat > "$temporary/interrupted-copy-bin/cp" <<EOF
#!/bin/sh
for destination do :; done
case "\$destination" in */.incoming-*/observer-journal-helper) exit 1 ;; esac
exec "$real_cp" "\$@"
EOF
  chmod 700 "$temporary/interrupted-copy-bin/cp"
  expect_failure env PATH="$temporary/interrupted-copy-bin:$PATH" bash "$installer" --version 0.1.0-preview.4 --archive "$pair_archive" --checksum "$pair_hash" --manifest "$temporary/release-manifest.json" --checksums "$temporary/pair-checksums" --install-root "$pair_root"
  test "$(cat "$pair_root/current")" = "$v1"
  test ! -e "$pair_root/versions/0.1.0-preview.4"
  test -z "$(find "$pair_root/versions" -mindepth 1 -maxdepth 1 -name '.incoming-*' -print -quit)"
  bash "$installer" --version 0.1.0-preview.4 --archive "$pair_archive" --checksum "$pair_hash" --manifest "$temporary/release-manifest.json" --checksums "$temporary/pair-checksums" --install-root "$pair_root"
  test -f "$pair_root/versions/0.1.0-preview.4/observer-journal-helper"
  test "$(cat "$pair_root/current")" = 0.1.0-preview.4
  bash "$installer" --rollback "$v1" --install-root "$pair_root"
  test "$(cat "$pair_root/current")" = "$v1"
  expect_failure bash "$installer" --version "$v2" --archive "$archive2" --checksum "$checksum2" --manifest "$temporary/release-manifest.json" --checksums "$temporary/pair-checksums" --install-root "$pair_root"
  test "$(cat "$pair_root/current")" = "$v1"
  printf '%s  release-manifest.json\n' "$(printf '%064d' 0)" > "$temporary/bad-manifest-checksums"
  expect_failure bash "$installer" --version 0.1.0-preview.4 --archive "$pair_archive" --checksum "$pair_hash" --manifest "$temporary/release-manifest.json" --checksums "$temporary/bad-manifest-checksums" --install-root "$pair_root"
  bash "$installer" --uninstall --install-root "$pair_root"
  test ! -e "$pair_root"
fi
printf 'Bash installer lifecycle and safety tests passed.\n'
