import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import run_vm
import assert_guest


class HarnessAdmissionTest(unittest.TestCase):
    def test_ext4_type_is_explicit_after_resolving_mke2fs(self):
        command = run_vm.ext4_command(Path("/safe/payload"), Path("/safe/payload.ext4"))
        self.assertEqual(command[:3], ("mkfs.ext4", "-t", "ext4"))
        self.assertEqual(command[-2:], ("/safe/payload", "/safe/payload.ext4"))

    def test_guest_rejects_unknown_members_and_wrong_mode(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            target = directory / "fixed"
            target.write_bytes(b"fixture")
            target.chmod(0o600)
            assert_guest.names(directory, {"fixed"})
            assert_guest.plain(target, 0o600, target.stat().st_uid, target.stat().st_gid)
            with self.assertRaises(AssertionError):
                assert_guest.names(directory, set())
            target.chmod(0o644)
            with self.assertRaises(AssertionError):
                assert_guest.plain(target, 0o600, target.stat().st_uid, target.stat().st_gid)

    def test_qemu_detection_catches_libvirt_and_symlinked_names(self):
        for command, comm in ((b"/usr/bin/qemu-system-x86_64", "qemu-system-x86"), (b"/usr/bin/qemu-kvm", "qemu-kvm"), (b"/renamed/vm", "qemu-system-x86")):
            self.assertTrue(run_vm.qemu_process(command, comm))
        self.assertFalse(run_vm.qemu_process(b"/usr/bin/python3", "python3"))

    def test_qemu_option_paths_are_closed(self):
        self.assertTrue(run_vm.qemu_safe(Path("/data/hlo-fixture/input.qcow2")))
        for path in ("/data/x,y", "/data/x:y", "/data/x\ny"):
            self.assertFalse(run_vm.qemu_safe(Path(path)))

    def test_bundle_checksum_contract(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            archive = directory / "home-lab-observer-connected_0.1.0-canary.1_linux_amd64.tar.gz"
            manifest = directory / "connected-manifest.json"
            checksums = directory / "SHA256SUMS"
            archive.write_bytes(b"synthetic archive")
            commit = "a" * 40
            manifest.write_text(json.dumps({"schema": "observer-connected-bundle/v2", "commit": commit, "profile": "ubuntu24.04-systemd255-amd64-canary", "os": "linux", "arch": "amd64"}))
            checksums.write_text(
                f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n"
                f"{hashlib.sha256(manifest.read_bytes()).hexdigest()}  {manifest.name}\n"
            )
            digest = hashlib.sha256(manifest.read_bytes()).hexdigest()
            archive_digest = hashlib.sha256(archive.read_bytes()).hexdigest()
            run_vm.check_archive(archive, manifest, checksums, commit, digest, archive_digest)
            with self.assertRaises(RuntimeError):
                run_vm.check_archive(archive, manifest, checksums, "b" * 40, digest, archive_digest)
            with self.assertRaises(RuntimeError):
                run_vm.check_archive(archive, manifest, checksums, commit, "b" * 64, archive_digest)
            archive.write_bytes(b"tampered")
            checksums.write_text(
                f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n"
                f"{digest}  {manifest.name}\n"
            )
            with self.assertRaises(RuntimeError):
                run_vm.check_archive(archive, manifest, checksums, commit, digest, archive_digest)

    def test_no_matching_marker_is_not_success(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "console.log"
            path.write_text("HLO_VM_SUCCESS_FIRST_BOOT\n")
            class ExitedProcess:
                def poll(self):
                    return 0
            with self.assertRaises(RuntimeError):
                run_vm.wait_marker(ExitedProcess(), path, "HLO_VM_SUCCESS_REBOOT_PASS")


if __name__ == "__main__":
    unittest.main()
