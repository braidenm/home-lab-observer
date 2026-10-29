import hashlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest import mock

import docker_scope
import run_vm


class DockerScopeTest(unittest.TestCase):
    def test_scope_is_explicit_and_does_not_expand_existing_cases(self):
        self.assertEqual(docker_scope.cases("rich-docker"), ("rich-docker", "rich-docker-refusal"))
        for scope in ("all", "stopped", "activation", "rich-processes"):
            self.assertIsNone(docker_scope.cases(scope))
        self.assertEqual(run_vm.harness_files(), run_vm.HARNESS_FILES)
        self.assertEqual(run_vm.harness_files("rich-docker"), run_vm.HARNESS_FILES + run_vm.RICH_HARNESS_FILES + docker_scope.FILES)

    def test_harness_digest_binds_every_docker_source(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name in run_vm.harness_files("rich-docker"):
                (root / name).write_bytes(name.encode())
            initial = run_vm.harness_sha256(root, "rich-docker")
            for name in docker_scope.FILES:
                (root / name).write_bytes(b"changed")
                with self.assertRaisesRegex(RuntimeError, "^REVIEWED_HARNESS_REFUSED$"):
                    run_vm.check_harness(root, initial, "rich-docker")
                (root / name).write_bytes(name.encode())

    def test_all_pins_are_required_before_file_access(self):
        read = mock.Mock()
        for pins in ((None, "a" * 64, "b" * 64), ("bad", "a" * 64, "b" * 64)):
            with self.assertRaisesRegex(RuntimeError, "^DOCKER_PINS_REFUSED$"):
                docker_scope.engine_inputs("/x", *pins, read, None)
        read.assert_not_called()

    def test_rootfs_is_one_canonical_file_not_arbitrary_extraction(self):
        binary = b"\x7fELF" + b"synthetic" * 4
        raw = docker_scope.rootfs_bytes(binary)
        docker_scope.validate_rootfs(raw, binary)
        for changed in (raw + b"hidden", raw.replace(b"fixture", b"foreign", 1), raw[:-1]):
            with self.assertRaisesRegex(RuntimeError, "^DOCKER_ROOTFS_REFUSED$"):
                docker_scope.validate_rootfs(changed, binary)
        with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
            member, = archive.getmembers()
            self.assertEqual((member.name, member.uid, member.gid, member.mode, member.mtime), ("fixture", 0, 0, 0o555, 0))

    def test_engine_inputs_closed_signed_sizes_hashes_and_image_pins(self):
        digest = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            binary = root / "fixture"
            binary.write_bytes(b"\x7fELFsynthetic")
            image = root / "fixture-rootfs.tar"
            image.write_bytes(docker_scope.rootfs_bytes(binary.read_bytes()))
            packages = {}
            for index in range(3):
                p = root / ("package%d.deb" % index)
                p.write_bytes(bytes([index]) * 12)
                packages[p.name] = (12, digest(p))
            calls = []
            def owned(raw, limit):
                calls.append((Path(raw).name, limit))
                return Path(raw)
            with mock.patch.object(docker_scope, "PACKAGES", packages):
                sources, identity = docker_scope.engine_inputs(str(root), digest(image), digest(binary), "c" * 40, owned, digest)
                self.assertEqual(set(sources), set(packages) | {"fixture-rootfs.tar", "fixture"})
                self.assertEqual(identity["fixture_sha256"], digest(binary))
                binary.write_bytes(b"changed")
                with self.assertRaisesRegex(RuntimeError, "^DOCKER_IMAGE_REFUSED$"):
                    docker_scope.engine_inputs(str(root), digest(image), identity["fixture_sha256"], "c" * 40, owned, digest)
            self.assertEqual(len(calls), 7)  # changed binary refuses before any package access

    def test_four_binary_manifest_is_not_a_three_binary_profile(self):
        base = {"commit": "a" * 40, "profile": docker_scope.PROFILE, "schema": "observer-connected-bundle/v2", "os": "linux", "arch": "amd64", "files": [{"name": name} for name in docker_scope.BINARIES]}
        docker_scope.check_identity(base, "a" * 40)
        for changed in ({"profile": "ubuntu24.04-systemd255-amd64-rich-processes-canary"}, {"files": base["files"][:-1]}, {"commit": "b" * 40}):
            with self.assertRaisesRegex(RuntimeError, "^DOCKER_IDENTITY_REFUSED$"):
                docker_scope.check_identity({**base, **changed}, "a" * 40)


if __name__ == "__main__":
    unittest.main()
