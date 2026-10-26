"""Pure admission policy for owner-operated connected Linux publication.

Provider responses are data. This module grants no network/filesystem mutation
and never treats user-controlled provenance predicates as certificate identity.
"""
import datetime as dt
import hashlib
import io
import json
import re
import stat
import zipfile

from connected_candidate import (OWNER_ID, REPOSITORY_ID, WORKFLOW_PATH,
                                 asset_names, decode_metadata, validate_metadata)

REPOSITORY = "braidenm/home-lab-observer"
REPO_URI = "https://github.com/" + REPOSITORY
WORKFLOW_URI = REPO_URI + "/" + WORKFLOW_PATH + "@refs/heads/main"
MAX_ARTIFACT_BYTES = 64 * 1024 * 1024
MAX_JSON_BYTES = 32768
RESULTS = ("helper_corrupt", "archive_corrupt", "http_downgrade", "redirect_loop",
           "valid_stopped_reboot", "useful_upload", "stop_start_sequence",
           "explicit_reboot_start", "retained_identity")


def require(condition):
    if not condition:
        raise ValueError("connected_promotion_invalid")


def exact_keys(value, keys):
    require(isinstance(value, dict) and set(value) == set(keys))


def digest(value):
    return hashlib.sha256(value).hexdigest()


def sha(value):
    require(isinstance(value, str) and re.fullmatch(r"[a-f0-9]{64}", value))
    return value


def number(value):
    require(type(value) is int and 0 < value < 2**53)
    return value


def utc(value):
    require(isinstance(value, str) and
            re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", value))
    return dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=dt.timezone.utc)


def decode_json(data):
    require(isinstance(data, bytes) and 0 < len(data) <= MAX_JSON_BYTES)

    def unique(pairs):
        value = {}
        for key, item in pairs:
            require(key not in value)
            value[key] = item
        return value

    try:
        return json.loads(data, object_pairs_hook=unique,
                          parse_constant=lambda _: require(False))
    except (UnicodeError, json.JSONDecodeError, RecursionError) as exc:
        raise ValueError("connected_promotion_invalid") from exc


def artifact_name(candidate):
    return (f"connected-linux-{candidate['version']}-"
            f"{candidate['run_id']}-{candidate['run_attempt']}")


def validate_acceptance(data, workflow_id, now):
    record = decode_json(data)
    exact_keys(record, ("schema_version", "candidate", "candidate_sha256", "artifact", "vm"))
    require(record["schema_version"] == "observer-connected-acceptance/v1")
    candidate = validate_metadata(record["candidate"], expected_workflow_id=workflow_id)
    sha(record["candidate_sha256"])
    artifact = record["artifact"]
    exact_keys(artifact, ("id", "name", "sha256", "size", "expires_at"))
    number(artifact["id"])
    require(artifact["name"] == artifact_name(candidate))
    sha(artifact["sha256"])
    require(number(artifact["size"]) <= MAX_ARTIFACT_BYTES)
    require(utc(artifact["expires_at"]) > now)
    vm = record["vm"]
    exact_keys(vm, ("image_sha256", "receiver_sha256", "probe_sha256", "harness_sha256", "results"))
    for key in ("image_sha256", "receiver_sha256", "probe_sha256", "harness_sha256"):
        sha(vm[key])
    exact_keys(vm["results"], RESULTS)
    require(all(vm["results"][key] == "PASS" for key in RESULTS))
    return record


def validate_run(run, workflow, record):
    c = record["candidate"]
    require(workflow["id"] == c["workflow_id"] and workflow["path"] == WORKFLOW_PATH
            and workflow["state"] == "active")
    require(run["id"] == c["run_id"] and run["run_attempt"] == c["run_attempt"]
            and run["workflow_id"] == c["workflow_id"] and run["path"] == WORKFLOW_PATH
            and run["head_sha"] == c["source"] and run["head_branch"] == "main"
            and run["event"] == "workflow_dispatch" and run["status"] == "completed"
            and run["conclusion"] == "success")
    for key in ("repository", "head_repository"):
        require(run[key]["id"] == REPOSITORY_ID and
                run[key]["full_name"] == REPOSITORY and
                run[key]["owner"]["id"] == OWNER_ID)
    for key in ("actor", "triggering_actor"):
        require(run[key]["id"] == OWNER_ID and run[key]["login"] == "braidenm")


def validate_artifact(info, record):
    a, c = record["artifact"], record["candidate"]
    require(info["id"] == a["id"] and info["name"] == a["name"]
            and info["size_in_bytes"] == a["size"] and info["expired"] is False
            and info["digest"] == "sha256:" + a["sha256"]
            and info["expires_at"] == a["expires_at"])
    run = info["workflow_run"]
    require(run["id"] == c["run_id"] and run["repository_id"] == REPOSITORY_ID
            and run["head_repository_id"] == REPOSITORY_ID
            and run["head_sha"] == c["source"] and run["head_branch"] == "main")


def unpack_candidate(data, record):
    """Return a closed in-memory inventory, never generic archive extraction."""
    require(isinstance(data, bytes) and len(data) == record["artifact"]["size"]
            and len(data) <= MAX_ARTIFACT_BYTES and digest(data) == record["artifact"]["sha256"])
    candidate = record["candidate"]
    names = asset_names(candidate["version"]) + ["candidate.json"]
    payload = {}
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            entries = archive.infolist()
            require(len(entries) == len(names) and
                    sorted(item.filename for item in entries) == sorted(names))
            for item in entries:
                mode = item.external_attr >> 16
                require(not item.is_dir() and (mode == 0 or stat.S_IFMT(mode) in (0, stat.S_IFREG))
                        and not item.flag_bits & 1 and item.compress_type in
                        (zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED)
                        and 0 < item.file_size <= MAX_ARTIFACT_BYTES)
                limit = MAX_JSON_BYTES if item.filename == "candidate.json" else next(
                    x["size"] for x in candidate["assets"] if x["name"] == item.filename)
                require(item.file_size <= limit)
                with archive.open(item) as stream:
                    value = stream.read(limit + 1)
                    require(len(value) == item.file_size and len(value) <= limit)
                payload[item.filename] = value
    except (zipfile.BadZipFile, RuntimeError, OSError, EOFError) as exc:
        raise ValueError("connected_promotion_invalid") from exc
    require(digest(payload["candidate.json"]) == record["candidate_sha256"]
            and decode_metadata(payload["candidate.json"]) == candidate)
    for item in candidate["assets"]:
        value = payload[item["name"]]
        require(len(value) == item["size"] and digest(value) == item["sha256"])
    return payload


def validate_provenance(results, candidate, name, expected_digest,
                        predicate_type="https://slsa.dev/provenance/v1"):
    """Apply policy ONLY after gh cryptographic verification succeeds."""
    require(predicate_type in ("https://slsa.dev/provenance/v1", "https://spdx.dev/Document"))
    require(isinstance(results, list) and 0 < len(results) <= 30)
    for result in results:
        verification = result.get("verificationResult", {})
        cert = verification.get("signature", {}).get("certificate", {})
        expected = {
            "issuer": "https://token.actions.githubusercontent.com",
            "subjectAlternativeName": WORKFLOW_URI,
            "buildSignerURI": WORKFLOW_URI, "buildSignerDigest": candidate["source"],
            "buildConfigURI": WORKFLOW_URI, "buildConfigDigest": candidate["source"],
            "buildTrigger": "workflow_dispatch", "runnerEnvironment": "github-hosted",
            "sourceRepositoryURI": REPO_URI,
            "sourceRepositoryDigest": candidate["source"],
            "sourceRepositoryIdentifier": str(REPOSITORY_ID),
            "sourceRepositoryOwnerIdentifier": str(OWNER_ID),
            "sourceRepositoryRef": "refs/heads/main",
            "runInvocationURI": (REPO_URI + f"/actions/runs/{candidate['run_id']}"
                                 f"/attempts/{candidate['run_attempt']}"),
        }
        statement = verification.get("statement", {})
        subjects = statement.get("subject", [])
        if (all(cert.get(key) == value for key, value in expected.items())
                and verification.get("verifiedTimestamps")
                and statement.get("predicateType") == predicate_type
                and any(item.get("name") == name and
                        item.get("digest") == {"sha256": expected_digest} for item in subjects)):
            return
    require(False)


def evidence_path(record):
    return "docs/acceptance/connected-linux/" + record["candidate"]["version"] + ".json"


def validate_evidence_diff(raw_diff, record):
    # git diff --raw --no-abbrev --no-renames -z SOURCE HEAD
    parts = raw_diff.split(b"\0")
    require(len(parts) == 3 and parts[-1] == b"")
    metadata = parts[0].decode("ascii").split(" ")
    require(len(metadata) == 5 and metadata[0] == ":000000" and metadata[1] == "100644"
            and metadata[2] == "0" * 40 and re.fullmatch(r"[a-f0-9]{40}", metadata[3])
            and metadata[4] == "A" and parts[1].decode("ascii") == evidence_path(record))


def release_body(record, acceptance_bytes):
    c, a = record["candidate"], record["artifact"]
    return ("Connected Linux explicit-start beta. Ubuntu 24.04/systemd 255/amd64 only.\n\n"
            f"Source: {c['source']}\nCandidate: {c['run_id']}/{c['run_attempt']}\n"
            f"Artifact: {a['id']} sha256:{a['sha256']}\n"
            f"Acceptance: {evidence_path(record)} sha256:{digest(acceptance_bytes)}\n\n"
            "Unsigned beta with checksums and provenance, not a publisher-signed package.\n"
            "Install leaves services stopped; explicit verified start is required after reboot.\n"
            "No unattended lifecycle, upgrades/removal, container controls or remote host logs.\n")


def release_state(tag, release, record, acceptance_bytes):
    """Closed retry states; caller revalidates before each proposed mutation."""
    c = record["candidate"]
    if tag is None:
        require(release is None)
        return "CREATE_TAG", []
    require(tag["object"]["type"] == "commit" and tag["object"]["sha"] == c["source"])
    if release is None:
        return "CREATE_DRAFT", []
    number(release["id"])
    require(release["tag_name"] == "v" + c["version"]
            and release["target_commitish"] == c["source"]
            and release["name"] == "Home Lab Observer " + c["version"]
            and release["body"] == release_body(record, acceptance_bytes)
            and release["prerelease"] is True and type(release["draft"]) is bool)
    assets = release["assets"]
    require(isinstance(assets, list) and len(assets) <= 4)
    expected = {item["name"]: item for item in c["assets"]}
    seen = set()
    for item in assets:
        number(item["id"])
        require(item["name"] in expected and item["name"] not in seen)
        target = expected[item["name"]]
        require(item["state"] == "uploaded" and item["size"] == target["size"]
                and item["digest"] == "sha256:" + target["sha256"])
        seen.add(item["name"])
    missing = [name for name in expected if name not in seen]
    if not release["draft"]:
        require(release["immutable"] is True and not missing)
        return "VERIFY_PUBLISHED", []
    require(release.get("immutable", False) is False)
    return ("UPLOAD_MISSING", missing) if missing else ("PUBLISH", [])
