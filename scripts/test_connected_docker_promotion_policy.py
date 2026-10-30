"""Pure synthetic publication contracts; no provider, credential or execution."""
import ast
import copy
import datetime as dt
import io
import json
from pathlib import Path
import stat
import unittest
import zipfile

import _connected_candidate_core as candidate_core
import connected_docker_candidate as candidate
import connected_docker_promotion_policy as p
import connected_promotion_policy as numeric
from test_connected_docker_candidate import payload as candidate_payload


def encoded(value):
    return (json.dumps(value, separators=(",", ":"), ensure_ascii=True) + "\n").encode("ascii")


def packed(payload, extra=None):
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w") as archive:
        for name, value in payload.items():
            archive.writestr(name, value)
        if extra is not None:
            name, mode, value = extra
            entry = zipfile.ZipInfo(name)
            entry.external_attr = mode << 16
            archive.writestr(entry, value)
    return stream.getvalue()


def fixture():
    """Shared operator fixture: receipt, payload, two ZIPs, trusted nine blobs."""
    metadata, payload, trusted = candidate_payload()
    metadata["workflow_id"] = p.WORKFLOW_ID
    payload["candidate.json"] = candidate.encode_metadata(metadata)
    primary = packed(payload)
    artifacts = {}
    for key, identifier, suffix in (("primary", 5678, ""), ("independent", 5679, "-independent")):
        artifacts[key] = {"id": identifier, "name": p.artifact_name(metadata) + suffix,
                          "sha256": p.digest(primary), "size": len(primary), "expires_at": "2099-11-01T00:00:00Z"}
    baseline = {"build_record_sha256": "c" * 64}
    for profile in ("numeric", "processes"):
        baseline[profile] = dict.fromkeys(("archive_sha256", "manifest_sha256", "installer_sha256", "checksums_sha256"), "d" * 64)
    inputs = dict(p.DOCKER_PACKAGE_SHA256, fixture_sha256="e" * 64, rootfs_sha256="f" * 64)
    vm = {"scope": "rich-docker", "source": metadata["source"],
          **dict.fromkeys(("image_sha256", "receiver_sha256", "probe_sha256", "harness_sha256"), "c" * 64),
          "baseline": baseline, "docker_inputs": inputs,
          "cases": {name: dict.fromkeys(("first_boot", "reboot", "case"), "PASS")
                    for name in ("rich-docker", "rich-docker-refusal")},
          "all_pass": "PASS", "cleanup": dict.fromkeys(("qemu", "mounts", "loops"), "PASS")}
    record = {"schema_version": "observer-connected-docker-acceptance/v1", "candidate": metadata,
              "candidate_sha256": p.digest(payload["candidate.json"]), "artifacts": artifacts, "vm": vm}
    return record, payload, primary, primary, trusted


def artifact_info(record, independent=False):
    a = record["artifacts"]["independent" if independent else "primary"]
    c = record["candidate"]
    return {"id": a["id"], "name": a["name"], "size_in_bytes": a["size"], "expired": False,
            "digest": "sha256:" + a["sha256"], "expires_at": a["expires_at"],
            "workflow_run": {"id": c["run_id"], "repository_id": p.REPOSITORY_ID,
                             "head_repository_id": p.REPOSITORY_ID, "head_sha": c["source"], "head_branch": "main"}}


def run_info(record):
    c = record["candidate"]
    repo = {"id": p.REPOSITORY_ID, "full_name": p.REPOSITORY, "owner": {"id": p.OWNER_ID}}
    actor = {"id": p.OWNER_ID, "login": "braidenm"}
    return {"id": c["run_id"], "run_attempt": c["run_attempt"], "workflow_id": p.WORKFLOW_ID,
            "path": p.WORKFLOW_PATH, "head_sha": c["source"], "head_branch": "main", "event": "workflow_dispatch",
            "status": "completed", "conclusion": "success", "repository": repo, "head_repository": repo,
            "actor": actor, "triggering_actor": actor}


def jobs_info(record):
    c = record["candidate"]
    return {"total_count": 5, "jobs": [
        {"id": 100 + index, "name": name, "run_id": c["run_id"], "head_sha": c["source"],
         "status": "completed", "conclusion": "success"}
        for index, name in enumerate(("authorize", "build", "independent", "compare", "attest"))]}


def provenance(c, name, digest, predicate_type="https://slsa.dev/provenance/v1", predicate=None):
    cert = {"issuer": "https://token.actions.githubusercontent.com", "subjectAlternativeName": p.WORKFLOW_URI,
            "buildSignerURI": p.WORKFLOW_URI, "buildSignerDigest": c["source"],
            "buildConfigURI": p.WORKFLOW_URI, "buildConfigDigest": c["source"], "buildTrigger": "workflow_dispatch",
            "runnerEnvironment": "github-hosted", "sourceRepositoryURI": p.REPO_URI,
            "sourceRepositoryDigest": c["source"], "sourceRepositoryIdentifier": str(p.REPOSITORY_ID),
            "sourceRepositoryOwnerIdentifier": str(p.OWNER_ID), "sourceRepositoryRef": "refs/heads/main",
            "runInvocationURI": p.REPO_URI + f"/actions/runs/{c['run_id']}/attempts/{c['run_attempt']}"}
    return [{"verificationResult": {"signature": {"certificate": cert}, "verifiedTimestamps": [{"type": "Tlog"}],
             "statement": {"predicateType": predicate_type,
                           "subject": [{"name": name, "digest": {"sha256": digest}}], "predicate": predicate}}}]


def sbom_fixture(record, payload):
    c = record["candidate"]
    manifest = json.loads(payload["connected-manifest.json"])
    archive = c["assets"][0]
    checksums = lambda value: [{"algorithm": "SHA256", "checksumValue": value}]
    roles = {"observer-connected-" + role for role in candidate_core._DOCKER.roles}
    required = {"github.com/shirou/gopsutil/v4": "v4.26.9", "golang.org/x/sys": "v0.48.0", "modernc.org/sqlite": "v1.59.0"}
    predicate = {"spdxVersion": "SPDX-2.3", "files": [
        {"fileName": item["name"], "checksums": checksums(item["sha256"])}
        for item in manifest["files"] if item["name"] in roles],
        "packages": [{"name": archive["name"], "versionInfo": "sha256:" + archive["sha256"], "checksums": checksums(archive["sha256"])}] +
                    [{"name": "stdlib", "versionInfo": "go1.27.3"}] * 4 +
                    [{"name": p.REPOSITORY.replace("braidenm/", "github.com/braidenm/"), "versionInfo": c["version"]}] * 4 +
                    [{"name": name, "versionInfo": version} for name, version in required.items()]}
    return predicate, required


class DockerPromotionPolicyTests(unittest.TestCase):
    def setUp(self):
        self.record, self.payload, self.primary, self.independent, self.trusted = fixture()
        self.c = self.record["candidate"]
        self.now = dt.datetime(2026, 10, 30, tzinfo=dt.timezone.utc)

    def acceptance(self, record=None, workflow_id=None):
        return p.validate_acceptance(encoded(record or self.record), workflow_id or p.WORKFLOW_ID, self.now)

    def test_closed_complete_receipt_and_cross_profile_refusal(self):
        self.assertEqual(self.acceptance(), self.record)
        self.assertEqual(p.WORKFLOW_ID, 401377430)
        self.assertEqual(p.MAX_ARTIFACT_BYTES, 65 * 1024 * 1024)
        self.assertEqual(numeric.MAX_ARTIFACT_BYTES, 64 * 1024 * 1024)
        with self.assertRaises(ValueError): numeric.validate_acceptance(encoded(self.record), p.WORKFLOW_ID, self.now)
        with self.assertRaises(ValueError): self.acceptance(workflow_id=397507617)
        for raw in (b'{"x":1,"x":2}', b'{"x":NaN}', b'[' * 10000, b' ' * 32769):
            with self.assertRaises(ValueError): p.decode_json(raw)

    def test_every_nested_receipt_boundary_is_closed_and_pass_only(self):
        mutations = [lambda r: r.update(extra=1), lambda r: r.update(schema_version="observer-connected-acceptance/v1"),
            lambda r: r["candidate"].update(workflow_id=397507617), lambda r: r["candidate"].update(contract_sha256="0" * 64),
            lambda r: r["vm"].update(source="b" * 40), lambda r: r["vm"].update(scope="rich-processes"),
            lambda r: r["vm"].update(all_pass="NOT_RUN"), lambda r: r["vm"]["baseline"].pop("processes"),
            lambda r: r["vm"]["baseline"]["numeric"].update(extra="not allowed"),
            lambda r: r["vm"]["docker_inputs"].update(docker_ce_sha256="0" * 64),
            lambda r: r["vm"]["docker_inputs"].update(fixture_sha256="not-sha"),
            lambda r: r["vm"]["cases"].pop("rich-docker-refusal"),
            lambda r: r["vm"]["cases"]["rich-docker"].update(reboot="FAIL"),
            lambda r: r["vm"]["cleanup"].update(loops="UNKNOWN")]
        for mutate in mutations:
            changed = copy.deepcopy(self.record); mutate(changed)
            with self.subTest(mutate=mutate), self.assertRaises(ValueError): self.acceptance(changed)

    def test_two_provider_artifacts_may_have_equal_zip_hashes_but_never_ids_or_names(self):
        self.assertEqual(self.record["artifacts"]["primary"]["sha256"], self.record["artifacts"]["independent"]["sha256"])
        self.acceptance()
        for field, value in (("id", True), ("id", 5678), ("name", self.record["artifacts"]["primary"]["name"]),
                             ("size", True), ("size", p.MAX_ARTIFACT_BYTES + 1), ("expires_at", "2026-10-01T00:00:00Z")):
            changed = copy.deepcopy(self.record); changed["artifacts"]["independent"][field] = value
            with self.subTest(field=field, value=value), self.assertRaises(ValueError): self.acceptance(changed)

    def test_fixed_deb_hashes_match_source_literal_without_importing_the_fixture(self):
        source = Path(__file__).parent / "fixtures/connected-first-install/docker_scope.py"
        tree = ast.parse(source.read_text(encoding="utf8"))
        packages = next(ast.literal_eval(n.value) for n in tree.body if isinstance(n, ast.Assign) and
                        any(isinstance(name, ast.Name) and name.id == "PACKAGES" for name in n.targets))
        mapping = {"docker_ce_sha256": "docker-ce_", "docker_ce_cli_sha256": "docker-ce-cli_", "containerd_sha256": "containerd.io_"}
        expected = {field: next(value[1] for name, value in packages.items() if name.startswith(prefix)) for field, prefix in mapping.items()}
        self.assertEqual(dict(p.DOCKER_PACKAGE_SHA256), expected)

    def test_current_attempt_requires_the_complete_five_successful_jobs(self):
        response = jobs_info(self.record)
        self.assertIsNone(p.validate_attempt_jobs(response, self.record))
        self.assertTrue(all("run_attempt" not in job for job in response["jobs"]))
        mutations = [lambda r: r.update(total_count=True), lambda r: r["jobs"].pop(),
            lambda r: r["jobs"].append(copy.deepcopy(r["jobs"][0])), lambda r: r["jobs"][3].update(name="build"),
            lambda r: r["jobs"][3].update(name="foreign"), lambda r: r["jobs"][3].update(head_sha="b" * 40),
            lambda r: r["jobs"][3].update(run_id=self.c["run_id"] + 1), lambda r: r["jobs"][3].update(id=True),
            lambda r: r["jobs"][3].update(id=r["jobs"][0]["id"])]
        mutations += [lambda r, status=value: r["jobs"][3].update(conclusion=status)
                      for value in ("failure", "cancelled", "skipped", None)]
        for mutate in mutations:
            changed = copy.deepcopy(response); mutate(changed)
            with self.subTest(mutate=mutate), self.assertRaises(ValueError): p.validate_attempt_jobs(changed, self.record)

    def test_prepare_checks_run_and_attempt_jobs_before_candidate_metadata_exists(self):
        run = run_info(self.record)
        workflow = {"id": p.WORKFLOW_ID, "path": p.WORKFLOW_PATH, "state": "active"}
        self.assertEqual(p.validate_prepare_run(run, workflow, self.c["run_id"], self.c["source"]), self.c["run_attempt"])
        self.assertIsNone(p.validate_prepare_jobs(jobs_info(self.record), self.c["run_id"], self.c["source"]))
        for key, value in (("run_attempt", True), ("run_attempt", 0), ("head_sha", "b" * 40),
                           ("path", numeric.WORKFLOW_PATH), ("status", "in_progress")):
            changed = copy.deepcopy(run); changed[key] = value
            with self.subTest(key=key), self.assertRaises(ValueError):
                p.validate_prepare_run(changed, workflow, self.c["run_id"], self.c["source"])
        for run_id, source in ((True, self.c["source"]), (self.c["run_id"], "main")):
            with self.assertRaises(ValueError): p.validate_prepare_run(run, workflow, run_id, source)
            with self.assertRaises(ValueError): p.validate_prepare_jobs(jobs_info(self.record), run_id, source)
        bad_jobs = jobs_info(self.record); bad_jobs["jobs"][0]["conclusion"] = "failure"
        with self.assertRaises(ValueError): p.validate_prepare_jobs(bad_jobs, self.c["run_id"], self.c["source"])

    def test_run_and_both_artifacts_bind_repo_actor_source_expiry_and_current_attempt(self):
        run = run_info(self.record)
        workflow = {"id": p.WORKFLOW_ID, "path": p.WORKFLOW_PATH, "state": "active"}
        p.validate_run(run, workflow, self.record)
        for field, value in (("run_attempt", self.c["run_attempt"] + 1), ("workflow_id", 397507617),
                             ("head_sha", "b" * 40), ("event", "pull_request"), ("conclusion", "failure")):
            changed = copy.deepcopy(run); changed[field] = value
            with self.subTest(field=field), self.assertRaises(ValueError): p.validate_run(changed, workflow, self.record)
        for independent in (False, True):
            validate = p.validate_independent_artifact if independent else p.validate_artifact
            info = artifact_info(self.record, independent); validate(info, self.record)
            for field, value in (("id", True), ("name", "other"), ("expired", True), ("expires_at", "2099-11-02T00:00:00Z"),
                                 ("size_in_bytes", info["size_in_bytes"] + 1), ("digest", "sha256:" + "0" * 64)):
                changed = copy.deepcopy(info); changed[field] = value
                with self.subTest(independent=independent, field=field), self.assertRaises(ValueError): validate(changed, self.record)

    def test_closed_five_file_zip_digests_metadata_and_every_file_hash(self):
        self.assertEqual(p.unpack_candidate(self.primary, self.record), self.payload)
        self.assertEqual(p.unpack_independent_candidate(self.independent, self.record), self.payload)
        for independent in (False, True):
            unpack = p.unpack_independent_candidate if independent else p.unpack_candidate
            with self.assertRaises(ValueError): unpack(self.primary[:-1], self.record)
            for extra in (("../outside", 0, b"x"), ("link", stat.S_IFLNK | 0o777, b"outside"),
                          ("candidate.json", 0, self.payload["candidate.json"])):
                data = packed(self.payload, extra); changed = copy.deepcopy(self.record)
                changed["artifacts"]["independent" if independent else "primary"].update(size=len(data), sha256=p.digest(data))
                with self.subTest(extra=extra[0]), self.assertRaises(ValueError): unpack(data, changed)
        changed = copy.deepcopy(self.record); changed["candidate_sha256"] = "0" * 64
        with self.assertRaises(ValueError): p.unpack_candidate(self.primary, changed)

    def test_docker_provenance_is_slsa_only_and_keeps_certificate_binding(self):
        results = provenance(self.c, "candidate.json", "d" * 64)
        self.assertIsNone(p.validate_provenance(results, self.c, "candidate.json", "d" * 64))
        for field, value in (("sourceRepositoryIdentifier", "1"), ("runnerEnvironment", "self-hosted"),
                             ("runInvocationURI", p.REPO_URI + "/actions/runs/1234/attempts/1"),
                             ("subjectAlternativeName", numeric.WORKFLOW_URI)):
            changed = copy.deepcopy(results); changed[0]["verificationResult"]["signature"]["certificate"][field] = value
            with self.subTest(field=field), self.assertRaises(ValueError): p.validate_provenance(changed, self.c, "candidate.json", "d" * 64)
        with self.assertRaises(ValueError): p.validate_provenance(results, self.c, "candidate.json", "d" * 64, predicate_type=p.SPDX_PREDICATE_TYPE)

    def test_archive_spdx_uses_only_the_same_verified_sole_archive_statement(self):
        sbom, required = sbom_fixture(self.record, self.payload)
        name, digest = self.c["assets"][0]["name"], self.c["assets"][0]["sha256"]
        valid = provenance(self.c, name, digest, p.SPDX_PREDICATE_TYPE, sbom)
        validate = lambda results: p.validate_archive_spdx(results, self.c, self.payload["connected-manifest.json"], required)
        self.assertIsNone(validate(valid))
        changed_manifest = json.loads(self.payload["connected-manifest.json"])
        target = sbom["files"][0]["fileName"]
        next(item for item in changed_manifest["files"] if item["name"] == target)["sha256"] = "0" * 64
        changed_sbom = copy.deepcopy(sbom)
        changed_sbom["files"][0]["checksums"][0]["checksumValue"] = "0" * 64
        forged_manifest = encoded(changed_manifest)
        with self.assertRaises(ValueError):
            p.validate_archive_spdx(provenance(self.c, name, digest, p.SPDX_PREDICATE_TYPE, changed_sbom),
                                    self.c, forged_manifest, required)
        unrelated = copy.deepcopy(valid); unrelated[0]["verificationResult"]["signature"]["certificate"]["sourceRepositoryIdentifier"] = "1"
        wrong = copy.deepcopy(valid); wrong[0]["verificationResult"]["statement"]["predicate"] = {"spdxVersion": "SPDX-2.3"}
        with self.assertRaises(ValueError): validate(wrong + unrelated)
        for mutate in (lambda x: x[0]["verificationResult"]["statement"]["subject"].append({"name": "candidate.json", "digest": {"sha256": "a" * 64}}),
                       lambda x: x[0]["verificationResult"]["statement"].update(predicateType="https://spdx.dev/Document/v2.2"),
                       lambda x: x[0]["verificationResult"]["statement"]["predicate"]["files"].pop(),
                       lambda x: x[0]["verificationResult"]["statement"]["predicate"]["packages"].pop()):
            changed = copy.deepcopy(valid); mutate(changed)
            with self.subTest(mutate=mutate), self.assertRaises(ValueError): validate(changed)

    def test_evidence_path_and_exact_release_body_keep_profiles_separate(self):
        self.assertEqual(p.evidence_path(self.record), "docs/acceptance/connected-linux-docker/0.1.0-canary.3.json")
        receipt = encoded(self.record)
        body = p.release_body(self.record, receipt)
        self.assertIn("Connected Linux process-and-Docker explicit-start beta.", body)
        self.assertIn("Independent artifact: 5679 sha256:", body)
        self.assertNotEqual(body, numeric.release_body({"candidate": self.c, "artifact": self.record["artifacts"]["primary"]}, receipt))
        raw = b":000000 100644 " + b"0" * 40 + b" " + b"d" * 40 + b" A\0" + p.evidence_path(self.record).encode() + b"\0"
        p.validate_evidence_diff(raw, self.record)
        for changed in (raw.replace(b"100644", b"100755"), raw + raw, raw.replace(b"connected-linux-docker/", b"connected-linux/")):
            with self.assertRaises(ValueError): p.validate_evidence_diff(changed, self.record)

    def test_release_retries_remain_closed_and_never_adopt_foreign_authority(self):
        receipt = encoded(self.record)
        tag = {"object": {"type": "commit", "sha": self.c["source"]}}
        release = {"id": 900, "tag_name": "v" + self.c["version"], "target_commitish": self.c["source"],
                   "name": "Home Lab Observer " + self.c["version"], "body": p.release_body(self.record, receipt),
                   "prerelease": True, "draft": True, "immutable": False, "assets": []}
        self.assertEqual(p.release_state(None, None, self.record, receipt), ("CREATE_TAG", []))
        self.assertEqual(p.release_state(tag, None, self.record, receipt), ("CREATE_DRAFT", []))
        self.assertEqual(p.release_state(tag, release, self.record, receipt), ("UPLOAD_MISSING", candidate.asset_names(self.c["version"])))
        for index, asset in enumerate(self.c["assets"]):
            release["assets"].append({"id": 100 + index, "name": asset["name"], "state": "uploaded", "size": asset["size"], "digest": "sha256:" + asset["sha256"]})
        self.assertEqual(p.release_state(tag, release, self.record, receipt), ("PUBLISH", []))
        release.update(draft=False, immutable=True)
        self.assertEqual(p.release_state(tag, release, self.record, receipt), ("VERIFY_PUBLISHED", []))
        for field, value in (("body", "foreign"), ("target_commitish", "main"), ("immutable", False)):
            changed = copy.deepcopy(release); changed[field] = value
            with self.subTest(field=field), self.assertRaises(ValueError): p.release_state(tag, changed, self.record, receipt)

    def test_only_the_two_actual_candidate_presets_are_admitted_by_shared_policy(self):
        import _connected_promotion_core as core
        forged = copy.copy(candidate_core._DOCKER)
        with self.assertRaises(ValueError): core.artifact_name(forged, self.c)

    def test_numeric_release_body_and_none_return_remain_byte_compatible(self):
        from test_connected_promotion_policy import fixture as numeric_fixture, encoded as numeric_encoded
        record, _, _ = numeric_fixture()
        record["artifact"]["sha256"] = "f" * 64  # Remove the synthetic ZIP's wall-clock header from this golden.
        body = numeric.release_body(record, numeric_encoded(record))
        self.assertEqual(numeric.digest(body.encode("utf8")),
                         "c82dfd0af9e6eb5d62d244e56d0f38813d7a9cd109de792b7ef719caf46a0269")
        from test_connected_promotion_policy import PromotionPolicyTests
        fixture_test = PromotionPolicyTests(); fixture_test.setUp()
        self.assertIsNone(numeric.validate_provenance(fixture_test.provenance(), fixture_test.c, "candidate.json", "d" * 64))


if __name__ == "__main__":
    unittest.main()
