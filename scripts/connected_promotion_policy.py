"""Numeric publication facade; preserve its v1 contract and public signatures."""
import _connected_candidate_core as _candidate
import _connected_promotion_core as _core
from _connected_promotion_core import (REPOSITORY, REPO_URI, WORKFLOW_URI, MAX_ARTIFACT_BYTES,
    MAX_JSON_BYTES, PREDICATE_TYPES, RESULTS, require, exact_keys, digest, sha, number, utc, decode_json)
from connected_candidate import (OWNER_ID, REPOSITORY_ID, WORKFLOW_PATH,
                                 asset_names, decode_metadata, validate_metadata)


def artifact_name(candidate):
    return _core.artifact_name(_candidate._NUMERIC, candidate)


def validate_acceptance(data, workflow_id, now):
    return _core.validate_acceptance(_candidate._NUMERIC, data, workflow_id, now)


def validate_run(run, workflow, record):
    return _core.validate_run(_candidate._NUMERIC, run, workflow, record)


def validate_artifact(info, record):
    return _core.validate_artifact(_candidate._NUMERIC, info, record)


def unpack_candidate(data, record):
    return _core.unpack_candidate(_candidate._NUMERIC, data, record)


def validate_provenance(results, candidate, name, expected_digest,
                        predicate_type="https://slsa.dev/provenance/v1"):
    return _core.validate_provenance(_candidate._NUMERIC, results, candidate, name, expected_digest, predicate_type)


def evidence_path(record):
    return _core.evidence_path(_candidate._NUMERIC, record)


def validate_evidence_diff(raw_diff, record):
    return _core.validate_evidence_diff(_candidate._NUMERIC, raw_diff, record)


def release_body(record, acceptance_bytes):
    return _core.release_body(_candidate._NUMERIC, record, acceptance_bytes)


def release_state(tag, release, record, acceptance_bytes):
    return _core.release_state(_candidate._NUMERIC, tag, release, record, acceptance_bytes)
