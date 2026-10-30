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

from types import MappingProxyType
import _connected_candidate_core as candidate_core
from _connected_candidate_core import OWNER_ID, REPOSITORY_ID

WORKFLOW_PATH = candidate_core._NUMERIC.workflow_path

REPOSITORY = "braidenm/home-lab-observer"
REPO_URI = "https://github.com/" + REPOSITORY
WORKFLOW_URI = REPO_URI + "/" + WORKFLOW_PATH + "@refs/heads/main"
MAX_ARTIFACT_BYTES = 64 * 1024 * 1024
MAX_JSON_BYTES = 32768
DOCKER_WORKFLOW_ID = 401377430
SPDX_PREDICATE_TYPE = "https://spdx.dev/Document/v2.3"
DOCKER_PACKAGE_SHA256 = MappingProxyType({
    "docker_ce_sha256": "95ddf7a2d8b8c0eb82d8597d4d604e294dcf741081f2e228524d5ab9bfd7c8ae",
    "docker_ce_cli_sha256": "74747b5c99000279b9b2c7eb82543b4121d8436a58aa3be7e78ac6681d74794f",
    "containerd_sha256": "f811ab57d1f4bb0cce19df7d526da8c711d8ebea0447cc83da1999188e664122",
})
PREDICATE_TYPES = ("https://slsa.dev/provenance/v1", "https://spdx.dev/Document/v2.3")
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


def _preset(preset):
    require(preset is candidate_core._NUMERIC or preset is candidate_core._DOCKER)


def _candidate(preset, value):
    _preset(preset)
    if preset is candidate_core._DOCKER:
        candidate_core.validate_metadata(preset, value, DOCKER_WORKFLOW_ID)
    return value


def _artifact(preset, record, independent=False):
    _preset(preset)
    require(type(independent) is bool)
    if preset is candidate_core._NUMERIC:
        require(not independent)
        return record["artifact"]
    _candidate(preset, record["candidate"])
    return record["artifacts"]["independent" if independent else "primary"]


def _zip_limit(preset):
    _preset(preset)
    return MAX_ARTIFACT_BYTES + (1048576 if preset is candidate_core._DOCKER else 0)


def artifact_name(preset, candidate):
    _candidate(preset, candidate)
    return (f"{preset.artifact_prefix}-{candidate['version']}-"
            f"{candidate['run_id']}-{candidate['run_attempt']}")


def validate_acceptance(preset, data, workflow_id, now):
    _preset(preset)
    record = decode_json(data)
    if preset is candidate_core._DOCKER:
        return _docker_acceptance(record, workflow_id, now)
    exact_keys(record, ("schema_version", "candidate", "candidate_sha256", "artifact", "vm"))
    require(record["schema_version"] == "observer-connected-acceptance/v1")
    candidate = candidate_core.validate_metadata(preset, record["candidate"], expected_workflow_id=workflow_id)
    sha(record["candidate_sha256"])
    artifact = record["artifact"]
    exact_keys(artifact, ("id", "name", "sha256", "size", "expires_at"))
    number(artifact["id"])
    require(artifact["name"] == artifact_name(preset, candidate))
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


def _docker_acceptance(record, workflow_id, now):
    require(type(workflow_id) is int and workflow_id == DOCKER_WORKFLOW_ID)
    exact_keys(record, ("schema_version", "candidate", "candidate_sha256", "artifacts", "vm"))
    require(record["schema_version"] == "observer-connected-docker-acceptance/v1")
    candidate = _candidate(candidate_core._DOCKER, record["candidate"])
    sha(record["candidate_sha256"])
    require(record["candidate_sha256"] == digest(candidate_core.encode_metadata(candidate_core._DOCKER, candidate)))
    exact_keys(record["artifacts"], ("primary", "independent"))
    require(isinstance(now, dt.datetime) and now.utcoffset() == dt.timedelta(0))
    for key in ("primary", "independent"):
        artifact = record["artifacts"][key]
        exact_keys(artifact, ("id", "name", "sha256", "size", "expires_at"))
        number(artifact["id"])
        expected = artifact_name(candidate_core._DOCKER, candidate) + ("-independent" if key == "independent" else "")
        require(artifact["name"] == expected)
        sha(artifact["sha256"])
        require(number(artifact["size"]) <= _zip_limit(candidate_core._DOCKER))
        require(utc(artifact["expires_at"]) > now)
    require(record["artifacts"]["primary"]["id"] != record["artifacts"]["independent"]["id"])
    vm = record["vm"]
    exact_keys(vm, ("scope", "source", "image_sha256", "receiver_sha256", "probe_sha256",
                    "harness_sha256", "baseline", "docker_inputs", "cases", "all_pass", "cleanup"))
    require(vm["scope"] == "rich-docker" and vm["source"] == candidate["source"])
    for key in ("image_sha256", "receiver_sha256", "probe_sha256", "harness_sha256"):
        sha(vm[key])
    exact_keys(vm["baseline"], ("build_record_sha256", "numeric", "processes"))
    sha(vm["baseline"]["build_record_sha256"])
    for profile in ("numeric", "processes"):
        values = vm["baseline"][profile]
        exact_keys(values, ("archive_sha256", "manifest_sha256", "installer_sha256", "checksums_sha256"))
        for value in values.values():
            sha(value)
    inputs = vm["docker_inputs"]
    exact_keys(inputs, (*DOCKER_PACKAGE_SHA256, "fixture_sha256", "rootfs_sha256"))
    for key, value in inputs.items():
        sha(value)
        if key in DOCKER_PACKAGE_SHA256:
            require(value == DOCKER_PACKAGE_SHA256[key])
    exact_keys(vm["cases"], ("rich-docker", "rich-docker-refusal"))
    for values in vm["cases"].values():
        exact_keys(values, ("first_boot", "reboot", "case"))
        require(all(value == "PASS" for value in values.values()))
    require(vm["all_pass"] == "PASS")
    exact_keys(vm["cleanup"], ("qemu", "mounts", "loops"))
    require(all(value == "PASS" for value in vm["cleanup"].values()))
    return record


def validate_run(preset, run, workflow, record):
    c = _candidate(preset, record["candidate"])
    _validate_run_fields(preset, run, workflow, c["workflow_id"], c["run_id"], c["run_attempt"], c["source"])


def _validate_run_fields(preset, run, workflow, workflow_id, run_id, attempt, source):
    _preset(preset)
    if preset is candidate_core._DOCKER:
        require(type(run) is dict and type(workflow) is dict)
        for key in ("id", "run_attempt", "workflow_id"):
            number(run.get(key))
        number(workflow.get("id"))
        for key in ("repository", "head_repository"):
            require(type(run.get(key)) is dict and type(run[key].get("owner")) is dict)
            number(run[key].get("id"))
            number(run[key]["owner"].get("id"))
        for key in ("actor", "triggering_actor"):
            require(type(run.get(key)) is dict)
            number(run[key].get("id"))
    require(workflow["id"] == workflow_id and workflow["path"] == preset.workflow_path
            and workflow["state"] == "active")
    require(run["id"] == run_id and run["run_attempt"] == attempt
            and run["workflow_id"] == workflow_id and run["path"] == preset.workflow_path
            and run["head_sha"] == source and run["head_branch"] == "main"
            and run["event"] == "workflow_dispatch" and run["status"] == "completed"
            and run["conclusion"] == "success")
    for key in ("repository", "head_repository"):
        require(run[key]["id"] == REPOSITORY_ID and
                run[key]["full_name"] == REPOSITORY and
                run[key]["owner"]["id"] == OWNER_ID)
    for key in ("actor", "triggering_actor"):
        require(run[key]["id"] == OWNER_ID and run[key]["login"] == "braidenm")


def validate_attempt_jobs(response, record):
    c = _candidate(candidate_core._DOCKER, record["candidate"])
    validate_prepare_jobs(response, c["run_id"], c["source"])


def _prepare_identity(run_id, source):
    number(run_id)
    require(type(source) is str and re.fullmatch(r"[a-f0-9]{40}", source))


def validate_prepare_run(run, workflow, run_id, source):
    """Bind the latest run before any candidate bytes are downloaded."""
    _prepare_identity(run_id, source)
    require(type(run) is dict)
    attempt = number(run.get("run_attempt"))
    _validate_run_fields(candidate_core._DOCKER, run, workflow, DOCKER_WORKFLOW_ID, run_id, attempt, source)
    return attempt


def validate_prepare_jobs(response, run_id, source):
    """The caller fetches the fixed current-attempt jobs endpoint, never all attempts."""
    _prepare_identity(run_id, source)
    require(type(response) is dict and type(response.get("total_count")) is int and response["total_count"] == 5)
    jobs = response.get("jobs")
    require(type(jobs) is list and len(jobs) == 5)
    names, identifiers = set(), set()
    for job in jobs:
        require(type(job) is dict)
        identifier = number(job.get("id"))
        require(identifier not in identifiers)
        identifiers.add(identifier)
        name = job.get("name")
        require(type(name) is str and name in ("authorize", "build", "independent", "compare", "attest") and name not in names)
        names.add(name)
        require(type(job.get("run_id")) is int and job["run_id"] == run_id
                and job.get("head_sha") == source and job.get("status") == "completed"
                and job.get("conclusion") == "success")


def validate_artifact(preset, info, record, independent=False):
    a, c = _artifact(preset, record, independent), record["candidate"]
    if preset is candidate_core._DOCKER:
        require(type(info) is dict and type(info.get("workflow_run")) is dict)
        number(info.get("id"))
        number(info.get("size_in_bytes"))
        for key in ("id", "repository_id", "head_repository_id"):
            number(info["workflow_run"].get(key))
    require(info["id"] == a["id"] and info["name"] == a["name"]
            and info["size_in_bytes"] == a["size"] and info["expired"] is False
            and info["digest"] == "sha256:" + a["sha256"]
            and info["expires_at"] == a["expires_at"])
    run = info["workflow_run"]
    require(run["id"] == c["run_id"] and run["repository_id"] == REPOSITORY_ID
            and run["head_repository_id"] == REPOSITORY_ID
            and run["head_sha"] == c["source"] and run["head_branch"] == "main")


def unpack_candidate(preset, data, record, independent=False):
    """Return a closed in-memory inventory, never generic archive extraction."""
    artifact = _artifact(preset, record, independent)
    require(isinstance(data, bytes) and len(data) == artifact["size"]
            and len(data) <= _zip_limit(preset) and digest(data) == artifact["sha256"])
    candidate = record["candidate"]
    names = candidate_core.asset_names(preset, candidate["version"]) + ["candidate.json"]
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
            and candidate_core.decode_metadata(preset, payload["candidate.json"]) == candidate)
    for item in candidate["assets"]:
        value = payload[item["name"]]
        require(len(value) == item["size"] and digest(value) == item["sha256"])
    return payload


def _matched_statement(preset, results, candidate, name, expected_digest, predicate_type, sole_subject=False):
    """Match only the output of a successful external cryptographic verifier."""
    _candidate(preset, candidate)
    require(predicate_type in PREDICATE_TYPES)
    require(isinstance(results, list) and 0 < len(results) <= 30)
    workflow_uri = REPO_URI + "/" + preset.workflow_path + "@refs/heads/main"
    for result in results:
        if preset is candidate_core._DOCKER and not isinstance(result, dict):
            continue
        verification = result.get("verificationResult", {})
        if preset is candidate_core._DOCKER and (not isinstance(verification, dict) or
                not isinstance(verification.get("signature", {}), dict)):
            continue
        cert = verification.get("signature", {}).get("certificate", {})
        statement = verification.get("statement", {})
        if preset is candidate_core._DOCKER and (not isinstance(cert, dict) or not isinstance(statement, dict)):
            continue
        expected = {
            "issuer": "https://token.actions.githubusercontent.com",
            "subjectAlternativeName": workflow_uri,
            "buildSignerURI": workflow_uri, "buildSignerDigest": candidate["source"],
            "buildConfigURI": workflow_uri, "buildConfigDigest": candidate["source"],
            "buildTrigger": "workflow_dispatch", "runnerEnvironment": "github-hosted",
            "sourceRepositoryURI": REPO_URI,
            "sourceRepositoryDigest": candidate["source"],
            "sourceRepositoryIdentifier": str(REPOSITORY_ID),
            "sourceRepositoryOwnerIdentifier": str(OWNER_ID),
            "sourceRepositoryRef": "refs/heads/main",
            "runInvocationURI": (REPO_URI + f"/actions/runs/{candidate['run_id']}"
                                 f"/attempts/{candidate['run_attempt']}"),
        }
        timestamps = verification.get("verifiedTimestamps")
        if not (all(cert.get(key) == value for key, value in expected.items())
                and timestamps and (preset is candidate_core._NUMERIC or isinstance(timestamps, list))
                and statement.get("predicateType") == predicate_type):
            continue
        subjects = statement.get("subject", [])
        if sole_subject:
            subject_match = subjects == [{"name": name, "digest": {"sha256": expected_digest}}]
        else:
            if preset is candidate_core._DOCKER and (not isinstance(subjects, list) or
                    any(not isinstance(item, dict) for item in subjects)):
                continue
            subject_match = any(item.get("name") == name and
                                item.get("digest") == {"sha256": expected_digest} for item in subjects)
        if subject_match:
            return statement
    require(False)


def validate_provenance(preset, results, candidate, name, expected_digest,
                        predicate_type="https://slsa.dev/provenance/v1"):
    _preset(preset)
    if preset is candidate_core._DOCKER:
        require(predicate_type == "https://slsa.dev/provenance/v1")
    _matched_statement(preset, results, candidate, name, expected_digest, predicate_type)


def validate_archive_spdx(results, candidate, manifest_bytes, required_modules):
    _candidate(candidate_core._DOCKER, candidate)
    manifest = candidate["assets"][1]
    require(type(manifest_bytes) is bytes and len(manifest_bytes) == manifest["size"]
            and digest(manifest_bytes) == manifest["sha256"])
    archive = candidate["assets"][0]
    statement = _matched_statement(candidate_core._DOCKER, results, candidate, archive["name"],
                                   archive["sha256"], SPDX_PREDICATE_TYPE, sole_subject=True)
    predicate = statement.get("predicate")
    require(type(predicate) is dict)
    try:
        data = json.dumps(predicate, ensure_ascii=True, separators=(",", ":")).encode("ascii")
        require(len(data) <= 1024 * 1024)
        candidate_core.validate_sbom(candidate_core._DOCKER, data, candidate, manifest_bytes, required_modules)
    except (TypeError, ValueError, RecursionError) as error:
        raise ValueError("connected_promotion_invalid") from error


def evidence_path(preset, record):
    _candidate(preset, record["candidate"])
    directory = "connected-linux-docker" if preset is candidate_core._DOCKER else "connected-linux"
    return "docs/acceptance/" + directory + "/" + record["candidate"]["version"] + ".json"


def validate_evidence_diff(preset, raw_diff, record):
    # git diff --raw --no-abbrev --no-renames -z SOURCE HEAD
    parts = raw_diff.split(b"\0")
    require(len(parts) == 3 and parts[-1] == b"")
    metadata = parts[0].decode("ascii").split(" ")
    require(len(metadata) == 5 and metadata[0] == ":000000" and metadata[1] == "100644"
            and metadata[2] == "0" * 40 and re.fullmatch(r"[a-f0-9]{40}", metadata[3])
            and metadata[4] == "A" and parts[1].decode("ascii") == evidence_path(preset, record))


def release_body(preset, record, acceptance_bytes):
    c, a = _candidate(preset, record["candidate"]), _artifact(preset, record)
    header = ("Connected Linux process-and-Docker explicit-start beta." if preset is candidate_core._DOCKER
              else "Connected Linux explicit-start beta.")
    independent = record["artifacts"]["independent"] if preset is candidate_core._DOCKER else None
    extra = f"Independent artifact: {independent['id']} sha256:{independent['sha256']}\n" if independent else ""
    return (header + " Ubuntu 24.04/systemd 255/amd64 only.\n\n"
            f"Source: {c['source']}\nCandidate: {c['run_id']}/{c['run_attempt']}\n"
            f"Artifact: {a['id']} sha256:{a['sha256']}\n" + extra +
            f"Acceptance: {evidence_path(preset, record)} sha256:{digest(acceptance_bytes)}\n\n"
            "Unsigned beta with checksums and provenance, not a publisher-signed package.\n"
            "Install leaves services stopped; explicit verified start is required after reboot.\n"
            "No unattended lifecycle, upgrades/removal, container controls or remote host logs.\n")


def release_state(preset, tag, release, record, acceptance_bytes):
    """Closed retry states; caller revalidates before each proposed mutation."""
    c = _candidate(preset, record["candidate"])
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
            and release["body"] == release_body(preset, record, acceptance_bytes)
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
