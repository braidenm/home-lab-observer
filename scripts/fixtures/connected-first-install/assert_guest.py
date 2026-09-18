#!/usr/bin/env python3
"""Read-only, secret-redacting assertions inside the disposable no-NIC VM."""

import errno
import hashlib
import json
import os
from pathlib import Path
import pwd
import grp
import stat
import subprocess
import sys
import tarfile

PAYLOAD = Path("/mnt/hlo-first-install")
FIXTURE = Path("/var/lib/hlo-first-install-fixture")
CONFIG = Path("/etc/home-lab-observer-connected")
STATE = Path("/var/lib/home-lab-observer-connected")
RELEASES = Path("/opt/home-lab-observer-connected/releases")
SERVER = "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"


def need(condition: bool, code: str) -> None:
    if not condition:
        raise AssertionError(code)


def digest(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def plain(path: Path, mode: int, uid: int, gid: int, directory: bool = False) -> os.stat_result:
    info = path.lstat()
    need(stat.S_ISDIR(info.st_mode) if directory else stat.S_ISREG(info.st_mode), "TYPE")
    need(info.st_uid == uid and info.st_gid == gid and stat.S_IMODE(info.st_mode) == mode, "OWNERSHIP")
    need(directory or info.st_nlink == 1, "HARDLINK")
    for attr in ("system.posix_acl_access", "system.posix_acl_default"):
        try:
            os.getxattr(path, attr, follow_symlinks=False)
        except OSError as error:
            need(error.errno in (errno.ENODATA, errno.ENOTSUP), "ACL_QUERY")
        else:
            need(False, "ACL")
    return info


def names(path: Path, expected: set[str]) -> None:
    need({entry.name for entry in path.iterdir()} == expected, "UNKNOWN_MEMBER")


def run(*args: str) -> str:
    return subprocess.check_output(args, text=True, timeout=10, stderr=subprocess.DEVNULL)


def payload_assert() -> None:
    identity = json.loads((PAYLOAD / "reviewed-identity.json").read_text())
    expected = identity["manifest_sha256"]
    archive_files = list(PAYLOAD.glob("*.tar.gz"))
    need(len(archive_files) == 1, "ARCHIVE_COUNT")
    archive = archive_files[0]
    need(digest(archive) == identity["archive_sha256"] and digest(PAYLOAD / "connected-manifest.json") == expected and digest(PAYLOAD / "connected-first-install-receiver") == identity["receiver_sha256"], "PAYLOAD_PIN")
    manifest = json.loads((PAYLOAD / "connected-manifest.json").read_text())
    need(manifest["commit"] == identity["commit"] and manifest["schema"] == "observer-connected-bundle/v2", "PAYLOAD_IDENTITY")
    expected_files = {entry["name"]: (entry["size"], entry["mode"], entry["sha256"]) for entry in manifest["files"]}
    expected_files["connected-manifest.json"] = ((PAYLOAD / "connected-manifest.json").stat().st_size, 0o644, expected)
    with tarfile.open(archive, "r:gz") as entries:
        members = entries.getmembers()
        need(len(members) == len(expected_files) and {member.name for member in members} == set(expected_files), "ARCHIVE_MEMBERS")
        for member in members:
            size, mode, wanted = expected_files[member.name]
            need(member.isfile() and member.size == size and member.mode == mode and member.uid == 0 and member.gid == 0 and member.name == Path(member.name).name, "ARCHIVE_MEMBER_METADATA")
            file = entries.extractfile(member)
            need(file is not None, "ARCHIVE_MEMBER_READ")
            content = file.read(size + 1)
            need(len(content) == size and hashlib.sha256(content).hexdigest() == wanted, "ARCHIVE_MEMBER_BYTES")


def reviewed() -> tuple[str, str, dict]:
    identity = json.loads((PAYLOAD / "reviewed-identity.json").read_text())
    commit, expected, receiver, archive = identity["commit"], identity["manifest_sha256"], identity["receiver_sha256"], identity["archive_sha256"]
    need(len(commit) == 40 and len(expected) == 64 and len(receiver) == 64 and len(archive) == 64, "REVIEWED_IDENTITY")
    payload_assert()
    source = PAYLOAD / "connected-manifest.json"
    need(digest(source) == expected, "REVIEWED_MANIFEST")
    manifest = json.loads(source.read_text())
    need(manifest["commit"] == commit and manifest["schema"] == "observer-connected-bundle/v2" and manifest["profile"] == "ubuntu24.04-systemd255-amd64-canary", "MANIFEST_IDENTITY")
    bundle = Path((FIXTURE / "bundle-path").read_text().strip())
    need(bundle.is_dir() and digest(bundle / "connected-manifest.json") == expected, "EXTRACTED_MANIFEST")
    expected_files = {entry["name"] for entry in manifest["files"]}
    names(bundle, expected_files | {"connected-manifest.json"})
    for entry in manifest["files"]:
        target = bundle / entry["name"]
        plain(target, entry["mode"], 0, 0)
        need(target.stat().st_size == entry["size"] and digest(target) == entry["sha256"], "BUNDLE_FILE")
    return commit, expected, manifest


def snapshot() -> dict:
    _, expected, manifest = reviewed()
    release = RELEASES / expected
    plain(RELEASES, 0o755, 0, 0, True)
    names(RELEASES, {expected})
    plain(release, 0o755, 0, 0, True)
    names(release, {entry["name"] for entry in manifest["files"]} | {"connected-manifest.json"})
    plain(release / "connected-manifest.json", 0o644, 0, 0)
    for entry in manifest["files"]:
        target = release / entry["name"]
        plain(target, entry["mode"], 0, 0)
        need(target.stat().st_size == entry["size"] and digest(target) == entry["sha256"], "RELEASE_FILE")
    need(digest(release / "connected-manifest.json") == expected, "RELEASE_MANIFEST")
    plain(CONFIG, 0o755, 0, 0, True)
    names(CONFIG, {"preparing.json", "installed.json", "credentials"})
    plain(CONFIG / "preparing.json", 0o600, 0, 0)
    marker = json.loads((CONFIG / "preparing.json").read_text())
    need(marker == {"version": "observer-connected-preparing/v1", "state": "PREPARING", "server_id": SERVER, "manifest_sha256": expected}, "PREPARING_BINDING")
    installed = CONFIG / "installed.json"
    installed_stat = plain(installed, 0o644, 0, 0)
    profile = json.loads(installed.read_text())
    collector = pwd.getpwnam("hlo-connected-collector")
    uploader = pwd.getpwnam("hlo-connected-uploader")
    shared = grp.getgrnam("hlo-connected-read")
    need(profile["version"] == "observer-connected-install/v1" and profile["state"] == "INSTALLED_READY" and profile["server_id"] == SERVER and profile["artifact_sha256"] == expected and profile["policy_generation"] == 1, "CONFIG_BINDING")
    need(profile["collector_uid"] == collector.pw_uid and profile["uploader_uid"] == uploader.pw_uid and profile["uploader_gid"] == uploader.pw_gid and profile["shared_gid"] == shared.gr_gid, "PRINCIPAL_BINDING")
    need(profile["addresses"] == ["93.184.216.34"] and profile["connector_id"].startswith("agent_"), "NETWORK_BINDING")
    plain(CONFIG / "credentials", 0o700, 0, 0, True)
    names(CONFIG / "credentials", {"connector.json"})
    credential = CONFIG / "credentials/connector.json"
    credential_stat = plain(credential, 0o600, 0, 0)
    credential_record = json.loads(credential.read_text())
    need(credential_record.get("version") == "observer-connected-credential/v1" and credential_record.get("server_id") == SERVER and credential_record.get("connector_id") == profile["connector_id"] and credential_record.get("secret", "").startswith("hlc_"), "CREDENTIAL_BINDING")
    plain(STATE, 0o755, 0, 0, True)
    names(STATE, {"handoff", "collector-status", "uploader-status", "enrollment", "ledger", "uploader-root"})
    plain(STATE / "handoff", 0o750, collector.pw_uid, shared.gr_gid, True)
    plain(STATE / "collector-status", 0o700, collector.pw_uid, shared.gr_gid, True)
    plain(STATE / "uploader-status", 0o700, uploader.pw_uid, uploader.pw_gid, True)
    plain(STATE / "enrollment", 0o700, 0, 0, True)
    names(STATE / "enrollment", set())
    uploader_root = STATE / "uploader-root"
    plain(uploader_root, 0o755, 0, 0, True)
    names(uploader_root, {"bin", "etc", "state", "handoff", "activation"})
    names(uploader_root / "etc", {"ssl", "home-lab-observer-connected", "hosts", "resolv.conf", "nsswitch.conf"})
    names(uploader_root / "state", {"enrollment", "ledger", "status"})
    ledger = STATE / "ledger"
    ledger_stat = plain(ledger, 0o700, uploader.pw_uid, uploader.pw_gid, True)
    ledger_files = {}
    for entry in ledger.iterdir():
        need(entry.is_file() and not entry.is_symlink(), "LEDGER_TYPE")
        info = plain(entry, 0o600, uploader.pw_uid, uploader.pw_gid)
        ledger_files[entry.name] = [info.st_ino, info.st_size, digest(entry)]
    need(bool(ledger_files), "LEDGER_EMPTY")
    units = ("home-lab-observer-connected-collector.service", "home-lab-observer-connected-uploader.service")
    substitutions = {
        "CollectorUID": str(collector.pw_uid),
        "UploaderUID": str(uploader.pw_uid),
        "UploaderGID": str(uploader.pw_gid),
        "SharedGID": str(shared.gr_gid),
        "ArtifactSHA256": expected,
        "AddressRules": "IPAddressAllow=93.184.216.34/32\n",
    }
    for unit in units:
        text = Path("/etc/systemd/system", unit).read_text()
        template = (release / (unit + ".tmpl")).read_text()
        for key, value in substitutions.items():
            template = template.replace("{{." + key + "}}", value)
        need("{{" not in template and text == template, "UNIT_PROFILE")
        need("LoadState=loaded" in run("/usr/bin/systemctl", "show", "--property=LoadState", unit), "UNIT_LOAD")
    collector_unit = Path("/etc/systemd/system/home-lab-observer-connected-collector.service").read_text()
    uploader_unit = Path("/etc/systemd/system/home-lab-observer-connected-uploader.service").read_text()
    need("PrivateNetwork=yes\n" in collector_unit and "LoadCredential=" not in collector_unit and "InaccessiblePaths=-/etc/home-lab-observer-connected/credentials" in collector_unit, "COLLECTOR_ISOLATION")
    need("RootDirectory=" + str(STATE / "uploader-root") in uploader_unit and "LoadCredential=connector.json:/etc/home-lab-observer-connected/credentials/connector.json" in uploader_unit and "IPAddressDeny=any\n" in uploader_unit and "IPAddressAllow=93.184.216.34/32\n" in uploader_unit, "UPLOADER_ISOLATION")
    effective_collector = run("/usr/bin/systemctl", "show", "--property=PrivateNetwork", "--property=RootDirectory", "--property=BindPaths", "--property=LoadCredential", "--property=NoNewPrivileges", units[0])
    effective_uploader = run("/usr/bin/systemctl", "show", "--property=RootDirectory", "--property=BindPaths", "--property=BindReadOnlyPaths", "--property=LoadCredential", "--property=NoNewPrivileges", "--property=IPAddressDeny", units[1])
    need("PrivateNetwork=yes\n" in effective_collector and "RootDirectory=\n" in effective_collector and "LoadCredential=\n" in effective_collector and "BindPaths=\n" in effective_collector and "NoNewPrivileges=yes\n" in effective_collector, "COLLECTOR_EFFECTIVE")
    need("RootDirectory=" + str(STATE / "uploader-root") in effective_uploader and "connector.json" in effective_uploader and str(STATE / "ledger") in effective_uploader and "NoNewPrivileges=yes\n" in effective_uploader and "IPAddressDeny=any\n" in effective_uploader, "UPLOADER_EFFECTIVE")
    transient = run("/usr/bin/systemctl", "show", "--property=LoadState", "--property=ActiveState", "--property=Transient", "--property=FragmentPath", "home-lab-observer-connected-enrollment.service")
    need("LoadState=not-found\n" in transient and "ActiveState=inactive\n" in transient and "Transient=no\n" in transient and "FragmentPath=\n" in transient, "TRANSIENT_RETAINED")
    return {"installed": [installed_stat.st_ino, installed_stat.st_size, digest(installed)], "credential": [credential_stat.st_ino, credential_stat.st_size, digest(credential)], "ledger": [ledger_stat.st_ino, ledger_files]}


def recovery() -> None:
    _, expected, _ = reviewed()
    plain(CONFIG, 0o755, 0, 0, True)
    names(CONFIG, {"preparing.json"})
    plain(CONFIG / "preparing.json", 0o600, 0, 0)
    marker = json.loads((CONFIG / "preparing.json").read_text())
    need(marker == {"version": "observer-connected-preparing/v1", "state": "PREPARING", "server_id": SERVER, "manifest_sha256": expected}, "RECOVERY_MARKER")
    need(not STATE.exists() and not (RELEASES.parent).exists(), "RECOVERY_LATE_CUT")
    for unit in ("home-lab-observer-connected-collector.service", "home-lab-observer-connected-uploader.service", "home-lab-observer-connected-enrollment.service"):
        need("LoadState=not-found" in run("/usr/bin/systemctl", "show", "--property=LoadState", unit), "RECOVERY_UNIT")
    for name in ("hlo-connected-collector", "hlo-connected-uploader"):
        try:
            pwd.getpwnam(name)
        except KeyError:
            pass
        else:
            need(False, "RECOVERY_ACCOUNT")
    for name in ("hlo-connected-read", "hlo-connected-uploader"):
        try:
            grp.getgrnam(name)
        except KeyError:
            pass
        else:
            need(False, "RECOVERY_GROUP")


def main() -> None:
    mode = sys.argv[1]
    if mode == "payload":
        payload_assert()
    elif mode == "preflight":
        reviewed()
    elif mode == "capture":
        data = snapshot()
        (FIXTURE / "snapshot.json").write_text(json.dumps(data, sort_keys=True))
    elif mode == "verify":
        before = json.loads((FIXTURE / "snapshot.json").read_text())
        need(snapshot() == before, "REBOOT_CHANGED_STATE")
    elif mode == "recovery":
        recovery()
    else:
        raise AssertionError("MODE")


if __name__ == "__main__":
    try:
        main()
    except (AssertionError, OSError, KeyError, ValueError, subprocess.SubprocessError) as error:
        code = str(error) if isinstance(error, AssertionError) else "ASSERTION_OPERATION"
        print(f"FIXTURE_ASSERT_{code}", file=sys.stderr)
        raise SystemExit(1)
