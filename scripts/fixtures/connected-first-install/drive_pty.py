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
                        json.load(marker)
                except (OSError, ValueError):
                    continue
                os.kill(child, signal.SIGSTOP)
                stopped, status = os.waitpid(child, os.WUNTRACED)
                if stopped != child or not os.WIFSTOPPED(status):
                    print("FIXTURE_DRIVER_STOP_REFUSED", flush=True)
                    return 1
                os.sync()
                print("POWER_CUT_READY", flush=True)
                while True:
                    time.sleep(60)
            ready, _, _ = select.select([terminal], [], [], 0.01)
            if ready:
                try:
                    data = os.read(terminal, 4096)
                except OSError:
                    data = b""
                if data:
                    output.extend(data)
                    if len(output) > 8192:
                        print("FIXTURE_DRIVER_OUTPUT_OVERFLOW", flush=True)
                        return 1
                    if not prompted and b"One-use enrollment grant" in output:
                        prompted = True
                        os.write(terminal, GRANT)
            finished, status = os.waitpid(child, os.WNOHANG)
            if finished:
                child_running = False
                if os.waitstatus_to_exitcode(status) == 0 and prompted and b"INSTALLED_PENDING_ACCEPTANCE" in output:
                    print("DRIVER_INSTALLED", flush=True)
                    return 0
                print("FIXTURE_DRIVER_INSTALL_FAILED", flush=True)
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
