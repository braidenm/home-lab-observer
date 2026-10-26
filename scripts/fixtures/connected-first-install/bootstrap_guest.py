#!/usr/bin/env python3
"""Manual no-NIC guest only: real owner download/hash/sudo/PTY install flow."""
import hashlib
import json
import os
from pathlib import Path
import pty
import pwd
import grp
import re
import select
import signal
import socket
import subprocess
import sys
import time

ROOT = Path("/var/lib/hlo-first-install-fixture")
PAYLOAD = Path("/mnt/hlo-first-install")
HOME = Path("/home/hlo-fixture-owner")
OWNER = "hlo-fixture-owner"
UID = 22001
SERVER = "srv_" + "a" * 32
GRANT = b"hle_" + b"A" * 43 + b"\n"
ENV = {"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL": "C"}
MODES = ("bootstrap-helper-corrupt", "bootstrap-corrupt", "bootstrap-downgrade", "bootstrap-loop", "bootstrap-valid")
FIELDS = {"helpers", "archives", "redirects", "enrollments", "http_accepts", "refused"}


def need(condition):
    if not condition:
        raise ValueError("fixture assertion")


def command(*argv):
    subprocess.run(argv, check=True, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                   stderr=subprocess.DEVNULL, timeout=15, env=ENV)


def counts():
    path = ROOT / "bootstrap-counts.json"
    if not path.exists():
        return dict.fromkeys(FIELDS, 0)
    need(path.stat().st_size <= 1024)
    data = json.loads(path.read_text())
    need(set(data) == FIELDS and all(type(n) is int and 0 <= n <= 128 for n in data.values()))
    return data


def receiver(mode):
    process = subprocess.Popen([str(PAYLOAD / "connected-first-install-receiver"), "93.184.216.34:443",
                                str(ROOT / "cert.pem"), str(ROOT / "key.pem"), mode],
                               stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env=ENV)
    try:
        handle = os.pidfd_open(process.pid, 0)
    except BaseException:
        # Acquisition failed before any poll/reap: this Popen-owned child
        # retains its PID. Join it before propagating; never leave a listener.
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
        raise
    try:
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            need(process.poll() is None)
            try:
                with socket.create_connection(("93.184.216.34", 443), timeout=0.1):
                    return process, handle
            except OSError:
                time.sleep(0.05)
        raise ValueError("fixture readiness")
    except BaseException:
        join(process, handle)
        raise


def join(process, handle):
    try:
        try:
            signal.pidfd_send_signal(handle, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            signal.pidfd_send_signal(handle, signal.SIGKILL)
            process.wait(timeout=5)
    finally:
        os.close(handle)
    # Joined listener closure, not an assumption based on a PID disappearing.
    for port in (443, 80):
        try:
            with socket.create_connection(("93.184.216.34", port), timeout=0.1):
                raise ValueError("fixture listener remains")
        except (ConnectionRefusedError, TimeoutError):
            pass


def group_members(group):
    members = []
    for path in Path("/proc").glob("[0-9]*/stat"):
        try:
            fields = path.read_text().rsplit(")", 1)[1].split()
        except (FileNotFoundError, ProcessLookupError):
            continue
        if int(fields[2]) == group:
            members.append(int(path.parent.name))
    return members


def owner_command(version, digest):
    need(re.fullmatch(r"0\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})-canary\.[1-9][0-9]{0,3}", version))
    need(re.fullmatch(r"[a-f0-9]{64}", digest))
    url = "https://github.com/braidenm/home-lab-observer/releases/download/v" + version + "/install-connected-linux.sh"
    return '''set -eu
umask 077
stage="$(mktemp -d /home/hlo-fixture-owner/observer-setup.XXXXXXXX)"
trap 'rm -f -- "$stage/install-connected-linux.sh"; rmdir -- "$stage"' EXIT
[ "$(stat -c '%u:%g:%a' "$stage")" = 22001:22001:700 ]
/usr/bin/curl -q --fail --remove-on-error --show-error --silent --location --proto '=https' --proto-redir '=https' --max-redirs 5 --connect-timeout 10 --max-time 120 --max-filesize 65536 ''' + "'" + url + "'" + ''' --output "$stage/install-connected-linux.sh"
[ "$(stat -c '%u:%g:%a' "$stage/install-connected-linux.sh")" = 22001:22001:600 ]
printf '%s  %s\\n' ''' + "'" + digest + "'" + ''' "$stage/install-connected-linux.sh" | sha256sum --check --strict --status
identity="$(stat -c '%d:%i:%u:%g:%a:%s' "$stage/install-connected-linux.sh")"
printf '%s  %s\\n' ''' + "'" + digest + "'" + ''' "$stage/install-connected-linux.sh" | sha256sum --check --strict --status
sudo -- /bin/bash "$stage/install-connected-linux.sh" ''' + SERVER + '''
[ "$(stat -c '%d:%i:%u:%g:%a:%s' "$stage/install-connected-linux.sh")" = "$identity" ]
printf '%s  %s\\n' ''' + "'" + digest + "'" + ''' "$stage/install-connected-linux.sh" | sha256sum --check --strict --status
'''


def owner_journey(version, digest, mode):
    success = mode == "bootstrap-valid"
    script = owner_command(version, digest)
    child, terminal = pty.fork()
    if child == 0:
        os.setgroups([])
        os.setgid(UID)
        os.setuid(UID)
        os.chdir(HOME)
        os.execve("/bin/bash", ["/bin/bash", "-c", script], ENV | {"HOME": str(HOME), "USER": OWNER, "LOGNAME": OWNER})
    output = bytearray()
    prompted, joined = False, False
    try:
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            ready, _, _ = select.select([terminal], [], [], 0.02)
            if ready:
                try:
                    data = os.read(terminal, 4096)
                except OSError:
                    data = b""
                need(len(output) + len(data) <= 16384)
                output.extend(data)
                if not prompted and b"One-use enrollment grant" in output:
                    need(success)
                    prompted = True
                    os.write(terminal, GRANT)
            finished, status = os.waitpid(child, os.WNOHANG)
            if finished:
                joined = True
                need(not group_members(child))
                code = os.waitstatus_to_exitcode(status)
                expected = 0 if success else (1 if mode == "bootstrap-helper-corrupt" else 22)
                if code != expected:
                    phase = "HELPER" if b"CONNECTED_BOOTSTRAP_REFUSED:" in output else "PREFLIGHT" if b"PREFLIGHT_REFUSED:" in output else "UNKNOWN"
                    print("HLO_VM_BOOTSTRAP_DRIVER_FAILURE PHASE=" + phase + " EXIT=" + str(code if 0 <= code <= 255 else 255) + " PROMPT=" + str(int(prompted)), flush=True)
                need(code == expected)
                need((prompted and b"INSTALLED_PENDING_ACCEPTANCE" in output and b"Installed and stopped" in output) if success else (not prompted and b"Installed and stopped" not in output))
                return
        raise ValueError("fixture terminal deadline")
    finally:
        if not joined:
            # Only this pty.fork-owned isolated foreground process group.
            try:
                os.killpg(child, signal.SIGKILL)
            except ProcessLookupError:
                pass
            os.waitpid(child, 0)
        os.close(terminal)


def no_residue(success):
    need(not list(Path("/var/lib").glob("home-lab-observer-download.*")))
    need(not list(HOME.glob("observer-setup.*")))
    for process in Path("/proc").glob("[0-9]*"):
        try:
            args = (process / "cmdline").read_bytes()
        except (FileNotFoundError, ProcessLookupError):
            continue
        need(not any(name in args for name in (b"observer-connected-install", b"observer-connected-collector", b"observer-connected-uploader")))
        need(b"home-lab-observer-download." not in args and b"observer-setup." not in args)
        fields = args.split(b"\0")
        need(not (b"-I" in fields and b"-" in fields and SERVER.encode() in fields))
        need(not (fields and fields[0] == b"/usr/bin/curl" and b"github.com/braidenm/home-lab-observer/releases/download/" in args))
    if not success:
        for path in ("/etc/home-lab-observer-connected", "/var/lib/home-lab-observer-connected", "/opt/home-lab-observer-connected"):
            need(not os.path.lexists(path))
        for account in ("hlo-connected-collector", "hlo-connected-uploader"):
            try:
                pwd.getpwnam(account)
            except KeyError:
                continue
            raise ValueError("fixture principal residue")
        for group in ("hlo-connected-read", "hlo-connected-uploader"):
            try:
                grp.getgrnam(group)
            except KeyError:
                continue
            raise ValueError("fixture group residue")
        for unit in ("collector", "uploader", "enrollment"):
            name = "home-lab-observer-connected-" + unit + ".service"
            need(not os.path.lexists("/etc/systemd/system/" + name))
            need(not os.path.lexists("/etc/systemd/system/" + name + ".d"))
            state = subprocess.check_output(["/usr/bin/systemctl", "show", "--property=LoadState", "--property=ActiveState", "--", name], timeout=5, env=ENV)
            need(set(state.decode().splitlines()) == {"LoadState=not-found", "ActiveState=inactive"})


def main():
    phase = "ADMISSION"
    try:
        need(len(sys.argv) == 1 and os.geteuid() == 0)
        need({path.name for path in Path("/sys/class/net").iterdir()} == {"lo"})
        mount = subprocess.check_output(["/usr/bin/findmnt", "-no", "OPTIONS", str(PAYLOAD)], timeout=5, env=ENV)
        need("ro" in mount.decode().strip().split(","))
        signal.signal(signal.SIGCHLD, signal.SIG_DFL)
        handle = os.pidfd_open(os.getpid(), 0)
        try:
            signal.pidfd_send_signal(handle, 0)
        finally:
            os.close(handle)
        identity = json.loads((PAYLOAD / "reviewed-identity.json").read_text())
        digest = identity["helper_sha256"]
        need(re.fullmatch(r"[a-f0-9]{64}", digest))
        need(hashlib.sha256((PAYLOAD / "install-connected-linux.sh").read_bytes()).hexdigest() == digest)
        manifest = json.loads((PAYLOAD / "connected-manifest.json").read_text())
        phase = "OWNER_SETUP"
        command("/usr/sbin/groupadd", "--gid", str(UID), OWNER)
        command("/usr/sbin/useradd", "--uid", str(UID), "--gid", str(UID), "--create-home", "--home-dir", str(HOME), "--shell", "/bin/bash", OWNER)
        os.chmod(HOME, 0o700)
        rule = Path("/etc/sudoers.d/hlo-fixture-owner")
        with rule.open("x") as output:
            output.write(OWNER + " ALL=(root) NOPASSWD: /bin/bash /home/hlo-fixture-owner/observer-setup.????????/install-connected-linux.sh " + SERVER + "\n")
        rule.chmod(0o440)
        command("/usr/sbin/visudo", "--check", "--file", str(rule))
        for mode in MODES:
            phase = mode.upper().replace("-", "_") + "_RECEIVER"
            before = counts()
            process, handle = receiver(mode)
            try:
                if mode == MODES[0]:
                    phase = "PREFLIGHT"
                    command("/usr/bin/python3", str(PAYLOAD / "preflight_probe.py"))
                    command(str(PAYLOAD / "connected-first-install-preflight-probe"), "-test.run=^TestVMReadonlyPreflightStages$", "-hlo-vm-readonly-probe=true")
                phase = mode.upper().replace("-", "_") + "_OWNER"
                owner_journey(manifest["version"], digest, mode)
                phase = mode.upper().replace("-", "_") + "_COUNTERS"
                after = counts()
                delta = {key: after[key] - before[key] for key in FIELDS}
                expected = dict.fromkeys(FIELDS, 0)
                expected["helpers"] = 1
                if mode in ("bootstrap-corrupt", "bootstrap-valid"):
                    expected["archives"] = 1
                    expected["redirects"] = 1
                if mode == "bootstrap-downgrade":
                    expected["redirects"] = 1
                if mode == "bootstrap-loop":
                    expected["redirects"] = 6
                if mode == "bootstrap-valid":
                    expected["enrollments"] = 1
                if delta != expected:
                    print("HLO_VM_BOOTSTRAP_COUNTER_DELTA " + " ".join(key.upper() + "=" + str(delta[key]) for key in sorted(FIELDS)), flush=True)
                need(delta == expected)
            finally:
                join(process, handle)
            phase = mode.upper().replace("-", "_") + "_RESIDUE"
            no_residue(mode == "bootstrap-valid")
            print("HLO_VM_" + mode.upper().replace("-", "_") + "_PASS", flush=True)
        return 0
    except (OSError, ValueError, KeyError, AttributeError, subprocess.SubprocessError):
        print("HLO_VM_FAIL_BOOTSTRAP_" + phase, flush=True)
        return 1


if __name__ == "__main__":
    sys.exit(main())
