from pathlib import Path
import unittest
from unittest import mock

import docker_engine


class DockerEngineTest(unittest.TestCase):
    def test_daemon_has_only_fixed_unix_socket_and_no_network_side_effects(self):
        command = docker_engine.daemon_command()
        for flag in ("--bridge=none", "--iptables=false", "--ip6tables=false", "--ip-forward=false", "--ip-masq=false", "--userland-proxy=false", "--storage-driver=vfs", "--host=unix:///run/docker.sock"):
            self.assertEqual(command.count(flag), 1)
        self.assertFalse(any("tcp://" in value or "0.0.0.0" in value for value in command))
        self.assertIn("--config-file=/var/lib/hlo-first-install-fixture/docker-daemon.json", command)

    def test_fixed_containers_keep_default_apparmor_seccomp_and_caps(self):
        for name in docker_engine.NAMES:
            args = docker_engine.container_command(name)
            for flag in ("--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges=true", "--memory=32m", "--memory-swap=32m", "--cpus=0.25", "--pids-limit=32", "--restart=no", "--log-driver=none"):
                self.assertIn(flag, args)
            self.assertFalse(any("unconfined" in value or "--privileged" in value or "--mount" in value or "--volume" in value for value in args))
        with self.assertRaisesRegex(AssertionError, "^ENGINE$"):
            docker_engine.container_command("foreign")

    def test_engine_version_must_really_negotiate_adapter_api(self):
        good = {"Version": "29.9.2", "ApiVersion": "1.54", "MinAPIVersion": "1.40", "Os": "linux"}
        docker_engine.check_version(good)
        for changed in ({"MinAPIVersion": "1.46"}, {"ApiVersion": "1.40"}, {"Version": "29.9.1"}, {"Os": "windows"}, {"ApiVersion": "1.54junk"}, {"MinAPIVersion": "2.0"}):
            with self.assertRaisesRegex(AssertionError, "^ENGINE_API$"):
                docker_engine.check_version({**good, **changed})

    def test_private_import_has_fixed_nonroot_user_and_entrypoint(self):
        args = docker_engine.import_command()
        self.assertEqual(args[-2:], ("/mnt/hlo-first-install/fixture-rootfs.tar", docker_engine.IMAGE))
        self.assertIn("USER 65532:65532", args)
        self.assertIn('ENTRYPOINT ["/fixture"]', args)

    def test_network_guard_requires_both_families_and_only_loopback(self):
        with mock.patch.object(Path, "iterdir", return_value=[Path("lo")]), mock.patch.object(docker_engine.preflight_probe, "command", return_value=b"") as command:
            docker_engine.no_network()
        self.assertEqual(command.call_args_list, [mock.call("/usr/sbin/ip", family, "route", "show", "default", limit=1024) for family in ("-4", "-6")])
        with mock.patch.object(Path, "iterdir", return_value=[Path("lo"), Path("eth0")]):
            with self.assertRaisesRegex(AssertionError, "^ENGINE_NETWORK$"):
                docker_engine.no_network()


if __name__ == "__main__":
    unittest.main()
