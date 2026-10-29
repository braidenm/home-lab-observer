import contextlib
import io
from pathlib import Path
import tempfile
import grp
import pwd
import types
import ctypes
import json
import unittest
from unittest import mock

import docker_guest


class DockerGuestTest(unittest.TestCase):
    def test_reader_binding_requires_socket_and_same_active_manager(self):
        import stat
        initial = {"ActiveState": "active", "MainPID": "100", "InvocationID": "a" * 32}
        for wrong in (None, "not-socket", "changed-pid", "inactive"):
            final = {**initial, **({"MainPID": "101"} if wrong == "changed-pid" else {})}
            before = {**initial, **({"ActiveState": "inactive"} if wrong == "inactive" else {})}
            node = types.SimpleNamespace(st_dev=7, st_ino=8, st_mode=stat.S_IFREG if wrong == "not-socket" else stat.S_IFSOCK)
            with mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=[before, final]), mock.patch.object(Path, "stat", return_value=node):
                if wrong is None:
                    self.assertEqual(docker_guest.reader_binding(), {"manager": initial, "socket": [7, 8]})
                else:
                    with self.assertRaisesRegex(AssertionError, "^FAULT$"):
                        docker_guest.reader_binding()

    def test_namespace_probe_checks_target_root_identity_before_dropping_ids(self):
        for changed in (False, True):
            expected = types.SimpleNamespace(st_dev=7, st_ino=8)
            entered = types.SimpleNamespace(st_dev=7, st_ino=9 if changed else 8)
            libc = types.SimpleNamespace(setns=lambda *unused: 0, prctl=lambda op, *unused: 1 if op == 39 else 0)
            with mock.patch.object(docker_guest.os, "stat", side_effect=[expected, entered]) as stat, mock.patch.object(docker_guest.os, "open", return_value=17), mock.patch.object(docker_guest.os, "close"), mock.patch.object(ctypes, "CDLL", return_value=libc), mock.patch.object(docker_guest.sys, "argv", ["-c", "100", "60001", "60002", json.dumps([[60003], [], False])]), mock.patch.object(docker_guest.os, "setgroups") as groups, mock.patch.object(docker_guest.os, "setresgid"), mock.patch.object(docker_guest.os, "setresuid"), mock.patch.object(docker_guest.os, "getresuid", return_value=(60001,) * 3), mock.patch.object(docker_guest.os, "getresgid", return_value=(60002,) * 3), mock.patch.object(docker_guest.os, "getgroups", return_value=[60003]):
                if changed:
                    with self.assertRaises(SystemExit) as refused:
                        exec(docker_guest.NAMESPACE_PROBE, {})
                    self.assertEqual(refused.exception.code, 1)
                    groups.assert_not_called()
                else:
                    exec(docker_guest.NAMESPACE_PROBE, {})
                    groups.assert_called_once_with([60003])
                self.assertEqual(stat.call_args_list, [mock.call("/proc/100/root"), mock.call("/")])

    def test_socket_replacement_requires_same_active_reader_and_old_bound_inode(self):
        initial = {"manager": {"ActiveState": "active", "MainPID": "100", "InvocationID": "a" * 32}, "socket": [7, 8]}
        for change in (None, "pid", "invocation", "inactive", "adopted"):
            after = {"manager": dict(initial["manager"]), "socket": initial["socket"][:]}
            if change == "pid":
                after["manager"]["MainPID"] = "101"
            if change == "invocation":
                after["manager"]["InvocationID"] = "b" * 32
            if change == "inactive":
                after["manager"]["ActiveState"] = "inactive"
            if change == "adopted":
                after["socket"] = [7, 9]
            fault = {"unavailable_count": 2, "healthy_sequence": 0, "sequence": 8}
            good = {**{name: True for name in ("large_body", "foreign_process", "running_container", "exited_null", "warmed_metrics")}, "healthy_sequence": 9, "sequence": 9}
            receipts = iter((fault, good))
            def wait(predicate):
                value = next(receipts)
                self.assertTrue(predicate(value))
                return value
            with mock.patch.object(docker_guest, "reader_binding", side_effect=[initial, after]), mock.patch.object(Path, "stat", side_effect=[types.SimpleNamespace(st_dev=7, st_ino=8), types.SimpleNamespace(st_dev=7, st_ino=9)]), mock.patch.object(docker_guest, "counts", return_value={"unavailable_count": 1}), mock.patch.object(docker_guest.docker_engine, "stop"), mock.patch.object(docker_guest.docker_engine, "start"), mock.patch.object(docker_guest.docker_engine, "cli"), mock.patch.object(docker_guest, "wait_counts", side_effect=wait), mock.patch.object(docker_guest.activation_guest, "command") as command, mock.patch.object(docker_guest, "stopped"):
                if change is None:
                    docker_guest.socket_replacement(Path("/fixed"))
                    self.assertEqual(command.call_args_list, [mock.call(Path("/fixed"), "stop"), mock.call(Path("/fixed"), "start")])
                else:
                    with self.assertRaisesRegex(AssertionError, "^FAULT$"):
                        docker_guest.socket_replacement(Path("/fixed"))
                    command.assert_not_called()

    def test_service_only_frame_grant_is_absent_from_persistent_nss(self):
        c = {"collector_uid": 60101, "reader_uid": 60102, "uploader_uid": 60103, "shared_gid": 60104, "frame_gid": 60105, "uploader_gid": 60106, "socket_gid": docker_guest.docker_engine.DAEMON_GID}
        collector, reader, uploader, shared, frame = ("hlo-rich-docker-" + role for role in ("collector", "reader", "uploader", "read", "frame"))
        for bad in (None, "collector-frame", "frame-member"):
            users = {name: pwd.struct_passwd((name, "x", c[uid], c[gid], "", "/nonexistent", "/usr/sbin/nologin")) for name, uid, gid in ((collector, "collector_uid", "shared_gid"), (reader, "reader_uid", "frame_gid"), (uploader, "uploader_uid", "uploader_gid"))}
            groups = {shared: grp.struct_group((shared, "x", c["shared_gid"], [uploader])), frame: grp.struct_group((frame, "x", c["frame_gid"], [collector] if bad == "frame-member" else [])), uploader: grp.struct_group((uploader, "x", c["uploader_gid"], [])), "daemon": grp.struct_group(("daemon", "x", c["socket_gid"], []))}
            def command(*args, **kwargs):
                name = args[-1]
                if args[0].endswith("passwd"):
                    return f"{name} L 2026-10-29 0 99999 7 -1\n".encode()
                return f"{name}:!::{','.join(groups[name].gr_mem)}\n".encode()
            def grouplist(name, gid):
                if name == uploader:
                    return [gid, c["shared_gid"]]
                if name == collector and bad == "collector-frame":
                    return [gid, c["frame_gid"]]
                return [gid]
            with mock.patch.object(docker_guest.pwd, "getpwnam", side_effect=users.__getitem__), mock.patch.object(docker_guest.pwd, "getpwuid", side_effect=lambda uid: next(u for u in users.values() if u.pw_uid == uid)), mock.patch.object(docker_guest.pwd, "getpwall", return_value=list(users.values())), mock.patch.object(docker_guest.grp, "getgrnam", side_effect=groups.__getitem__), mock.patch.object(docker_guest.grp, "getgrgid", side_effect=lambda gid: next(g for g in groups.values() if g.gr_gid == gid)), mock.patch.object(docker_guest.grp, "getgrall", return_value=list(groups.values())), mock.patch.object(docker_guest.os, "getgrouplist", side_effect=grouplist), mock.patch.object(docker_guest.preflight_probe, "command", side_effect=command), mock.patch.object(docker_guest.preflight_probe, "fixed_file", return_value=b"nss"), mock.patch.object(docker_guest.preflight_probe, "supported_nss", return_value=True):
                if bad is None:
                    docker_guest.principals(c)
                else:
                    with self.assertRaisesRegex(AssertionError, "^IDENTITY$"):
                        docker_guest.principals(c)

    def test_actual_live_credentials_require_service_group_without_extras(self):
        good = b"Uid:\t60101\t60101\t60101\t60101\nGid:\t60104\t60104\t60104\t60104\nGroups:\t60104 60105\n"
        for raw in (good, good.replace(b"60104 60105", b"60105"), good.replace(b"60104 60105", b"60104"), good.replace(b"60104 60105", b"60104 60105 60106"), good.replace(b"Uid:\t60101", b"Uid:\t0"), good.replace(b"Gid:\t60104", b"Gid:\t60105"), good + b"Groups:\t60104 60105\n"):
            with mock.patch.object(docker_guest.preflight_probe, "fixed_file", return_value=raw):
                if raw in (good, good.replace(b"60104 60105", b"60105")):
                    docker_guest.live_identity(100, 60101, 60104, 60105)
                else:
                    with self.assertRaisesRegex(AssertionError, "^ISOLATION$"):
                        docker_guest.live_identity(100, 60101, 60104, 60105)

    def test_historical_rows_cannot_prove_current_recovery(self):
        good = {name: True for name in ("large_body", "foreign_process", "running_container", "exited_null", "warmed_metrics")}
        self.assertTrue(docker_guest.healthy({**good, "sequence": 9, "healthy_sequence": 9}))
        self.assertFalse(docker_guest.healthy({**good, "sequence": 10, "healthy_sequence": 9}))
        self.assertFalse(docker_guest.healthy({**good, "sequence": 10, "healthy_sequence": 0}))

    def test_stopped_rejects_any_reader_or_enrollment_authority(self):
        def properties(unit, fields):
            if unit.endswith("-enrollment.service"):
                return {"Id": unit, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}
            return {"Id": unit, "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/" + unit, "ActiveState": "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}
        for target, key, value in ((docker_guest.UNITS[1], "MainPID", "77"), (docker_guest.UNITS[1], "FragmentPath", "/foreign"), (docker_guest.ENROLLMENT, "Transient", "yes"), (docker_guest.ENROLLMENT, "FragmentPath", "/foreign")):
            def changed(unit, fields):
                state = properties(unit, fields)
                if unit == target:
                    state[key] = value
                return state
            with mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=changed), mock.patch.object(docker_guest.activation_guest, "command", return_value=b'{"state":"STOPPED"}'), mock.patch.object(docker_guest.rich_guest, "verify_numeric"):
                with self.assertRaisesRegex(AssertionError, "^STOPPED$"):
                    docker_guest.stopped(Path("/fixed"))

    def test_pending_requires_schema_three_kind_two_and_actual_large_body(self):
        import hashlib
        import sqlite3
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "ledger").mkdir()
            body = b"x" * 16385
            with sqlite3.connect(root / "ledger/upload.sqlite") as connection:
                connection.execute("PRAGMA user_version=3")
                connection.execute("CREATE TABLE upload_record(singleton,pending_sequence,pending_body,pending_digest,watermark,profile_kind,policy_digest,pending_at)")
                connection.execute("INSERT INTO upload_record VALUES(1,7,?,?,6,2,?,?)", (body, hashlib.sha256(body).digest(), b"p" * 32, "2026-10-29T23:59:30Z"))
            (root / "installed.json").write_text('{"collection_policy_sha256":"' + (b"p" * 32).hex() + '"}')
            with mock.patch.object(docker_guest, "STATE", root), mock.patch.object(docker_guest, "CONFIG", root):
                self.assertEqual(docker_guest.pending()["size"], len(body))
                for bad in (b"q" * 32, b"p" * 31, b"p" * 33, None, "p" * 32):
                    with sqlite3.connect(root / "ledger/upload.sqlite") as connection:
                        connection.execute("UPDATE upload_record SET policy_digest=?", (bad,))
                    with self.assertRaisesRegex(AssertionError, "^PENDING$"):
                        docker_guest.pending()
                with sqlite3.connect(root / "ledger/upload.sqlite") as connection:
                    connection.execute("UPDATE upload_record SET policy_digest=?", (b"p" * 32,))
            with sqlite3.connect(root / "ledger/upload.sqlite") as connection:
                connection.execute("PRAGMA user_version=2")
            with mock.patch.object(docker_guest, "STATE", root), mock.patch.object(docker_guest, "CONFIG", root), self.assertRaisesRegex(AssertionError, "^PENDING$"):
                docker_guest.pending()

    def test_failure_emits_only_allowlisted_marker_even_on_postboot(self):
        for reason, expected in (("ENGINE_API", "ENGINE_API"), ("raw token", "UNKNOWN")):
            output = io.StringIO()
            with mock.patch.object(Path, "open", side_effect=OSError()), contextlib.redirect_stderr(output):
                docker_guest.emit_failure(reason)
            self.assertEqual(output.getvalue(), "HLO_VM_FAIL_DOCKER_" + expected + "\n")

    def test_namespace_probe_never_executes_after_setns_and_binds_invocation(self):
        self.assertNotIn("exec(", docker_guest.NAMESPACE_PROBE)
        self.assertNotIn("subprocess", docker_guest.NAMESPACE_PROBE)
        self.assertLess(docker_guest.NAMESPACE_PROBE.index("setns(fd"), docker_guest.NAMESPACE_PROBE.index("os.setgroups"))
        self.assertLess(docker_guest.NAMESPACE_PROBE.index("os.setresuid"), docker_guest.NAMESPACE_PROBE.index("for p in denied"))
        for field in ("MainPID", "InvocationID", "ActiveState"):
            initial = {"MainPID": "100", "InvocationID": "a" * 32, "ActiveState": "active"}
            final = {**initial, field: "different"}
            with mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=[initial, final]), mock.patch.object(docker_guest, "run"), mock.patch.object(docker_guest, "live_identity"):
                with self.assertRaisesRegex(AssertionError, "^ISOLATION$"):
                    docker_guest.probe_role(docker_guest.UNITS[0], 60001, 60002, [60003], ["/fixed"], False)


if __name__ == "__main__":
    unittest.main()
