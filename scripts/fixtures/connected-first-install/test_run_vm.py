import hashlib
import contextlib
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

import run_vm
import assert_guest
import drive_pty
import preflight_probe


class HarnessAdmissionTest(unittest.TestCase):
    def test_enrollment_records_are_closed_private_and_bound_without_secret_output(self):
        if not hasattr(os, "getxattr"):
            self.skipTest("Linux ACL fixture only")
        connector = "agent_" + "a" * 32
        secret = "hlc_" + "B" * 43
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for filename, state in (("attempt.json", "ATTEMPTED"),
                                    ("credential.json", "CREDENTIAL"), ("ready.json", "READY")):
                record = {"version": "observer-enrollment/v1", "state": state,
                          "server_id": assert_guest.SERVER, "connector_id": connector}
                if state == "CREDENTIAL":
                    record["secret"] = secret
                target = root / filename
                target.write_bytes(json.dumps(record, separators=(",", ":")).encode("ascii"))
                target.chmod(0o600)
            lock = root / ".enrollment-lock"
            lock.write_bytes(b"")
            lock.chmod(0o600)
            identities = assert_guest.enrollment_records(root, os.getuid(), os.getgid(), connector, secret)
            self.assertEqual(set(identities), assert_guest.ENROLLMENT_MEMBERS)
            self.assertEqual(identities[".enrollment-lock"][1], 0)
            (root / "foreign-member").write_bytes(b"not a grant")
            with self.assertRaisesRegex(AssertionError, "^UNKNOWN_MEMBER_ENROLLMENT$") as error:
                assert_guest.enrollment_records(root, os.getuid(), os.getgid(), connector, secret)
            self.assertNotIn(secret, str(error.exception))
            (root / "foreign-member").unlink()
            with self.assertRaisesRegex(AssertionError, "^ENROLLMENT_RECORD_BINDING$"):
                assert_guest.enrollment_records(root, os.getuid(), os.getgid(), connector, "hlc_" + "C" * 43)
            (root / "ready.json").write_bytes(b"x" * 513)
            with self.assertRaisesRegex(AssertionError, "^ENROLLMENT_RECORD_SIZE$"):
                assert_guest.enrollment_records(root, os.getuid(), os.getgid(), connector, secret)

    def test_member_stage_label_is_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "private-extra").write_bytes(b"private")
            with self.assertRaisesRegex(AssertionError, "^UNKNOWN_MEMBER_CONFIG$") as error:
                assert_guest.names(root, set(), "CONFIG")
            self.assertNotIn("private-extra", str(error.exception))
            with self.assertRaisesRegex(AssertionError, "^UNKNOWN_MEMBER_UNKNOWN$"):
                assert_guest.names(root, set(), "hle_private")

    def test_systemd_root_scaffold_is_closed_and_checks_private_mount_points(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ("root", "usr", "var", "proc", "sys", "dev/mqueue", "run/systemd/incoming"):
                (root / name).mkdir(parents=True)
            with mock.patch.object(assert_guest, "plain") as check:
                assert_guest.systemd_root_scaffold(root)
                self.assertEqual(check.call_count, 10)
                self.assertIn(mock.call(root / "proc", 0o555, 0, 0, True), check.call_args_list)
                self.assertIn(mock.call(root / "root", 0o750, 0, 0, True), check.call_args_list)
                (root / "dev/foreign-member").write_bytes(b"private")
                with self.assertRaisesRegex(AssertionError, "^UNKNOWN_MEMBER_UPLOADER_DEV$") as error:
                    assert_guest.systemd_root_scaffold(root)
                self.assertNotIn("foreign-member", str(error.exception))

    def test_effective_systemd_properties_are_exact_and_bounded(self):
        fields = ("FragmentPath", "DropInPaths", "NeedDaemonReload")
        with mock.patch.object(assert_guest, "run", return_value="DropInPaths=\nNeedDaemonReload=no\nFragmentPath=/etc/systemd/system/example.service\n"):
            self.assertEqual(assert_guest.unit_properties("example.service", fields), {
                "FragmentPath": "/etc/systemd/system/example.service", "DropInPaths": "",
                "NeedDaemonReload": "no"})
        for output in ("FragmentPath=/etc/systemd/system/example.service\n",
                       "FragmentPath=/etc/systemd/system/example.service\nDropInPaths=\nNeedDaemonReload=no\nDropInPaths=/etc/override.conf\n",
                       "FragmentPath=/etc/systemd/system/example.service\nDropInPaths=\nNeedDaemonReload=no\nForeign=private\n",
                       "x" * 4097):
            with mock.patch.object(assert_guest, "run", return_value=output):
                with self.assertRaisesRegex(AssertionError, "^SYSTEMD_PROPERTY_OUTPUT$"):
                    assert_guest.unit_properties("example.service", fields)

    def test_systemd_ip_denial_requires_both_families_in_any_order(self):
        self.assertTrue(assert_guest.denies_all_ip("0.0.0.0/0 ::/0"))
        self.assertTrue(assert_guest.denies_all_ip("::/0 0.0.0.0/0"))
        for value in ("0.0.0.0/0", "::/0", "0.0.0.0/0 ::/0 10.0.0.0/8",
                      "0.0.0.0/0 0.0.0.0/0", "any", ""):
            self.assertFalse(assert_guest.denies_all_ip(value))

    def test_driver_diagnostic_is_allowlisted_and_redacted(self):
        for label, phase in drive_pty.PHASE_LABELS:
            self.assertEqual(drive_pty.classify_failure(b"prefix " + label + b" fixed text"), phase)
        private = b"hle_" + b"A" * 43 + b" hlc_" + b"B" * 43
        self.assertEqual(drive_pty.classify_failure(private), "UNRECOGNIZED")
        marker = drive_pty.failure_marker(private, 22, False)
        self.assertEqual(marker, "FIXTURE_DRIVER_FAILURE PHASE=UNRECOGNIZED EXIT=22 PROMPT=0")
        self.assertNotIn("hle_", marker)
        self.assertNotIn("hlc_", marker)
        self.assertEqual(drive_pty.classify_failure(b"PREFLIGHT_REFUSED: LOCAL_SETUP_INCOMPLETE:"), "UNRECOGNIZED")
        self.assertEqual(drive_pty.normalized_exit(22), 22)
        self.assertEqual(drive_pty.normalized_exit(-9), 137)

    def test_driver_output_overflow_never_retains_extra_bytes(self):
        output = bytearray(b"x" * (drive_pty.MAX_OUTPUT - 1))
        self.assertFalse(drive_pty.append_bounded(output, b"private"))
        self.assertEqual(len(output), drive_pty.MAX_OUTPUT - 1)
        self.assertTrue(drive_pty.append_bounded(output, b"y"))
        self.assertEqual(len(output), drive_pty.MAX_OUTPUT)
        self.assertFalse(drive_pty.append_bounded(output, b"z"))

    def test_preflight_classifier_never_echoes_unknown_or_oversized_output(self):
        for private in (b"hle_" + b"A" * 43, b"hlc_" + b"B" * 43):
            with self.assertRaises(preflight_probe.Refused):
                preflight_probe.fields(b"Id=system.slice\nSecret=" + private + b"\n", {"Id"}, "PARENT")
            self.assertEqual(preflight_probe.marker(private.decode()), "HLO_VM_FAIL_PREFLIGHT_UNKNOWN")
        with self.assertRaises(preflight_probe.Refused):
            preflight_probe.bounded(b"x" * (preflight_probe.MAX_OUTPUT + 1))
        with self.assertRaises(preflight_probe.Refused):
            preflight_probe.bounded(b"x" * 1025, 1024)
        with self.assertRaises(preflight_probe.Refused):
            preflight_probe.bounded(b"Id=system.slice\r\n")
        self.assertEqual(preflight_probe.marker("NSS"), "HLO_VM_FAIL_PREFLIGHT_NSS")

    def test_preflight_nss_requires_only_local_files_first(self):
        good = b"passwd: files systemd\ngroup: files\nshadow: files systemd\ngshadow: files\n"
        self.assertTrue(preflight_probe.supported_nss(good))
        for bad in (good.replace(b"passwd: files systemd", b"passwd: ldap files"),
                    good + b"initgroups: files\n", good + b"group: files\n"):
            self.assertFalse(preflight_probe.supported_nss(bad))

    def test_preflight_stage_failure_redacts_exception_text(self):
        output = io.StringIO()
        with mock.patch.object(preflight_probe, "host_nss", side_effect=OSError("hle_private")), contextlib.redirect_stderr(output):
            self.assertEqual(preflight_probe.main(), 1)
        self.assertEqual(output.getvalue(), "HLO_VM_FAIL_PREFLIGHT_HOST\n")
        output = io.StringIO()
        with mock.patch.object(preflight_probe, "host_nss", side_effect=RuntimeError("hlc_private")), contextlib.redirect_stderr(output):
            self.assertEqual(preflight_probe.main(), 1)
        self.assertEqual(output.getvalue(), "HLO_VM_FAIL_PREFLIGHT_UNKNOWN\n")

    @unittest.skipUnless(os.name == "posix", "guest-only bounded pipe semantics")
    def test_preflight_command_kills_on_oversized_output(self):
        with self.assertRaises(preflight_probe.Refused):
            preflight_probe.command(sys.executable, "-c", "import sys; sys.stdout.write('x' * 8192)")
        with self.assertRaises(preflight_probe.Refused):
            preflight_probe.command(sys.executable, "-c", "import sys; sys.stdout.write('x' * 2048)", limit=1024)

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
            assert_guest.names(directory, {"fixed"}, "CONFIG")
            assert_guest.plain(target, 0o600, target.stat().st_uid, target.stat().st_gid)
            with self.assertRaises(AssertionError):
                assert_guest.names(directory, set(), "CONFIG")
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

    def test_fixture_only_probe_requires_independent_pin_and_fixed_name(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            probe = directory / "connected-first-install-preflight-probe"
            probe.write_bytes(b"fixture-only")
            digest = hashlib.sha256(probe.read_bytes()).hexdigest()
            run_vm.check_probe(probe, digest)
            with self.assertRaises(RuntimeError):
                run_vm.check_probe(probe, "0" * 64)
            renamed = directory / "unreviewed-probe"
            renamed.write_bytes(probe.read_bytes())
            with self.assertRaises(RuntimeError):
                run_vm.check_probe(renamed, digest)

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
