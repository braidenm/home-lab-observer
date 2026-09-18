#!/usr/bin/env python3
"""Manual, no-NIC, disposable QEMU acceptance for the stopped installer."""

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import time

MIB = 1024 * 1024
GIB = 1024 * MIB
CASE_TIMEOUT = 9 * 60
CASES = ("success", "interrupt", "success-cut")
SAFE_PATH = "/usr/sbin:/usr/bin:/sbin:/bin"
TOOLS: dict[str, str] = {}


def refuse(reason: str) -> None:
    raise RuntimeError(reason)


def qemu_safe(path: Path) -> bool:
    return not any(char in str(path) for char in (",", ":", "\n", "\r", "\x00"))


def trusted_ancestors(path: Path) -> bool:
    for ancestor in path.parents:
        info = ancestor.stat()
        if info.st_uid != 0 or info.st_mode & 0o022:
            return False
    return True


def owned_file(raw: str, limit: int) -> Path:
    path = Path(raw)
    if not path.is_absolute() or path.is_symlink() or not path.is_file():
        refuse("INPUT_PATH_REFUSED")
    resolved = path.resolve(strict=True)
    info = resolved.stat()
    if resolved != path or not qemu_safe(resolved) or not trusted_ancestors(resolved) or info.st_uid != 0 or info.st_mode & 0o022 or info.st_size < 1 or info.st_size > limit:
        refuse("INPUT_OWNERSHIP_OR_SIZE_REFUSED")
    return resolved


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(MIB), b""):
            digest.update(block)
    return digest.hexdigest()


def qemu_process(command: bytes, comm: str) -> bool:
    executable = Path(os.fsdecode(command)).name
    return executable.startswith("qemu-system-") or executable in ("qemu-kvm", "kvm") or comm.startswith("qemu-system-") or comm in ("qemu-kvm", "kvm")


def no_other_qemu() -> bool:
    for entry in Path("/proc").glob("[0-9]*"):
        try:
            command = (entry / "cmdline").read_bytes().split(b"\0", 1)[0]
            comm = (entry / "comm").read_text().strip()
        except (FileNotFoundError, ProcessLookupError):
            continue
        except (PermissionError, OSError):
            return False
        if qemu_process(command, comm):
            return False
    return True


def run(*args: str) -> None:
    subprocess.run((TOOLS[args[0]], *args[1:]), check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=90, env={"PATH": SAFE_PATH, "LANG": "C"})


def image_format(path: Path) -> None:
    output = subprocess.check_output(
        [TOOLS["qemu-img"], "info", "--output=json", str(path)], stderr=subprocess.DEVNULL, timeout=30, env={"PATH": SAFE_PATH, "LANG": "C"}
    )
    data = json.loads(output)
    if data.get("format") != "qcow2" or not 1 * GIB <= data.get("virtual-size", 0) <= 12 * GIB:
        refuse("BASE_IMAGE_FORMAT_REFUSED")
    if data.get("backing-filename"):
        refuse("BASE_IMAGE_BACKING_REFUSED")


def check_archive(archive: Path, manifest: Path, checksums: Path, expected_commit: str, expected_manifest_sha256: str, expected_archive_sha256: str) -> None:
    if manifest.name != "connected-manifest.json" or checksums.name != "SHA256SUMS" or not archive.name.endswith(".tar.gz"):
        refuse("BUNDLE_NAMES_REFUSED")
    lines = checksums.read_text(encoding="ascii").splitlines()
    expected = {
        f"{sha256(archive)}  {archive.name}",
        f"{sha256(manifest)}  {manifest.name}",
    }
    if len(lines) != 2 or set(lines) != expected or sha256(archive) != expected_archive_sha256:
        refuse("BUNDLE_CHECKSUM_REFUSED")
    if sha256(manifest) != expected_manifest_sha256:
        refuse("REVIEWED_MANIFEST_REFUSED")
    data = json.loads(manifest.read_text(encoding="utf-8"))
    if data.get("commit") != expected_commit or data.get("profile") != "ubuntu24.04-systemd255-amd64-canary" or data.get("os") != "linux" or data.get("arch") != "amd64" or data.get("schema") != "observer-connected-bundle/v2":
        refuse("REVIEWED_IDENTITY_REFUSED")


def capacity(work_root: Path) -> None:
    if (os.cpu_count() or 0) < 2 or shutil.disk_usage(work_root).free < 20 * GIB:
        refuse("CAPACITY_REFUSED")
    available_kib = 0
    for line in Path("/proc/meminfo").read_text().splitlines():
        if line.startswith("MemAvailable:"):
            available_kib = int(line.split()[1])
    if available_kib < 4 * 1024 * 1024 or not no_other_qemu():
        refuse("VM_CAPACITY_OR_BUSY_REFUSED")


def ext4_command(directory: Path, target: Path) -> tuple[str, ...]:
    # Resolve mkfs.ext4 to Ubuntu's mke2fs without relying on argv[0] defaults.
    return ("mkfs.ext4", "-t", "ext4", "-F", "-q", "-L", "HLOFIX", "-d", str(directory), str(target))


def payload_image(instance: Path, archive: Path, manifest: Path, checksums: Path, receiver: Path, expected_commit: str, expected_manifest_sha256: str, expected_archive_sha256: str, expected_receiver_sha256: str) -> Path:
    directory = instance / "payload"
    directory.mkdir(mode=0o700)
    for source in (archive, manifest, checksums, receiver):
        shutil.copyfile(source, directory / ("connected-first-install-receiver" if source == receiver else source.name))
    scripts = Path(__file__).resolve().parent
    for name in ("guest.sh", "drive_pty.py", "assert_guest.py", "preflight_probe.py"):
        shutil.copyfile(scripts / name, directory / name)
    (directory / "reviewed-identity.json").write_text(json.dumps({"commit": expected_commit, "manifest_sha256": expected_manifest_sha256, "archive_sha256": expected_archive_sha256, "receiver_sha256": expected_receiver_sha256}, sort_keys=True) + "\n", encoding="ascii")
    (directory / "connected-first-install-receiver").chmod(0o755)
    target = instance / "payload.ext4"
    size = max(512 * MIB, 2 * sum(p.stat().st_size for p in directory.iterdir()) + 128 * MIB)
    if size > 1200 * MIB:
        refuse("PAYLOAD_TOO_LARGE")
    with target.open("xb") as image:
        image.truncate(size)
    run(*ext4_command(directory, target))
    if directory.resolve() != instance.resolve() / "payload":
        refuse("PAYLOAD_CLEANUP_SCOPE_REFUSED")
    for source in directory.iterdir():
        if not source.is_file() or source.is_symlink():
            refuse("PAYLOAD_CLEANUP_TYPE_REFUSED")
        source.unlink()
    directory.rmdir()
    return target


def seed_for(case_dir: Path, mode: str) -> Path:
    user_data = case_dir / "user-data"
    meta_data = case_dir / "meta-data"
    seed = case_dir / "seed.iso"
    command = (
        "mkdir -p /mnt/hlo-first-install; "
        "for n in $(seq 1 50); do [ -e /dev/disk/by-label/HLOFIX ] && break; sleep 0.2; done; "
        "mount -o ro /dev/disk/by-label/HLOFIX /mnt/hlo-first-install && "
        f"exec bash /mnt/hlo-first-install/guest.sh {mode} > /dev/ttyS0 2>&1"
    )
    user_data.write_text(
        "#cloud-config\npackage_update: false\npackage_upgrade: false\nruncmd:\n  - "
        + json.dumps(["bash", "-c", command]) + "\n",
        encoding="utf-8",
    )
    meta_data.write_text(f"instance-id: hlo-first-install-{case_dir.name}\nlocal-hostname: hlo-fixture\n")
    run("cloud-localds", str(seed), str(user_data), str(meta_data))
    return seed


def overlay_for(case_dir: Path, base: Path) -> Path:
    target = case_dir / "guest.qcow2"
    run("qemu-img", "create", "-f", "qcow2", "-F", "qcow2", "-b", str(base), "-o", "size=12G", str(target))
    return target


def boot(case_dir: Path, overlay: Path, payload: Path, seed: Path, boot_number: int) -> tuple[subprocess.Popen, Path]:
    if not no_other_qemu():
        refuse("VM_BECAME_BUSY_REFUSED")
    console = case_dir / f"console-{boot_number}.log"
    command = [
        TOOLS["qemu-system-x86_64"], "-name", "hlo-first-install-fixture", "-machine", "q35,accel=kvm",
        "-cpu", "host", "-smp", "2", "-m", "3072", "-display", "none", "-monitor", "none",
        "-serial", f"file:{console}", "-no-reboot", "-no-user-config",
        "-sandbox", "on,obsolete=deny,elevateprivileges=deny,spawn=deny,resourcecontrol=deny",
        "-drive", f"file={overlay},if=virtio,format=qcow2,cache=none",
        "-drive", f"file={seed},media=cdrom,format=raw,readonly=on",
        "-drive", f"file={payload},if=virtio,format=raw,readonly=on",
        "-device", "virtio-rng-pci",
        "-nic", "none",
    ]
    return subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env={"PATH": SAFE_PATH, "LANG": "C"}), console


def wait_marker(process: subprocess.Popen, console: Path, wanted: str) -> None:
    deadline = time.monotonic() + CASE_TIMEOUT
    while time.monotonic() < deadline:
        if console.exists():
            if console.stat().st_size > 2 * MIB:
                refuse("GUEST_CONSOLE_OVERFLOW")
            lines = console.read_text(errors="replace").splitlines()
            if any("HLO_VM_FAIL_" in line for line in lines):
                refuse("GUEST_ASSERTION_FAILED")
            if any(wanted in line for line in lines):
                return
        if process.poll() is not None:
            refuse("GUEST_EXITED_BEFORE_MARKER")
        time.sleep(0.5)
    refuse("GUEST_TIMEOUT")


def stop_exact(process: subprocess.Popen, abrupt: bool) -> None:
    if process.poll() is None:
        if abrupt:
            process.kill()
        else:
            process.terminate()
    try:
        process.wait(timeout=20)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=20)


def case(instance: Path, name: str, base: Path, payload: Path, keep_disks: bool) -> None:
    case_dir = instance / name
    case_dir.mkdir(mode=0o700)
    (case_dir / ".hlo-owned-case").write_text(name)
    seed = seed_for(case_dir, name)
    overlay = overlay_for(case_dir, base)
    first, console = boot(case_dir, overlay, payload, seed, 1)
    try:
        first_marker = {
            "success": "HLO_VM_SUCCESS_FIRST_BOOT",
            "interrupt": "HLO_VM_POWER_CUT_READY",
            "success-cut": "HLO_VM_SUCCESS_READY_FOR_CUT",
        }[name]
        wait_marker(first, console, first_marker)
        abrupt = name != "success"
        if abrupt:
            stop_exact(first, True)
        else:
            first.wait(timeout=90)
            if first.returncode != 0:
                refuse("FIRST_BOOT_EXIT_FAILED")
        second, second_console = boot(case_dir, overlay, payload, seed, 2)
        try:
            wanted = "HLO_VM_INTERRUPT_REBOOT_PASS" if name == "interrupt" else "HLO_VM_SUCCESS_REBOOT_PASS"
            wait_marker(second, second_console, wanted)
            second.wait(timeout=90)
            if second.returncode != 0:
                refuse("SECOND_BOOT_EXIT_FAILED")
        finally:
            stop_exact(second, False)
    finally:
        stop_exact(first, False)
    if not keep_disks:
        if overlay.resolve() != case_dir.resolve() / "guest.qcow2" or not (case_dir / ".hlo-owned-case").is_file():
            refuse("CLEANUP_SCOPE_REFUSED")
        overlay.unlink()
    print(f"HLO_VM_CASE_PASS {name}", flush=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--image-sha256", required=True)
    parser.add_argument("--archive", required=True)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--checksums", required=True)
    parser.add_argument("--receiver", required=True)
    parser.add_argument("--expected-commit", required=True)
    parser.add_argument("--expected-manifest-sha256", required=True)
    parser.add_argument("--expected-archive-sha256", required=True)
    parser.add_argument("--expected-receiver-sha256", required=True)
    parser.add_argument("--work-root", required=True)
    parser.add_argument("--keep-disks", action="store_true")
    args = parser.parse_args()
    try:
        if os.geteuid() != 0 or not re.fullmatch(r"[a-f0-9]{64}", args.image_sha256) or not re.fullmatch(r"[a-f0-9]{40}", args.expected_commit) or not re.fullmatch(r"[a-f0-9]{64}", args.expected_manifest_sha256) or not re.fullmatch(r"[a-f0-9]{64}", args.expected_archive_sha256) or not re.fullmatch(r"[a-f0-9]{64}", args.expected_receiver_sha256):
            refuse("ADMISSION_REFUSED")
        for tool in ("qemu-img", "qemu-system-x86_64", "mkfs.ext4", "cloud-localds"):
            found = shutil.which(tool, path=SAFE_PATH)
            if found is None:
                refuse("TOOL_MISSING")
            # Ubuntu's mkfs.ext4 is commonly a link to mke2fs. Execute only
            # the resolved root-owned, non-writable system binary.
            TOOLS[tool] = str(owned_file(str(Path(found).resolve(strict=True)), 50 * MIB))
        root = Path(args.work_root)
        if not root.is_absolute() or root.is_symlink() or root.resolve(strict=True) != root or not qemu_safe(root):
            refuse("WORK_ROOT_REFUSED")
        info = root.stat()
        if not root.is_dir() or not trusted_ancestors(root) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o700:
            refuse("WORK_ROOT_OWNERSHIP_REFUSED")
        for name in ("run_vm.py", "guest.sh", "drive_pty.py", "assert_guest.py", "preflight_probe.py"):
            owned_file(str(Path(__file__).resolve().parent / name), 512 * 1024)
        image = owned_file(args.image, 12 * GIB)
        archive = owned_file(args.archive, 600 * MIB)
        manifest = owned_file(args.manifest, 16 * 1024)
        checksums = owned_file(args.checksums, 4096)
        receiver = owned_file(args.receiver, 20 * MIB)
        if sha256(receiver) != args.expected_receiver_sha256:
            refuse("REVIEWED_RECEIVER_REFUSED")
        if sha256(image) != args.image_sha256:
            refuse("BASE_IMAGE_CHECKSUM_REFUSED")
        image_format(image)
        check_archive(archive, manifest, checksums, args.expected_commit, args.expected_manifest_sha256, args.expected_archive_sha256)
        lease = root / ".hlo-first-install-vm.lock"
        with lease.open("a+b") as handle:
            fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            capacity(root)
            instance = Path(tempfile.mkdtemp(prefix="hlo-first-install-", dir=root))
            instance.chmod(0o700)
            (instance / ".hlo-owned-instance").write_text("first-install-vm/v1\n")
            payload = payload_image(instance, archive, manifest, checksums, receiver, args.expected_commit, args.expected_manifest_sha256, args.expected_archive_sha256, args.expected_receiver_sha256)
            for name in CASES:
                case(instance, name, image, payload, args.keep_disks)
            if not args.keep_disks:
                if payload.resolve() != instance.resolve() / "payload.ext4" or not (instance / ".hlo-owned-instance").is_file():
                    refuse("PAYLOAD_IMAGE_CLEANUP_SCOPE_REFUSED")
                payload.unlink()
            print("HLO_VM_ALL_PASS", flush=True)
        return 0
    except (OSError, subprocess.SubprocessError, RuntimeError, ValueError) as error:
        # Fixed code only; no paths, guest output or secret-bearing request data.
        code = str(error) if isinstance(error, RuntimeError) else "HOST_OPERATION_FAILED"
        print(f"HLO_VM_FAIL {code}", file=sys.stderr, flush=True)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
