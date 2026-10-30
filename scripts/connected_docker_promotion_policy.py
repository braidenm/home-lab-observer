"""Fixed Docker publication policy; no caller-selected profile or provider I/O."""
import _connected_candidate_core as _candidate
import _connected_promotion_core as _core
from _connected_promotion_core import (REPOSITORY, REPO_URI, MAX_JSON_BYTES, PREDICATE_TYPES,
    DOCKER_PACKAGE_SHA256, SPDX_PREDICATE_TYPE, require, exact_keys, digest, sha, number, utc, decode_json)
from _connected_candidate_core import OWNER_ID, REPOSITORY_ID

WORKFLOW_ID = 401377430
WORKFLOW_PATH = _candidate._DOCKER.workflow_path
WORKFLOW_URI = REPO_URI + "/" + WORKFLOW_PATH + "@refs/heads/main"
MAX_ARTIFACT_BYTES = 65 * 1024 * 1024


def asset_names(version):
    return _candidate.asset_names(_candidate._DOCKER, version)


def validate_metadata(metadata, expected_workflow_id=None):
    require(expected_workflow_id is None or type(expected_workflow_id) is int and expected_workflow_id == WORKFLOW_ID)
    return _candidate.validate_metadata(_candidate._DOCKER, metadata, WORKFLOW_ID)


def decode_metadata(data):
    return _candidate.decode_metadata(_candidate._DOCKER, data, WORKFLOW_ID)


def artifact_name(candidate):
    return _core.artifact_name(_candidate._DOCKER, candidate)


def validate_acceptance(data, workflow_id, now):
    return _core.validate_acceptance(_candidate._DOCKER, data, workflow_id, now)


def validate_run(run, workflow, record):
    return _core.validate_run(_candidate._DOCKER, run, workflow, record)


def validate_artifact(info, record):
    return _core.validate_artifact(_candidate._DOCKER, info, record)


def validate_independent_artifact(info, record):
    return _core.validate_artifact(_candidate._DOCKER, info, record, independent=True)


def unpack_candidate(data, record):
    return _core.unpack_candidate(_candidate._DOCKER, data, record)


def unpack_independent_candidate(data, record):
    return _core.unpack_candidate(_candidate._DOCKER, data, record, independent=True)


def validate_attempt_jobs(response, record):
    return _core.validate_attempt_jobs(response, record)


def validate_prepare_run(run, workflow, run_id, source):
    return _core.validate_prepare_run(run, workflow, run_id, source)


def validate_prepare_jobs(response, run_id, source):
    return _core.validate_prepare_jobs(response, run_id, source)


def validate_provenance(results, candidate, name, expected_digest,
                        predicate_type="https://slsa.dev/provenance/v1"):
    return _core.validate_provenance(_candidate._DOCKER, results, candidate, name, expected_digest, predicate_type)


def validate_archive_spdx(results, candidate, manifest_bytes, required_modules):
    return _core.validate_archive_spdx(results, candidate, manifest_bytes, required_modules)


def evidence_path(record):
    return _core.evidence_path(_candidate._DOCKER, record)


def validate_evidence_diff(raw_diff, record):
    return _core.validate_evidence_diff(_candidate._DOCKER, raw_diff, record)


def release_body(record, acceptance_bytes):
    return _core.release_body(_candidate._DOCKER, record, acceptance_bytes)


def release_state(tag, release, record, acceptance_bytes):
    return _core.release_state(_candidate._DOCKER, tag, release, record, acceptance_bytes)
