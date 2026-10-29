import contextlib
import grp
import hashlib
import io
import json
from pathlib import Path
import pwd
import sqlite3
import tempfile
import unittest
from unittest import mock

import rich_guest
import rich_scope
import run_vm
import types


class RichFixtureTest(unittest.TestCase):
    def test_rich_stopped_requires_exact_loaded_fragment_and_no_transient(self):
        def state(unit, fields):
            if unit.endswith("-enrollment.service"):
                return {"Id": unit, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}
            return {"Id": unit, "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/" + unit, "ActiveState": "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}
        for field, wrong in (("Id", "foreign.service"), ("LoadState", "not-found"), ("FragmentPath", "/foreign")):
            def changed(unit, fields):
                value = {**state(unit, fields), field: wrong}
                return {key: value[key] for key in fields}
            with mock.patch.object(rich_guest.assert_guest, "unit_properties", side_effect=changed), mock.patch.object(rich_guest.activation_guest, "command", return_value=b'{"state":"STOPPED"}'), mock.patch.object(rich_guest, "verify_numeric"):
                with self.assertRaisesRegex(AssertionError, "^STOPPED$"):
                    rich_guest.stopped(Path("/fixed"))
        for field, wrong in (("LoadState", "loaded"), ("ActiveState", "active"), ("MainPID", "17"), ("Transient", "yes"), ("FragmentPath", "/run/systemd/transient/foreign.service")):
            def changed(unit, fields):
                value = state(unit, fields)
                if unit.endswith("-enrollment.service"):
                    value[field] = wrong
                return {key: value[key] for key in fields}
            with mock.patch.object(rich_guest.assert_guest, "unit_properties", side_effect=changed), mock.patch.object(rich_guest.activation_guest, "command", return_value=b'{"state":"STOPPED"}'), mock.patch.object(rich_guest, "verify_numeric"):
                with self.assertRaisesRegex(AssertionError, "^STOPPED$"):
                    rich_guest.stopped(Path("/fixed"))

    def test_rich_principal_mismatch_is_refused(self):
        config = {"collector_uid": 60101, "uploader_uid": 60102, "uploader_gid": 60102, "shared_gid": 60103}
        bad = types.SimpleNamespace(pw_uid=60101, pw_gid=60103, pw_dir="/nonexistent", pw_shell="/bin/bash", pw_passwd="x")
        with mock.patch.object(rich_guest.pwd, "getpwnam", return_value=bad):
            with self.assertRaisesRegex(AssertionError, "^IDENTITY$"):
                rich_guest.principals(config)

    def test_rich_principals_are_locked_exact_and_group_isolated(self):
        config = {"collector_uid": 60101, "uploader_uid": 60102, "uploader_gid": 60102, "shared_gid": 60103}
        collector, uploader, shared = "hlo-connected-rich-collector", "hlo-connected-rich-uploader", "hlo-connected-rich-read"
        for failure in (None, "uid", "supplementary", "members", "unlocked-user", "unlocked-group", "nss", "alias"):
            users = {collector: pwd.struct_passwd((collector, "x", 60101, 60103, "", "/nonexistent", "/usr/sbin/nologin")), uploader: pwd.struct_passwd((uploader, "x", 60102, 60102, "", "/nonexistent", "/usr/sbin/nologin"))}
            groups = {shared: grp.struct_group((shared, "x", 60103, [uploader])), uploader: grp.struct_group((uploader, "x", 60102, []))}
            all_users = list(users.values())
            if failure == "uid":
                users[collector] = pwd.struct_passwd((collector, "x", 60104, 60103, "", "/nonexistent", "/usr/sbin/nologin"))
            if failure == "members":
                groups[shared] = grp.struct_group((shared, "x", 60103, [uploader, "foreign"]))
            if failure == "alias":
                all_users.append(pwd.struct_passwd(("foreign", "x", 60101, 10, "", "/nonexistent", "/usr/sbin/nologin")))
            def command(*args, **kwargs):
                name = args[-1]
                if args[0].endswith("passwd"):
                    return f"{name} {'P' if failure == 'unlocked-user' else 'L'} 2026-10-29 0 99999 7 -1\n".encode()
                return f"{name}:{'hash' if failure == 'unlocked-group' else '!'}::{uploader if name == shared else ''}\n".encode()
            def group_list(name, gid):
                return [gid, 12345] if failure == "supplementary" else ([60103] if name == collector else [60102, 60103])
            nss = b"passwd: files\ngroup: files\nshadow: files\ngshadow: files\n" if failure != "nss" else b"passwd: ldap\n"
            with mock.patch.object(rich_guest.pwd, "getpwnam", side_effect=users.__getitem__), mock.patch.object(rich_guest.pwd, "getpwuid", side_effect=lambda uid: next(v for v in users.values() if v.pw_uid == uid)), mock.patch.object(rich_guest.pwd, "getpwall", return_value=all_users), mock.patch.object(rich_guest.grp, "getgrnam", side_effect=groups.__getitem__), mock.patch.object(rich_guest.grp, "getgrgid", side_effect=lambda gid: next(v for v in groups.values() if v.gr_gid == gid)), mock.patch.object(rich_guest.grp, "getgrall", return_value=list(groups.values())), mock.patch.object(rich_guest.os, "getgrouplist", side_effect=group_list), mock.patch.object(rich_guest.preflight_probe, "command", side_effect=command), mock.patch.object(rich_guest.preflight_probe, "fixed_file", return_value=nss):
                if failure is None:
                    rich_guest.principals(config)
                else:
                    with self.assertRaisesRegex(AssertionError, "^IDENTITY$"):
                        rich_guest.principals(config)

    def test_scope_never_expands_legacy_default_or_all(self):
        self.assertEqual(rich_scope.cases("stopped", run_vm.CASES), run_vm.CASES)
        self.assertEqual(rich_scope.cases("all", run_vm.CASES), (*run_vm.CASES, "activation"))
        self.assertEqual(rich_scope.cases("rich-processes", run_vm.CASES), ("rich-processes",))
        self.assertEqual(run_vm.harness_files(), run_vm.HARNESS_FILES)
        self.assertEqual(run_vm.harness_files("rich-processes"), run_vm.HARNESS_FILES + ("rich_scope.py", "rich_guest.py"))
        for scope in ("stopped", "all", "activation", "bootstrap"):
            with self.assertRaisesRegex(RuntimeError, "^RICH_SCOPE_REFUSED$"):
                rich_scope.inputs(scope, "/x", "a" * 64, "b" * 64, "c" * 40, None, None)

    def test_closed_scope_requires_all_independent_pins_before_inputs(self):
        for values in ((None, None, None), ("/x", "bad", "b" * 64), ("/x", "a" * 64, None)):
            read = mock.Mock()
            with self.assertRaisesRegex(RuntimeError, "^RICH_PINS_REFUSED$"):
                rich_scope.inputs("rich-processes", *values, "c" * 40, read, None)
            read.assert_not_called()

    def test_package_identity_hashes_and_exact_checksums(self):
        digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / "public.tar.gz"
            manifest = root / "connected-manifest.json"
            helper = root / "install-connected-rich-processes-linux.sh"
            checksums = root / "SHA256SUMS"
            archive.write_bytes(b"synthetic archive")
            helper.write_bytes(b"data-only helper")
            helper.chmod(0o644)
            manifest.write_text(json.dumps({"commit": "c" * 40, "profile": rich_scope.PROFILE, "schema": "observer-connected-bundle/v2", "os": "linux", "arch": "amd64"}))
            checksums.write_text("".join(f"{digest(v)}  {v.name}\n" for v in (archive, manifest, helper)))
            pins = digest(archive), digest(manifest)
            args = ("rich-processes", str(archive), *pins, "c" * 40, lambda p, limit: Path(p), digest)
            sources, identity = rich_scope.inputs(*args)
            self.assertEqual(set(sources), {"rich-package.tgz", "rich-manifest.json", "rich-SHA256SUMS"})
            self.assertEqual(identity["manifest_sha256"], pins[1])
            checksums.write_text(checksums.read_text() + "unknown\n")
            with self.assertRaisesRegex(RuntimeError, "^RICH_CHECKSUM_REFUSED$"):
                rich_scope.inputs(*args)
            archive.write_bytes(b"changed")
            with self.assertRaisesRegex(RuntimeError, "^RICH_PACKAGE_REFUSED$"):
                rich_scope.inputs(*args)

    def test_rich_harness_pin_binds_new_sources(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name in run_vm.harness_files("rich-processes"):
                (root / name).write_bytes(name.encode())
            pin = run_vm.harness_sha256(root, "rich-processes")
            (root / "rich_guest.py").write_bytes(b"changed")
            with self.assertRaisesRegex(RuntimeError, "^REVIEWED_HARNESS_REFUSED$"):
                run_vm.check_harness(root, pin, "rich-processes")

    def test_pending_reader_proves_large_exact_bytes_without_output(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "ledger").mkdir()
            body = b"x" * 16385
            digest = hashlib.sha256(body).digest()
            with sqlite3.connect(root / "ledger/upload.sqlite") as connection:
                connection.execute("CREATE TABLE upload_record(singleton,pending_sequence,pending_body,pending_digest,watermark,profile_kind,policy_digest)")
                connection.execute("INSERT INTO upload_record VALUES(1,7,?,?,6,1,?)", (body, digest, b"p" * 32))
            with mock.patch.object(rich_guest, "STATE", root):
                result = rich_guest.pending()
                self.assertEqual(result["sequence"], 7)
                self.assertEqual(result["size"], 16385)
                self.assertEqual(result["digest"], digest.hex())
                self.assertNotIn(body.decode(), json.dumps(result))
            with sqlite3.connect(root / "ledger/upload.sqlite") as connection:
                connection.execute("UPDATE upload_record SET pending_digest=?", (b"d" * 32,))
            with mock.patch.object(rich_guest, "STATE", root), self.assertRaisesRegex(AssertionError, "^PENDING$"):
                rich_guest.pending()

    def test_failure_output_is_closed(self):
        output = io.StringIO()
        with mock.patch.object(rich_guest.sys, "argv", ["rich_guest.py", "first"]), mock.patch.object(rich_guest, "exercise", side_effect=OSError("private credential")), contextlib.redirect_stderr(output):
            self.assertEqual(rich_guest.main(), 1)
        self.assertEqual(output.getvalue(), "HLO_VM_FAIL_RICH_UNKNOWN\n")

    def test_numeric_fingerprint_cannot_hide_active_or_enabled_workers(self):
        expected = {"LoadState": "loaded", "ActiveState": "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "snapshot.json").write_text("{}")
            for key, wrong in (("ActiveState", "active"), ("MainPID", "101"), ("UnitFileState", "enabled"), ("DropInPaths", "/foreign"), ("NeedDaemonReload", "yes")):
                with mock.patch.object(rich_guest, "ROOT", root), mock.patch.object(rich_guest.assert_guest, "snapshot", return_value={}), mock.patch.object(rich_guest.assert_guest, "unit_properties", return_value={**expected, key: wrong}):
                    with self.assertRaisesRegex(AssertionError, "^NUMERIC$"):
                        rich_guest.verify_numeric()

    def test_numeric_process_refusal_is_independent_of_stopped_unit_metadata(self):
        expected = {"LoadState": "loaded", "ActiveState": "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "snapshot.json").write_text("{}")
            with mock.patch.object(rich_guest, "ROOT", root), mock.patch.object(rich_guest.assert_guest, "snapshot", return_value={}), mock.patch.object(rich_guest.assert_guest, "unit_properties", return_value=expected), mock.patch.object(rich_guest, "numeric_workers_absent", return_value=False):
                with self.assertRaisesRegex(AssertionError, "^NUMERIC$"):
                    rich_guest.verify_numeric()

    def test_guest_branch_is_reachable_only_after_numeric_install(self):
        source = (Path(__file__).parent / "guest.sh").read_text()
        rich = source.index('if [ "$1" = rich-processes ]; then')
        self.assertGreater(rich, source.index('python3 "$payload/drive_pty.py" install'))
        self.assertLess(rich, source.index('emit SUCCESS_FIRST_BOOT'))


if __name__ == "__main__":
    unittest.main()
