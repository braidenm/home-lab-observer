"""Synthetic closed-candidate tests; no downloaded command or host collector runs."""

import copy
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import connected_candidate as policy


VERSION = "0.1.0-canary.1"
SOURCE = "a" * 40
CONTRACT = "b" * 64
WORKFLOW_ID = 12345


def fixture_metadata():
    return dict(zip(policy.FIELDS, (policy.SCHEMA_VERSION, policy.REPOSITORY_ID, policy.OWNER_ID,
        policy.WORKFLOW_PATH, WORKFLOW_ID, "refs/heads/main", 98765, 2, SOURCE, VERSION, CONTRACT,
        [{"name": name, "size": 10, "sha256": "c" * 64} for name in policy.asset_names(VERSION)])))


def fixture_payload(contract=CONTRACT):
    """Portable synthetic bytes, not a real package or release-acceptance fixture."""
    names = policy.asset_names(VERSION)
    manifest = {"schema": "observer-connected-bundle/v2", "version": VERSION, "commit": SOURCE,
                "contract_sha256": contract, "os": "linux", "arch": "amd64",
                "profile": "ubuntu24.04-systemd255-amd64-canary", "files": []}
    content = [b"synthetic-archive-not-executed", json.dumps(manifest).encode(), b"synthetic-helper-not-executed"]
    sums = "".join(hashlib.sha256(data).hexdigest() + "  " + name + "\n"
                   for name, data in zip(names, content)).encode()
    contents = dict(zip(names, content + [sums]))
    metadata = fixture_metadata()
    metadata["contract_sha256"] = contract
    metadata["assets"] = [{"name": name, "size": len(data), "sha256": hashlib.sha256(data).hexdigest()}
                          for name, data in contents.items()]
    contents["candidate.json"] = policy.encode_metadata(metadata)
    return metadata, contents


def write_assets(directory, contract=CONTRACT):
    _, contents = fixture_payload(contract)
    for name, data in contents.items():
        if name == "candidate.json":
            continue
        path = directory / name
        path.write_bytes(data)
        path.chmod(0o644)


class MetadataTests(unittest.TestCase):
    def test_canonical_roundtrip_and_exact_workflow_identity(self):
        metadata = fixture_metadata()
        encoded = policy.encode_metadata(metadata)
        self.assertEqual(metadata, policy.decode_metadata(encoded, WORKFLOW_ID))
        self.assertTrue(encoded.endswith(b"\n"))
        self.assertEqual("connected-linux-0.1.0-canary.1-98765-2", policy.artifact_name(metadata))
        with self.assertRaisesRegex(ValueError, "^connected_candidate_invalid$"):
            policy.decode_metadata(encoded, WORKFLOW_ID + 1)

    def test_noncanonical_duplicate_unknown_and_oversized_json_refuse(self):
        good = policy.encode_metadata(fixture_metadata())
        bad = [good[:-1], b" " + good, good.replace(b'"run_id":98765', b'"run_id":98765,"run_id":98765'),
               good.replace(b'"run_id":98765', b'"run_id":98765,"extra":1'),
               good.replace(b'"run_id":98765', b'"run_id":NaN'), b" " * 4097,
               good.replace(b'"run_id":98765', b'"run_id":9.8765e4')]
        for data in bad:
            with self.subTest(data=data[:60]), self.assertRaises(ValueError):
                policy.decode_metadata(data)

    def test_identity_and_asset_mutations_refuse(self):
        mutations = [("repository_id", 1), ("owner_id", 1), ("workflow_id", True), ("run_id", 0),
                     ("run_attempt", -1), ("run_id", 2**53), ("ref", "refs/heads/feature"),
                     ("workflow_path", ".github/workflows/other.yml"), ("source", "A" * 40),
                     ("contract_sha256", "B" * 64), ("schema_version", "other"),
                     ("version", "0.1.0-preview.1"), ("version", "0.1.0-canary.01")]
        for key, value in mutations:
            metadata = fixture_metadata()
            metadata[key] = value
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                policy.validate_metadata(metadata)
        for field, value in (("name", "../archive"), ("size", True), ("size", 0),
                             ("size", policy.MAX_PAYLOAD_BYTES), ("sha256", "x" * 64)):
            metadata = fixture_metadata()
            metadata["assets"][0][field] = value
            with self.subTest(field=field), self.assertRaises(ValueError):
                policy.validate_metadata(metadata)
        metadata = fixture_metadata()
        metadata["assets"].reverse()
        with self.assertRaises(ValueError):
            policy.validate_metadata(metadata)


class PayloadTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="hlo-candidate-test-")
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        write_assets(self.directory)

    def create(self):
        return policy.create_metadata(self.directory, VERSION, SOURCE, WORKFLOW_ID, 98765, 2, CONTRACT)

    def test_create_verifies_five_files_and_refuses_overwrite(self):
        metadata = self.create()
        self.assertEqual(5, len(list(self.directory.iterdir())))
        self.assertEqual(0o644, stat.S_IMODE((self.directory / "candidate.json").stat().st_mode))
        policy.validate_payload(self.directory, metadata)
        contents = {item.name: item.read_bytes() for item in self.directory.iterdir()}
        policy.validate_payload_bytes(contents, metadata)
        contents["candidate.json"] += b"changed"
        with self.assertRaises(ValueError):
            policy.validate_payload_bytes(contents, metadata)
        with self.assertRaises(ValueError):
            self.create()

    def test_changed_or_unknown_bytes_refuse(self):
        metadata = self.create()
        for name in policy.asset_names(VERSION):
            path = self.directory / name
            original = path.read_bytes()
            path.write_bytes(original + b"changed")
            with self.subTest(name=name), self.assertRaises(ValueError):
                policy.validate_payload(self.directory, metadata)
            path.write_bytes(original)
        (self.directory / "extra").write_bytes(b"x")
        with self.assertRaises(ValueError):
            policy.validate_payload(self.directory, metadata)

    def test_canonical_checksums_and_manifest_binding_are_not_self_asserted(self):
        (self.directory / "SHA256SUMS").write_bytes(b"different-checksum-order\n")
        with self.assertRaises(ValueError):
            self.create()
        (self.directory / "candidate.json").unlink()
        write_assets(self.directory, "d" * 64)
        with self.assertRaises(ValueError):
            self.create()

    @unittest.skipUnless(os.name == "posix", "Linux file-admission boundary")
    def test_links_modes_and_nonregular_entries_refuse(self):
        name = "install-connected-linux.sh"
        path = self.directory / name
        original = path.read_bytes()
        outside = self.directory / "outside"
        path.unlink()
        outside.write_bytes(original)
        path.symlink_to(outside)
        with self.assertRaises(ValueError):
            self.create()
        path.unlink()
        outside.unlink()
        path.write_bytes(original)
        path.chmod(0o755)
        with self.assertRaises(ValueError):
            self.create()
        path.chmod(0o644)
        outside = self.directory.parent / (self.directory.name + "-hardlink")
        self.addCleanup(lambda: outside.unlink(missing_ok=True))
        os.link(path, outside)
        with self.assertRaises(ValueError):
            self.create()

    def test_cli_create_and_verify_actual_wiring(self):
        contract = hashlib.sha256((Path(policy.__file__).parent.parent / "internal/connectedcompat/descriptor.json").read_bytes()).hexdigest()
        write_assets(self.directory, contract)
        script = str(Path(policy.__file__).resolve())
        create = subprocess.run([sys.executable, "-I", script, "create", "--directory", str(self.directory),
            "--version", VERSION, "--source", SOURCE, "--workflow-id", str(WORKFLOW_ID),
            "--run-id", "98765", "--run-attempt", "2"], capture_output=True, timeout=10)
        self.assertEqual(0, create.returncode, create.stderr)
        verify = subprocess.run([sys.executable, "-I", script, "verify", "--directory", str(self.directory)],
                                capture_output=True, timeout=10)
        self.assertEqual((0, b"CONNECTED_CANDIDATE_OK\n"), (verify.returncode, verify.stdout))


def authorization_fixture():
    env = {"GITHUB_REPOSITORY": policy.REPOSITORY, "GITHUB_REPOSITORY_ID": str(policy.REPOSITORY_ID),
        "GITHUB_REPOSITORY_OWNER_ID": str(policy.OWNER_ID), "GITHUB_ACTOR": "braidenm",
        "GITHUB_TRIGGERING_ACTOR": "braidenm", "GITHUB_ACTOR_ID": str(policy.OWNER_ID),
        "GITHUB_EVENT_NAME": "workflow_dispatch", "GITHUB_REF": "refs/heads/main",
        "GITHUB_WORKFLOW_REF": f"{policy.REPOSITORY}/{policy.WORKFLOW_PATH}@refs/heads/main",
        "GITHUB_SHA": SOURCE, "GITHUB_RUN_ID": "98765", "GITHUB_RUN_ATTEMPT": "2"}
    repo = {"id": policy.REPOSITORY_ID, "owner": {"id": policy.OWNER_ID}, "full_name": policy.REPOSITORY,
            "default_branch": "main", "visibility": "public"}
    actor = {"id": policy.OWNER_ID, "login": "braidenm"}
    run = {"id": 98765, "run_attempt": 2, "workflow_id": WORKFLOW_ID, "path": policy.WORKFLOW_PATH,
           "head_sha": SOURCE, "head_branch": "main", "event": "workflow_dispatch", "actor": actor,
           "triggering_actor": actor, "repository": repo, "head_repository": repo}
    prefix = f"/repos/{policy.REPOSITORY}"
    records = {prefix: repo,
        prefix + "/git/ref/heads/main": {"object": {"type": "commit", "sha": SOURCE,
            "url": f"https://api.github.com/repos/{policy.REPOSITORY}/git/commits/{SOURCE}"}},
        prefix + "/actions/workflows/connected-linux-candidate.yml":
            {"id": WORKFLOW_ID, "path": policy.WORKFLOW_PATH, "state": "active"},
        prefix + "/actions/runs/98765/attempts/2": run,
        prefix + f"/git/ref/tags/v{VERSION}": None,
        prefix + f"/releases/tags/v{VERSION}": None}
    return env, records


class AuthorizationTests(unittest.TestCase):
    def test_exact_owner_attempt_current_main_authorizes(self):
        env, records = authorization_fixture()
        calls = []
        def get(endpoint, missing=False):
            calls.append((endpoint, missing))
            return records[endpoint]
        values = policy.authorize(VERSION, env, get)
        self.assertEqual({"version": VERSION, "source": SOURCE, "workflow_id": WORKFLOW_ID,
                          "run_id": 98765, "run_attempt": 2}, values)
        self.assertEqual(6, len(calls))
        self.assertEqual([True, True], [item[1] for item in calls[-2:]])

    def test_untrusted_dispatch_environment_refuses_before_api(self):
        env, _ = authorization_fixture()
        for key, value in (("GITHUB_ACTOR", "other"), ("GITHUB_TRIGGERING_ACTOR", "other"),
                           ("GITHUB_ACTOR_ID", "1"), ("GITHUB_REPOSITORY_ID", "1"),
                           ("GITHUB_REPOSITORY_OWNER_ID", "1"), ("GITHUB_EVENT_NAME", "pull_request"),
                           ("GITHUB_REF", "refs/heads/feature"), ("GITHUB_RUN_ATTEMPT", "01"),
                           ("GITHUB_RUN_ID", "1/evil"), ("GITHUB_SHA", "wrong"),
                           ("GITHUB_WORKFLOW_REF", "other")):
            changed = dict(env, **{key: value})
            with self.subTest(key=key), self.assertRaises(ValueError):
                policy.authorize(VERSION, changed, lambda *_a, **_k: self.fail("unexpected API call"))

    def test_provider_mismatch_stale_attempt_source_and_version_conflicts_refuse(self):
        env, records = authorization_fixture()
        prefix = f"/repos/{policy.REPOSITORY}"
        changes = [(prefix, "id", 1), (prefix, "visibility", "private"),
            (prefix + "/actions/workflows/connected-linux-candidate.yml", "path", "other"),
            (prefix + "/actions/workflows/connected-linux-candidate.yml", "state", "disabled_manually"),
            (prefix + "/actions/runs/98765/attempts/2", "run_attempt", 1),
            (prefix + "/actions/runs/98765/attempts/2", "head_sha", "d" * 40),
            (prefix + "/actions/runs/98765/attempts/2", "workflow_id", WORKFLOW_ID + 1),
            (prefix + "/actions/runs/98765/attempts/2", "triggering_actor", {"id": 1, "login": "braidenm"})]
        for endpoint, key, value in changes:
            changed = copy.deepcopy(records)
            changed[endpoint][key] = value
            with self.subTest(key=key), self.assertRaises(ValueError):
                policy.authorize(VERSION, env, lambda endpoint, **_: changed[endpoint])
        for suffix in (f"/git/ref/tags/v{VERSION}", f"/releases/tags/v{VERSION}"):
            changed = dict(records, **{prefix + suffix: {"id": 1}})
            with self.subTest(suffix=suffix), self.assertRaises(ValueError):
                policy.authorize(VERSION, env, lambda endpoint, **_: changed[endpoint])

    def test_malformed_provider_records_return_fixed_error(self):
        env, records = authorization_fixture()
        for endpoint in list(records)[:4]:
            changed = dict(records, **{endpoint: []})
            with self.subTest(endpoint=endpoint), self.assertRaisesRegex(ValueError, "^connected_candidate_invalid$"):
                policy.authorize(VERSION, env, lambda endpoint, **_: changed[endpoint])

    def test_provider_errors_are_never_absent_and_raw_output_is_not_reported(self):
        for code, status in ((0, b"200"), (1, b"404"), (1, b"500")):
            response = subprocess.CompletedProcess([], code, b"HTTP/2.0 " + status + b" Result\r\ncontent-type: application/json\r\n\r\n{}")
            with mock.patch.object(policy.subprocess, "run", return_value=response) as call:
                if status == b"200":
                    self.assertEqual({}, policy._gh_json("/repos/braidenm/home-lab-observer"))
                elif status == b"404":
                    self.assertIsNone(policy._gh_json("/repos/braidenm/home-lab-observer", missing=True))
                    with self.assertRaisesRegex(ValueError, "^connected_candidate_invalid$"):
                        policy._gh_json("/repos/braidenm/home-lab-observer")
                else:
                    with self.assertRaisesRegex(ValueError, "^connected_candidate_invalid$"):
                        policy._gh_json("/repos/braidenm/home-lab-observer", missing=True)
                self.assertEqual(["gh", "api", "--hostname", "github.com"], call.call_args.args[0][:4])
                self.assertEqual(subprocess.DEVNULL, call.call_args.kwargs["stderr"])


class SBOMTests(unittest.TestCase):
    def fixture(self):
        metadata = fixture_metadata()
        roles = ["observer-connected-" + role for role in ("collector", "uploader", "install")]
        manifest = {"files": [{"name": role, "sha256": "d" * 64} for role in roles]}
        required = {"github.com/shirou/gopsutil/v4": "v4.26.6", "golang.org/x/sys": "v0.47.0", "modernc.org/sqlite": "v1.58.0"}
        sbom = {"spdxVersion": "SPDX-2.3", "files": [
            {"fileName": role, "checksums": [{"algorithm": "SHA256", "checksumValue": "d" * 64}]} for role in roles],
            "packages": [{"name": name, "versionInfo": version} for name, version in required.items()]
                        + [{"name": "stdlib", "versionInfo": "go1.27.3"}] * 3
                        + [{"name": "github.com/braidenm/home-lab-observer", "versionInfo": "UNKNOWN"}] * 3
                        + [{"name": metadata["assets"][0]["name"], "versionInfo": "sha256:" + "c" * 64,
                            "checksums": [{"algorithm": "SHA256", "checksumValue": "c" * 64}]}]}
        return metadata, manifest, required, sbom

    def test_final_archive_roles_and_core_reachable_modules_are_present(self):
        metadata, manifest, required, sbom = self.fixture()
        policy.validate_sbom(json.dumps(sbom).encode(), metadata, json.dumps(manifest).encode(), required)

    def test_empty_outer_only_changed_roles_and_incomplete_modules_refuse(self):
        metadata, manifest, required, sbom = self.fixture()
        mutations = []
        for key, value in (("files", []), ("files", sbom["files"][:2]), ("packages", []),
                           ("packages", sbom["packages"][1:]), ("spdxVersion", "other")):
            mutations.append(dict(sbom, **{key: value}))
        changed = copy.deepcopy(sbom)
        changed["files"][0]["checksums"][0]["checksumValue"] = "e" * 64
        mutations.append(changed)
        changed = copy.deepcopy(sbom)
        changed["packages"][-1]["checksums"][0]["checksumValue"] = "e" * 64
        mutations.append(changed)
        for changed in mutations:
            with self.subTest(changed=changed), self.assertRaises(ValueError):
                policy.validate_sbom(json.dumps(changed).encode(), metadata, json.dumps(manifest).encode(), required)


class WorkflowTests(unittest.TestCase):
    def test_workflow_is_manual_hosted_read_only_and_attests_exact_final_subjects(self):
        root = Path(policy.__file__).parent.parent
        workflow = (root / policy.WORKFLOW_PATH).read_text()
        self.assertIn("workflow_dispatch:", workflow)
        for forbidden in ("pull_request:", "push:", "self-hosted", "contents: write", "secrets.",
                          "gh release", "gh api --method POST", "sudo", "run_vm.py", "upload-release-assets: true"):
            self.assertNotIn(forbidden, workflow)
        self.assertEqual(3, workflow.count("runs-on: ubuntu-24.04"))
        self.assertEqual(1, workflow.count("id-token: write"))
        self.assertEqual(1, workflow.count("attestations: write"))
        self.assertEqual(2, workflow.count("uses: actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6"))
        self.assertIn("artifact-ids: ${{ needs.build.outputs.artifact_id }}", workflow)
        self.assertIn("${{ needs.authorize.outputs.run_id }}-${{ needs.authorize.outputs.run_attempt }}", workflow)
        self.assertIn("path: dist/connected/payload/", workflow)
        self.assertIn("sbom-path: dist/connected.spdx.json", workflow)
        self.assertIn("path: dist/payload/home-lab-observer-connected_${{ needs.authorize.outputs.version }}_linux_amd64.tar.gz", workflow)
        self.assertIn("verify-sbom --directory dist/payload --sbom dist/connected.spdx.json", workflow)
        self.assertIn("dist/payload/*\n            dist/connected.spdx.json", workflow)
        self.assertIn("go-version: '1.27.3'", workflow)
        self.assertIn("govulncheck@v1.9.0", workflow)
        self.assertEqual(3, workflow.count("persist-credentials: false"))
        script = (root / "scripts/build-connected-linux.sh").read_text()
        self.assertIn("for round in first second", script)
        self.assertIn('cmp -- "$output/first/packed/$name" "$output/second/packed/$name"', script)
        self.assertIn("GOTOOLCHAIN=local", script)
        self.assertIn("GOEXPERIMENT=", script)
        self.assertIn("-trimpath", script)
        self.assertIn("-buildid=", script)


if __name__ == "__main__":
    unittest.main()
