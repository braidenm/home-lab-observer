"""Synthetic provider contracts; no credential, host observation or publication."""
import copy
import datetime as dt
import io
import json
import stat
import unittest
import zipfile

import connected_promotion_policy as p


def encoded(value):
    return (json.dumps(value, separators=(",", ":"), ensure_ascii=True) + "\n").encode()


def fixture():
    c = {"schema_version": "observer-connected-candidate/v1",
         "repository_id": p.REPOSITORY_ID, "owner_id": p.OWNER_ID,
         "workflow_path": p.WORKFLOW_PATH, "workflow_id": 555,
         "ref": "refs/heads/main", "run_id": 1234, "run_attempt": 2,
         "source": "a" * 40, "version": "0.1.0-canary.1",
         "contract_sha256": "b" * 64, "assets": []}
    payload = {}
    for name in p.asset_names(c["version"]):
        value = ("synthetic-" + name).encode()
        payload[name] = value
        c["assets"].append({"name": name, "size": len(value), "sha256": p.digest(value)})
    payload["candidate.json"] = encoded(c)
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w") as archive:
        for name, value in payload.items():
            archive.writestr(name, value)
    packed = stream.getvalue()
    record = {"schema_version": "observer-connected-acceptance/v1", "candidate": c,
              "candidate_sha256": p.digest(payload["candidate.json"]),
              "artifact": {"id": 5678, "name": p.artifact_name(c),
                           "sha256": p.digest(packed), "size": len(packed),
                           "expires_at": "2026-11-01T00:00:00Z"},
              "vm": {key: "c" * 64 for key in
                     ("image_sha256", "receiver_sha256", "probe_sha256", "harness_sha256")}}
    record["vm"]["results"] = dict.fromkeys(p.RESULTS, "PASS")
    return record, payload, packed


class PromotionPolicyTests(unittest.TestCase):
    def setUp(self):
        self.record, self.payload, self.packed = fixture()
        self.c = self.record["candidate"]
        self.now = dt.datetime(2026, 10, 26, tzinfo=dt.timezone.utc)

    def test_acceptance_requires_every_exact_case_and_nonexpired_identity(self):
        self.assertEqual(p.validate_acceptance(encoded(self.record), 555, self.now), self.record)
        mutations = [lambda r: r["vm"]["results"].pop("useful_upload"),
                     lambda r: r["vm"]["results"].update(useful_upload="NOT_RUN"),
                     lambda r: r["artifact"].update(expires_at="2026-10-01T00:00:00Z"),
                     lambda r: r["artifact"].update(size=True),
                     lambda r: r["candidate"].update(run_attempt=1),
                     lambda r: r.update(extra="not allowed")]
        for mutate in mutations:
            with self.subTest(mutate=mutate):
                r = copy.deepcopy(self.record)
                mutate(r)
                with self.assertRaises(ValueError):
                    p.validate_acceptance(encoded(r), 555, self.now)
        with self.assertRaises(ValueError):
            p.validate_acceptance(encoded(self.record), 556, self.now)

    def test_duplicate_json_keys_are_not_last_value_wins(self):
        for value in (b'{"x":1,"x":2}', b'{"x":NaN}', b'[' * 10000):
            with self.assertRaises(ValueError):
                p.decode_json(value)

    def test_exact_zip_inventory_and_metadata_binding(self):
        self.assertEqual(p.unpack_candidate(self.packed, self.record), self.payload)
        for name, mode, data in (("../outside", 0, b"bad"),
                                 ("candidate.json", 0, self.payload["candidate.json"]),
                                 ("link", stat.S_IFLNK | 0o777, b"outside")):
            stream = io.BytesIO()
            with zipfile.ZipFile(stream, "w") as archive:
                for key, value in self.payload.items():
                    archive.writestr(key, value)
                item = zipfile.ZipInfo(name)
                item.external_attr = mode << 16
                archive.writestr(item, data)
            packed = stream.getvalue()
            r = copy.deepcopy(self.record)
            r["artifact"].update(size=len(packed), sha256=p.digest(packed))
            with self.assertRaises(ValueError):
                p.unpack_candidate(packed, r)
        with self.assertRaises(ValueError):
            p.unpack_candidate(self.packed[:-1], self.record)

    def test_evidence_only_diff_rejects_other_changes_and_modes(self):
        raw = (b":000000 100644 " + b"0" * 40 + b" " + b"d" * 40 + b" A\0" +
               p.evidence_path(self.record).encode() + b"\0")
        p.validate_evidence_diff(raw, self.record)
        for changed in (raw.replace(b"100644", b"100755"),
                        raw.replace(b" A\0", b" M\0"),
                        raw.replace(b"docs/acceptance", b"scripts/acceptance"), raw + raw):
            with self.assertRaises(ValueError):
                p.validate_evidence_diff(changed, self.record)

    def test_run_and_artifact_bind_current_attempt_and_canonical_repository(self):
        repo = {"id": p.REPOSITORY_ID, "full_name": p.REPOSITORY, "owner": {"id": p.OWNER_ID}}
        actor = {"id": p.OWNER_ID, "login": "braidenm"}
        run = {"id": 1234, "run_attempt": 2, "workflow_id": 555, "path": p.WORKFLOW_PATH,
               "head_sha": self.c["source"], "head_branch": "main", "event": "workflow_dispatch",
               "status": "completed", "conclusion": "success", "repository": repo,
               "head_repository": repo, "actor": actor, "triggering_actor": actor}
        workflow = {"id": 555, "path": p.WORKFLOW_PATH, "state": "active"}
        p.validate_run(run, workflow, self.record)
        for field, value in (("run_attempt", 3), ("conclusion", "failure"),
                             ("head_branch", "feature"), ("event", "pull_request")):
            changed = copy.deepcopy(run)
            changed[field] = value
            with self.assertRaises(ValueError):
                p.validate_run(changed, workflow, self.record)
        info = {"id": 5678, "name": self.record["artifact"]["name"],
                "size_in_bytes": len(self.packed), "expired": False,
                "digest": "sha256:" + p.digest(self.packed),
                "expires_at": self.record["artifact"]["expires_at"],
                "workflow_run": {"id": 1234, "repository_id": p.REPOSITORY_ID,
                                 "head_repository_id": p.REPOSITORY_ID,
                                 "head_sha": self.c["source"], "head_branch": "main"}}
        p.validate_artifact(info, self.record)
        for field, value in (("id", 5679), ("expired", True), ("digest", "sha256:" + "e" * 64)):
            changed = copy.deepcopy(info)
            changed[field] = value
            with self.assertRaises(ValueError):
                p.validate_artifact(changed, self.record)

    def provenance(self):
        cert = {"issuer": "https://token.actions.githubusercontent.com",
                "subjectAlternativeName": p.WORKFLOW_URI,
                "buildSignerURI": p.WORKFLOW_URI, "buildSignerDigest": self.c["source"],
                "buildConfigURI": p.WORKFLOW_URI, "buildConfigDigest": self.c["source"],
                "buildTrigger": "workflow_dispatch", "runnerEnvironment": "github-hosted",
                "sourceRepositoryURI": p.REPO_URI, "sourceRepositoryDigest": self.c["source"],
                "sourceRepositoryIdentifier": str(p.REPOSITORY_ID),
                "sourceRepositoryOwnerIdentifier": str(p.OWNER_ID),
                "sourceRepositoryRef": "refs/heads/main",
                "runInvocationURI": p.REPO_URI + "/actions/runs/1234/attempts/2"}
        return [{"verificationResult": {"signature": {"certificate": cert},
                  "verifiedTimestamps": [{"type": "Tlog"}],
                  "statement": {"predicateType": "https://slsa.dev/provenance/v1",
                                "subject": [{"name": "candidate.json",
                                             "digest": {"sha256": "d" * 64}}]}}}]

    def test_certificate_not_workflow_controlled_predicate_is_authority(self):
        p.validate_provenance(self.provenance(), self.c, "candidate.json", "d" * 64)
        for field, value in (("sourceRepositoryIdentifier", "1"),
                             ("sourceRepositoryOwnerIdentifier", "1"),
                             ("runnerEnvironment", "self-hosted"),
                             ("sourceRepositoryDigest", "f" * 40),
                             ("runInvocationURI", p.REPO_URI + "/actions/runs/1234/attempts/1")):
            result = self.provenance()
            result[0]["verificationResult"]["signature"]["certificate"][field] = value
            result[0]["verificationResult"]["statement"]["predicate"] = self.provenance()[0]
            with self.assertRaises(ValueError):
                p.validate_provenance(result, self.c, "candidate.json", "d" * 64)

    def test_sbom_attestation_keeps_same_certificate_and_subject_boundary(self):
        result = self.provenance()
        result[0]["verificationResult"]["statement"]["predicateType"] = "https://spdx.dev/Document/v2.3"
        p.validate_provenance(result, self.c, "candidate.json", "d" * 64,
                              predicate_type="https://spdx.dev/Document/v2.3")
        with self.assertRaises(ValueError):
            p.validate_provenance(result, self.c, "candidate.json", "d" * 64)
        with self.assertRaises(ValueError):
            p.validate_provenance(result, self.c, "candidate.json", "e" * 64,
                                  predicate_type="https://spdx.dev/Document/v2.3")
        for invalid in ("https://spdx.dev/Document", "https://spdx.dev/Document/v2.2",
                        "https://spdx.dev/Document/v2.3/extra", "https://spdx.dev", "arbitrary"):
            changed = copy.deepcopy(result)
            changed[0]["verificationResult"]["statement"]["predicateType"] = invalid
            with self.subTest(predicate=invalid), self.assertRaises(ValueError):
                p.validate_provenance(changed, self.c, "candidate.json", "d" * 64,
                                      predicate_type="https://spdx.dev/Document/v2.3")
            with self.assertRaises(ValueError):
                p.validate_provenance(changed, self.c, "candidate.json", "d" * 64,
                                      predicate_type=invalid)
        result[0]["verificationResult"]["signature"]["certificate"]["sourceRepositoryIdentifier"] = "1"
        with self.assertRaises(ValueError):
            p.validate_provenance(result, self.c, "candidate.json", "d" * 64,
                                  predicate_type="https://spdx.dev/Document/v2.3")

    def release(self):
        return {"id": 900, "tag_name": "v" + self.c["version"],
                "target_commitish": self.c["source"],
                "name": "Home Lab Observer " + self.c["version"],
                "body": p.release_body(self.record, encoded(self.record)),
                "prerelease": True, "draft": True, "immutable": False, "assets": []}

    def test_release_closed_retry_states_never_clobber(self):
        tag = {"object": {"type": "commit", "sha": self.c["source"]}}
        call = lambda t, r: p.release_state(t, r, self.record, encoded(self.record))
        self.assertEqual(call(None, None), ("CREATE_TAG", []))
        self.assertEqual(call(tag, None), ("CREATE_DRAFT", []))
        release = self.release()
        self.assertEqual(call(tag, release), ("UPLOAD_MISSING", p.asset_names(self.c["version"])))
        for index, asset in enumerate(self.c["assets"]):
            release["assets"].append({"id": 100 + index, "name": asset["name"], "state": "uploaded",
                                      "size": asset["size"], "digest": "sha256:" + asset["sha256"]})
        self.assertEqual(call(tag, release), ("PUBLISH", []))
        release.update(draft=False, immutable=True)
        self.assertEqual(call(tag, release), ("VERIFY_PUBLISHED", []))
        for field, value in (("immutable", False), ("target_commitish", "main"),
                             ("body", "changed"), ("prerelease", False)):
            changed = copy.deepcopy(release)
            changed[field] = value
            with self.assertRaises(ValueError):
                call(tag, changed)
        with self.assertRaises(ValueError):
            call({"object": {"type": "commit", "sha": "e" * 40}}, release)
        with self.assertRaises(ValueError):
            call(None, release)


if __name__ == "__main__":
    unittest.main()
