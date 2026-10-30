#!/usr/bin/env python3
"""Fixed real-Docker journey, only in the admitted disposable no-NIC guest."""
import grp
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import sqlite3
import stat
import subprocess
import sys
import tarfile
import time

import activation_guest
import assert_guest
import docker_engine
import docker_scope
import preflight_probe
import rich_guest
from rich_guest import need, record, run

ROOT, PAYLOAD, CONFIG, STATE = rich_guest.ROOT, rich_guest.PAYLOAD, rich_guest.CONFIG, rich_guest.STATE
SERVER = "srv_" + "c" * 32
UNITS = tuple("home-lab-observer-connected-rich-docker-" + role + ".service" for role in ("collector", "reader", "uploader"))
ENROLLMENT = "home-lab-observer-connected-rich-docker-enrollment.service"
FAILURES = frozenset(("INPUT", "PACKAGE", "INSTALL", "STOPPED", "IDENTITY", "ISOLATION", "PENDING", "TAMPER", "RETRY", "RETRY_WINDOW", "REFUSAL", "BOUNDS", "NUMERIC", "RECEIVER", "FAULT", "ENGINE", "ENGINE_API", "ENGINE_NETWORK", "ENGINE_DEPENDENCY", "ENGINE_SECURITY", "ENGINE_INPUT", "ENGINE_IMAGE", "ENGINE_START", "ENGINE_SOCKET", "ENGINE_CONTAINER"))
FAILURES |= frozenset(("FAULT_INITIAL_BINDING", "FAULT_SOCKET_REPLACEMENT", "FAULT_READER_TERMINAL", "FAULT_UNAVAILABLE", "FAULT_STOP", "FAULT_START", "FAULT_RECOVERY"))


def package():
    identity = record(PAYLOAD / "reviewed-docker-identity.json")
    archive, manifest = PAYLOAD / "docker-package.tgz", PAYLOAD / "docker-manifest.json"
    need(set(identity) == {"commit", "archive_sha256", "manifest_sha256"}, "PACKAGE")
    need(assert_guest.digest(archive) == identity["archive_sha256"] and assert_guest.digest(manifest) == identity["manifest_sha256"], "PACKAGE")
    need(identity["commit"] == record(PAYLOAD / "reviewed-identity.json")["commit"], "PACKAGE")
    value = record(manifest, 16384)
    docker_scope.check_identity(value, identity["commit"])
    members = {v["name"]: (v["size"], v["mode"], v["sha256"]) for v in value["files"]}
    need(len(members) == len(value["files"]), "PACKAGE")
    members["connected-manifest.json"] = (manifest.stat().st_size, 0o644, identity["manifest_sha256"])
    target = Path("/opt/observer-fixture/bundle/docker-release")
    target.mkdir(mode=0o755)
    with tarfile.open(archive, "r:gz") as source:
        entries = source.getmembers()
        need(len(entries) == len(members) and {v.name for v in entries} == set(members), "PACKAGE")
        for v in entries:
            size, mode, digest = members[v.name]
            need(v.isfile() and v.name == Path(v.name).name and 0 < size <= 64 * 1024 * 1024 and v.size == size and v.mode == mode and v.uid == v.gid == 0, "PACKAGE")
            with source.extractfile(v) as content:
                raw = content.read(size + 1)
            need(len(raw) == size and hashlib.sha256(raw).hexdigest() == digest, "PACKAGE")
            with (target / v.name).open("xb") as output:
                output.write(raw)
            (target / v.name).chmod(mode)
    return target, identity["manifest_sha256"]


def install(target, digest, mode):
    # Driver retains raw PTY output in bounded process memory; only its fixed
    # completion marker is accepted here. Never forward arbitrary child output.
    result = subprocess.run(["/usr/bin/python3", str(PAYLOAD / "drive_pty.py"), mode, str(target), digest], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=200)
    need(result.returncode == 0 and result.stdout == b"DRIVER_INSTALLED\n", "INSTALL")


def fingerprint():
    files = [CONFIG / "installed.json", CONFIG / "collection-policy.json", CONFIG / "credentials/connector.json"]
    files += [Path("/etc/systemd/system") / unit for unit in UNITS]
    return {str(path): [path.stat().st_ino, assert_guest.digest(path)] for path in files} | {"ledger_inode": (STATE / "ledger").stat().st_ino}


def stopped(binary):
    for unit in UNITS:
        fields = ("Id", "LoadState", "FragmentPath", "ActiveState", "MainPID", "UnitFileState", "DropInPaths", "NeedDaemonReload")
        need(assert_guest.unit_properties(unit, fields) == {"Id": unit, "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/" + unit, "ActiveState": "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}, "STOPPED")
    fields = ("Id", "LoadState", "ActiveState", "MainPID", "Transient", "FragmentPath")
    need(assert_guest.unit_properties(ENROLLMENT, fields) == {"Id": ENROLLMENT, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}, "STOPPED")
    need(json.loads(activation_guest.command(binary, "status"))["state"] == "STOPPED", "STOPPED")
    rich_guest.verify_numeric()


def principals(c):
    collector, reader, uploader = ("hlo-rich-docker-" + role for role in ("collector", "reader", "uploader"))
    shared, frame = "hlo-rich-docker-read", "hlo-rich-docker-frame"
    users = ((collector, c["collector_uid"], c["shared_gid"], {c["shared_gid"]}),
             (reader, c["reader_uid"], c["frame_gid"], {c["frame_gid"]}),
             (uploader, c["uploader_uid"], c["uploader_gid"], {c["shared_gid"], c["uploader_gid"]}))
    for name, uid, gid, groups in users:
        user = pwd.getpwnam(name)
        need(user.pw_uid == uid and user.pw_gid == gid and user.pw_dir == "/nonexistent" and user.pw_shell == "/usr/sbin/nologin" and user.pw_passwd == "x", "IDENTITY")
        need(pwd.getpwuid(uid) == user and set(os.getgrouplist(name, gid)) == groups, "IDENTITY")
        fields = preflight_probe.command("/usr/bin/passwd", "--status", "--", name, limit=1024).decode("ascii").split()
        need(len(fields) == 7 and fields[:2] == [name, "L"], "IDENTITY")
    need(preflight_probe.supported_nss(preflight_probe.fixed_file("/etc/nsswitch.conf", 65536)), "IDENTITY")
    groups = ((shared, c["shared_gid"], [uploader]), (frame, c["frame_gid"], []), (uploader, c["uploader_gid"], []))
    for name, gid, members in groups:
        group = grp.getgrnam(name)
        need(group.gr_gid == gid and group.gr_passwd == "x" and group.gr_mem == members and grp.getgrgid(gid) == group, "IDENTITY")
        locked = preflight_probe.command("/usr/bin/getent", "gshadow", name, limit=1024).decode("ascii").removesuffix("\n").split(":")
        need(len(locked) == 4 and locked[0] == name and locked[1].startswith(("!", "*")) and locked[2] == "" and locked[3] == ",".join(members), "IDENTITY")
    need(c["socket_gid"] == docker_engine.DAEMON_GID and grp.getgrgid(c["socket_gid"]).gr_mem == [], "IDENTITY")
    for user in pwd.getpwall():
        if user.pw_name not in (collector, reader, uploader):
            need(user.pw_uid not in {v[1] for v in users} and user.pw_gid not in {c["shared_gid"], c["frame_gid"], c["uploader_gid"]}, "IDENTITY")
    for group in grp.getgrall():
        if group.gr_name not in {v[0] for v in groups}:
            need(group.gr_gid not in {c["shared_gid"], c["frame_gid"], c["uploader_gid"]} and not ({collector, reader, uploader} & set(group.gr_mem)), "IDENTITY")


def pending():
    policy = record(CONFIG / "installed.json").get("collection_policy_sha256")
    need(isinstance(policy, str) and re.fullmatch(r"[a-f0-9]{64}", policy) is not None, "PENDING")
    with sqlite3.connect("file:" + str(STATE / "ledger/upload.sqlite") + "?mode=ro", uri=True) as connection:
        schema = connection.execute("PRAGMA user_version").fetchone()[0]
        row = connection.execute("SELECT pending_sequence,pending_body,pending_digest,watermark,profile_kind,policy_digest,pending_at FROM upload_record WHERE singleton=1").fetchone()
    need(schema == 3 and row is not None and row[0] is not None and isinstance(row[1], bytes) and 16384 < len(row[1]) <= 294912 and isinstance(row[2], bytes) and len(row[2]) == 32 and hashlib.sha256(row[1]).digest() == row[2] and row[4] == 2 and isinstance(row[5], bytes) and len(row[5]) == 32 and row[5].hex() == policy, "PENDING")
    return {"sequence": row[0], "digest": row[2].hex(), "size": len(row[1]), "watermark": row[3], "policy": row[5].hex(), "collected_at": row[6]}


def receiver():
    process = subprocess.Popen([str(PAYLOAD / "connected-first-install-receiver"), "93.184.216.34:443", str(ROOT / "cert.pem"), str(ROOT / "key.pem"), "rich-docker"], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    import socket
    for _ in range(50):
        need(process.poll() is None, "RECEIVER")
        try:
            with socket.create_connection(("93.184.216.34", 443), timeout=0.2):
                return process
        except OSError:
            time.sleep(0.1)
    process.terminate(); process.wait(timeout=5)
    raise AssertionError("RECEIVER")


def counts():
    return record(ROOT / "docker-receiver.json")


def wait_counts(predicate, held=None):
    deadline = time.monotonic() + 115
    while time.monotonic() < deadline:
        if held is not None:
            rich_guest.pending_fresh(held)
        try:
            value = counts()
            if predicate(value):
                return value
        except FileNotFoundError:
            pass
        time.sleep(0.5)
    raise AssertionError("RETRY")


def healthy(value):
    return value["healthy_sequence"] == value["sequence"] and value["sequence"] > 0 and all(value[name] for name in ("large_body", "foreign_process", "running_container", "exited_null", "warmed_metrics"))


NAMESPACE_PROBE = """import ctypes, json, os, sys
pid, uid, gid = map(int, sys.argv[1:4])
groups, denied, empty_proc = json.loads(sys.argv[4])
if pid < 2 or not 0 < uid < 4294967295 or not 0 < gid < 4294967295: raise SystemExit(1)
libc = ctypes.CDLL(None, use_errno=True)
if libc.prctl(38, 1, 0, 0, 0) != 0: raise SystemExit(1)
expected_root = os.stat('/proc/%d/root' % pid)
fd = os.open('/proc/%d/ns/mnt' % pid, os.O_RDONLY | os.O_CLOEXEC)
try:
 if libc.setns(fd, 0x20000) != 0: raise SystemExit(1)
finally: os.close(fd)
entered_root = os.stat('/')
if (entered_root.st_dev, entered_root.st_ino) != (expected_root.st_dev, expected_root.st_ino): raise SystemExit(1)
os.setgroups(groups)
os.setresgid(gid, gid, gid)
os.setresuid(uid, uid, uid)
if os.getresuid() != (uid, uid, uid) or os.getresgid() != (gid, gid, gid) or set(os.getgroups()) != set(groups) or libc.prctl(39, 0, 0, 0, 0) != 1: raise SystemExit(1)
if empty_proc and os.listdir('/proc') != []: raise SystemExit(1)
for p in denied:
 try: fd = os.open(p, os.O_RDONLY)
 except (PermissionError, FileNotFoundError): continue
 os.close(fd)
 raise SystemExit(1)
"""


def probe_role(unit, uid, gid, groups, denied, empty_proc):
    fields = ("ActiveState", "MainPID", "InvocationID")
    before = assert_guest.unit_properties(unit, fields)
    need(before["ActiveState"] == "active" and re.fullmatch(r"[1-9][0-9]{0,9}", before["MainPID"]) is not None and int(before["MainPID"]) > 1 and re.fullmatch(r"[a-f0-9]{32}", before["InvocationID"]) is not None, "ISOLATION")
    need(len(groups) == 1, "ISOLATION")
    live_identity(int(before["MainPID"]), uid, gid, groups[0])
    run("/usr/bin/python3", "-I", "-c", NAMESPACE_PROBE, before["MainPID"], str(uid), str(gid), json.dumps([groups, denied, empty_proc], separators=(",", ":")))
    need(assert_guest.unit_properties(unit, fields) == before, "ISOLATION")


def live_identity(pid, uid, gid, supplementary):
    # Check the actual worker, not merely the identity we give the probe. Match
    # production's exact supplementary grant; Linux may also list primary GID.
    raw = preflight_probe.fixed_file(f"/proc/{pid}/status", 16384).decode("ascii")
    fields = {}
    for line in raw.splitlines():
        name, separator, value = line.partition(":")
        if name in ("Uid", "Gid", "Groups"):
            need(separator == ":" and name not in fields and re.fullmatch(r"\s*[0-9]+(?:\s+[0-9]+)*\s*", value) is not None, "ISOLATION")
            fields[name] = [int(v) for v in value.split()]
    need(set(fields) == {"Uid", "Gid", "Groups"} and fields["Uid"] == [uid] * 4 and fields["Gid"] == [gid] * 4, "ISOLATION")
    need(len(fields["Groups"]) == len(set(fields["Groups"])) and set(fields["Groups"]) | {gid} == {gid, supplementary} and supplementary in fields["Groups"], "ISOLATION")


def isolation(c):
    creds = "/etc/home-lab-observer-connected-rich/credentials/connector.json"
    probe_role(UNITS[0], c["collector_uid"], c["shared_gid"], [c["frame_gid"]], [creds, "/var/lib/home-lab-observer-connected/uploader-root", "/run/docker.sock", "/var/run/docker.sock", "/engine/docker.sock", "/run/home-lab-observer-connected-rich/activation", "/run/home-lab-observer-connected-rich/reader-activation"], False)
    probe_role(UNITS[1], c["reader_uid"], c["frame_gid"], [c["socket_gid"]], [creds, "/state/ledger/upload.sqlite", "/etc/ssl/certs/ca-certificates.crt"], True)
    probe_role(UNITS[2], c["uploader_uid"], c["uploader_gid"], [c["shared_gid"]], ["/run/docker.sock", "/engine/docker.sock", "/var/lib/home-lab-observer-connected-rich/docker-frame/snapshot.json", "/frame/snapshot.json"], True)
    for unit in UNITS:
        props = assert_guest.unit_properties(unit, ("CapabilityBoundingSet", "AmbientCapabilities", "NoNewPrivileges"))
        need(props == {"CapabilityBoundingSet": "", "AmbientCapabilities": "", "NoNewPrivileges": "yes"}, "ISOLATION")
    for unit, memory, tasks in zip(UNITS, (268435456, 134217728, 268435456), (64, 32, 64)):
        props = assert_guest.unit_properties(unit, ("MemoryMax", "TasksMax", "Restart", "StandardOutput", "StandardError"))
        need(props == {"MemoryMax": str(memory), "TasksMax": str(tasks), "Restart": "no", "StandardOutput": "null", "StandardError": "null"}, "BOUNDS")
    need(assert_guest.unit_properties(UNITS[1], ("CPUQuotaPerSecUSec",)) == {"CPUQuotaPerSecUSec": "250ms"}, "BOUNDS")
    rich_guest.verify_numeric()


def fault_journey(binary):
    before = counts()
    # Fixture-only stopped helper: an aging frame must become explicitly
    # unavailable, not an invented healthy empty list or retimestamped cache.
    run("/usr/bin/systemctl", "stop", "--", UNITS[1])
    fault = wait_counts(lambda v: v["unavailable_count"] > before["unavailable_count"] and v["healthy_sequence"] == 0)
    activation_guest.command(binary, "stop"); stopped(binary)
    activation_guest.command(binary, "start")
    wait_counts(lambda v: healthy(v) and v["sequence"] > fault["sequence"])


def reader_binding():
    fields = ("ActiveState", "MainPID", "InvocationID")
    manager = assert_guest.unit_properties(UNITS[1], fields)
    need(manager["ActiveState"] == "active" and re.fullmatch(r"[1-9][0-9]{0,9}", manager["MainPID"]) is not None and int(manager["MainPID"]) > 1 and re.fullmatch(r"[a-f0-9]{32}", manager["InvocationID"]) is not None, "FAULT")
    node = Path(f"/proc/{manager['MainPID']}/root/engine/docker.sock").stat()
    need(stat.S_ISSOCK(node.st_mode), "FAULT")
    # /proc/PID/root deliberately follows the kernel task's root view, not a
    # user-controlled path. Recheck manager identity after this fixed metadata.
    need(assert_guest.unit_properties(UNITS[1], fields) == manager, "FAULT")
    return {"manager": manager, "socket": [node.st_dev, node.st_ino]}


def reader_fault_state():
    fields = ("ActiveState", "MainPID", "InvocationID", "Result", "ExecMainCode", "ExecMainStatus", "Restart", "NRestarts", "Job")
    value = assert_guest.unit_properties(UNITS[1], fields)
    need(set(value) == set(fields) and value["Restart"] == "no" and value["NRestarts"] == "0" and value["Job"] == "", "FAULT")
    if value["ActiveState"] == "active":
        need(value["Result"] == "success" and value["ExecMainCode"] == value["ExecMainStatus"] == "0", "FAULT")
    return value


def wait_reader_termination(reader):
    # The old namespace may survive briefly, but the unlinked pinned socket is
    # no longer approved. Never substitute a different process or invocation.
    old = reader["manager"]
    def exited(value):
        return value["Result"] == "exit-code" and value["ExecMainCode"] == "1" and value["ExecMainStatus"] == "22"
    def terminal(value):
        return value["InvocationID"] == old["InvocationID"] and value["ActiveState"] == "failed" and value["MainPID"] == "0" and exited(value)
    def retiring(value):
        return value["InvocationID"] == old["InvocationID"] and value["ActiveState"] == "deactivating" and value["MainPID"] in (old["MainPID"], "0") and exited(value)
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        before = reader_fault_state()
        need(before["InvocationID"] == old["InvocationID"], "FAULT")
        if terminal(before):
            need(reader_fault_state() == before, "FAULT")
            return before
        need(retiring(before) or (before["ActiveState"] == "active" and before["MainPID"] == old["MainPID"]), "FAULT")
        try:
            node = Path(f"/proc/{old['MainPID']}/root/engine/docker.sock").stat() if before["MainPID"] != "0" else None
        except FileNotFoundError:
            # The same process can exit between manager and /proc snapshots.
            # Only its exact stable terminal result can complete this proof.
            node = None
        if node is not None:
            need(stat.S_ISSOCK(node.st_mode) and [node.st_dev, node.st_ino] == reader["socket"], "FAULT")
        after = reader_fault_state()
        if terminal(after):
            need(reader_fault_state() == after, "FAULT")
            return after
        need(after == before or retiring(after), "FAULT")
        time.sleep(0.5)
    raise AssertionError("FAULT")


def recovery_stopped(binary, failed):
    # Supported Stop intentionally does not reset an already-failed reader.
    # Keep the normal all-inactive stopped() assertion strict everywhere else.
    need(reader_fault_state() == failed, "FAULT")
    for unit in UNITS:
        fields = ("Id", "LoadState", "FragmentPath", "ActiveState", "MainPID", "UnitFileState", "DropInPaths", "NeedDaemonReload")
        need(assert_guest.unit_properties(unit, fields) == {"Id": unit, "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/" + unit, "ActiveState": "failed" if unit == UNITS[1] else "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}, "FAULT")
    fields = ("Id", "LoadState", "ActiveState", "MainPID", "Transient", "FragmentPath")
    need(assert_guest.unit_properties(ENROLLMENT, fields) == {"Id": ENROLLMENT, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}, "FAULT")
    need(json.loads(activation_guest.command(binary, "status"))["state"] == "RECOVERY_REQUIRED", "FAULT")
    rich_guest.verify_numeric()
    need(reader_fault_state() == failed, "FAULT")


def socket_replacement(binary):
    phase = "FAULT_INITIAL_BINDING"
    try:
        old = Path("/run/docker.sock").stat()
        reader = reader_binding()
        initial = reader_fault_state()
        need({key: initial[key] for key in reader["manager"]} == reader["manager"] and reader["socket"] == [old.st_dev, old.st_ino], "FAULT")
        before = counts()
        phase = "FAULT_SOCKET_REPLACEMENT"
        docker_engine.stop()
        docker_engine.start()
        docker_engine.cli("container", "start", docker_engine.NAMES[0])
        current = Path("/run/docker.sock").stat()
        need((current.st_dev, current.st_ino) != (old.st_dev, old.st_ino), "FAULT")
        phase = "FAULT_READER_TERMINAL"
        failed = wait_reader_termination(reader)
        phase = "FAULT_UNAVAILABLE"
        fault = wait_counts(lambda v: v["unavailable_count"] > before["unavailable_count"] and v["healthy_sequence"] == 0)
        need(reader_fault_state() == failed, "FAULT")
        phase = "FAULT_STOP"
        activation_guest.command(binary, "stop"); recovery_stopped(binary, failed)
        phase = "FAULT_START"
        activation_guest.command(binary, "start")
        fresh = reader_binding()
        current_state = reader_fault_state()
        need(fresh["manager"]["InvocationID"] != reader["manager"]["InvocationID"] and fresh["socket"] == [current.st_dev, current.st_ino] and {key: current_state[key] for key in fresh["manager"]} == fresh["manager"], "FAULT")
        phase = "FAULT_RECOVERY"
        wait_counts(lambda v: healthy(v) and v["sequence"] > fault["sequence"])
        need(reader_binding() == fresh and reader_fault_state() == current_state, "FAULT")
        final_socket = Path("/run/docker.sock").stat()
        need((final_socket.st_dev, final_socket.st_ino) == (current.st_dev, current.st_ino), "FAULT")
    except AssertionError as error:
        # Preserve known lifecycle refusals in their existing outer handler;
        # only closed fixture assertions become a finite fault subphase.
        if str(error) in FAILURES:
            raise AssertionError(phase) from None
        raise


def unchanged_process_state():
    roots = (CONFIG, STATE, Path("/opt/home-lab-observer-connected-rich"))
    paths = [Path("/etc/passwd"), Path("/etc/group"), Path("/etc/shadow"), Path("/etc/gshadow")]
    paths += [Path("/etc/systemd/system") / unit for unit in rich_guest.UNITS]
    for root in roots:
        paths += [root, *root.rglob("*")]
    result = {}
    for path in paths:
        info = path.lstat()
        need(not path.is_symlink(), "REFUSAL")
        result[str(path)] = [info.st_dev, info.st_ino, info.st_mode, info.st_uid, info.st_gid, assert_guest.digest(path) if path.is_file() else None]
    return result


def refusal_targets_absent():
    for name in ("hlo-rich-docker-reader", "hlo-rich-docker-collector", "hlo-rich-docker-uploader"):
        try:
            pwd.getpwnam(name)
        except KeyError:
            pass
        else:
            raise AssertionError("REFUSAL")
    for role in ("uploader", "read", "frame", "collector", "reader"):
        try:
            grp.getgrnam("hlo-rich-docker-" + role)
        except KeyError:
            pass
        else:
            raise AssertionError("REFUSAL")
    for unit in (*UNITS, ENROLLMENT):
        for suffix in ("", ".d"):
            try:
                Path("/etc/systemd/system", unit + suffix).lstat()
            except FileNotFoundError:
                continue
            raise AssertionError("REFUSAL")
        fields = ("Id", "LoadState", "ActiveState", "MainPID", "Transient", "FragmentPath")
        need(assert_guest.unit_properties(unit, fields) == {"Id": unit, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}, "REFUSAL")
    for name in ("activation", "reader-activation"):
        try:
            Path("/run/home-lab-observer-connected-rich", name).lstat()
        except FileNotFoundError:
            continue
        raise AssertionError("REFUSAL")


def refusal_prerequisites(process):
    try:
        docker_engine.prove_containers()
        info = Path("/run/docker.sock").lstat()
        need(stat.S_ISSOCK(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o660 and info.st_uid == 0 and info.st_gid == docker_engine.DAEMON_GID and info.st_nlink == 1, "REFUSAL")
        need(process.poll() is None, "REFUSAL")
        preflight_probe.dns_tls()
        preflight_probe.parent_policy()
    except (OSError, ValueError, AssertionError, subprocess.SubprocessError, preflight_probe.Refused):
        raise AssertionError("REFUSAL") from None


def refusal(mode):
    if mode == "first":
        rich_guest.prepare_reboot()
        target, digest = rich_guest.package()
        process = rich_guest.receiver()
        try:
            docker_engine.prepare(True)
            try:
                install(target, digest, "install-rich")
                config = record(CONFIG / "installed.json")
                rich_guest.principals(config)
                binary = Path("/opt/home-lab-observer-connected-rich/releases") / config["artifact_sha256"] / "observer-connected-rich-install"
                rich_guest.stopped(binary)
                target, digest = package()
                refusal_targets_absent()
                before = unchanged_process_state()
                receipt = preflight_probe.fixed_file(str(ROOT / "rich-receiver.json"), 1024)
                refusal_prerequisites(process)
                result = subprocess.run([str(target / "observer-connected-rich-docker-install"), "install", str(target), digest, SERVER], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20)
                diagnostic = b"PREFLIGHT_REFUSED: verify privileges, supported host and rich bundle before enrollment.\n"
                need(result.returncode == 22 and result.stdout == b"" and len(result.stderr) <= 1024 and result.stderr == diagnostic, "REFUSAL")
                need(unchanged_process_state() == before, "REFUSAL")
                need(preflight_probe.fixed_file(str(ROOT / "rich-receiver.json"), 1024) == receipt, "REFUSAL")
                (ROOT / "docker-refusal.json").write_text(json.dumps(before, separators=(",", ":")))
                (ROOT / "docker-refusal.json").chmod(0o600)
            finally:
                primary = sys.exception()
                try:
                    docker_engine.stop()
                except (OSError, AssertionError, subprocess.SubprocessError, preflight_probe.Refused):
                    # The journey remains failed; a cleanup error must not hide
                    # its original closed refusal. Never claim cleanup passed.
                    if primary is None:
                        raise
        finally:
            primary = sys.exception()
            try:
                try:
                    process.terminate()
                finally:
                    process.wait(timeout=5)
            except (OSError, subprocess.SubprocessError):
                if primary is None:
                    raise
    else:
        fields = ("ActiveState", "MainPID", "UnitFileState")
        need(assert_guest.unit_properties(docker_engine.UNIT, fields) == {"ActiveState": "inactive", "MainPID": "0", "UnitFileState": "static"}, "REFUSAL")
        need(unchanged_process_state() == record(ROOT / "docker-refusal.json", 1024 * 1024), "REFUSAL")
        config = record(CONFIG / "installed.json")
        rich_guest.principals(config)
        binary = Path("/opt/home-lab-observer-connected-rich/releases") / config["artifact_sha256"] / "observer-connected-rich-install"
        rich_guest.stopped(binary)
    refusal_targets_absent()
    rich_guest.verify_numeric()


def exercise(scope, mode):
    need(os.geteuid() == 0 and mode in ("first", "reboot") and scope in ("rich-docker", "rich-docker-refusal"), "INPUT")
    rich_guest.verify_numeric()
    if scope == "rich-docker-refusal":
        refusal(mode)
        return
    if mode == "first":
        rich_guest.prepare_reboot()
    try:
        docker_engine.prepare(mode == "first")
    except (OSError, subprocess.SubprocessError, preflight_probe.Refused):
        raise AssertionError("ENGINE") from None
    process, children = receiver(), []
    try:
        if mode == "first":
            target, digest = package()
            install(target, digest, "install-rich-docker")
        config = record(CONFIG / "installed.json")
        need(config["collection_profile"] == "RICH_PROCESSES_DOCKER_V1" and config["server_id"] == SERVER, "IDENTITY")
        principals(config)
        binary = Path("/opt/home-lab-observer-connected-rich/releases") / config["artifact_sha256"] / "observer-connected-rich-docker-install"
        before = fingerprint()
        stopped(binary)
        children = rich_guest.sleepers(mode == "first")
        if mode == "first":
            activation_guest.command(binary, "start")
            wait_counts(healthy)
            isolation(config)
            fault_journey(binary)
            activation_guest.command(binary, "stop"); stopped(binary)
            (ROOT / "docker-hold").touch(mode=0o600, exist_ok=False)
            prior = counts()
            activation_guest.command(binary, "start")
            held_counts = wait_counts(lambda v: healthy(v) and v["sequence"] > prior["sequence"])
            activation_guest.command(binary, "stop"); stopped(binary)
            held = pending()
            need(held["sequence"] == held_counts["sequence"] and held["digest"] == held_counts["sha256"], "PENDING")
            policy, original = CONFIG / "collection-policy.json", (CONFIG / "collection-policy.json").read_bytes()
            counts_before = (ROOT / "docker-receiver.json").read_bytes()
            try:
                changed = original.replace(b'"containers_enabled":true', b'"containers_enabled":false')
                need(changed != original, "TAMPER")
                policy.write_bytes(changed)
                try:
                    activation_guest.command(binary, "start")
                except activation_guest.Refused:
                    pass
                else:
                    raise AssertionError("TAMPER")
                need(pending() == held and (ROOT / "docker-receiver.json").read_bytes() == counts_before, "TAMPER")
            finally:
                policy.write_bytes(original)
            need(fingerprint() == before, "IDENTITY")
            docker_engine.stop()
            value = {"identity": before, "pending": held, "duplicates": held_counts["duplicates"], "reboot_requested_at_ns": time.time_ns()}
            (ROOT / "docker-result.json").write_text(json.dumps(value, separators=(",", ":")))
            (ROOT / "docker-result.json").chmod(0o600)
        else:
            prior = record(ROOT / "docker-result.json", 16384)
            need(prior["identity"] == before and prior["pending"] == pending(), "PENDING")
            rich_guest.retry_window(prior)
            (ROOT / "docker-hold").unlink()
            activation_guest.command(binary, "start")
            wait_counts(lambda v: v["duplicates"] > prior["duplicates"] and v["sequence"] == prior["pending"]["sequence"] and v["sha256"] == prior["pending"]["digest"], prior["pending"])
            rich_guest.pending_fresh(prior["pending"])
            wait_counts(lambda v: healthy(v) and v["sequence"] > prior["pending"]["sequence"])
            deadline = time.monotonic() + 35
            while json.loads(activation_guest.command(binary, "status"))["state"] != "ACKNOWLEDGED_FRESH":
                need(time.monotonic() < deadline, "RETRY"); time.sleep(0.5)
            isolation(config)
            socket_replacement(binary)
            activation_guest.command(binary, "stop"); stopped(binary)
            need(fingerprint() == before, "IDENTITY")
            docker_engine.stop()
        need(sum(v.stat().st_size for v in (STATE / "ledger").iterdir() if v.is_file()) <= 2097152, "BOUNDS")
        for directory, limit in (("docker-frame", 262144), ("handoff", 590848), ("reader-status", 16384), ("uploader-status", 16384)):
            need(sum(v.stat().st_size for v in (STATE / directory).iterdir() if v.is_file()) <= limit, "BOUNDS")
        rich_guest.verify_numeric()
    finally:
        rich_guest.join_sleepers(children)
        process.terminate(); process.wait(timeout=5)


def emit_failure(code):
    if code not in FAILURES and not (code in activation_guest.FAILURES and code.startswith(("START_", "STOP_", "STATUS_"))):
        code = "UNKNOWN"
    marker = "HLO_VM_FAIL_DOCKER_" + code + "\n"
    try:
        with Path("/dev/ttyS0").open("w", encoding="ascii") as serial:
            serial.write(marker)
    except OSError:
        print(marker, end="", file=sys.stderr)


def main():
    try:
        need(len(sys.argv) == 3, "INPUT")
        exercise(*sys.argv[1:])
        return 0
    except (OSError, ValueError, KeyError, TypeError, AttributeError, AssertionError, RuntimeError, sqlite3.Error, tarfile.TarError, subprocess.SubprocessError, activation_guest.Refused, preflight_probe.Refused) as error:
        code = "UNKNOWN"
        if isinstance(error, AssertionError) and str(error) in FAILURES:
            code = str(error)
        elif isinstance(error, activation_guest.Refused) and str(error) in activation_guest.FAILURES and str(error).startswith(("START_", "STOP_", "STATUS_")):
            code = str(error)
        emit_failure(code)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
