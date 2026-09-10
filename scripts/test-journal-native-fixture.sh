#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
remote=${OBSERVER_TEST_JOURNAL_REMOTE:-/usr/lib/systemd/systemd-journal-remote}
case "$remote" in /*) ;; *) echo "journal fixture writer path must be absolute" >&2; exit 1 ;; esac
if [ ! -f "$remote" ] || [ ! -x "$remote" ] || [ -L "$remote" ]; then
  echo "fixed systemd-journal-remote path is unavailable" >&2
  exit 1
fi

temporary=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/observer-journal-fixture.XXXXXX")
cleanup() {
  status=$?
  trap - EXIT
  rm -rf -- "$temporary"
  exit "$status"
}
trap cleanup EXIT
fixture="$temporary/journal"
mkdir -m 700 "$fixture"
printf '%s\n' 'observer-owned-synthetic-journal-v1' > "$fixture/.observer-owned-fixture"
export_file="$temporary/fixture.export"

query_seconds=$(( $(date -u +%s) + 60 ))
query_micros=$(( query_seconds * 1000000 ))
oversized_priority=$(printf '%4097s' '' | tr ' ' x)
write_entry() {
  at=$1
  priority=$2
  message_id=$3
  message=$4
  printf '__REALTIME_TIMESTAMP=%s\n' "$at"
  printf '__MONOTONIC_TIMESTAMP=%s\n' "$at"
  printf '_BOOT_ID=11111111111111111111111111111111\n'
  printf '_MACHINE_ID=22222222222222222222222222222222\n'
  printf '_HOSTNAME=owned-synthetic-fixture\n'
  printf '_TRANSPORT=journal\n'
  printf 'PRIORITY=%s\n' "$priority"
  if [ -n "$message_id" ]; then printf 'MESSAGE_ID=%s\n' "$message_id"; fi
  printf 'MESSAGE=%s\n\n' "$message"
}
{
  write_entry $(( query_micros - 3000000 )) 4 '' 'PRIVATE_FIXTURE_BODY_MUST_NOT_ESCAPE'
  write_entry $(( query_micros - 2000000 )) "$oversized_priority" '' 'PRIVATE_FIXTURE_BODY_MUST_NOT_ESCAPE'
  write_entry $(( query_micros - 1000000 )) 6 '0123456789abcdef0123456789abcdef' 'PRIVATE_FIXTURE_BODY_MUST_NOT_ESCAPE'
} > "$export_file"
test "$(stat -c %s "$export_file")" -le 65536

"$remote" --output="$fixture/synthetic.journal" --split-mode=none --compress=no "$export_file" >/dev/null
test -f "$fixture/synthetic.journal"
test "$(find "$fixture" -maxdepth 1 -type f -name '*.journal' | wc -l)" -eq 1
test "$(stat -c %s "$fixture/synthetic.journal")" -le 16777216

cd "$repo_root"
OBSERVER_TEST_LIBSYSTEMD_LOAD=1 \
OBSERVER_TEST_SYNTHETIC_JOURNAL_DIR="$fixture" \
OBSERVER_TEST_SYNTHETIC_QUERY_MICROS="$query_micros" \
CGO_ENABLED=0 go test ./internal/journalnative -run 'Test(FixedLibrarySymbolSetLoadsWhenRequested|FixedLibraryMissingMapsToUnavailable|SyntheticJournalDirectory)$' -count=1
