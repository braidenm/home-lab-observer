import contextlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import docker_guest


class DockerGuestTest(unittest.TestCase):
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
            with mock.patch.object(docker_guest, "STATE", root):
                self.assertEqual(docker_guest.pending()["size"], len(body))
            with sqlite3.connect(root / "ledger/upload.sqlite") as connection:
                connection.execute("PRAGMA user_version=2")
            with mock.patch.object(docker_guest, "STATE", root), self.assertRaisesRegex(AssertionError, "^PENDING$"):
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
            with mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=[initial, final]), mock.patch.object(docker_guest, "run"):
                with self.assertRaisesRegex(AssertionError, "^ISOLATION$"):
                    docker_guest.probe_role(docker_guest.UNITS[0], 60001, 60002, [60003], ["/fixed"], False)


if __name__ == "__main__":
    unittest.main()
