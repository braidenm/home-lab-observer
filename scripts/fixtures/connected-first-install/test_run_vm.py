import hashlib
from pathlib import Path
import tempfile
import unittest

import run_vm


class HarnessAdmissionTest(unittest.TestCase):
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
            manifest.write_bytes(b"synthetic manifest")
            checksums.write_text(
                f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n"
                f"{hashlib.sha256(manifest.read_bytes()).hexdigest()}  {manifest.name}\n"
            )
            run_vm.check_archive(archive, manifest, checksums)
            archive.write_bytes(b"tampered")
            with self.assertRaises(RuntimeError):
                run_vm.check_archive(archive, manifest, checksums)

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
