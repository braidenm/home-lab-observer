#!/usr/bin/env python3
"""Read-only, fixed-code prerequisite diagnostics inside the disposable guest.

This is not an installer admission decision. It cannot authorize enrollment and
never prints manager output, certificate data, paths from the guest, or secrets.
"""

import grp
import ipaddress
import errno
import os
from pathlib import Path
import pwd
import select
import signal
import socket
import ssl
import stat
import subprocess
import sys
import time


HOST = "app.braidenmiller.com"
EXPECTED_IP = "93.184.216.34"
SYSTEMCTL = "/usr/bin/systemctl"
MAX_OUTPUT = 4096
UNIT_NAMES = (
    "home-lab-observer-connected-collector.service",
    "home-lab-observer-connected-uploader.service",
    "home-lab-observer-connected-enrollment.service",
)
FAILURES = frozenset(("HOST", "NSS", "TARGETS", "DNS_TLS", "PARENT", "UNKNOWN"))


class Refused(Exception):
    def __init__(self, code: str):
        self.code = code if code in FAILURES else "UNKNOWN"


def marker(code: str) -> str:
    return "HLO_VM_FAIL_PREFLIGHT_" + (code if code in FAILURES else "UNKNOWN")


def bounded(data: bytes, limit: int = MAX_OUTPUT) -> bytes:
    if len(data) > limit or b"\x00" in data or b"\r" in data:
        raise Refused("UNKNOWN")
    return data


def command(*args: str, category: str = "UNKNOWN", limit: int = MAX_OUTPUT) -> bytes:
    process = subprocess.Popen(args, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                               stderr=subprocess.DEVNULL, env={"PATH": "/usr/bin:/bin", "LC_ALL": "C"})
    output = bytearray()
    deadline = time.monotonic() + 5
    try:
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise Refused("UNKNOWN")
            ready, _, _ = select.select([process.stdout], [], [], remaining)
            if not ready:
                raise Refused("UNKNOWN")
            block = os.read(process.stdout.fileno(), limit + 1 - len(output))
            if not block:
                break
            output.extend(block)
            bounded(output, limit)
        if process.wait(timeout=max(0.1, deadline - time.monotonic())) != 0:
            raise Refused(category)
        return bounded(output, limit)
    finally:
        if process.poll() is None:
            process.kill()
            process.wait(timeout=5)
        process.stdout.close()


def fixed_file(path: str, limit: int) -> bytes:
    with open(path, "rb") as handle:
        data = handle.read(limit + 1)
    if not data or len(data) > limit:
        raise Refused("UNKNOWN")
    return data


def supported_nss(data: bytes) -> bool:
    if not data or len(data) > 64 * 1024 or b"\r" in data or b"\x00" in data:
        return False
    seen = set()
    for raw in data.decode("utf-8", errors="strict").split("\n"):
        line = raw.split("#", 1)[0]
        key, separator, value = line.partition(":")
        key = key.strip()
        if key == "initgroups":
            return False
        if key not in ("passwd", "group", "shadow", "gshadow"):
            continue
        if not separator or key in seen or value.split() not in (["files"], ["files", "systemd"]):
            return False
        seen.add(key)
    return seen == {"passwd", "group", "shadow", "gshadow"}


def fields(data: bytes, names: set[str], category: str) -> dict[str, str]:
    text = bounded(data).decode("ascii", errors="strict")
    parsed = {}
    for line in text.removesuffix("\n").split("\n"):
        key, separator, value = line.partition("=")
        if not separator or key not in names or key in parsed:
            raise Refused(category)
        parsed[key] = value
    if set(parsed) != names:
        raise Refused(category)
    return parsed


def root_path(path: str, directory: bool = False) -> None:
    current = Path("/")
    for part in ("", *Path(path).parts[1:]):
        if part:
            current /= part
        info = current.lstat()
        if (info.st_uid != 0 or info.st_mode & 0o022 or info.st_mode & 0o7000
                or stat.S_ISLNK(info.st_mode)):
            raise Refused("HOST")
        for attr in ("system.posix_acl_access", "system.posix_acl_default"):
            try:
                os.getxattr(current, attr, follow_symlinks=False)
            except OSError as error:
                if error.errno not in (errno.ENODATA, errno.ENOTSUP):
                    raise Refused("HOST") from None
            else:
                raise Refused("HOST")
        if current != Path(path) and not stat.S_ISDIR(info.st_mode):
            raise Refused("HOST")
    if (directory and not stat.S_ISDIR(info.st_mode)) or (not directory and
            (not stat.S_ISREG(info.st_mode) or info.st_nlink != 1)):
        raise Refused("HOST")


def host_nss() -> None:
    for path in ("/usr/lib/os-release", "/etc/nsswitch.conf", "/etc/ssl/certs/ca-certificates.crt"):
        root_path(path)
    release = fixed_file("/usr/lib/os-release", 4096).decode("utf-8", errors="strict")
    values = {}
    for line in release.split("\n"):
        key, separator, value = line.partition("=")
        if separator and key in ("ID", "VERSION_ID"):
            if key in values:
                raise Refused("HOST")
            values[key] = value.strip('"')
    if values != {"ID": "ubuntu", "VERSION_ID": "24.04"}:
        raise Refused("HOST")
    if not Path("/sys/fs/cgroup/cgroup.controllers").is_file():
        raise Refused("HOST")
    for path in ("/etc", "/var/lib", "/opt", "/etc/systemd/system"):
        root_path(path, directory=True)
    version = command(SYSTEMCTL, "--version", category="HOST").splitlines()
    if not version or version[0].split()[:2] != [b"systemd", b"255"]:
        raise Refused("HOST")
    if not supported_nss(fixed_file("/etc/nsswitch.conf", 64 * 1024)):
        raise Refused("NSS")


def absent_targets() -> None:
    for path in ("/etc/home-lab-observer-connected", "/var/lib/home-lab-observer-connected",
                 "/opt/home-lab-observer-connected", *("/etc/systemd/system/" + unit for unit in UNIT_NAMES[:2]),
                 *("/etc/systemd/system/" + unit + ".d" for unit in UNIT_NAMES[:2])):
        if os.path.lexists(path):
            raise Refused("TARGETS")
    for name in ("hlo-connected-collector", "hlo-connected-uploader"):
        try:
            pwd.getpwnam(name)
        except KeyError:
            pass
        else:
            raise Refused("TARGETS")
    for name in ("hlo-connected-read", "hlo-connected-uploader"):
        try:
            grp.getgrnam(name)
        except KeyError:
            pass
        else:
            raise Refused("TARGETS")
    names = {"Id", "LoadState", "ActiveState", "UnitFileState", "FragmentPath", "DropInPaths", "NeedDaemonReload"}
    for unit in UNIT_NAMES:
        state = fields(command(SYSTEMCTL, "show", "--no-pager", *("--property=" + key for key in names),
                               "--", unit, category="TARGETS", limit=1024), names, "TARGETS")
        if state["Id"] != unit or state["LoadState"] != "not-found":
            raise Refused("TARGETS")


def dns_tls() -> None:
    # The probe mirrors the installer's fixed-name TLS reachability, sends no HTTP
    # request, and accepts only the guest's synthetic public-address loopback alias.
    def timeout(_signal, _frame):
        raise Refused("DNS_TLS")

    prior = signal.signal(signal.SIGALRM, timeout)
    signal.setitimer(signal.ITIMER_REAL, 5)
    try:
        addresses = {item[4][0] for item in socket.getaddrinfo(HOST + ".", 443, type=socket.SOCK_STREAM)}
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, prior)
    if addresses != {EXPECTED_IP} or not ipaddress.ip_address(EXPECTED_IP).is_global:
        raise Refused("DNS_TLS")
    context = ssl.create_default_context(cafile="/etc/ssl/certs/ca-certificates.crt")
    with socket.create_connection((EXPECTED_IP, 443), timeout=3) as connection:
        with context.wrap_socket(connection, server_hostname=HOST):
            pass


def parent_policy() -> None:
    names = {"Id", "Slice", "IPAddressAllow", "IPAddressDeny", "DropInPaths", "LoadState", "NeedDaemonReload"}
    for unit, parent in (("system.slice", "-.slice"), ("-.slice", "")):
        state = fields(command(SYSTEMCTL, "show", "--no-pager", *("--property=" + key for key in names),
                               "--", unit, category="PARENT"), names, "PARENT")
        if (state["Id"] != unit or state["Slice"] != parent or state["LoadState"] != "loaded"
                or state["NeedDaemonReload"] != "no"):
            raise Refused("PARENT")
        for field in ("IPAddressAllow", "IPAddressDeny"):
            values = state[field].split()
            if len(values) > 128 or " ".join(values) != state[field]:
                raise Refused("PARENT")
            for value in values:
                network = ipaddress.ip_network(value, strict=True)
                if str(network) != value or (field == "IPAddressAllow" and value != EXPECTED_IP + "/32"):
                    raise Refused("PARENT")


def main() -> int:
    try:
        for category, check in (("HOST", host_nss), ("TARGETS", absent_targets),
                                ("DNS_TLS", dns_tls), ("PARENT", parent_policy)):
            try:
                check()
            except Refused:
                raise
            except (OSError, UnicodeError, ValueError, ssl.SSLError, subprocess.SubprocessError):
                raise Refused(category) from None
    except Refused as error:
        print(marker(error.code), file=sys.stderr, flush=True)
        return 1
    except Exception:
        print(marker("UNKNOWN"), file=sys.stderr, flush=True)
        return 1
    print("HLO_VM_PREFLIGHT_PASS", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
