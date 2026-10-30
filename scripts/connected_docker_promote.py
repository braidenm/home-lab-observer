#!/usr/bin/env python3
"""Fixed owner-operated publication interface; no generic profile selector."""
import sys
from pathlib import Path
sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import _connected_promote_core as _core
from _connected_candidate_core import _NUMERIC, _DOCKER
REPO = _core.REPO
PREFIX = _core.PREFIX
INVALID = _core.INVALID
JSON_LIMIT = _core.JSON_LIMIT
ROOT = _core.ROOT
CONFLICT = _core.CONFLICT
subprocess = _core.subprocess
shutil = _core.shutil
import connected_docker_promotion_policy as policy
import connected_docker_candidate as candidate
environment = _core.environment
bounded_process = _core.bounded_process
decode_response = _core.decode_response
bounded_json = _core.bounded_json
ReleaseRedirect = _core.ReleaseRedirect
validate_download_url = _core.validate_download_url
anonymous_download = _core.anonymous_download
trusted_main = _core.trusted_main
ensure_same_main = _core.ensure_same_main
verify_assets = _core.verify_assets
release_authority = _core.release_authority
require = _core.require

# Registered fixed-path workflow after reviewed PR148. No CLI override/fallback.
WORKFLOW_ID = 401377430


class Provider(_core.Provider):
    def __init__(self, root=ROOT, run=bounded_process, source_environment=None):
        super().__init__(_DOCKER, root, run, source_environment)


def read_metadata(packed):
    return _core.read_metadata(_DOCKER, packed)


def artifact_record(info, independent_info, metadata, metadata_bytes):
    return _core.artifact_record(_DOCKER, info, metadata, metadata_bytes, independent_info)


def verify_candidate(provider, record, packed, independent_packed, workflow_id, directory, now):
    return _core.verify_candidate(_DOCKER, provider, record, packed, workflow_id, directory, now, independent_packed)


def prepare(provider, run_id, artifact_id, independent_artifact_id, output, workflow_id, now):
    return _core.prepare(_DOCKER, provider, run_id, artifact_id, output, workflow_id, now, independent_artifact_id)


def version_lock(provider, version):
    return _core.version_lock(_DOCKER, provider, version)


def release_snapshot(provider, record, acceptance):
    return _core.release_snapshot(_DOCKER, provider, record, acceptance)


def guard(provider, head, record, acceptance):
    return _core.guard(_DOCKER, provider, head, record, acceptance)


def publish(provider, version, workflow_id, now):
    return _core.publish(_DOCKER, provider, version, workflow_id, now)


def main(argv=None):
    return _core.main(_DOCKER, WORKFLOW_ID, Provider, prepare, publish, argv)


if __name__ == "__main__":
    sys.exit(main())
