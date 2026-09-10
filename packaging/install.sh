#!/usr/bin/env bash
set -euo pipefail

readonly REPOSITORY="braidenm/home-lab-observer"
readonly MANAGED_MARKER="home-lab-observer-managed-v1"
readonly MAX_ARCHIVE_BYTES=230686720

die() {
  printf 'Home Lab Observer installer: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Install or upgrade (online):
  install.sh --version VERSION [--install-root PATH]

Install or upgrade from a verified local archive:
  install.sh --version VERSION --archive PATH --checksum SHA256 [--install-root PATH]

Switch to an installed version:
  install.sh --rollback VERSION [--install-root PATH]

Remove managed program files (observation and token state is preserved):
  install.sh --uninstall [--install-root PATH]

Installation is per-user and does not start the observer, elevate privileges,
change PATH, register a service, enable Docker, or configure auto-updates.
EOF
}

valid_version() {
  value=$1
  [ "$(printf '%s' "$value" | wc -c | tr -d ' ')" -le 64 ] || return 1
  printf '%s\n' "$value" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$' || return 1
  prerelease=${value#*-}
  old_ifs=$IFS
  IFS=.
  for identifier in $prerelease; do
    case "$identifier" in
      0|*[!0-9]*) ;;
      0*) IFS=$old_ifs; return 1 ;;
    esac
  done
  IFS=$old_ifs
}

valid_checksum() {
  printf '%s\n' "$1" | grep -Eq '^[0-9A-Fa-f]{64}$'
}

default_install_root() {
  case "$(uname -s)" in
    Darwin) printf '%s\n' "${HOME:?HOME is required}/Applications/Home Lab Observer" ;;
    Linux) printf '%s\n' "${XDG_DATA_HOME:-${HOME:?HOME is required}/.local/share}/home-lab-observer" ;;
    *) die "this Bash installer supports Linux and macOS only" ;;
  esac
}

platform() {
  case "$(uname -s)" in
    Darwin) observer_os=darwin ;;
    Linux) observer_os=linux ;;
    *) die "unsupported operating system" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) observer_arch=amd64 ;;
    arm64|aarch64) observer_arch=arm64 ;;
    *) die "unsupported architecture" ;;
  esac
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "SHA-256 verification requires sha256sum or shasum"
  fi
}

file_size() {
  wc -c < "$1" | tr -d ' '
}

download() {
  command -v curl >/dev/null 2>&1 || die "online installation requires curl"
  if ! curl -q --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 15 --max-time 120 --max-filesize "$3" "$1" | head -c $(( $3 + 1 )) > "$2"; then
    [ "$(file_size "$2")" -gt "$3" ] && die "download exceeds its size limit"
    die "download failed"
  fi
  [ "$(file_size "$2")" -le "$3" ] || die "download exceeds its size limit"
}

resolve_install_root() {
  requested=$1
  case "$requested" in
    ''|/|.|..|/usr|/etc|/var|/tmp|/opt|/Applications) die "refusing broad install root: $requested" ;;
  esac
  case "$requested" in
    /*) candidate=$requested ;;
    *) candidate=$PWD/$requested ;;
  esac
  [ ! -L "$candidate" ] || die "install root must not be a symbolic link"
  mkdir -p "$candidate" || die "cannot create install root"
  install_root=$(CDPATH= cd -- "$candidate" && pwd -P)
  case "$install_root" in
    /|/usr|/etc|/var|/tmp|/opt|/Applications) die "refusing broad install root: $install_root" ;;
  esac
  if [ -n "${HOME:-}" ]; then
    home_root=$(CDPATH= cd -- "$HOME" 2>/dev/null && pwd -P || printf '%s' "$HOME")
    [ "$install_root" != "$home_root" ] || die "install root must not be the user home directory"
  fi
}

root_is_empty() {
  [ -z "$(find "$install_root" -mindepth 1 -maxdepth 1 -print -quit)" ]
}

require_managed_root() {
  [ -f "$install_root/.home-lab-observer-managed" ] && [ ! -L "$install_root/.home-lab-observer-managed" ] || die "refusing unmanaged install root"
  [ "$(cat "$install_root/.home-lab-observer-managed")" = "$MANAGED_MARKER" ] || die "managed-root marker is invalid"
}

write_atomic() {
  destination=$1
  value=$2
  temporary="$lock_dir/$(basename "$destination").next"
  printf '%s\n' "$value" > "$temporary"
  chmod 600 "$temporary"
  mv -f "$temporary" "$destination"
}

validate_version_directory() {
  directory=$1
  expected_version=$2
  [ -d "$directory" ] && [ ! -L "$directory" ] || return 1
  valid_version "$expected_version" || return 1
  for name in observer LICENSE START-HERE.md run-observer.sh; do
    [ -f "$directory/$name" ] && [ ! -L "$directory/$name" ] || return 1
  done
  [ -z "$(find "$directory" -mindepth 1 -maxdepth 1 ! -name observer ! -name LICENSE ! -name START-HERE.md ! -name run-observer.sh -print -quit)" ]
}

validate_background_area() {
  background="$install_root/background"
  [ -d "$background" ] && [ ! -L "$background" ] || die "managed background area is unsafe"
  active_background=false
  if [ -e "$background/.managed" ] || [ -L "$background/.managed" ]; then
    [ -f "$background/.managed" ] && [ ! -L "$background/.managed" ] || die "background marker is unsafe"
    [ "$(cat "$background/.managed")" = "home-lab-observer-background-v1" ] || die "background marker is invalid"
    active_background=true
  fi
  registration_count=0
  for path in "$background"/* "$background"/.[!.]* "$background"/..?*; do
    [ -e "$path" ] || [ -L "$path" ] || continue
    name=$(basename "$path")
    case "$name" in
      .managed|settings.json|.operation-lock)
        [ -f "$path" ] && [ ! -L "$path" ] || die "background area contains an unsafe $name entry"
        ;;
      home-lab-observer.service|com.braidenm.home-lab-observer.plist|task.xml)
        [ -f "$path" ] && [ ! -L "$path" ] || die "background area contains an unsafe registration"
        registration_count=$((registration_count + 1))
        ;;
      *) die "background area contains an unknown entry: $name" ;;
    esac
  done
  if [ "$active_background" = true ]; then
    [ -f "$background/settings.json" ] && [ ! -L "$background/settings.json" ] || die "managed background settings are missing"
    [ "$registration_count" -eq 1 ] || die "managed background registration is missing or ambiguous"
  else
    [ "$registration_count" -eq 0 ] && [ ! -e "$background/settings.json" ] || die "partial background registration requires background disable/recovery"
  fi
}

validate_managed_tree() {
  require_managed_root
  for path in "$install_root"/* "$install_root"/.[!.]* "$install_root"/..?*; do
    [ -e "$path" ] || [ -L "$path" ] || continue
    name=$(basename "$path")
    case "$name" in
      .home-lab-observer-managed|current|previous)
        [ -f "$path" ] && [ ! -L "$path" ] || die "managed root contains an unsafe $name entry"
        ;;
      bin|versions|background|.install-lock)
        [ -d "$path" ] && [ ! -L "$path" ] || die "managed root contains an unsafe $name entry"
        ;;
      *) die "managed root contains an unknown entry: $name" ;;
    esac
  done
  if [ -d "$install_root/background" ]; then
    validate_background_area
  fi
  if [ -d "$install_root/bin" ]; then
    [ -z "$(find "$install_root/bin" -mindepth 1 -maxdepth 1 ! -name observer -print -quit)" ] || die "managed launcher directory contains unknown files"
    if [ -e "$install_root/bin/observer" ] || [ -L "$install_root/bin/observer" ]; then
      [ -f "$install_root/bin/observer" ] && [ ! -L "$install_root/bin/observer" ] || die "managed launcher is unsafe"
    fi
  fi
  if [ -d "$install_root/versions" ]; then
    [ -z "$(find "$install_root/versions" -mindepth 1 -maxdepth 1 -name '.*' -print -quit)" ] || die "versions contains an unknown hidden entry"
    for directory in "$install_root/versions"/*; do
      [ -e "$directory" ] || [ -L "$directory" ] || continue
      version_name=$(basename "$directory")
      validate_version_directory "$directory" "$version_name" || die "managed version directory is unsafe: $version_name"
    done
  fi
  for pointer in current previous; do
    if [ -e "$install_root/$pointer" ] || [ -L "$install_root/$pointer" ]; then
      [ -f "$install_root/$pointer" ] && [ ! -L "$install_root/$pointer" ] || die "$pointer version pointer is unsafe"
      pointer_version=$(cat "$install_root/$pointer")
      valid_version "$pointer_version" || die "$pointer version pointer is invalid"
      validate_version_directory "$install_root/versions/$pointer_version" "$pointer_version" || die "$pointer version is not installed"
    fi
  done
}

acquire_lock() {
  candidate_lock="$install_root/.install-lock"
  mkdir "$candidate_lock" 2>/dev/null || die "another installer operation is active"
  lock_dir=$candidate_lock
}

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ -n "${incoming_dir:-}" ] && [ -d "$incoming_dir" ] && [ ! -L "$incoming_dir" ]; then
    rm -f "$incoming_dir/observer" "$incoming_dir/LICENSE" "$incoming_dir/START-HERE.md" "$incoming_dir/run-observer.sh" "$incoming_dir/Run-Observer.command"
    rmdir "$incoming_dir" 2>/dev/null || true
  fi
  if [ -n "${lock_dir:-}" ] && [ -d "$lock_dir" ] && [ ! -L "$lock_dir" ]; then
    rm -f "$lock_dir/current.next" "$lock_dir/previous.next" "$lock_dir/observer.next"
    rmdir "$lock_dir" 2>/dev/null || true
  fi
  if [ -n "${stage_dir:-}" ] && [ -d "$stage_dir" ] && [ ! -L "$stage_dir" ]; then
    rm -f "$stage_dir/archive" "$stage_dir/SHA256SUMS" "$stage_dir/plain.tar" "$stage_dir/list" "$stage_dir/types" "$stage_dir/version.stderr"
    if [ -d "$stage_dir/extract" ] && [ ! -L "$stage_dir/extract" ]; then
      find "$stage_dir/extract" -type f -exec rm -f {} \;
      find "$stage_dir/extract" -depth -type d -exec rmdir {} \; 2>/dev/null || true
    fi
    rmdir "$stage_dir" 2>/dev/null || true
  fi
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

validate_archive() {
  archive=$1
  root_name=$2
  if [ "$observer_os" = darwin ]; then
    archive_helper=Run-Observer.command
  else
    archive_helper=run-observer.sh
  fi
  [ "$(file_size "$archive")" -le "$MAX_ARCHIVE_BYTES" ] || die "archive exceeds the 220 MiB limit"
  if ! gzip -dc "$archive" | head -c $((MAX_ARCHIVE_BYTES + 1)) > "$stage_dir/plain.tar"; then
    [ "$(file_size "$stage_dir/plain.tar")" -gt "$MAX_ARCHIVE_BYTES" ] && die "expanded archive exceeds the 220 MiB limit"
    die "archive is not a readable gzip file"
  fi
  [ "$(file_size "$stage_dir/plain.tar")" -le "$MAX_ARCHIVE_BYTES" ] || die "expanded archive exceeds the 220 MiB limit"
  tar -tf "$stage_dir/plain.tar" > "$stage_dir/list" || die "archive is not a readable tar file"
  tar -tvf "$stage_dir/plain.tar" | awk '{print substr($1,1,1) " " $NF}' > "$stage_dir/types" || die "archive metadata cannot be read"
  [ "$(awk -v name="$root_name/" '$0 == name { count += 1 } END { print count + 0 }' "$stage_dir/list")" -eq 1 ] || die "archive must contain exactly one root directory"
  [ "$(wc -l < "$stage_dir/list" | tr -d ' ')" -eq 5 ] || die "archive must contain exactly one root directory and four files"
  expected_files="$root_name/observer $root_name/LICENSE $root_name/START-HERE.md $root_name/$archive_helper"
  count=0
  for expected in $expected_files; do
    matches=$(awk -v name="$expected" '$0 == name { count += 1 } END { print count + 0 }' "$stage_dir/list")
    [ "$matches" -eq 1 ] || die "archive must contain exactly one $expected"
    count=$((count + 1))
  done
  while IFS= read -r member; do
    case "$member" in
      "$root_name/") ;;
      "$root_name/observer"|"$root_name/LICENSE"|"$root_name/START-HERE.md"|"$root_name/$archive_helper") ;;
      *) die "archive contains an unexpected or unsafe member: $member" ;;
    esac
  done < "$stage_dir/list"
  while IFS=' ' read -r member_type member; do
    case "$member" in
      "$root_name/") [ "$member_type" = d ] || die "archive root must be a directory" ;;
      "$root_name/observer"|"$root_name/LICENSE"|"$root_name/START-HERE.md"|"$root_name/$archive_helper") [ "$member_type" = - ] || die "archive files must be regular files" ;;
      *) die "archive metadata contains an unexpected member" ;;
    esac
  done < "$stage_dir/types"
}

extract_archive() {
  root_name=$1
  mkdir "$stage_dir/extract"
  chmod 700 "$stage_dir/extract"
  tar -xf "$stage_dir/plain.tar" -C "$stage_dir/extract" || die "archive extraction failed"
  source_dir="$stage_dir/extract/$root_name"
  for name in observer LICENSE START-HERE.md "$archive_helper"; do
    [ -f "$source_dir/$name" ] && [ ! -L "$source_dir/$name" ] || die "extracted archive contains an unsafe file"
  done
  [ -z "$(find "$source_dir" -mindepth 1 -maxdepth 1 ! -name observer ! -name LICENSE ! -name START-HERE.md ! -name "$archive_helper" -print -quit)" ] || die "extracted archive contains unexpected files"
  [ "$(file_size "$source_dir/observer")" -gt 0 ] && [ "$(file_size "$source_dir/observer")" -le 209715200 ] || die "observer binary size is invalid"
  for name in LICENSE START-HERE.md "$archive_helper"; do
    [ "$(file_size "$source_dir/$name")" -le 1048576 ] || die "$name exceeds its size limit"
  done
}

verify_binary_identity() {
  binary=$1
  chmod 700 "$binary"
  : > "$stage_dir/version.stderr"
  if ! build_line=$("$binary" version 2> "$stage_dir/version.stderr"); then
    die "observer version check failed"
  fi
  [ ! -s "$stage_dir/version.stderr" ] || die "observer version check wrote unexpected diagnostics"
  prefix="Home Lab Observer $version ($observer_os/$observer_arch, commit "
  case "$build_line" in
    "$prefix"*")") commit=${build_line#"$prefix"}; commit=${commit%?} ;;
    *) die "observer binary identity does not match the requested version and platform" ;;
  esac
  printf '%s\n' "$commit" | grep -Eq '^[0-9a-f]{40}$' || die "observer binary commit identity is invalid"
}

install_launcher() {
  cat > "$lock_dir/observer.next" <<'EOF'
#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
VERSION=$(cat "$ROOT/current")
printf '%s\n' "$VERSION" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$' || {
  echo "Home Lab Observer launcher: current version pointer is invalid" >&2
  exit 1
}
BINARY="$ROOT/versions/$VERSION/observer"
[ -f "$BINARY" ] && [ ! -L "$BINARY" ] || {
  echo "Home Lab Observer launcher: selected version is unavailable" >&2
  exit 1
}
exec "$BINARY" "$@"
EOF
  chmod 700 "$lock_dir/observer.next"
  mkdir -p "$install_root/bin"
  chmod 700 "$install_root/bin"
  mv -f "$lock_dir/observer.next" "$install_root/bin/observer"
}

remove_managed_version() {
  directory=$1
  rm -f "$directory/observer" "$directory/LICENSE" "$directory/START-HERE.md" "$directory/run-observer.sh"
  rmdir "$directory"
}

action=install
version=
archive_source=
checksum=
install_root_arg=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || die "--version requires a value"; version=$2; shift 2 ;;
    --archive) [ "$#" -ge 2 ] || die "--archive requires a value"; archive_source=$2; shift 2 ;;
    --checksum) [ "$#" -ge 2 ] || die "--checksum requires a value"; checksum=$2; shift 2 ;;
    --install-root) [ "$#" -ge 2 ] || die "--install-root requires a value"; install_root_arg=$2; shift 2 ;;
    --rollback) [ "$#" -ge 2 ] || die "--rollback requires a value"; [ "$action" = install ] || die "choose one action"; action=rollback; version=$2; shift 2 ;;
    --uninstall) [ "$action" = install ] || die "choose one action"; action=uninstall; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

[ -n "$install_root_arg" ] || install_root_arg=$(default_install_root)

if [ "$action" = install ]; then
  [ -n "$version" ] && valid_version "$version" || die "--version must be an explicit SemVer prerelease without a leading v"
  if [ -n "$archive_source" ] || [ -n "$checksum" ]; then
    [ -n "$archive_source" ] && [ -n "$checksum" ] || die "offline installation requires both --archive and --checksum"
    valid_checksum "$checksum" || die "--checksum must be exactly 64 hexadecimal characters"
    [ -f "$archive_source" ] && [ ! -L "$archive_source" ] || die "offline archive must be a regular file, not a link"
    [ "$(file_size "$archive_source")" -le "$MAX_ARCHIVE_BYTES" ] || die "offline archive exceeds the 220 MiB limit"
  fi
  platform
  root_name="home-lab-observer_${version}_${observer_os}_${observer_arch}"
  archive_name="$root_name.tar.gz"
  umask 077
  stage_dir=$(mktemp -d "${TMPDIR:-/tmp}/home-lab-observer-install.XXXXXX") || die "cannot create private staging directory"
  chmod 700 "$stage_dir"
  if [ -n "$archive_source" ]; then
    cp "$archive_source" "$stage_dir/archive"
    expected_checksum=$checksum
  else
    base_url="https://github.com/$REPOSITORY/releases/download/v$version"
    download "$base_url/$archive_name" "$stage_dir/archive" "$MAX_ARCHIVE_BYTES"
    download "$base_url/SHA256SUMS" "$stage_dir/SHA256SUMS" 1048576
    checksum_count=$(awk -v name="$archive_name" '$2 == name || $2 == "*" name { count += 1; hash = $1 } END { print count + 0 }' "$stage_dir/SHA256SUMS")
    [ "$checksum_count" -eq 1 ] || die "SHA256SUMS does not contain exactly one entry for $archive_name"
    expected_checksum=$(awk -v name="$archive_name" '$2 == name || $2 == "*" name { print $1 }' "$stage_dir/SHA256SUMS")
    valid_checksum "$expected_checksum" || die "release checksum is invalid"
  fi
  actual_checksum=$(sha256_file "$stage_dir/archive")
  [ "$(printf '%s' "$actual_checksum" | tr 'A-F' 'a-f')" = "$(printf '%s' "$expected_checksum" | tr 'A-F' 'a-f')" ] || die "archive checksum mismatch"
  validate_archive "$stage_dir/archive" "$root_name"
  extract_archive "$root_name"
  verify_binary_identity "$source_dir/observer"
  resolve_install_root "$install_root_arg"
  if root_is_empty; then
    printf '%s\n' "$MANAGED_MARKER" > "$install_root/.home-lab-observer-managed"
    chmod 600 "$install_root/.home-lab-observer-managed"
  else
    require_managed_root
    validate_managed_tree
  fi
  acquire_lock
  # Background lifecycle operations acquire the same guard. Revalidate only
  # after owning it so enable/disable cannot race program-file mutation.
  validate_managed_tree
  mkdir -p "$install_root/versions"
  chmod 700 "$install_root" "$install_root/versions"
  target_dir="$install_root/versions/$version"
  if [ -e "$target_dir" ] || [ -L "$target_dir" ]; then
    validate_version_directory "$target_dir" "$version" || die "existing version directory is unsafe: $version"
    for name in observer LICENSE START-HERE.md; do
      [ "$(sha256_file "$source_dir/$name")" = "$(sha256_file "$target_dir/$name")" ] || die "existing unselected version does not match the verified archive"
    done
    [ "$(sha256_file "$source_dir/$archive_helper")" = "$(sha256_file "$target_dir/run-observer.sh")" ] || die "existing unselected version helper does not match the verified archive"
  else
    incoming_dir="$install_root/versions/.incoming-$version-$$"
    mkdir "$incoming_dir"
    chmod 700 "$incoming_dir"
    cp "$source_dir/observer" "$source_dir/LICENSE" "$source_dir/START-HERE.md" "$source_dir/$archive_helper" "$incoming_dir/"
    if [ "$archive_helper" != run-observer.sh ]; then
      mv "$incoming_dir/$archive_helper" "$incoming_dir/run-observer.sh"
    fi
    chmod 700 "$incoming_dir/observer" "$incoming_dir/run-observer.sh"
    chmod 600 "$incoming_dir/LICENSE" "$incoming_dir/START-HERE.md"
    mv "$incoming_dir" "$target_dir"
    incoming_dir=
  fi
  install_launcher
  if [ -f "$install_root/current" ]; then
    old_version=$(cat "$install_root/current")
    valid_version "$old_version" || die "current version pointer is invalid"
    if [ "$old_version" != "$version" ]; then
      write_atomic "$install_root/previous" "$old_version"
    fi
  fi
  write_atomic "$install_root/current" "$version"
  printf 'Installed Home Lab Observer %s. Nothing was started.\nRun: %s/bin/observer\n' "$version" "$install_root"
elif [ "$action" = rollback ]; then
  [ -z "$archive_source" ] && [ -z "$checksum" ] || die "rollback does not accept archive options"
  [ -n "$version" ] && valid_version "$version" || die "--rollback requires an installed SemVer prerelease"
  [ -d "$install_root_arg" ] && [ ! -L "$install_root_arg" ] || die "install root does not exist or is unsafe"
  resolve_install_root "$install_root_arg"
  validate_managed_tree
  validate_version_directory "$install_root/versions/$version" "$version" || die "rollback version is not installed: $version"
  acquire_lock
  validate_managed_tree
  validate_version_directory "$install_root/versions/$version" "$version" || die "rollback version is not installed: $version"
  old_version=$(cat "$install_root/current")
  [ "$old_version" != "$version" ] || die "version is already selected: $version"
  write_atomic "$install_root/previous" "$old_version"
  write_atomic "$install_root/current" "$version"
  printf 'Selected Home Lab Observer %s. Nothing was started.\n' "$version"
else
  [ -z "$version" ] && [ -z "$archive_source" ] && [ -z "$checksum" ] || die "uninstall does not accept version or archive options"
  [ -d "$install_root_arg" ] && [ ! -L "$install_root_arg" ] || die "install root does not exist or is unsafe"
  resolve_install_root "$install_root_arg"
  validate_managed_tree
  if [ -f "$install_root/background/.managed" ]; then
    die "background operation is enabled; run 'observer background disable' before uninstalling"
  fi
  acquire_lock
  validate_managed_tree
  if [ -f "$install_root/background/.managed" ]; then
    die "background operation is enabled; run 'observer background disable' before uninstalling"
  fi
  if [ -d "$install_root/bin" ]; then
    rm -f "$install_root/bin/observer"
    rmdir "$install_root/bin"
  fi
  if [ -d "$install_root/versions" ]; then
    for directory in "$install_root/versions"/*; do
      [ -d "$directory" ] || continue
      remove_managed_version "$directory"
    done
    rmdir "$install_root/versions"
  fi
  rm -f "$install_root/current" "$install_root/previous" "$install_root/.home-lab-observer-managed"
  if [ -d "$install_root/background" ]; then
    rm -f "$install_root/background/.operation-lock"
    rmdir "$install_root/background"
  fi
  rmdir "$lock_dir"
  lock_dir=
  rmdir "$install_root"
  printf 'Removed managed Home Lab Observer program files. Observation and token state were preserved.\n'
fi
