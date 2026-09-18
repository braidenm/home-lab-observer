#!/usr/bin/env bash
# Runs only in the no-NIC disposable Ubuntu fixture guest, never on an owner host.
set -euo pipefail

fixture_root=/var/lib/hlo-first-install-fixture
payload=/mnt/hlo-first-install
server=srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
alias_ip=93.184.216.34
collector=home-lab-observer-connected-collector.service
uploader=home-lab-observer-connected-uploader.service

emit() { printf '%s\n' "HLO_VM_$1" > /dev/ttyS0; }
fail() { emit "FAIL_$1"; exit 1; }

baseline() {
  [ "$(id -u)" -eq 0 ] || fail NOT_ROOT
  for tool in ip openssl update-ca-certificates python3 tar sha256sum systemctl getent findmnt pgrep; do
    command -v "$tool" >/dev/null || fail GUEST_TOOL
  done
  grep -qx 'ID=ubuntu' /etc/os-release || fail OS
  grep -qx 'VERSION_ID="24.04"' /etc/os-release || fail VERSION
  [ "$(systemctl --version | head -n 1 | cut -d' ' -f1-2)" = 'systemd 255' ] || fail SYSTEMD
  [ "$(find /sys/class/net -mindepth 1 -maxdepth 1 -printf '%f\n')" = lo ] || fail NIC
  ! ip route show default | grep -q . || fail ROUTE
  mountpoint -q "$payload" || fail PAYLOAD_MOUNT
  findmnt -no OPTIONS "$payload" | grep -qE '(^|,)ro(,|$)' || fail PAYLOAD_WRITABLE
}

unit_stopped() {
  local unit=$1 state
  state="$(systemctl show --no-pager --property=Id --property=LoadState --property=ActiveState --property=UnitFileState --property=FragmentPath --property=DropInPaths --property=NeedDaemonReload --property=MainPID -- "$unit")" || return 1
  grep -qx "Id=$unit" <<< "$state" &&
    grep -qx 'LoadState=loaded' <<< "$state" &&
    grep -qx 'ActiveState=inactive' <<< "$state" &&
    grep -qx 'UnitFileState=disabled' <<< "$state" &&
    grep -qx "FragmentPath=/etc/systemd/system/$unit" <<< "$state" &&
    grep -qx 'DropInPaths=' <<< "$state" &&
    grep -qx 'NeedDaemonReload=no' <<< "$state" &&
    grep -qx 'MainPID=0' <<< "$state"
}

no_worker() {
  ! pgrep -f 'observer-connected-(collector|uploader)' >/dev/null
}

installed_assertions() {
  [ -f /etc/home-lab-observer-connected/installed.json ] || fail CONFIG_MISSING
  [ -f /etc/home-lab-observer-connected/credentials/connector.json ] || fail CREDENTIAL_MISSING
  [ -d /var/lib/home-lab-observer-connected/ledger ] || fail LEDGER_MISSING
  [ "$(stat -c '%u:%g:%a' /etc/home-lab-observer-connected/credentials/connector.json)" = 0:0:600 ] || fail CREDENTIAL_MODE
  [ "$(stat -c '%u:%g:%a' /etc/home-lab-observer-connected/installed.json)" = 0:0:644 ] || fail CONFIG_MODE
  [ "$(getent passwd hlo-connected-collector | cut -d: -f7)" = /usr/sbin/nologin ] || fail COLLECTOR_LOGIN
  [ "$(getent passwd hlo-connected-uploader | cut -d: -f7)" = /usr/sbin/nologin ] || fail UPLOADER_LOGIN
  [ "$(id -u hlo-connected-collector)" != "$(id -u hlo-connected-uploader)" ] || fail SHARED_UID
  unit_stopped "$collector" || fail COLLECTOR_NOT_STOPPED
  unit_stopped "$uploader" || fail UPLOADER_NOT_STOPPED
  no_worker || fail WORKER_PROCESS
}

recovery_assertions() {
  [ -f /etc/home-lab-observer-connected/preparing.json ] || fail PREPARING_MISSING
  [ ! -e /etc/home-lab-observer-connected/installed.json ] || fail UNEXPECTED_CONFIG
  no_worker || fail WORKER_PROCESS
  for unit in "$collector" "$uploader"; do
    state="$(systemctl show --no-pager --property=ActiveState --property=UnitFileState -- "$unit")" || fail UNIT_QUERY
    grep -qx 'ActiveState=inactive' <<< "$state" || fail ACTIVE_AFTER_CUT
    ! grep -qx 'UnitFileState=enabled' <<< "$state" || fail ENABLED_AFTER_CUT
  done
}

prepare_guest() {
  mkdir -p "$fixture_root" /opt/observer-fixture/bundle
  chmod 0700 "$fixture_root"
  (cd "$payload" && sha256sum --check --strict SHA256SUMS >/dev/null) || fail PAYLOAD_CHECKSUM
  archive=("$payload"/*.tar.gz)
  [ "${#archive[@]}" -eq 1 ] && [ -f "${archive[0]}" ] || fail ARCHIVE_COUNT
  tar -xzf "${archive[0]}" -C /opt/observer-fixture/bundle || fail ARCHIVE_EXTRACT
  bundle=(/opt/observer-fixture/bundle/home-lab-observer-connected_*)
  [ "${#bundle[@]}" -eq 1 ] && [ -d "${bundle[0]}" ] || fail BUNDLE_COUNT
  printf '%s\n' "${bundle[0]}" > "$fixture_root/bundle-path"
  sha256sum "${bundle[0]}/connected-manifest.json" | cut -d' ' -f1 > "$fixture_root/manifest-sha"

  ip addr add "$alias_ip/32" dev lo || fail LOOPBACK_ALIAS
  printf '%s %s %s\n' "$alias_ip" app.braidenmiller.com app.braidenmiller.com. >> /etc/hosts
  openssl req -new -x509 -newkey rsa:2048 -nodes -days 1 \
    -subj '/CN=app.braidenmiller.com' \
    -addext 'subjectAltName=DNS:app.braidenmiller.com' \
    -addext 'basicConstraints=critical,CA:TRUE' \
    -keyout "$fixture_root/key.pem" -out "$fixture_root/cert.pem" >/dev/null 2>&1 || fail CERT
  cp "$fixture_root/cert.pem" /usr/local/share/ca-certificates/hlo-first-install-fixture.crt
  update-ca-certificates >/dev/null 2>&1 || fail TRUST
  "$payload/connected-first-install-receiver" "$alias_ip:443" "$fixture_root/cert.pem" "$fixture_root/key.pem" >/dev/null 2>&1 &
  receiver_pid=$!
  for _ in $(seq 1 50); do
    if (echo >/dev/tcp/$alias_ip/443) 2>/dev/null; then
      break
    fi
    sleep 0.1
  done
  kill -0 "$receiver_pid" 2>/dev/null || fail RECEIVER
}

install_reboot_assertion() {
  local mode=$1
  printf '%s\n' "$mode" > "$fixture_root/postboot-mode"
  printf 'LABEL=HLOFIX %s ext4 ro,nofail 0 0\n' "$payload" >> /etc/fstab
  cat > /etc/systemd/system/hlo-first-install-fixture-postboot.service <<EOF
[Unit]
Description=Disposable first-install fixture postboot assertions
RequiresMountsFor=$payload
ConditionPathExists=$fixture_root/postboot-mode
[Service]
Type=oneshot
ExecStart=/bin/bash $payload/guest.sh postboot
StandardOutput=null
StandardError=null
[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable hlo-first-install-fixture-postboot.service >/dev/null
  sync
}

case "${1:-}" in
  success|success-cut|interrupt)
    baseline
    prepare_guest
    bundle="$(cat "$fixture_root/bundle-path")"
    digest="$(cat "$fixture_root/manifest-sha")"
    if "$bundle/observer-connected-install" install "$bundle" "$(printf '%064d' 0)" "$server" </dev/null >/dev/null 2>&1; then
      fail BAD_DIGEST_ADMITTED
    fi
    [ ! -e /etc/home-lab-observer-connected ] || fail PREFLIGHT_MUTATED
    install_reboot_assertion "$1"
    if [ "$1" != interrupt ]; then
      python3 "$payload/drive_pty.py" install "$bundle" "$digest" || fail INSTALL
      installed_assertions
      sync
      if [ "$1" = success-cut ]; then
        emit SUCCESS_READY_FOR_CUT
        while true; do sleep 60; done
      fi
      emit SUCCESS_FIRST_BOOT
      systemctl reboot
    else
      python3 "$payload/drive_pty.py" interrupt "$bundle" "$digest" || fail INTERRUPT
      fail INTERRUPT_RETURNED
    fi
    ;;
  postboot)
    baseline
    mode="$(cat "$fixture_root/postboot-mode")"
    if [ "$mode" = success ] || [ "$mode" = success-cut ]; then
      installed_assertions
      emit SUCCESS_REBOOT_PASS
    elif [ "$mode" = interrupt ]; then
      recovery_assertions
      emit INTERRUPT_REBOOT_PASS
    else
      fail UNKNOWN_MODE
    fi
    systemctl poweroff
    ;;
  *) fail INVALID_MODE ;;
esac
