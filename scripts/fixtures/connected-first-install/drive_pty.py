#!/usr/bin/env python3
"""Synthetic terminal driver for the disposable guest only; never use real grants."""

import os
import pty
import json
import re
import select
import signal
import sys
import time

GRANT = b"hle_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n"
SERVER = "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
PREPARING = "/etc/home-lab-observer-connected/preparing.json"
STATE_ROOT = "/var/lib/home-lab-observer-connected"
RELEASE_ROOT = "/opt/home-lab-observer-connected"
MAX_OUTPUT = 8192
PHASE_LABELS = (
    (b"PREFLIGHT_REFUSED:", "PREFLIGHT_REFUSED"),
    (b"LOCAL_SETUP_INCOMPLETE:", "LOCAL_SETUP_INCOMPLETE"),
    (b"ENROLLMENT_RECOVERY_REQUIRED:", "ENROLLMENT_RECOVERY_REQUIRED"),
)


def append_bounded(output: bytearray, data: bytes) -> bool:
    if len(data) > MAX_OUTPUT - len(output):
        return False
    output.extend(data)
    return True


def classify_failure(output: bytes) -> str:
    found = [phase for label, phase in PHASE_LABELS if label in output]
    return found[0] if len(found) == 1 else "UNRECOGNIZED"


def normalized_exit(code: int) -> int:
    if 0 <= code <= 255:
        return code
    if -64 <= code < 0:
        return 128 - code
    return 255


def failure_marker(output: bytes, code: int, prompted: bool) -> str:
    return (
        f"FIXTURE_DRIVER_FAILURE PHASE={classify_failure(output)} "
        f"EXIT={normalized_exit(code)} PROMPT={int(prompted)}"
    )


def main() -> int:
    if (
        len(sys.argv) != 4
        or sys.argv[1] not in ("install", "interrupt")
        or not re.fullmatch(r"[a-f0-9]{64}", sys.argv[3])
    ):
        print("FIXTURE_DRIVER_ARGS_REFUSED", flush=True)
        return 22
    bundle, digest = sys.argv[2], sys.argv[3]
    if not bundle.startswith("/opt/observer-fixture/bundle/"):
        print("FIXTURE_DRIVER_BUNDLE_REFUSED", flush=True)
        return 22
    binary = os.path.join(bundle, "observer-connected-install")
    child, terminal = pty.fork()
    if child == 0:
        os.execv(binary, [binary, "install", bundle, digest, SERVER])

    prompted = False
    child_running = True
    output = bytearray()
    deadline = time.monotonic() + 180
    try:
        while time.monotonic() < deadline:
            if sys.argv[1] == "interrupt" and os.path.isfile(PREPARING):
                try:
                    with open(PREPARING, "rb") as marker:
                        record = json.load(marker)
                except (OSError, ValueError):
                    continue
                os.kill(child, signal.SIGSTOP)
                stopped, status = os.waitpid(child, os.WUNTRACED)
                if stopped != child or not os.WIFSTOPPED(status):
                    print("FIXTURE_DRIVER_STOP_REFUSED", flush=True)
                    return 1
                if record != {
                    "version": "observer-connected-preparing/v1",
                    "state": "PREPARING",
                    "server_id": SERVER,
                    "manifest_sha256": digest,
                } or os.path.exists(STATE_ROOT) or os.path.exists(RELEASE_ROOT):
                    print("FIXTURE_DRIVER_PHASE_REFUSED", flush=True)
                    return 1
                os.sync()
                print("HLO_VM_POWER_CUT_READY", flush=True)
                while True:
                    time.sleep(60)
            ready, _, _ = select.select([terminal], [], [], 0.01)
            if ready:
                try:
                    data = os.read(terminal, 4096)
                except OSError:
                    data = b""
                if data:
                    if not append_bounded(output, data):
                        print("FIXTURE_DRIVER_OUTPUT_OVERFLOW", flush=True)
                        return 1
                    if not prompted and b"One-use enrollment grant" in output:
                        prompted = True
                        os.write(terminal, GRANT)
            finished, status = os.waitpid(child, os.WNOHANG)
            if finished:
                child_running = False
                exit_code = os.waitstatus_to_exitcode(status)
                if exit_code == 0 and prompted and b"INSTALLED_PENDING_ACCEPTANCE" in output:
                    print("DRIVER_INSTALLED", flush=True)
                    return 0
                print(failure_marker(output, exit_code, prompted), flush=True)
                return 1
        print("FIXTURE_DRIVER_TIMEOUT_OR_CLOSED", flush=True)
        return 1
    finally:
        if child_running:
            try:
                os.killpg(child, signal.SIGKILL)
            except ProcessLookupError:
                pass
            try:
                os.waitpid(child, 0)
            except ChildProcessError:
                pass
        os.close(terminal)


if __name__ == "__main__":
    raise SystemExit(main())
