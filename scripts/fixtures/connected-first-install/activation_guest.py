#!/usr/bin/env python3
"""Fixed synthetic activation journey; only the no-NIC acceptance guest runs it."""

import hashlib
import json
import os
from pathlib import Path
import re
import select
import subprocess
import sys
import time

import assert_guest

ROOT = Path("/var/lib/hlo-first-install-fixture")
CONFIG = Path("/etc/home-lab-observer-connected/installed.json")
SECRET = Path("/etc/home-lab-observer-connected/credentials/connector.json")
LEDGER = Path("/var/lib/home-lab-observer-connected/ledger")
UNITS = ("home-lab-observer-connected-collector.service", "home-lab-observer-connected-uploader.service")
PHASES = frozenset(("REQUEST", "LEASE", "INSTALLED_STATE", "HOST_PROFILE", "PRINCIPALS", "SERVICE_STATE", "RUNTIME_GATE", "ACTIVATION_PREPARE", "ACTIVATION_CLEANUP", "UPLOADER_START", "UPLOADER_AUDIT", "REQUEST_PUBLICATION", "UPLOADER_RESPONSE", "COLLECTOR_START", "COLLECTOR_READY", "FINAL_AUDIT", "COMMIT_PUBLICATION", "UPLOADER_READY", "UNKNOWN"))
FAILURES = frozenset(("RECORD", "OPERATION", "COMMAND", "UPLOAD", "STOP", "PERSISTENCE", "BOUNDS", "INPUT", "BINDING")) | frozenset(operation + "_" + reason for operation in ("START", "STOP", "STATUS") for reason in ("COMMAND", "RECOVERY", *("COMMAND_" + phase for phase in PHASES), *("RECOVERY_" + phase for phase in PHASES)))


class Refused(Exception):
    pass


def read_json(path: Path) -> dict:
    with path.open("rb") as source:
        body = source.read(4097)
    if not body or len(body) > 4096:
        raise Refused("RECORD")
    value = json.loads(body)
    if not isinstance(value, dict):
        raise Refused("RECORD")
    return value


def command_failure(data: bytes) -> str:
    if data == b"RECOVERY_REQUIRED: preserve installed state and inspect the owned services before retrying.\n":
        return "RECOVERY"
    match = re.fullmatch(rb"RECOVERY_REQUIRED: ([A-Z_]+)\. Preserve installed state and inspect the owned services before retrying\.\n", data)
    if match and match[1].decode("ascii") in PHASES:
        return "RECOVERY_" + match[1].decode("ascii")
    match = re.fullmatch(rb"OPERATION_REFUSED: ([A-Z_]+)\. Verify the installed bundle, service state and policy\.\n", data)
    if match and match[1].decode("ascii") in PHASES:
        return "COMMAND_" + match[1].decode("ascii")
    return "COMMAND"


def worker_diagnostic(values: dict) -> str:
    """Only closed manager states and bounded numeric exit status reach serial."""
    if set(values) != {"ActiveState", "Result", "ExecMainCode", "ExecMainStatus"}:
        return "UNKNOWN"
    active = values["ActiveState"]
    result = values["Result"]
    if active not in ("inactive", "activating", "active", "deactivating", "failed") or result not in ("success", "exit-code", "signal", "core-dump", "timeout", "resources", "start-limit-hit", "protocol", "oom-kill"):
        return "UNKNOWN"
    code, status = values["ExecMainCode"], values["ExecMainStatus"]
    if re.fullmatch(r"[0-3]", code) is None or re.fullmatch(r"0|[1-9][0-9]{0,2}", status) is None or int(status) > 255:
        return "UNKNOWN"
    return f"{active}_{result}_CODE_{code}_STATUS_{status}".upper().replace("-", "_")


def diagnose_workers() -> None:
    fields = ("ActiveState", "Result", "ExecMainCode", "ExecMainStatus")
    for role, unit in zip(("COLLECTOR", "UPLOADER"), UNITS):
        try:
            code = worker_diagnostic(assert_guest.unit_properties(unit, fields))
        except (OSError, ValueError, subprocess.SubprocessError, AssertionError):
            code = "UNKNOWN"
        print("HLO_VM_DIAG_" + role + "_" + code, file=sys.stderr)


def command(binary: Path, operation: str) -> bytes:
    if operation not in ("start", "stop", "status"):
        raise Refused("OPERATION")
    process = subprocess.Popen([str(binary), operation], stdin=subprocess.DEVNULL,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                               env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL": "C"})
    data = bytearray()
    deadline = time.monotonic() + 95
    try:
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0 or not select.select([process.stdout], [], [], max(0, remaining))[0]:
                raise Refused("COMMAND")
            part = os.read(process.stdout.fileno(), 4097 - len(data))
            if not part:
                break
            data.extend(part)
            if len(data) > 4096:
                raise Refused("COMMAND")
        if process.wait(timeout=max(0.1, deadline - time.monotonic())) != 0:
            raise Refused(operation.upper() + "_" + command_failure(bytes(data)))
        return bytes(data)
    finally:
        if process.poll() is None:
            process.kill()
            process.wait(timeout=5)
        process.stdout.close()


def fingerprint() -> dict:
    return {"config": hashlib.sha256(CONFIG.read_bytes()).hexdigest(),
            "credential": hashlib.sha256(SECRET.read_bytes()).hexdigest(),
            "ledger_inode": LEDGER.stat().st_ino}


def wait_fresh(binary: Path, after: int) -> int:
    deadline = time.monotonic() + 115
    while time.monotonic() < deadline:
        status = json.loads(command(binary, "status"))
        try:
            counts = read_json(ROOT / "receiver-counts.json")
        except (FileNotFoundError, json.JSONDecodeError):
            counts = {}
        if status.get("state") == "ACKNOWLEDGED_FRESH" and counts.get("sequence", 0) > after and counts.get("useful_host") is True:
            return counts["sequence"]
        if status.get("state") in ("RECOVERY_REQUIRED", "CREDENTIAL_REJECTED"):
            raise Refused("UPLOAD")
        time.sleep(1)
    raise Refused("UPLOAD")


def stopped(binary: Path) -> None:
    command(binary, "stop")
    if json.loads(command(binary, "status")).get("state") != "STOPPED":
        raise Refused("STOP")
    for unit in UNITS:
        values = assert_guest.unit_properties(unit, ("ActiveState", "MainPID", "UnitFileState"))
        if values != {"ActiveState": "inactive", "MainPID": "0", "UnitFileState": "disabled"}:
            raise Refused("STOP")


def exercise(mode: str) -> None:
    if mode not in ("first", "reboot") or os.geteuid() != 0:
        raise Refused("INPUT")
    config = read_json(CONFIG)
    digest = config.get("artifact_sha256", "")
    if re.fullmatch(r"[a-f0-9]{64}", digest) is None or config.get("server_id") != assert_guest.SERVER:
        raise Refused("BINDING")
    binary = Path("/opt/home-lab-observer-connected/releases") / digest / "observer-connected-install"
    before = fingerprint()
    prior = 0
    if mode == "reboot":
        previous = read_json(ROOT / "activation-result.json")
        if previous["identity"] != before:
            raise Refused("PERSISTENCE")
        prior = previous["sequence"]
    if json.loads(command(binary, "status")).get("state") != "STOPPED":
        raise Refused("STOP")
    command(binary, "start")
    first = wait_fresh(binary, prior)
    stopped(binary)
    if mode == "first":
        command(binary, "start")
        sequence = wait_fresh(binary, first)
        stopped(binary)
    else:
        sequence = first
    if fingerprint() != before:
        raise Refused("PERSISTENCE")
    if sum(path.stat().st_size for path in LEDGER.iterdir() if path.is_file()) > 1024 * 1024:
        raise Refused("BOUNDS")
    result = ROOT / "activation-result.json"
    result.write_text(json.dumps({"identity": before, "sequence": sequence}, separators=(",", ":")))
    result.chmod(0o600)


def main() -> int:
    try:
        if len(sys.argv) != 2:
            raise Refused("INPUT")
        exercise(sys.argv[1])
        return 0
    except (OSError, ValueError, KeyError, TypeError, AttributeError, subprocess.SubprocessError, AssertionError, Refused) as error:
        # Never propagate raw command output, credential bytes or host records.
        diagnose_workers()
        code = str(error) if isinstance(error, Refused) and str(error) in FAILURES else "UNKNOWN"
        print("HLO_VM_FAIL_ACTIVATION_" + code, file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
