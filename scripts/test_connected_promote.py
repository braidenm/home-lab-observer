"""Closed synthetic publication orchestration; no real credentials or provider writes."""
import copy
import datetime as dt
import io
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import zipfile

sys.dont_write_bytecode = True
import connected_candidate as candidate
import connected_promote as cli
import connected_promotion_policy as policy
from test_connected_candidate import fixture_payload


def encoded(value):
    return (json.dumps(value, separators=(",", ":")) + "\n").encode()


def fixture():
    c, payload = fixture_payload()
    c.update(workflow_id=555, run_id=1234)
    payload["candidate.json"] = candidate.encode_metadata(c)
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w") as archive:
        for name, value in payload.items():
            archive.writestr(name, value)
    packed = buffer.getvalue()
    record = {"schema_version": "observer-connected-acceptance/v1", "candidate": c,
              "candidate_sha256": policy.digest(payload["candidate.json"]),
              "artifact": {"id": 5678, "name": policy.artifact_name(c), "size": len(packed),
                           "sha256": policy.digest(packed), "expires_at": "2099-11-01T00:00:00Z"},
              "vm": {key: "d" * 64 for key in ("image_sha256", "receiver_sha256",
                                               "probe_sha256", "harness_sha256")}}
    record["vm"]["results"] = dict.fromkeys(policy.RESULTS, "PASS")
    return record, payload, packed


class FakeProvider:
    def __init__(self, directory):
        self.root = Path(directory)
        self.record, self.payload, self.packed = fixture()
        self.head = "b" * 40
        self.calls, self.mutations = [], []
        self.tag, self.release = None, None
        self.immutable = True
        self.rerun = False
        self.hook = lambda *_: None
        self.anon_changed = False
        self.race = False
        self.dirty = False
        self.owner = {"id": candidate.OWNER_ID, "login": "braidenm"}

    def git_bytes(self, *args, **_):
        if args[0] == "status":
            return b"?? untracked\n" if self.dirty else b""
        if args[:2] == ("rev-parse", "--verify"):
            return self.head.encode() + b"\n"
        if args == ("rev-parse", "--path-format=absolute", "--git-path", "info/grafts"):
            return str(self.root / "info" / "grafts").encode() + b"\n"
        if args[0] == "rev-parse":
            return str(self.root).encode() + b"\n"
        if args[0] == "show":
            return encoded(self.record)
        if args[0] == "merge-base":
            return b""
        if args[0] == "diff":
            return (b":000000 100644 " + b"0" * 40 + b" " + b"e" * 40 + b" A\0" +
                    policy.evidence_path(self.record).encode() + b"\0")
        raise AssertionError(args)

    def run_record(self):
        c = self.record["candidate"]
        repo = {"id": candidate.REPOSITORY_ID, "full_name": cli.REPO,
                "owner": {"id": candidate.OWNER_ID}}
        actor = {"id": candidate.OWNER_ID, "login": "braidenm"}
        return {"id": c["run_id"], "run_attempt": 3 if self.rerun else c["run_attempt"],
                "workflow_id": 555, "path": candidate.WORKFLOW_PATH, "head_sha": c["source"],
                "head_branch": "main", "event": "workflow_dispatch", "status": "completed",
                "conclusion": "success", "repository": repo, "head_repository": repo,
                "actor": actor, "triggering_actor": actor}

    def artifact(self):
        a, c = self.record["artifact"], self.record["candidate"]
        return {"id": a["id"], "name": a["name"], "size_in_bytes": a["size"],
                "expired": False, "digest": "sha256:" + a["sha256"], "expires_at": a["expires_at"],
                "workflow_run": {"id": c["run_id"], "repository_id": candidate.REPOSITORY_ID,
                                 "head_repository_id": candidate.REPOSITORY_ID,
                                 "head_sha": c["source"], "head_branch": "main"}}

    def json(self, endpoint, method="GET", data=None, **_):
        self.calls.append((method, endpoint))
        self.hook(method, endpoint)
        if method != "GET":
            self.mutations.append((method, endpoint, copy.deepcopy(data)))
            if endpoint.endswith("/git/refs"):
                self.tag = {"object": {"type": "commit", "sha": data["sha"]}}
            elif method == "POST" and endpoint.endswith("/releases"):
                self.release = dict(data, id=900, immutable=False, assets=[])
            elif method == "PATCH":
                self.release.update(data, immutable=self.immutable)
            return cli.CONFLICT if self.race else {}
        if endpoint == "/user":
            return self.owner
        if endpoint == cli.PREFIX:
            return {"id": candidate.REPOSITORY_ID, "owner": {"id": candidate.OWNER_ID},
                    "full_name": cli.REPO, "default_branch": "main", "visibility": "public"}
        if endpoint.endswith("/git/ref/heads/main"):
            return {"object": {"type": "commit", "sha": self.head}}
        if endpoint.endswith("/immutable-releases"):
            return {"enabled": self.immutable}
        if "/actions/workflows/" in endpoint:
            return {"id": 555, "path": candidate.WORKFLOW_PATH, "state": "active"}
        if "/actions/runs/" in endpoint:
            return self.run_record()
        if "/actions/artifacts/" in endpoint:
            return self.artifact()
        if "/git/ref/tags/" in endpoint:
            return copy.deepcopy(self.tag)
        if endpoint.endswith("/releases?per_page=100&page=1"):
            return [] if self.release is None else [copy.deepcopy(self.release)]
        if endpoint.endswith("/releases/900"):
            return copy.deepcopy(self.release)
        raise AssertionError(endpoint)

    def binary(self, endpoint, limit):
        if "/actions/artifacts/" in endpoint:
            return self.packed
        ident = int(endpoint.rsplit("/", 1)[1])
        asset = next(item for item in self.release["assets"] if item["id"] == ident)
        return self.payload[asset["name"]]

    def attest(self, path, source, predicate_type):
        c = self.record["candidate"]
        cert = {"issuer": "https://token.actions.githubusercontent.com",
                "subjectAlternativeName": policy.WORKFLOW_URI, "buildSignerURI": policy.WORKFLOW_URI,
                "buildSignerDigest": source, "buildConfigURI": policy.WORKFLOW_URI,
                "buildConfigDigest": source, "buildTrigger": "workflow_dispatch",
                "runnerEnvironment": "github-hosted", "sourceRepositoryURI": policy.REPO_URI,
                "sourceRepositoryDigest": source, "sourceRepositoryIdentifier": str(candidate.REPOSITORY_ID),
                "sourceRepositoryOwnerIdentifier": str(candidate.OWNER_ID),
                "sourceRepositoryRef": "refs/heads/main",
                "runInvocationURI": policy.REPO_URI + f"/actions/runs/{c['run_id']}/attempts/{c['run_attempt']}"}
        return [{"verificationResult": {"signature": {"certificate": cert},
            "verifiedTimestamps": [{"type": "Tlog"}], "statement": {
                "predicateType": predicate_type, "subject": [
                    {"name": path.name, "digest": {"sha256": policy.digest(path.read_bytes())}}]}}}]

    def upload(self, version, path):
        self.mutations.append(("UPLOAD", path.name, None))
        item = next(asset for asset in self.record["candidate"]["assets"] if asset["name"] == path.name)
        self.release["assets"].append(dict(item, id=100 + len(self.release["assets"]), state="uploaded",
                                           digest="sha256:" + item["sha256"]))
        return not self.race

    def anonymous(self, version, name, expected):
        return b"changed" if self.anon_changed else self.payload[name]


class OrchestrationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="hlo-promotion-test-")
        self.addCleanup(self.temporary.cleanup)
        self.provider = FakeProvider(self.temporary.name)
        self.now = dt.datetime(2026, 10, 26, tzinfo=dt.timezone.utc)

    def publish(self):
        return cli.publish(self.provider, "0.1.0-canary.1", 555, self.now)

    def test_exact_publication_and_idempotent_retry_never_rebuild_or_overwrite(self):
        self.assertEqual(900, self.publish())
        self.assertEqual(7, len(self.provider.mutations))
        self.assertEqual(["POST", "POST", "UPLOAD", "UPLOAD", "UPLOAD", "UPLOAD", "PATCH"],
                         [item[0] for item in self.provider.mutations])
        self.assertFalse(self.provider.release["draft"])
        self.assertTrue(self.provider.release["immutable"])
        self.assertEqual(900, self.publish())
        self.assertEqual(7, len(self.provider.mutations))

    def test_partial_draft_and_same_candidate_creation_races_recover(self):
        self.provider.race = True
        self.assertEqual(900, self.publish())
        uploads = [item for item in self.provider.mutations if item[0] == "UPLOAD"]
        self.assertEqual(4, len(uploads))

    def test_unprotected_dirty_and_stale_attempt_refuse_before_any_mutation(self):
        for field in ("dirty", "rerun"):
            with self.subTest(field=field):
                setattr(self.provider, field, True)
                with self.assertRaises(ValueError):
                    self.publish()
                setattr(self.provider, field, False)
        self.provider.immutable = False
        with self.assertRaises(ValueError):
            self.publish()
        self.assertEqual([], self.provider.mutations)

    def test_wrong_stored_account_is_not_owner_publication_authority(self):
        self.provider.owner = {"id": 1, "login": "other"}
        with self.assertRaises(ValueError):
            self.publish()
        self.assertEqual([], self.provider.mutations)

    def test_main_advance_between_download_and_mutation_refuses(self):
        original = self.provider.binary
        def binary(*args):
            value = original(*args)
            self.provider.head = "e" * 40
            return value
        self.provider.binary = binary
        with self.assertRaises(ValueError):
            self.publish()
        self.assertEqual([], self.provider.mutations)

    def test_changed_asset_after_download_refuses_publication_and_retains_draft(self):
        original = self.provider.binary
        def binary(endpoint, limit):
            value = original(endpoint, limit)
            if "/releases/assets/" in endpoint:
                self.provider.release["assets"][0]["id"] += 1000
            return value
        self.provider.binary = binary
        with self.assertRaises(ValueError):
            self.publish()
        self.assertTrue(self.provider.release["draft"])
        self.assertFalse(any(item[0] == "PATCH" for item in self.provider.mutations))

    def test_draft_download_telemetry_does_not_change_closed_authority(self):
        original = self.provider.binary
        def binary(endpoint, limit):
            value = original(endpoint, limit)
            if "/releases/assets/" in endpoint:
                for asset in self.provider.release["assets"]:
                    asset["download_count"] = asset.get("download_count", 0) + 1
                    asset["updated_at"] = "synthetic-provider-telemetry"
                self.provider.release["updated_at"] = "synthetic-provider-telemetry"
            return value
        self.provider.binary = binary
        self.assertEqual(900, self.publish())
        self.assertFalse(self.provider.release["draft"])
        self.assertTrue(self.provider.release["immutable"])

    def test_published_anonymous_byte_failure_is_not_success_or_deletion(self):
        self.provider.anon_changed = True
        with self.assertRaises(ValueError):
            self.publish()
        self.assertFalse(self.provider.release["draft"])
        self.assertTrue(self.provider.release["immutable"])
        self.assertFalse(any(item[0] == "DELETE" for item in self.provider.mutations))

    def test_prepare_exports_only_verified_closed_nonexecutable_payload(self):
        self.provider.head = self.provider.record["candidate"]["source"]
        output = Path(self.temporary.name) / "prepared"
        record = cli.prepare(self.provider, 1234, 5678, output, 555, self.now)
        self.assertEqual(self.provider.record["candidate"], record["candidate"])
        self.assertEqual(sorted(self.provider.payload), sorted(item.name for item in (output / "payload").iterdir()))
        self.assertEqual([], self.provider.mutations)
        with self.assertRaises(FileExistsError):
            cli.prepare(self.provider, 1234, 5678, output, 555, self.now)

    def test_lock_refuses_existing_and_releases_only_own_file(self):
        version = "0.1.0-canary.1"
        path = self.provider.root / ("hlo-connected-" + version + ".lock")
        with cli.version_lock(self.provider, version):
            self.assertTrue(path.exists())
            with self.assertRaises(FileExistsError):
                with cli.version_lock(self.provider, version):
                    self.fail("competing publisher")
        self.assertFalse(path.exists())

    def test_crypto_failure_and_candidate_bytes_never_authorize_mutation(self):
        original = self.provider.attest
        self.provider.attest = lambda *_: []
        with self.assertRaises(ValueError):
            self.publish()
        self.assertEqual([], self.provider.mutations)
        self.provider.attest = original
        self.provider.packed += b"changed"
        with self.assertRaises(ValueError):
            self.publish()
        self.assertEqual([], self.provider.mutations)

    def test_run_rerun_after_verification_and_before_publish_stops_at_draft(self):
        original = self.provider.upload
        def upload(*args):
            result = original(*args)
            self.provider.rerun = True
            return result
        self.provider.upload = upload
        with self.assertRaises(ValueError):
            self.publish()
        self.assertTrue(self.provider.release["draft"])
        self.assertFalse(any(item[0] == "PATCH" for item in self.provider.mutations))


class ProviderTests(unittest.TestCase):
    def provider(self, run):
        with patch.object(cli.shutil, "which", side_effect=lambda name, **_: str(Path(tempfile.gettempdir()) / (name + ".exe"))):
            return cli.Provider(root=tempfile.gettempdir(), run=run, source_environment={
                "PATH": "synthetic", "GH_TOKEN": "not-used", "GITHUB_TOKEN": "not-used",
                "GH_DEBUG": "api", "GIT_CONFIG_COUNT": "1", "PAGER": "evil", "GH_CONFIG_DIR": "evil"})

    def test_fixed_shellfree_api_and_upload_arguments_and_credential_environment(self):
        calls = []
        def run(argv, cwd, env, **bounds):
            calls.append((argv, env, bounds))
            return 0, b'HTTP/2.0 200 OK\r\n\r\n{"enabled":true}'
        provider = self.provider(run)
        self.assertEqual({"enabled": True}, provider.json(cli.PREFIX + "/immutable-releases"))
        provider.upload("0.1.0-canary.1", Path(tempfile.gettempdir()) / "SHA256SUMS")
        argv, env, bounds = calls[0]
        self.assertEqual(["api", "--hostname", "github.com", "--method", "GET"], argv[1:6])
        self.assertEqual(cli.PREFIX + "/immutable-releases", argv[-1])
        for key in ("GH_TOKEN", "GITHUB_TOKEN", "GH_DEBUG", "GH_CONFIG_DIR", "GIT_CONFIG_COUNT", "PAGER"):
            self.assertNotIn(key, env)
        self.assertEqual("1", env["GH_PROMPT_DISABLED"])
        self.assertEqual(120, bounds["timeout"])
        self.assertNotIn("--clobber", calls[1][0])
        self.assertEqual(["--repo", cli.REPO], calls[1][0][-2:])

    def test_attestation_actual_argv_uses_certificate_and_source_restrictions(self):
        calls = []
        def run(argv, *_args, **_kwargs):
            calls.append(argv)
            return 0, b"[]"
        provider = self.provider(run)
        provider.attest(Path(tempfile.gettempdir()) / "candidate.json", "a" * 40,
                        "https://slsa.dev/provenance/v1")
        argv = calls[0]
        for flag, expected in (("--hostname", "github.com"), ("--cert-identity", policy.WORKFLOW_URI),
                               ("--source-ref", "refs/heads/main"), ("--source-digest", "a" * 40)):
            self.assertEqual(expected, argv[argv.index(flag) + 1])
        self.assertIn("--deny-self-hosted-runners", argv)
        self.assertFalse(any("auth" == item for item in argv))

    def test_response_parsing_only_closed_missing_and_conflict_states(self):
        self.assertIsNone(cli.decode_response(b"HTTP/2.0 404 Not Found\n\n{}", missing=True))
        self.assertIs(cli.CONFLICT, cli.decode_response(b"HTTP/2.0 422 Conflict\n\n{}", conflict=True))
        for value in (b"private provider error", b"HTTP/2.0 500 Error\n\n{}",
                      b'HTTP/2.0 200 OK\n\n{"x":1,"x":2}'):
            with self.assertRaises(ValueError):
                cli.decode_response(value)
        self.assertEqual({"large": "x" * 40000}, cli.bounded_json(encoded({"large": "x" * 40000})))
        with self.assertRaises(ValueError):
            cli.bounded_json(b"x" * (cli.JSON_LIMIT + 1))

    def test_anonymous_redirect_hosts_https_and_count_are_closed(self):
        cli.validate_download_url("https://release-assets.githubusercontent.com/a?signature=synthetic")
        for url in ("http://github.com/a", "https://evil.example/a", "https://u:p@github.com/a",
                    "https://github.com:444/a", "https://github.com/a#fragment"):
            with self.assertRaises(ValueError):
                cli.validate_download_url(url)

    def test_bounded_process_timeout_and_output_are_real_not_mocked(self):
        code, value = cli.bounded_process([sys.executable, "-I", "-c", "print('ok')"],
                                          tempfile.gettempdir(), cli.environment(os.environ), limit=16, timeout=5)
        self.assertEqual((0, ("ok" + os.linesep).encode()), (code, value))
        with self.assertRaises(ValueError):
            cli.bounded_process([sys.executable, "-I", "-c", "print('x'*100000)"],
                                tempfile.gettempdir(), cli.environment(os.environ), limit=16, timeout=5)
        with self.assertRaises(subprocess.TimeoutExpired):
            cli.bounded_process([sys.executable, "-I", "-c", "import time; time.sleep(10)"],
                                tempfile.gettempdir(), cli.environment(os.environ), limit=16, timeout=.1)

    def test_unregistered_workflow_cli_fails_before_provider_or_credentials(self):
        with patch.object(cli, "WORKFLOW_ID", None), patch.object(cli, "Provider") as provider:
            self.assertEqual(1, cli.main(["--prepare", "--run-id", "1", "--artifact-id", "2", "--output", "unused"]))
            provider.assert_not_called()

    def test_malformed_zip_metadata_is_sanitized_at_cli_boundary(self):
        _, _, packed = fixture()
        for encrypted, compression in ((True, None), (False, 99)):
            data = bytearray(packed)
            position = data.find(b"PK\x01\x02")
            # Modify all matching local/central headers, including candidate.json.
            while position >= 0:
                name_length = struct.unpack_from("<H", data, position + 28)[0]
                name = bytes(data[position + 46:position + 46 + name_length])
                if name == b"candidate.json":
                    local = struct.unpack_from("<I", data, position + 42)[0]
                    if encrypted:
                        struct.pack_into("<H", data, position + 8, 1)
                        struct.pack_into("<H", data, local + 6, 1)
                    else:
                        struct.pack_into("<H", data, position + 10, compression)
                        struct.pack_into("<H", data, local + 8, compression)
                    break
                position = data.find(b"PK\x01\x02", position + 4)
            self.assertGreaterEqual(position, 0)
            with patch.object(cli, "WORKFLOW_ID", 555), patch.object(cli, "Provider"), \
                    patch.object(cli, "prepare", side_effect=lambda *_: cli.read_metadata(bytes(data))), \
                    patch.object(sys, "stderr", new_callable=io.StringIO) as error:
                result = cli.main(["--prepare", "--run-id", "1", "--artifact-id", "2", "--output", "unused"])
                self.assertEqual((1, "CONNECTED_PROMOTION_FAILED\n"), (result, error.getvalue()))

    def test_actual_git_replacement_cannot_hide_runtime_diff_and_grafts_refuse(self):
        git = shutil.which("git")
        self.assertIsNotNone(git)
        with tempfile.TemporaryDirectory(prefix="hlo-git-policy-test-") as directory:
            root = Path(directory)
            env = cli.environment(os.environ)
            def command(*args):
                value = subprocess.run([git, "-C", directory, *args], env=env, capture_output=True,
                                       timeout=10, check=True)
                return value.stdout.decode().strip()
            command("init", "--initial-branch=main")
            command("config", "user.email", "synthetic@example.invalid")
            command("config", "user.name", "Synthetic fixture")
            (root / "runtime.txt").write_text("original")
            command("add", "runtime.txt")
            command("commit", "-m", "base")
            source = command("rev-parse", "HEAD")
            (root / "runtime.txt").write_text("unaccepted change")
            command("add", "runtime.txt")
            command("commit", "-m", "runtime change")
            tree = command("rev-parse", "HEAD^{tree}")
            forged = command("commit-tree", tree, "-m", "forged root")
            record, _, _ = fixture()
            record["candidate"]["source"] = source
            evidence = root / policy.evidence_path(record)
            evidence.parent.mkdir(parents=True)
            evidence.write_bytes(encoded(record))
            command("add", str(evidence.relative_to(root)))
            command("commit", "-m", "evidence")
            command("replace", source, forged)
            vulnerable = subprocess.run([git, "-C", directory, "diff", "--raw", "--no-abbrev",
                "--no-renames", "-z", source, "HEAD"], capture_output=True, check=True, timeout=10).stdout
            policy.validate_evidence_diff(vulnerable, record)
            original_which = cli.shutil.which
            with patch.object(cli.shutil, "which", side_effect=lambda name, **kw:
                              str(root / "gh.exe") if name == "gh" else original_which(name, **kw)):
                provider = cli.Provider(root=root)
            safe = provider.git_bytes("diff", "--raw", "--no-abbrev", "--no-renames", "-z", source, "HEAD")
            with self.assertRaises(ValueError):
                policy.validate_evidence_diff(safe, record)
            grafts = Path(provider.git_bytes("rev-parse", "--path-format=absolute", "--git-path",
                                            "info/grafts").decode().strip())
            grafts.parent.mkdir(exist_ok=True)
            grafts.write_bytes(b"")
            provider.json = lambda *_args, **_kw: self.fail("grafts must refuse before provider access")
            with self.assertRaises(ValueError):
                cli.trusted_main(provider)


if __name__ == "__main__":
    unittest.main()
