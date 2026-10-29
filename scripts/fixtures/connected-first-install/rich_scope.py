"""Closed, opt-in package admission for the disposable process-only scope."""
import json
from pathlib import Path
import re

PROFILE = "ubuntu24.04-systemd255-amd64-rich-processes-canary"
FILES = ("rich_scope.py", "rich_guest.py")


def inputs(scope, archive, archive_pin, manifest_pin, commit, owned_file, digest):
    values = (archive, archive_pin, manifest_pin)
    if scope != "rich-processes":
        if any(value is not None for value in values):
            raise RuntimeError("RICH_SCOPE_REFUSED")
        return None
    if not all(values) or any(re.fullmatch(r"[a-f0-9]{64}", value) is None for value in (archive_pin, manifest_pin)):
        raise RuntimeError("RICH_PINS_REFUSED")
    package = owned_file(archive, 600 * 1024 * 1024)
    manifest = owned_file(str(package.parent / "connected-manifest.json"), 16384)
    checksums = owned_file(str(package.parent / "SHA256SUMS"), 4096)
    helper = owned_file(str(package.parent / "install-connected-rich-processes-linux.sh"), 65536)
    if helper.stat().st_mode & 0o7777 != 0o644:
        raise RuntimeError("RICH_HELPER_REFUSED")
    if digest(package) != archive_pin or digest(manifest) != manifest_pin:
        raise RuntimeError("RICH_PACKAGE_REFUSED")
    expected = [f"{digest(package)}  {package.name}", f"{digest(manifest)}  connected-manifest.json", f"{digest(helper)}  {helper.name}"]
    if checksums.read_text(encoding="ascii").splitlines() != expected:
        raise RuntimeError("RICH_CHECKSUM_REFUSED")
    value = json.loads(manifest.read_bytes())
    if not isinstance(value, dict) or value.get("commit") != commit or value.get("profile") != PROFILE or value.get("schema") != "observer-connected-bundle/v2" or value.get("os") != "linux" or value.get("arch") != "amd64":
        raise RuntimeError("RICH_IDENTITY_REFUSED")
    return {"rich-package.tgz": package, "rich-manifest.json": manifest, "rich-SHA256SUMS": checksums}, {"commit": commit, "archive_sha256": archive_pin, "manifest_sha256": manifest_pin}


def cases(scope, stopped):
    if scope == "rich-processes":
        return ("rich-processes",)
    return stopped if scope == "stopped" else ((scope,) if scope in ("activation", "bootstrap") else (*stopped, "activation"))
