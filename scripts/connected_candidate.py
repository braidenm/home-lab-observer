#!/usr/bin/env python3
"""Closed numeric candidate interface; compatibility/default wire is unchanged."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys
sys.dont_write_bytecode = True
# Isolated Python excludes the script directory. Import only this source-owned
# sibling directory, never the caller's working directory or PYTHONPATH.
sys.path.insert(0, str(Path(__file__).resolve().parent))
import _connected_candidate_core as _core
from _connected_candidate_core import (REPOSITORY, REPOSITORY_ID, OWNER_ID, MAX_PAYLOAD_BYTES,
    MAX_METADATA_BYTES, MAX_SAFE_INTEGER, INVALID, FIELDS, VERSION, SHA256, SHA1,
    _require, _integer, _pairs, _json, _read_at, _gh_json)

WORKFLOW_PATH = _core._NUMERIC.workflow_path
SCHEMA_VERSION = _core._NUMERIC.schema

def asset_names(version):
    return _core.asset_names(_core._NUMERIC, version)


def artifact_name(metadata):
    return _core.artifact_name(_core._NUMERIC, metadata)


def encode_metadata(metadata):
    return _core.encode_metadata(_core._NUMERIC, metadata)


def validate_metadata(metadata, expected_workflow_id=None):
    return _core.validate_metadata(_core._NUMERIC, metadata, expected_workflow_id)


def decode_metadata(data, expected_workflow_id=None):
    return _core.decode_metadata(_core._NUMERIC, data, expected_workflow_id)


def validate_payload_bytes(contents, metadata):
    return _core.validate_payload_bytes(_core._NUMERIC, contents, metadata)


def validate_payload(directory, metadata):
    return _core.validate_payload(_core._NUMERIC, directory, metadata)


def authorize(version, environment, get=_gh_json):
    return _core.authorize(_core._NUMERIC, version, environment, get)


def _authorize(version, environment, get):
    return _core._authorize(_core._NUMERIC, version, environment, get)


def validate_sbom(data, metadata, manifest_data, required_modules):
    return _core.validate_sbom(_core._NUMERIC, data, metadata, manifest_data, required_modules)


def create_metadata(directory, version, source, workflow_id, run_id, run_attempt, contract_sha256):
    return _core.create_metadata(_core._NUMERIC, directory, version, source, workflow_id, run_id, run_attempt, contract_sha256)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    auth = commands.add_parser("authorize")
    auth.add_argument("--version", required=True)
    create = commands.add_parser("create")
    create.add_argument("--directory", required=True)
    create.add_argument("--version", required=True)
    create.add_argument("--source", required=True)
    for key in ("workflow-id", "run-id", "run-attempt"):
        create.add_argument("--" + key, type=int, required=True)
    verify = commands.add_parser("verify")
    verify.add_argument("--directory", required=True)
    sbom = commands.add_parser("verify-sbom")
    sbom.add_argument("--directory", required=True)
    sbom.add_argument("--sbom", required=True)
    args = parser.parse_args()
    try:
        if args.command == "authorize":
            values = authorize(args.version, os.environ)
            with open(os.environ["GITHUB_OUTPUT"], "a", encoding="ascii", newline="\n") as output:
                for key, value in values.items():
                    output.write(f"{key}={value}\n")
        elif args.command == "create":
            contract = hashlib.sha256((Path(__file__).resolve().parent.parent / "internal/connectedcompat/descriptor.json").read_bytes()).hexdigest()
            create_metadata(args.directory, args.version, args.source, args.workflow_id, args.run_id, args.run_attempt, contract)
        else:
            metadata = decode_metadata((Path(args.directory) / "candidate.json").read_bytes())
            validate_payload(args.directory, metadata)
            if args.command == "verify-sbom":
                module_file = (Path(__file__).resolve().parent.parent / "go.mod").read_text(encoding="ascii")
                required = {}
                for name in ("github.com/shirou/gopsutil/v4", "golang.org/x/sys", "modernc.org/sqlite"):
                    matches = re.findall(r"^\s*" + re.escape(name) + r" (v[^\s]+)(?: // indirect)?$", module_file, re.MULTILINE)
                    _require(len(matches) == 1)
                    required[name] = matches[0]
                sbom_path = Path(args.sbom)
                _require(sbom_path.is_file() and 0 < sbom_path.stat().st_size <= 16 * 1024 * 1024)
                validate_sbom(sbom_path.read_bytes(), metadata,
                              (Path(args.directory) / "connected-manifest.json").read_bytes(), required)
        print("CONNECTED_CANDIDATE_OK")
        return 0
    except (ValueError, OSError, KeyError, TypeError):
        print("CONNECTED_CANDIDATE_FAILED", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
