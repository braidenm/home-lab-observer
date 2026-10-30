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


def reader_state(**changes):
    return {"ActiveState": "active", "MainPID": "100", "InvocationID": "a" * 32,
            "Result": "success", "ExecMainCode": "0", "ExecMainStatus": "0",
            "NRestarts": "0", "Restart": "no", "Job": "", **changes}


class DockerGuestTest(unittest.TestCase):
    def test_existing_process_refusal_requires_exact_stderr_and_live_prerequisites(self):
        diagnostic = b"PREFLIGHT_REFUSED: verify privileges, supported host and rich bundle before enrollment.\n"
        cases = {
            "correct": (22, b"", diagnostic),
            "stdout-diagnostic": (22, diagnostic, b""),
            "wrong-exit": (0, b"", diagnostic),
            "unknown": (22, b"", b"private rejected output\n"),
            "oversized": (22, b"", diagnostic + b"x" * 1024),
            "extra-stdout": (22, b"unexpected", diagnostic),
            "extra-stderr": (22, b"", diagnostic + b"unexpected\n"),
            "changed-receipt": (22, b"", diagnostic),
            "primary-and-stop-failure": (22, b"unexpected", diagnostic),
            "stop-failure-only": (22, b"", diagnostic),
            "primary-and-terminate-failure": (22, b"unexpected", diagnostic),
            "primary-and-wait-failure": (22, b"unexpected", diagnostic),
            "terminate-failure-only": (22, b"", diagnostic),
            "wait-failure-only": (22, b"", diagnostic),
        }
        for prerequisite in ("containers", "socket", "receiver", "dns", "parent"):
            cases["unhealthy-" + prerequisite] = (22, b"", diagnostic)
        for name, (code, stdout, stderr) in cases.items():
            events, process, before = [], mock.Mock(), {"fixed": [1, 2, 3]}
            config = {"artifact_sha256": "a" * 64}
            def attempt(*args, **kwargs):
                events.append("attempt")
                return types.SimpleNamespace(returncode=code, stdout=stdout, stderr=stderr)
            def fingerprint():
                events.append("fingerprint")
                return before
            def receipt(path, limit):
                self.assertEqual((path, limit), (str(docker_guest.ROOT / "rich-receiver.json"), 1024))
                events.append("receipt")
                return b"changed" if name == "changed-receipt" and events.count("receipt") > 1 else b"fixed-receiver-receipt"
            def stop_engine():
                events.append("engine-stop")
                if name in ("primary-and-stop-failure", "stop-failure-only"):
                    raise AssertionError("ENGINE")
            def check_prerequisite(which):
                events.append(which)
                if name == "unhealthy-" + which:
                    raise AssertionError("ENGINE")
            def lstat(path):
                if path == Path("/run/docker.sock"):
                    check_prerequisite("socket")
                    return types.SimpleNamespace(st_mode=0o140660, st_uid=0, st_gid=22003, st_nlink=1)
                raise FileNotFoundError()
            def poll():
                events.append("receiver")
                return 22 if name == "unhealthy-receiver" else None
            def terminate():
                events.append("receiver-stop")
                if name in ("primary-and-terminate-failure", "terminate-failure-only"):
                    raise OSError("synthetic cleanup")
            process.poll.side_effect = poll
            if name in ("primary-and-wait-failure", "wait-failure-only"):
                process.wait.side_effect = docker_guest.subprocess.TimeoutExpired("fixed receiver", 5)
            with self.subTest(case=name), contextlib.ExitStack() as stack:
                for attribute, value in {"prepare_reboot": lambda: None, "package": lambda: (Path("/fixed/process"), "process-digest"),
                                         "receiver": lambda: process, "principals": lambda _: None, "stopped": lambda _: None,
                                         "verify_numeric": lambda: events.append("numeric")}.items():
                    stack.enter_context(mock.patch.object(docker_guest.rich_guest, attribute, side_effect=value))
                stack.enter_context(mock.patch.object(docker_guest, "install", side_effect=lambda *unused: events.append("process-install")))
                stack.enter_context(mock.patch.object(docker_guest, "package", return_value=(Path("/fixed/docker"), "digest")))
                stack.enter_context(mock.patch.object(docker_guest, "record", return_value=config))
                fingerprint_mock = stack.enter_context(mock.patch.object(docker_guest, "unchanged_process_state", side_effect=fingerprint))
                stack.enter_context(mock.patch.object(docker_guest.docker_engine, "prepare", side_effect=lambda first: events.append(("engine-prepare", first))))
                stack.enter_context(mock.patch.object(docker_guest.docker_engine, "prove_containers", side_effect=lambda: check_prerequisite("containers")))
                stack.enter_context(mock.patch.object(docker_guest.preflight_probe, "dns_tls", side_effect=lambda: check_prerequisite("dns")))
                stack.enter_context(mock.patch.object(docker_guest.preflight_probe, "parent_policy", side_effect=lambda: check_prerequisite("parent")))
                stop = stack.enter_context(mock.patch.object(docker_guest.docker_engine, "stop", side_effect=stop_engine))
                command = stack.enter_context(mock.patch.object(docker_guest.subprocess, "run", side_effect=attempt))
                stack.enter_context(mock.patch.object(docker_guest.preflight_probe, "fixed_file", side_effect=receipt))
                write = stack.enter_context(mock.patch.object(Path, "write_text"))
                stack.enter_context(mock.patch.object(Path, "chmod"))
                stack.enter_context(mock.patch.object(docker_guest.pwd, "getpwnam", side_effect=KeyError()))
                stack.enter_context(mock.patch.object(docker_guest.grp, "getgrnam", side_effect=KeyError()))
                stack.enter_context(mock.patch.object(Path, "lstat", autospec=True, side_effect=lstat))
                stack.enter_context(mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=lambda unit, fields: {"Id": unit, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}))
                process.terminate.side_effect = terminate
                if name == "correct":
                    docker_guest.refusal("first")
                    self.assertEqual(fingerprint_mock.call_count, 2)
                    self.assertEqual(events.count("receipt"), 2)
                    write.assert_called_once()
                    self.assertLess(events.index(("engine-prepare", True)), events.index("process-install"))
                    self.assertLess(events.index(("engine-prepare", True)), events.index("fingerprint"))
                    self.assertLess(events.index("fingerprint"), events.index("attempt"))
                    self.assertLess(events.index("attempt"), events.index("engine-stop"))
                    self.assertLess(events.index("engine-stop"), events.index("receiver-stop"))
                    self.assertGreater(events.index("receiver-stop"), max(i for i, event in enumerate(events) if event == "receipt"))
                    ordered = ["fingerprint", "receipt", "containers", "socket", "receiver", "dns", "parent", "attempt"]
                    self.assertEqual([events.index(event) for event in ordered], sorted(events.index(event) for event in ordered))
                    self.assertEqual(events[-1], "numeric")
                elif name == "stop-failure-only":
                    with self.assertRaisesRegex(AssertionError, "^ENGINE$"):
                        docker_guest.refusal("first")
                    write.assert_called_once()
                    self.assertNotIn("numeric", events)
                elif name in ("terminate-failure-only", "wait-failure-only"):
                    with self.assertRaises((OSError, docker_guest.subprocess.TimeoutExpired)):
                        docker_guest.refusal("first")
                    write.assert_called_once()
                    self.assertNotIn("numeric", events)
                else:
                    with self.assertRaisesRegex(AssertionError, "^REFUSAL$"):
                        docker_guest.refusal("first")
                    self.assertEqual(fingerprint_mock.call_count, 2 if name == "changed-receipt" else 1)
                    write.assert_not_called()
                    self.assertNotIn("numeric", events)
                stop.assert_called_once_with()
                process.terminate.assert_called_once_with()
                process.wait.assert_called_once_with(timeout=5)
                if name.startswith("unhealthy-"):
                    command.assert_not_called()
                else:
                    self.assertEqual(command.call_args.args[0], ["/fixed/docker/observer-connected-rich-docker-install", "install", "/fixed/docker", "digest", docker_guest.SERVER])
                    self.assertEqual(command.call_args.kwargs, {"stdin": docker_guest.subprocess.DEVNULL, "stdout": docker_guest.subprocess.PIPE,
                                                               "stderr": docker_guest.subprocess.PIPE, "timeout": 20})

    def test_refusal_reboot_checks_daemon_without_changing_it(self):
        for changed in (None, "active", "pid", "enabled", "fingerprint"):
            daemon = {"ActiveState": "inactive", "MainPID": "0", "UnitFileState": "static"}
            if changed == "active": daemon["ActiveState"] = "active"
            if changed == "pid": daemon["MainPID"] = "41"
            if changed == "enabled": daemon["UnitFileState"] = "enabled"
            def properties(unit, fields):
                if unit == docker_guest.docker_engine.UNIT:
                    self.assertEqual(fields, ("ActiveState", "MainPID", "UnitFileState"))
                    return daemon
                return {"Id": unit, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}
            def record(path, *unused):
                return {"artifact_sha256": "a" * 64} if path.name == "installed.json" else {"fixed": "before"}
            with self.subTest(changed=changed), mock.patch.object(docker_guest, "record", side_effect=record), \
                 mock.patch.object(docker_guest, "unchanged_process_state", return_value={"fixed": "changed" if changed == "fingerprint" else "before"}), \
                 mock.patch.object(docker_guest.rich_guest, "principals"), mock.patch.object(docker_guest.rich_guest, "stopped"), \
                 mock.patch.object(docker_guest.rich_guest, "verify_numeric") as numeric, \
                 mock.patch.object(docker_guest.pwd, "getpwnam", side_effect=KeyError()), \
                 mock.patch.object(docker_guest.grp, "getgrnam", side_effect=KeyError()), \
                 mock.patch.object(Path, "lstat", side_effect=FileNotFoundError()), \
                 mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=properties), \
                 mock.patch.object(docker_guest.docker_engine, "prepare") as prepare, \
                 mock.patch.object(docker_guest.docker_engine, "start") as start, \
                 mock.patch.object(docker_guest.docker_engine, "stop") as stop:
                if changed is None:
                    docker_guest.refusal("reboot")
                    numeric.assert_called_once_with()
                else:
                    with self.assertRaisesRegex(AssertionError, "^REFUSAL$"):
                        docker_guest.refusal("reboot")
                    numeric.assert_not_called()
                prepare.assert_not_called(); start.assert_not_called(); stop.assert_not_called()

    def test_refusal_requires_fixed_user_group_and_unloaded_unit_paths_absent(self):
        groups = tuple("hlo-rich-docker-" + role for role in ("uploader", "read", "frame", "collector", "reader"))
        users = tuple("hlo-rich-docker-" + role for role in ("reader", "collector", "uploader"))
        unit_paths = [Path("/etc/systemd/system", unit + suffix) for unit in (*docker_guest.UNITS, docker_guest.ENROLLMENT) for suffix in ("", ".d")]
        unit_paths += [Path("/run/home-lab-observer-connected-rich", name) for name in ("activation", "reader-activation")]
        for kind, target in [(None, None), *(("group", group) for group in groups), *(("user", user) for user in users),
                             *(("path", path) for path in unit_paths), *(("manager-" + field, docker_guest.ENROLLMENT) for field in ("Id", "LoadState", "ActiveState", "MainPID", "Transient", "FragmentPath"))]:
            def lookup(which, name):
                if kind == which and name == target:
                    return object()
                raise KeyError()
            def lstat(path):
                if kind == "path" and path == target:
                    # Includes a dangling symlink; existence alone must refuse.
                    return types.SimpleNamespace(st_mode=0o120777)
                raise FileNotFoundError()
            def properties(unit, fields):
                self.assertEqual(fields, ("Id", "LoadState", "ActiveState", "MainPID", "Transient", "FragmentPath"))
                result = {"Id": unit, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}
                if kind and kind.startswith("manager-") and target == unit:
                    result[kind.removeprefix("manager-")] = "unexpected"
                return result
            with self.subTest(kind=kind, target=target), mock.patch.object(docker_guest.pwd, "getpwnam", side_effect=lambda name: lookup("user", name)) as user_check, \
                 mock.patch.object(docker_guest.grp, "getgrnam", side_effect=lambda name: lookup("group", name)) as group_check, \
                 mock.patch.object(Path, "lstat", autospec=True, side_effect=lstat) as path_check, \
                 mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=properties):
                if kind is None:
                    docker_guest.refusal_targets_absent()
                    self.assertEqual({call.args[0] for call in user_check.call_args_list}, set(users))
                    self.assertEqual({call.args[0] for call in group_check.call_args_list}, set(groups))
                    self.assertEqual({call.args[0] for call in path_check.call_args_list}, set(unit_paths))
                else:
                    with self.assertRaisesRegex(AssertionError, "^REFUSAL$"):
                        docker_guest.refusal_targets_absent()

    def test_refusal_prerequisite_socket_metadata_is_exact(self):
        expected = {"st_mode": 0o140660, "st_uid": 0, "st_gid": 22003, "st_nlink": 1}
        for changed in ({}, {"st_mode": 0o100660}, {"st_mode": 0o140666}, {"st_uid": 1}, {"st_gid": 0}, {"st_gid": 22004}, {"st_nlink": 0}, {"st_nlink": 2}):
            with self.subTest(changed=changed), mock.patch.object(Path, "lstat", return_value=types.SimpleNamespace(**(expected | changed))), \
                 mock.patch.object(docker_guest.docker_engine, "prove_containers"), mock.patch.object(docker_guest.preflight_probe, "dns_tls"), \
                 mock.patch.object(docker_guest.preflight_probe, "parent_policy"):
                process = mock.Mock()
                process.poll.return_value = None
                if changed:
                    with self.assertRaisesRegex(AssertionError, "^REFUSAL$"):
                        docker_guest.refusal_prerequisites(process)
                else:
                    docker_guest.refusal_prerequisites(process)

    def test_stale_reader_fault_keeps_its_sequence_and_never_replaces_socket(self):
        events = []
        fault = {"sequence": 8, "unavailable_count": 2, "healthy_sequence": 0}
        good = {"sequence": 9, "healthy_sequence": 9, **{name: True for name in ("large_body", "foreign_process", "running_container", "exited_null", "warmed_metrics")}}
        values = iter((fault, good))
        def wait(predicate):
            value = next(values)
            self.assertTrue(predicate(value))
            events.append("unavailable" if value is fault else "healthy")
            return value
        with mock.patch.object(docker_guest, "counts", return_value={"unavailable_count": 1}), \
             mock.patch.object(docker_guest, "run", side_effect=lambda *args: events.append(args)), \
             mock.patch.object(docker_guest, "wait_counts", side_effect=wait), \
             mock.patch.object(docker_guest.activation_guest, "command", side_effect=lambda _, operation: events.append(operation)), \
             mock.patch.object(docker_guest, "stopped", side_effect=lambda _: events.append("stopped")), \
             mock.patch.object(docker_guest, "socket_replacement") as replacement:
            docker_guest.fault_journey(Path("/fixed"))
        self.assertEqual(events, [("/usr/bin/systemctl", "stop", "--", docker_guest.UNITS[1]), "unavailable", "stop", "stopped", "start", "healthy"])
        replacement.assert_not_called()

    def test_socket_replacement_runs_only_after_reboot_retry_fresh_ack_and_isolation(self):
        held = {"sequence": 5, "digest": "fixed-digest"}
        config = {"collection_profile": "RICH_PROCESSES_DOCKER_V1", "server_id": docker_guest.SERVER, "artifact_sha256": "a" * 64}
        identity = {"fixed": "identity"}
        prior = {"identity": identity, "pending": held, "duplicates": 0}
        healthy_fields = {name: True for name in ("large_body", "foreign_process", "running_container", "exited_null", "warmed_metrics")}
        for mode in ("first", "reboot"):
            events, starts, waits, status_checks, acknowledged = [], 0, 0, 0, False
            process = mock.Mock()
            def command(_, operation):
                nonlocal starts, status_checks, acknowledged
                if operation == "start":
                    starts += 1
                    if mode == "first" and starts == 3:
                        events.append("tamper-refused")
                        raise docker_guest.activation_guest.Refused("COMMAND")
                events.append(operation)
                if operation == "status":
                    status_checks += 1
                    if status_checks == 1:
                        return b'{"state":"PENDING"}'
                    acknowledged = True
                    events.append("ack")
                return b'{"state":"ACKNOWLEDGED_FRESH"}'
            def replacement(_):
                self.assertTrue(acknowledged)
                events.append("socket-replacement")
            def wait(predicate, pending=None):
                nonlocal waits
                waits += 1
                sequence = 5 if mode == "first" or pending is not None else 6
                value = {**healthy_fields, "sequence": sequence, "healthy_sequence": sequence,
                         "sha256": held["digest"], "duplicates": 1}
                self.assertTrue(predicate(value))
                events.append("retry" if pending is not None else "held" if mode == "first" and waits == 2 else "fresh")
                return value
            replacements = {
                "record": lambda path, *unused: config if path.name == "installed.json" else prior,
                "package": lambda: (Path("/fixed/package"), "fixed-digest"), "install": lambda *unused: None,
                "receiver": lambda: process, "principals": lambda _: None,
                "fingerprint": lambda: identity, "stopped": lambda _: events.append("stopped"),
                "pending": lambda: events.append("pending") or held,
                "counts": lambda: {"sequence": 1}, "wait_counts": wait,
                "isolation": lambda _: events.append("isolation"),
                "fault_journey": lambda _: events.append("stale-fault"),
                "socket_replacement": replacement,
            }
            with self.subTest(mode=mode), contextlib.ExitStack() as stack:
                stack.enter_context(mock.patch.object(docker_guest.os, "geteuid", return_value=0))
                stack.enter_context(mock.patch.object(docker_guest.time, "sleep"))
                for name, value in replacements.items(): stack.enter_context(mock.patch.object(docker_guest, name, side_effect=value))
                stack.enter_context(mock.patch.object(docker_guest.activation_guest, "command", side_effect=command))
                for name, value in {"prepare_reboot": lambda: None, "verify_numeric": lambda: events.append("numeric"),
                                    "sleepers": lambda _: [], "join_sleepers": lambda _: None,
                                    "retry_window": lambda _: events.append("retry-window"),
                                    "pending_fresh": lambda _: events.append("pending-fresh")}.items():
                    stack.enter_context(mock.patch.object(docker_guest.rich_guest, name, side_effect=value))
                stack.enter_context(mock.patch.object(docker_guest.docker_engine, "prepare"))
                stack.enter_context(mock.patch.object(docker_guest.docker_engine, "stop", side_effect=lambda: events.append("engine-stop")))
                stack.enter_context(mock.patch.object(Path, "read_bytes", return_value=b'{"containers_enabled":true}'))
                for name in ("touch", "write_bytes", "write_text", "chmod"):
                    stack.enter_context(mock.patch.object(Path, name))
                stack.enter_context(mock.patch.object(Path, "unlink", side_effect=lambda: events.append("unhold")))
                stack.enter_context(mock.patch.object(Path, "iterdir", side_effect=lambda: iter(())))
                docker_guest.exercise("rich-docker", mode)
            if mode == "first":
                self.assertEqual(events.count("stale-fault"), 1)
                self.assertNotIn("socket-replacement", events)
                self.assertLess(events.index("stale-fault"), events.index("held"))
                self.assertLess(events.index("held"), events.index("tamper-refused"))
            else:
                self.assertNotIn("stale-fault", events)
                self.assertEqual(events.count("socket-replacement"), 1)
                self.assertEqual(status_checks, 2)
                ordered = ["retry-window", "unhold", "start", "retry", "pending-fresh", "fresh", "status", "ack", "isolation", "socket-replacement", "stop", "stopped", "engine-stop"]
                indices = [events.index(name) for name in ordered]
                # The initial stopped() precondition precedes startup; select the final one.
                indices[-2] = events.index("stopped", events.index("stop"))
                self.assertEqual(indices, sorted(indices))
            process.terminate.assert_called_once_with()
            process.wait.assert_called_once_with(timeout=5)

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

    def test_reader_terminal_requires_exact_old_failure_and_no_adoption(self):
        import stat
        initial = {"manager": {"ActiveState": "active", "MainPID": "100", "InvocationID": "a" * 32}, "socket": [7, 8]}
        failed = reader_state(ActiveState="failed", MainPID="0", Result="exit-code", ExecMainCode="1", ExecMainStatus="22")
        negatives = ({"ActiveState": "inactive"}, {"MainPID": "100"}, {"InvocationID": ""}, {"InvocationID": "b" * 32},
                     {"Result": "success", "ExecMainStatus": "0"}, {"Result": "signal", "ExecMainCode": "2"},
                     {"ExecMainStatus": "0"}, {"ExecMainStatus": "23"}, {"ExecMainCode": "0"}, {"NRestarts": "1"},
                     {"Restart": "always"}, {"Job": "123"}, {"extra": "hlc_private"})
        for changes in ({}, *negatives):
            with self.subTest(changes=changes), mock.patch.object(docker_guest.assert_guest, "unit_properties", return_value={**failed, **changes}):
                if not changes:
                    self.assertEqual(docker_guest.wait_reader_termination(initial), failed)
                else:
                    with self.assertRaises(AssertionError):
                        docker_guest.wait_reader_termination(initial)
        for changes in ({}, {"MainPID": "101"}, {"InvocationID": "b" * 32}, {"NRestarts": "1"}, {"adopted": True}):
            active = reader_state(**{k: v for k, v in changes.items() if k != "adopted"})
            node = types.SimpleNamespace(st_dev=7, st_ino=9 if changes.get("adopted") else 8, st_mode=stat.S_IFSOCK)
            with self.subTest(active=changes), mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=[active, active, failed, failed]), mock.patch.object(Path, "stat", return_value=node), mock.patch.object(docker_guest.time, "sleep"):
                if not changes:
                    self.assertEqual(docker_guest.wait_reader_termination(initial), failed)
                else:
                    with self.assertRaises(AssertionError):
                        docker_guest.wait_reader_termination(initial)
        # The original process may exit between the active manager snapshot and
        # /proc metadata read; only its exact terminal failure can close that race.
        with mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=[reader_state(), failed, failed]), mock.patch.object(Path, "stat", side_effect=FileNotFoundError()):
            self.assertEqual(docker_guest.wait_reader_termination(initial), failed)
        retiring = reader_state(ActiveState="deactivating", MainPID="0", Result="exit-code", ExecMainCode="1", ExecMainStatus="22")
        node = types.SimpleNamespace(st_dev=7, st_ino=8, st_mode=stat.S_IFSOCK)
        with mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=[reader_state(), retiring, retiring, failed, failed]), mock.patch.object(Path, "stat", return_value=node), mock.patch.object(docker_guest.time, "sleep"):
            self.assertEqual(docker_guest.wait_reader_termination(initial), failed)
        for changes in ({"MainPID": "101"}, {"InvocationID": "b" * 32}, {"ExecMainStatus": "0"}, {"ExecMainCode": "2"}):
            with mock.patch.object(docker_guest.assert_guest, "unit_properties", return_value={**retiring, **changes}), self.assertRaises(AssertionError):
                docker_guest.wait_reader_termination(initial)
        with mock.patch.object(docker_guest.time, "monotonic", side_effect=[0, 46]), self.assertRaises(AssertionError):
            docker_guest.wait_reader_termination(initial)

    def test_socket_replacement_requires_terminal_then_explicit_fresh_recovery(self):
        initial = {"manager": {"ActiveState": "active", "MainPID": "100", "InvocationID": "a" * 32}, "socket": [7, 8]}
        failed = reader_state(ActiveState="failed", MainPID="0", Result="exit-code", ExecMainCode="1", ExecMainStatus="22")
        for change in (None, "reused-pid", "terminal-changed", "before-start-invocation", "stopped-reader-cleared", "same-invocation", "wrong-socket", "restart", "stale-unavailable", "stale-health", "unchanged-sequence", "post-health-invocation"):
            fresh = {"manager": {"ActiveState": "active", "MainPID": "101", "InvocationID": "b" * 32}, "socket": [7, 9]}
            if change == "reused-pid": fresh["manager"]["MainPID"] = "100"
            if change == "same-invocation": fresh["manager"]["InvocationID"] = "a" * 32
            if change == "wrong-socket": fresh["socket"] = [7, 8]
            final = {"manager": dict(fresh["manager"]), "socket": fresh["socket"][:]}
            if change == "post-health-invocation": final["manager"]["InvocationID"] = "c" * 32
            terminal = {**failed, **({"InvocationID": "b" * 32} if change == "terminal-changed" else {})}
            stopped = {**failed, **({"InvocationID": "c" * 32} if change == "before-start-invocation" else {})}
            if change == "stopped-reader-cleared": stopped.update(ActiveState="inactive", InvocationID="")
            active = reader_state(MainPID=fresh["manager"]["MainPID"], InvocationID=fresh["manager"]["InvocationID"], NRestarts="1" if change == "restart" else "0")
            fault = {"unavailable_count": 2, "healthy_sequence": 0, "sequence": 8}
            if change == "stale-unavailable": fault["unavailable_count"] = 1
            good = {**{name: True for name in ("large_body", "foreign_process", "running_container", "exited_null", "warmed_metrics")}, "healthy_sequence": 9, "sequence": 9}
            if change == "stale-health": good["healthy_sequence"] = 8
            if change == "unchanged-sequence": good["healthy_sequence"] = good["sequence"] = 8
            receipts = iter((fault, good))
            def wait(predicate):
                value = next(receipts)
                if not predicate(value): raise AssertionError("RETRY")
                return value
            states = iter((reader_state(), terminal, stopped, stopped, active, active))
            def properties(unit, fields):
                if "NRestarts" in fields: return next(states)
                if unit == docker_guest.ENROLLMENT: return {"Id": unit, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "no", "FragmentPath": ""}
                return {"Id": unit, "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/" + unit, "ActiveState": "failed" if unit == docker_guest.UNITS[1] else "inactive", "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "", "NeedDaemonReload": "no"}
            with self.subTest(change=change), mock.patch.object(docker_guest, "reader_binding", side_effect=[initial, fresh, final]), mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=properties), mock.patch.object(docker_guest, "wait_reader_termination", return_value=failed), mock.patch.object(Path, "stat", side_effect=[types.SimpleNamespace(st_dev=7, st_ino=8), types.SimpleNamespace(st_dev=7, st_ino=9), types.SimpleNamespace(st_dev=7, st_ino=9)]), mock.patch.object(docker_guest, "counts", return_value={"unavailable_count": 1}), mock.patch.object(docker_guest.docker_engine, "stop"), mock.patch.object(docker_guest.docker_engine, "start"), mock.patch.object(docker_guest.docker_engine, "cli"), mock.patch.object(docker_guest, "wait_counts", side_effect=wait), mock.patch.object(docker_guest.activation_guest, "command", return_value=b'{"state":"RECOVERY_REQUIRED"}') as command, mock.patch.object(docker_guest.rich_guest, "verify_numeric"):
                if change in (None, "reused-pid"):
                    docker_guest.socket_replacement(Path("/fixed"))
                    self.assertEqual(command.call_args_list, [mock.call(Path("/fixed"), "stop"), mock.call(Path("/fixed"), "status"), mock.call(Path("/fixed"), "start")])
                else:
                    with self.assertRaisesRegex(AssertionError, "^FAULT_[A-Z_]+$"):
                        docker_guest.socket_replacement(Path("/fixed"))
                    if change in ("terminal-changed", "stale-unavailable"): command.assert_not_called()
                    if change in ("before-start-invocation", "stopped-reader-cleared"): self.assertEqual(command.call_args_list, [mock.call(Path("/fixed"), "stop")])

    def test_recovery_stop_proves_failed_reader_without_weakening_normal_stop(self):
        failed = reader_state(ActiveState="failed", MainPID="0", Result="exit-code", ExecMainCode="1", ExecMainStatus="22")
        for change in (None, "reader-invocation", "collector-active", "drop-in", "enrollment", "fake-stopped-status"):
            def properties(unit, fields):
                if "NRestarts" in fields: return {**failed, **({"InvocationID": "b" * 32} if change == "reader-invocation" else {})}
                if unit == docker_guest.ENROLLMENT: return {"Id": unit, "LoadState": "not-found", "ActiveState": "inactive", "MainPID": "0", "Transient": "yes" if change == "enrollment" else "no", "FragmentPath": ""}
                return {"Id": unit, "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/" + unit, "ActiveState": "failed" if unit == docker_guest.UNITS[1] else ("active" if change == "collector-active" else "inactive"), "MainPID": "0", "UnitFileState": "disabled", "DropInPaths": "foreign" if change == "drop-in" else "", "NeedDaemonReload": "no"}
            with self.subTest(change=change), mock.patch.object(docker_guest.assert_guest, "unit_properties", side_effect=properties), mock.patch.object(docker_guest.activation_guest, "command", return_value=b'{"state":"STOPPED"}' if change == "fake-stopped-status" else b'{"state":"RECOVERY_REQUIRED"}') as command, mock.patch.object(docker_guest.rich_guest, "verify_numeric") as numeric:
                if change is None:
                    docker_guest.recovery_stopped(Path("/fixed"), failed)
                    command.assert_called_once_with(Path("/fixed"), "status")
                    numeric.assert_called_once_with()
                    with self.assertRaisesRegex(AssertionError, "^STOPPED$"): docker_guest.stopped(Path("/fixed"))
                else:
                    with self.assertRaises(AssertionError): docker_guest.recovery_stopped(Path("/fixed"), failed)

    def test_fault_phases_preserve_known_lifecycle_and_sanitize_unknown_errors(self):
        cases = ((AssertionError("FAULT"), "FAULT_INITIAL_BINDING"),
                 (AssertionError("hlc_private"), "UNKNOWN"), (OSError("hlc_private"), "UNKNOWN"),
                 (docker_guest.activation_guest.Refused("START_COMMAND_READER_SOCKET"), "START_COMMAND_READER_SOCKET"),
                 (docker_guest.activation_guest.Refused("START_COMMAND_hlc_private"), "UNKNOWN"))
        for error, expected in cases:
            output = io.StringIO()
            def exercise(*unused): docker_guest.socket_replacement(Path("/fixed"))
            with self.subTest(expected=expected), mock.patch.object(docker_guest.sys, "argv", ["docker_guest.py", "rich-docker", "first"]), mock.patch.object(docker_guest, "exercise", side_effect=exercise), mock.patch.object(Path, "stat", side_effect=error), mock.patch.object(Path, "open", side_effect=OSError()), contextlib.redirect_stderr(output):
                self.assertEqual(docker_guest.main(), 1)
            self.assertEqual(output.getvalue(), "HLO_VM_FAIL_DOCKER_" + expected + "\n")

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

    def test_lifecycle_refusal_emits_only_known_phase_and_operation(self):
        for reason, expected in (("START_COMMAND_READER_SOCKET", "START_COMMAND_READER_SOCKET"),
                                 ("START_RECOVERY_READER_STARTUP", "START_RECOVERY_READER_STARTUP"),
                                 ("STOP_COMMAND_READER_SOCKET", "STOP_COMMAND_READER_SOCKET"),
                                 ("STATUS_RECOVERY_READER_STARTUP", "STATUS_RECOVERY_READER_STARTUP"),
                                 ("START_COMMAND_READER_PRIVATE_TOKEN", "UNKNOWN"),
                                 ("COMMAND_READER_SOCKET", "UNKNOWN"),
                                 ("hlc_private", "UNKNOWN")):
            output = io.StringIO()
            with mock.patch.object(docker_guest.sys, "argv", ["docker_guest.py", "first", "bundle"]), \
                 mock.patch.object(docker_guest, "exercise", side_effect=docker_guest.activation_guest.Refused(reason)), \
                 mock.patch.object(Path, "open", side_effect=OSError()), \
                 contextlib.redirect_stderr(output):
                self.assertEqual(docker_guest.main(), 1)
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
