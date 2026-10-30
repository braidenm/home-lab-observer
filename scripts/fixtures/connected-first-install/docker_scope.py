"""Closed public-input admission for the explicit no-NIC Docker VM scope."""
import io
import json
from pathlib import Path
import re
import tarfile

PROFILE = "ubuntu24.04-systemd255-amd64-rich-processes-docker-canary"
BINARIES = tuple("observer-connected-rich-docker-" + role for role in ("collector", "reader", "uploader", "install"))
FILES = ("docker_scope.py", "docker_engine.py", "docker_guest.py")
# Verified signed Docker Noble stable Packages; provenance is in Spec013.
PACKAGES = {
    "docker-ce_29.9.2-1~ubuntu.24.04~noble_amd64.deb": (24558208, "95ddf7a2d8b8c0eb82d8597d4d604e294dcf741081f2e228524d5ab9bfd7c8ae"),
    "docker-ce-cli_29.9.2-1~ubuntu.24.04~noble_amd64.deb": (17595236, "74747b5c99000279b9b2c7eb82543b4121d8436a58aa3be7e78ac6681d74794f"),
    "containerd.io_2.4.2-3~ubuntu.24.04~noble_amd64.deb": (23271468, "f811ab57d1f4bb0cce19df7d526da8c711d8ebea0447cc83da1999188e664122"),
}


def refuse(code):
    raise RuntimeError(code)


def cases(scope):
    return ("rich-docker", "rich-docker-refusal") if scope == "rich-docker" else None


def rootfs_bytes(binary):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w", format=tarfile.USTAR_FORMAT) as archive:
        member = tarfile.TarInfo("fixture")
        member.mode, member.size, member.mtime = 0o555, len(binary), 0
        archive.addfile(member, io.BytesIO(binary))
    return output.getvalue()


def validate_rootfs(raw, binary):
    # Canonical reconstruction rejects extra members, links, hidden trailing
    # bytes, metadata extensions and ambiguous tar parsing without extraction.
    if not binary.startswith(b"\x7fELF") or raw != rootfs_bytes(binary):
        refuse("DOCKER_ROOTFS_REFUSED")


def check_identity(value, commit):
    if not isinstance(value, dict) or value.get("commit") != commit or value.get("profile") != PROFILE or value.get("schema") != "observer-connected-bundle/v2" or value.get("os") != "linux" or value.get("arch") != "amd64":
        refuse("DOCKER_IDENTITY_REFUSED")
    names = [item.get("name") for item in value.get("files", []) if isinstance(item, dict)]
    if len(names) != len(set(names)) or sorted(name for name in names if name in BINARIES) != sorted(BINARIES):
        refuse("DOCKER_IDENTITY_REFUSED")


def inputs(archive, archive_pin, manifest_pin, commit, owned_file, digest):
    if not archive or any(not isinstance(pin, str) or re.fullmatch(r"[a-f0-9]{64}", pin) is None for pin in (archive_pin, manifest_pin)):
        refuse("DOCKER_PINS_REFUSED")
    package = owned_file(archive, 600 * 1024 * 1024)
    manifest = owned_file(str(package.parent / "connected-manifest.json"), 16384)
    checksums = owned_file(str(package.parent / "SHA256SUMS"), 4096)
    helper = owned_file(str(package.parent / "install-connected-rich-processes-docker-linux.sh"), 65536)
    if helper.stat().st_mode & 0o7777 != 0o644 or digest(package) != archive_pin or digest(manifest) != manifest_pin:
        refuse("DOCKER_PACKAGE_REFUSED")
    expected = [f"{digest(v)}  {v.name}" for v in (package, manifest, helper)]
    if checksums.read_text(encoding="ascii").splitlines() != expected:
        refuse("DOCKER_CHECKSUM_REFUSED")
    check_identity(json.loads(manifest.read_bytes()), commit)
    return {"docker-package.tgz": package, "docker-manifest.json": manifest, "docker-SHA256SUMS": checksums}, {"commit": commit, "archive_sha256": archive_pin, "manifest_sha256": manifest_pin}


def engine_inputs(directory, rootfs_pin, binary_pin, commit, owned_file, digest):
    if not directory or any(not isinstance(pin, str) or re.fullmatch(r"[a-f0-9]{64}", pin) is None for pin in (rootfs_pin, binary_pin)) or not isinstance(commit, str) or re.fullmatch(r"[a-f0-9]{40}", commit) is None:
        refuse("DOCKER_PINS_REFUSED")
    root = Path(directory)
    image = owned_file(str(root / "fixture-rootfs.tar"), 10 * 1024 * 1024)
    binary = owned_file(str(root / "fixture"), 8 * 1024 * 1024)
    if digest(image) != rootfs_pin or digest(binary) != binary_pin:
        refuse("DOCKER_IMAGE_REFUSED")
    validate_rootfs(image.read_bytes(), binary.read_bytes())
    sources = {"fixture-rootfs.tar": image, "fixture": binary}
    for name, (size, pin) in PACKAGES.items():
        package = owned_file(str(root / name), size)
        if package.stat().st_size != size or digest(package) != pin:
            refuse("DOCKER_DEB_REFUSED")
        sources[name] = package
    return sources, {"source_commit": commit, "rootfs_sha256": rootfs_pin, "fixture_sha256": binary_pin}
