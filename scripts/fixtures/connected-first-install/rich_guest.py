#!/usr/bin/env python3
"""Fixed synthetic process proof; runs only inside the admitted no-NIC guest."""
import ctypes
import grp
import hashlib
import json
import os
from pathlib import Path
import pwd
import signal
import sqlite3
import subprocess
import sys
import tarfile
import time

import activation_guest
import assert_guest
import preflight_probe

ROOT = Path("/var/lib/hlo-first-install-fixture")
PAYLOAD = Path("/mnt/hlo-first-install")
CONFIG = Path("/etc/home-lab-observer-connected-rich")
STATE = Path("/var/lib/home-lab-observer-connected-rich")
SERVER = "srv_" + "b" * 32
UNITS = ("home-lab-observer-connected-rich-collector.service", "home-lab-observer-connected-rich-uploader.service")
FAILURES = frozenset(("INPUT", "PACKAGE", "INSTALL", "STOPPED", "IDENTITY", "PROCESSES", "ISOLATION", "PENDING", "TAMPER", "RETRY", "BOUNDS", "NUMERIC", "RECEIVER"))


def need(value, code):
    if not value:
        raise AssertionError(code)


def record(path, limit=4096):
    with path.open("rb") as source:
        raw = source.read(limit + 1)
    need(0 < len(raw) <= limit, "IDENTITY")
    return json.loads(raw)


def run(*args):
    return subprocess.run(args, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=20, check=True, env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL": "C"})


def package():
    identity = record(PAYLOAD / "reviewed-rich-identity.json")
    need(set(identity) == {"commit", "archive_sha256", "manifest_sha256"}, "PACKAGE")
    archive, manifest = PAYLOAD / "rich-package.tgz", PAYLOAD / "rich-manifest.json"
    need(assert_guest.digest(archive) == identity["archive_sha256"] and assert_guest.digest(manifest) == identity["manifest_sha256"], "PACKAGE")
    value = record(manifest, 16384)
    need(identity["commit"] == record(PAYLOAD / "reviewed-identity.json")["commit"], "PACKAGE")
    need(value["commit"] == identity["commit"] and value["profile"] == "ubuntu24.04-systemd255-amd64-rich-processes-canary", "PACKAGE")
    members = {v["name"]: (v["size"], v["mode"], v["sha256"]) for v in value["files"]}
    members["connected-manifest.json"] = (manifest.stat().st_size, 0o644, identity["manifest_sha256"])
    target = Path("/opt/observer-fixture/bundle/rich-release")
    target.mkdir(mode=0o755)
    with tarfile.open(archive, "r:gz") as source:
        entries = source.getmembers()
        need(len(entries) == len(members) and {v.name for v in entries} == set(members), "PACKAGE")
        for v in entries:
            size, mode, digest = members[v.name]
            need(v.isfile() and v.name == Path(v.name).name and 0 < size <= 64 * 1024 * 1024 and v.size == size and v.mode == mode and v.uid == v.gid == 0, "PACKAGE")
            f = source.extractfile(v)
            need(f is not None, "PACKAGE")
            raw = f.read(size + 1)
            need(len(raw) == size and hashlib.sha256(raw).hexdigest() == digest, "PACKAGE")
            with (target / v.name).open("xb") as output:
                output.write(raw)
            (target / v.name).chmod(mode)
    return target, identity["manifest_sha256"]


def fingerprint():
    fixed = [CONFIG / "installed.json", CONFIG / "collection-policy.json", CONFIG / "credentials/connector.json"]
    fixed += [Path("/etc/systemd/system") / unit for unit in UNITS]
    return {str(v): [v.stat().st_ino, assert_guest.digest(v)] for v in fixed} | {"ledger_inode": (STATE / "ledger").stat().st_ino}


def stopped(binary):
    for unit in UNITS:
        fields = assert_guest.unit_properties(unit, ("Id", "LoadState", "FragmentPath", "ActiveState", "MainPID", "UnitFileState", "DropInPaths", "NeedDaemonReload"))
        need(fields == {"Id": unit, "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/" + unit, "ActiveState": "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}, "STOPPED")
    enrollment = "home-lab-observer-connected-rich-enrollment.service"
    fields = assert_guest.unit_properties(enrollment, ("Id", "LoadState", "ActiveState", "MainPID", "Transient", "FragmentPath"))
    need(fields == {"Id": enrollment, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}, "STOPPED")
    need(json.loads(activation_guest.command(binary, "status"))["state"] == "STOPPED", "STOPPED")
    verify_numeric()


def principals(config):
    collector, uploader, shared = "hlo-connected-rich-collector", "hlo-connected-rich-uploader", "hlo-connected-rich-read"
    users = ((collector, config["collector_uid"], config["shared_gid"], {config["shared_gid"]}),
             (uploader, config["uploader_uid"], config["uploader_gid"], {config["shared_gid"], config["uploader_gid"]}))
    for name, uid, gid, groups in users:
        user = pwd.getpwnam(name)
        need(user.pw_uid == uid and user.pw_gid == gid and user.pw_dir == "/nonexistent" and user.pw_shell == "/usr/sbin/nologin" and user.pw_passwd == "x", "IDENTITY")
        need(pwd.getpwuid(uid) == user and set(os.getgrouplist(name, gid)) == groups, "IDENTITY")
        status = preflight_probe.command("/usr/bin/passwd", "--status", "--", name, limit=1024).decode("ascii").split()
        need(len(status) == 7 and status[:2] == [name, "L"], "IDENTITY")
    need(preflight_probe.supported_nss(preflight_probe.fixed_file("/etc/nsswitch.conf", 65536)), "IDENTITY")
    for name, gid, members in ((shared, config["shared_gid"], [uploader]), (uploader, config["uploader_gid"], [])):
        group = grp.getgrnam(name)
        need(group.gr_gid == gid and group.gr_passwd == "x" and group.gr_mem == members and grp.getgrgid(gid) == group, "IDENTITY")
        locked = preflight_probe.command("/usr/bin/getent", "gshadow", name, limit=1024).decode("ascii").removesuffix("\n").split(":")
        need(len(locked) == 4 and locked[0] == name and locked[1].startswith(("!", "*")) and locked[2] == "" and locked[3] == ",".join(members), "IDENTITY")
    for user in pwd.getpwall():
        if user.pw_name not in (collector, uploader):
            need(user.pw_uid not in (config["collector_uid"], config["uploader_uid"]) and user.pw_gid not in (config["shared_gid"], config["uploader_gid"]), "IDENTITY")
    for group in grp.getgrall():
        if group.gr_name not in (shared, uploader):
            need(group.gr_gid not in (config["shared_gid"], config["uploader_gid"]) and not ({collector, uploader} & set(group.gr_mem)), "IDENTITY")


def pending():
    # The production ledger uses one fixed DB; never run SQL against a live writer.
    database = STATE / "ledger/upload.sqlite"
    need(database.is_file(), "PENDING")
    with sqlite3.connect("file:" + str(database) + "?mode=ro", uri=True) as connection:
        row = connection.execute("SELECT pending_sequence,pending_body,pending_digest,watermark,profile_kind,policy_digest FROM upload_record WHERE singleton=1").fetchone()
    need(row is not None and row[0] is not None and 16384 < len(row[1]) <= 294912 and hashlib.sha256(row[1]).digest() == row[2] and row[4] == 1, "PENDING")
    return {"sequence": row[0], "digest": row[2].hex(), "size": len(row[1]), "watermark": row[3], "policy": row[5].hex()}


def sleepers(first):
    # One shared Python image, no exec/arguments/environment acquisition. The
    # receiver observes only names; no names enter serial evidence or artifacts.
    if first:
        run("/usr/sbin/groupadd", "--gid", "22002", "hlo-fixture-process")
        run("/usr/sbin/useradd", "--uid", "22002", "--gid", "22002", "--no-create-home", "--shell", "/usr/sbin/nologin", "hlo-fixture-process")
    user = pwd.getpwnam("hlo-fixture-process")
    need(user.pw_uid == user.pw_gid == 22002 and user.pw_shell == "/usr/sbin/nologin", "PROCESSES")
    children = []
    parent = os.getpid()
    try:
        for index in range(200):
            child = os.fork()
            if child == 0:
                try:
                    os.setgroups([]); os.setgid(22002); os.setuid(22002)
                    libc = ctypes.CDLL(None)
                    if libc.prctl(1, signal.SIGKILL, 0, 0, 0) != 0 or os.getppid() != parent or libc.prctl(15, ("hlofixture%05d" % index).encode(), 0, 0, 0) != 0:
                        os._exit(1)
                    time.sleep(480)
                finally:
                    os._exit(0)
            children.append(child)
    except OSError:
        join_sleepers(children)
        raise
    return children


def join_sleepers(children):
    for child in children:
        try:
            os.kill(child, signal.SIGKILL)
        except ProcessLookupError:
            pass
        os.waitpid(child, 0)


def receiver():
    p = subprocess.Popen([str(PAYLOAD / "connected-first-install-receiver"), "93.184.216.34:443", str(ROOT / "cert.pem"), str(ROOT / "key.pem"), "rich-processes"], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(50):
        if p.poll() is not None:
            raise AssertionError("RECEIVER")
        # Fixed guest-only TCP readiness, never an external probe.
        import socket
        try:
            with socket.create_connection(("93.184.216.34", 443), timeout=0.2):
                return p
        except OSError:
            time.sleep(0.1)
    p.terminate(); p.wait(timeout=5)
    raise AssertionError("RECEIVER")


def wait_counts(predicate):
    deadline = time.monotonic() + 115
    while time.monotonic() < deadline:
        try:
            value = record(ROOT / "rich-receiver.json")
            if predicate(value):
                return value
        except FileNotFoundError:
            pass
        time.sleep(0.5)
    raise AssertionError("RETRY")


def isolation(config):
    collector = int(assert_guest.unit_properties(UNITS[0], ("MainPID",))["MainPID"])
    uploader = int(assert_guest.unit_properties(UNITS[1], ("MainPID",))["MainPID"])
    need(collector > 1 and uploader > 1, "ISOLATION")
    need(not list(Path(f"/proc/{uploader}/root/proc").iterdir()), "ISOLATION")
    need(not Path(f"/proc/{uploader}/root/var/run/docker.sock").exists(), "ISOLATION")
    props = assert_guest.unit_properties(UNITS[0], ("PrivateNetwork", "ProtectProc", "CapabilityBoundingSet", "AmbientCapabilities", "NoNewPrivileges"))
    need(props == {"PrivateNetwork": "yes", "ProtectProc": "default", "CapabilityBoundingSet": "", "AmbientCapabilities": "", "NoNewPrivileges": "yes"}, "ISOLATION")
    # Execute a fixed metadata/read refusal inside the actual collector mount
    # namespace, dropped to the real collector UID/GID. No payload is printed.
    code = "import os; p='/etc/home-lab-observer-connected-rich/credentials/connector.json';\ntry: os.open(p,os.O_RDONLY)\nexcept (PermissionError,FileNotFoundError): raise SystemExit(0)\nraise SystemExit(1)"
    run("/usr/bin/nsenter", "--target", str(collector), "--mount", "--", "/usr/bin/setpriv", "--reuid", str(config["collector_uid"]), "--regid", str(config["shared_gid"]), "--clear-groups", "/usr/bin/python3", "-I", "-c", code)
    verify_numeric()


def exercise(mode):
    need(mode in ("first", "reboot") and os.geteuid() == 0, "INPUT")
    verify_numeric()
    children = []
    p = receiver()
    try:
        if mode == "first":
            target, digest = package()
            subprocess.run(["/usr/bin/python3", str(PAYLOAD / "drive_pty.py"), "install-rich", str(target), digest], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=200, check=True)
        config = record(CONFIG / "installed.json")
        need(config["collection_profile"] == "RICH_PROCESSES_V1" and config["server_id"] == SERVER, "IDENTITY")
        principals(config)
        binary = Path("/opt/home-lab-observer-connected-rich/releases") / config["artifact_sha256"] / "observer-connected-rich-install"
        before = fingerprint()
        stopped(binary)
        children = sleepers(mode == "first")
        if mode == "first":
            (ROOT / "rich-hold").touch(mode=0o600, exist_ok=False)
            activation_guest.command(binary, "start")
            wait_counts(lambda v: v["lost_ack"] and v["large_body"] and v["foreign_process"])
            isolation(config)
            activation_guest.command(binary, "stop"); stopped(binary)
            held = pending()
            policy = CONFIG / "collection-policy.json"
            original = policy.read_bytes()
            counts = (ROOT / "rich-receiver.json").read_bytes()
            try:
                policy.write_bytes(original.replace(b'"processes_enabled":true', b'"processes_enabled":false'))
                try:
                    activation_guest.command(binary, "start")
                except activation_guest.Refused:
                    pass
                else:
                    raise AssertionError("TAMPER")
                need(pending() == held and (ROOT / "rich-receiver.json").read_bytes() == counts, "TAMPER")
            finally:
                policy.write_bytes(original)
            need(fingerprint() == before, "IDENTITY")
            result = {"identity": before, "pending": held, "duplicates": json.loads(counts)["duplicates"]}
            (ROOT / "rich-result.json").write_text(json.dumps(result, separators=(",", ":")))
            (ROOT / "rich-result.json").chmod(0o600)
        else:
            prior = record(ROOT / "rich-result.json")
            need(prior["identity"] == before and prior["pending"] == pending(), "PENDING")
            (ROOT / "rich-hold").unlink()
            activation_guest.command(binary, "start")
            wait_counts(lambda v: v["duplicates"] > prior["duplicates"] and v["sequence"] == prior["pending"]["sequence"] and v["sha256"] == prior["pending"]["digest"] and v["large_body"] and v["foreign_process"])
            deadline = time.monotonic() + 30
            while json.loads(activation_guest.command(binary, "status"))["state"] != "ACKNOWLEDGED_FRESH":
                need(time.monotonic() < deadline, "RETRY"); time.sleep(0.5)
            isolation(config)
            activation_guest.command(binary, "stop"); stopped(binary)
            need(fingerprint() == before, "IDENTITY")
        need(sum(v.stat().st_size for v in (STATE / "ledger").iterdir() if v.is_file()) <= 2097152, "BOUNDS")
        verify_numeric()
    finally:
        join_sleepers(children)
        p.terminate(); p.wait(timeout=5)


def verify_numeric():
    need(assert_guest.snapshot() == json.loads((ROOT / "snapshot.json").read_text()), "NUMERIC")
    fields = ("LoadState", "ActiveState", "MainPID", "UnitFileState", "DropInPaths", "NeedDaemonReload")
    expected = {"LoadState": "loaded", "ActiveState": "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}
    for unit in ("home-lab-observer-connected-collector.service", "home-lab-observer-connected-uploader.service"):
        need(assert_guest.unit_properties(unit, fields) == expected, "NUMERIC")
    need(numeric_workers_absent(), "NUMERIC")


def numeric_workers_absent():
    for process in Path("/proc").glob("[0-9]*"):
        try:
            name = (process / "exe").readlink().name.removesuffix(" (deleted)")
        except (FileNotFoundError, ProcessLookupError):
            continue
        except OSError:
            return False
        if name in ("observer-connected-collector", "observer-connected-uploader"):
            return False
    return True


def main():
    try:
        need(len(sys.argv) == 2, "INPUT")
        exercise(sys.argv[1])
        return 0
    except (OSError, ValueError, KeyError, TypeError, AttributeError, AssertionError, sqlite3.Error, tarfile.TarError, subprocess.SubprocessError, activation_guest.Refused, preflight_probe.Refused) as error:
        code = str(error) if isinstance(error, AssertionError) and str(error) in FAILURES else "UNKNOWN"
        print("HLO_VM_FAIL_RICH_" + code, file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
