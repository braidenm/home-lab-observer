import contextlib
import io
from pathlib import Path
import unittest
from unittest import mock

import rich_guest


class RichRebootTest(unittest.TestCase):
    def test_only_nicless_fixed_wait_provider_is_masked_before_pending_creation(self):
        for wrong in (None, "nic", "route", "alternate", "instance", "fragment", "active", "mask"):
            calls = []
            def properties(unit, fields):
                if unit == "NetworkManager-wait-online.service":
                    return {"LoadState": "loaded" if wrong == "alternate" else "not-found"}
                if fields == ("LoadState", "UnitFileState"):
                    return {"LoadState": "loaded" if wrong == "mask" else "masked", "UnitFileState": "masked"}
                return {"Id": unit, "LoadState": "loaded", "FragmentPath": "/foreign" if wrong == "fragment" else "/usr/lib/systemd/system/" + unit, "DropInPaths": "", "NeedDaemonReload": "no", "ActiveState": "active" if wrong == "active" else "failed", "MainPID": "0"}
            def command(*args, **kwargs):
                if args[0].endswith("ip"):
                    return b"default via synthetic" if wrong == "route" else b""
                return b"unexpected instance" if wrong == "instance" else b""
            with mock.patch.object(Path, "iterdir", return_value=iter((Path("lo"), Path("eth0")) if wrong == "nic" else (Path("lo"),))), mock.patch.object(rich_guest.preflight_probe, "command", side_effect=command), mock.patch.object(rich_guest.assert_guest, "unit_properties", side_effect=properties), mock.patch.object(rich_guest, "run", side_effect=lambda *args: calls.append(args)):
                if wrong is None:
                    rich_guest.prepare_reboot()
                    self.assertEqual(calls, [("/usr/bin/systemctl", "mask", "--", "systemd-networkd-wait-online.service")])
                else:
                    with self.assertRaisesRegex(AssertionError, "^REBOOT_LAYOUT$"):
                        rich_guest.prepare_reboot()
                    if wrong != "mask":
                        self.assertEqual(calls, [])
        source = Path(rich_guest.__file__).read_text()
        exercise = source[source.index("def exercise("):]
        self.assertLess(exercise.index("prepare_reboot()"), exercise.index("p = receiver()"))
        self.assertNotIn("--now", source)
        reboot = exercise[exercise.index('prior = record(ROOT / "rich-result.json")'):]
        self.assertLess(reboot.index("retry_window(prior)"), reboot.index('activation_guest.command(binary, "start")'))
        self.assertIn('v["sequence"] == prior["pending"]["sequence"]', reboot)
        self.assertIn('v["sha256"] == prior["pending"]["digest"]', reboot)
        self.assertIn('pending_fresh(prior["pending"])', reboot)

    def test_retry_window_refuses_expired_future_and_insufficient_remaining_time(self):
        now = 1793318400 * 1_000_000_000
        prior = {"reboot_requested_at_ns": now - 20_000_000_000, "pending": {"collected_at": "2026-10-29T23:59:30Z"}}
        with mock.patch.object(rich_guest.time, "time_ns", return_value=now):
            rich_guest.retry_window(prior)
            for changed in ({"reboot_requested_at_ns": now - 90_000_000_000}, {"reboot_requested_at_ns": now + 1}, {"pending": {"collected_at": "2026-10-29T23:58:30Z"}}, {"pending": {"collected_at": "2026-10-30T00:00:00.000000001Z"}}):
                with self.assertRaisesRegex(AssertionError, "^RETRY_WINDOW$"):
                    rich_guest.retry_window({**prior, **changed})
            rich_guest.pending_fresh(prior["pending"])
            with self.assertRaisesRegex(AssertionError, "^RETRY_WINDOW$"):
                rich_guest.pending_fresh({"collected_at": "2026-10-29T23:58:00Z"})
            rich_guest.pending_fresh({"collected_at": "2026-10-29T23:58:00.000000001Z"})
            for invalid in ("2026-10-30T00:00:00+00:00", "2026-10-29T23:59:59.1234567890Z", "2026-13-29T23:59:59Z"):
                with self.assertRaisesRegex(AssertionError, "^RETRY_WINDOW$"):
                    rich_guest.pending_fresh({"collected_at": invalid})
            with mock.patch.object(rich_guest, "record") as read, self.assertRaisesRegex(AssertionError, "^RETRY_WINDOW$"):
                rich_guest.wait_counts(lambda _: True, {"collected_at": "2026-10-29T23:58:00Z"})
            read.assert_not_called()

    def test_failure_routes_only_closed_marker_to_serial_or_safe_fallback(self):
        for code, expected in (("RETRY_WINDOW", "RETRY_WINDOW"), ("START_RECOVERY_COLLECTOR_READY", "START_RECOVERY_COLLECTOR_READY"), ("private credential\nraw body", "UNKNOWN")):
            with mock.patch.object(Path, "open", autospec=True) as output:
                rich_guest.emit_failure(code)
            output.assert_called_once_with(Path("/dev/ttyS0"), "w", encoding="ascii")
            output.return_value.__enter__.return_value.write.assert_called_once_with("HLO_VM_FAIL_RICH_" + expected + "\n")
            stderr = io.StringIO()
            with mock.patch.object(Path, "open", side_effect=OSError("private failure")), contextlib.redirect_stderr(stderr):
                rich_guest.emit_failure(code)
            self.assertEqual(stderr.getvalue(), "HLO_VM_FAIL_RICH_" + expected + "\n")


if __name__ == "__main__":
    unittest.main()
