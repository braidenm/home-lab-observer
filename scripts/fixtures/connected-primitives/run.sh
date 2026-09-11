#!/usr/bin/env bash
# Explicit manual root-owned disposable fixture. No accounts/global settings changed.
set -euo pipefail
fail() { printf '%s\n' 'CONNECTED_PRIMITIVE_RUN_FAILED' >&2; exit 1; }
[[ $# == 0 && $EUID == 0 && $(uname -m) == x86_64 ]] || fail
[[ $(systemd --version | head -n 1) == 'systemd 255 '* ]] || fail
[[ $(id -u nobody) != 0 && $(id -gn nobody) == nogroup ]] || fail
[[ -x /usr/bin/python3 && -r /usr/include/x86_64-linux-gnu/asm/unistd_64.h ]] || fail
grep -qE '^#define __NR_io_uring_setup 425$' /usr/include/x86_64-linux-gnu/asm/unistd_64.h || fail
source_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
probe_root=$(mktemp -d /tmp/observer-connected-primitives.XXXXXXXX) || fail
[[ $(realpath -e -- "$probe_root") == "$probe_root" && "$probe_root" == /tmp/observer-connected-primitives.* ]] || fail
unit="observer-connected-primitives-${BASHPID}-${RANDOM}"
owned_units=()
cleanup() {
  for owned_unit in "${owned_units[@]}"; do
    systemctl stop "$owned_unit.service" >/dev/null 2>&1 || true
  done
  [[ -d "$probe_root" && ! -L "$probe_root" && $(realpath -e -- "$probe_root") == "$probe_root" && "$probe_root" == /tmp/observer-connected-primitives.* ]] || return
  rm -r -- "$probe_root"
}
trap cleanup EXIT
for mode in baseline denied credential; do
  [[ $(systemctl show --property=LoadState --value "$unit-$mode.service") == not-found ]] || fail
done
chmod 0711 "$probe_root"
install -m 0644 "$source_dir/probe.py" "$probe_root/probe.py"
install -m 0600 /dev/null "$probe_root/synthetic"
mkdir -m 0755 "$probe_root/root"
filter='~@mount @module @raw-io @reboot @swap @ipc ptrace process_vm_readv process_vm_writev kcmp perf_event_open bpf userfaultfd socketpair io_uring_setup io_uring_enter io_uring_register mknod mknodat'
common=(--quiet --wait --pipe --collect -p User=nobody -p Group=nogroup -p NoNewPrivileges=yes -p PrivateIPC=yes -p RuntimeMaxSec=20s -p TimeoutStopSec=5s -p KillMode=control-group -p LimitCORE=0)
run_mode() {
  local mode=$1 expected=$2
  shift 2
  [[ $(systemctl show --property=LoadState --value "$unit-$mode.service") == not-found ]] || fail
  owned_units+=("$unit-$mode")
  if ! timeout --signal=TERM --kill-after=5s 30s systemd-run "${common[@]}" --unit="$unit-$mode" "$@" >"$probe_root/output" 2>&1; then fail; fi
  [[ $(cat -- "$probe_root/output") == "$expected" ]] || fail
  printf '%s\n' "$expected"
}
run_mode baseline CONNECTED_BASELINE_PRIMITIVE_OK /usr/bin/python3 "$probe_root/probe.py" baseline
run_mode denied CONNECTED_DENIED_PRIMITIVE_OK -p 'RestrictAddressFamilies=AF_INET AF_INET6' -p SystemCallArchitectures=native -p "SystemCallFilter=$filter" -p SystemCallErrorNumber=EPERM /usr/bin/python3 "$probe_root/probe.py" denied
# This credential-only primitive deliberately binds Python's public runtime;
# it is NOT the final uploader filesystem profile or host-confidentiality proof.
run_mode credential CONNECTED_CREDENTIAL_PRIMITIVE_OK -p "RootDirectory=$probe_root/root" -p "BindReadOnlyPaths=/usr /bin /lib /lib64 $probe_root/probe.py:/probe.py" -p "LoadCredential=connector.json:$probe_root/synthetic" /usr/bin/python3 /probe.py credential
