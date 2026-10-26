#!/usr/bin/env python3
"""Closed connected-beta candidate metadata; never execute or publish its assets."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys

REPOSITORY = "braidenm/home-lab-observer"
REPOSITORY_ID = 1362278980
OWNER_ID = 44215293
WORKFLOW_PATH = ".github/workflows/connected-linux-candidate.yml"
SCHEMA_VERSION = "observer-connected-candidate/v1"
MAX_PAYLOAD_BYTES = 64 * 1024 * 1024
MAX_METADATA_BYTES = 4096
MAX_SAFE_INTEGER = 9007199254740991
INVALID = "connected_candidate_invalid"
FIELDS = ("schema_version", "repository_id", "owner_id", "workflow_path", "workflow_id", "ref",
          "run_id", "run_attempt", "source", "version", "contract_sha256", "assets")
VERSION = re.compile(r"0\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})-canary\.([1-9][0-9]{0,3})")
SHA256 = re.compile(r"[a-f0-9]{64}")
SHA1 = re.compile(r"[a-f0-9]{40}")


def _require(condition):
    if not condition:
        raise ValueError(INVALID)


def _integer(value):
    return type(value) is int and 0 < value <= MAX_SAFE_INTEGER


def asset_names(version):
    _require(type(version) is str and VERSION.fullmatch(version) is not None and len(version) <= 40)
    return [f"home-lab-observer-connected_{version}_linux_amd64.tar.gz", "connected-manifest.json",
            "install-connected-linux.sh", "SHA256SUMS"]


def artifact_name(metadata):
    validate_metadata(metadata)
    return f"connected-linux-{metadata['version']}-{metadata['run_id']}-{metadata['run_attempt']}"


def encode_metadata(metadata):
    validate_metadata(metadata)
    return (json.dumps(metadata, ensure_ascii=True, separators=(",", ":")) + "\n").encode("ascii")


def validate_metadata(metadata, expected_workflow_id=None):
    _require(type(metadata) is dict and tuple(metadata) == FIELDS)
    _require(metadata["schema_version"] == SCHEMA_VERSION)
    _require(type(metadata["repository_id"]) is int and metadata["repository_id"] == REPOSITORY_ID)
    _require(type(metadata["owner_id"]) is int and metadata["owner_id"] == OWNER_ID)
    _require(metadata["workflow_path"] == WORKFLOW_PATH and metadata["ref"] == "refs/heads/main")
    for field in ("workflow_id", "run_id", "run_attempt"):
        _require(_integer(metadata[field]))
    if expected_workflow_id is not None:
        _require(_integer(expected_workflow_id) and metadata["workflow_id"] == expected_workflow_id)
    _require(type(metadata["source"]) is str and SHA1.fullmatch(metadata["source"]) is not None)
    _require(type(metadata["contract_sha256"]) is str and SHA256.fullmatch(metadata["contract_sha256"]) is not None)
    names = asset_names(metadata["version"])
    _require(type(metadata["assets"]) is list and len(metadata["assets"]) == 4)
    total = 0
    for name, item in zip(names, metadata["assets"]):
        _require(type(item) is dict and tuple(item) == ("name", "size", "sha256"))
        _require(item["name"] == name and _integer(item["size"]))
        limit = MAX_PAYLOAD_BYTES if name == names[0] else (16384 if name == names[1] else 65536)
        _require(item["size"] <= limit)
        _require(type(item["sha256"]) is str and SHA256.fullmatch(item["sha256"]) is not None)
        total += item["size"]
    _require(total + MAX_METADATA_BYTES <= MAX_PAYLOAD_BYTES)
    return metadata


def _pairs(pairs):
    result = {}
    for key, value in pairs:
        _require(key not in result)
        result[key] = value
    return result


def _json(data):
    try:
        return json.loads(data, object_pairs_hook=_pairs,
                          parse_constant=lambda _: (_ for _ in ()).throw(ValueError(INVALID)))
    except (UnicodeError, json.JSONDecodeError, RecursionError) as error:
        raise ValueError(INVALID) from error


def decode_metadata(data, expected_workflow_id=None):
    _require(type(data) is bytes and 0 < len(data) <= MAX_METADATA_BYTES)
    metadata = validate_metadata(_json(data), expected_workflow_id)
    _require(encode_metadata(metadata) == data)
    return metadata


def _read_at(directory_fd, name, limit):
    descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=directory_fd)
    try:
        before = os.fstat(descriptor)
        _require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and 0 < before.st_size <= limit)
        _require(stat.S_IMODE(before.st_mode) == 0o644)
        with os.fdopen(os.dup(descriptor), "rb") as stream:
            data = stream.read(limit + 1)
        after = os.fstat(descriptor)
        current = os.stat(name, dir_fd=directory_fd, follow_symlinks=False)
        def identity(info):
            return (info.st_dev, info.st_ino, info.st_mode, info.st_nlink, info.st_uid, info.st_gid,
                    info.st_size, info.st_mtime_ns, info.st_ctime_ns)
        _require(len(data) == before.st_size and identity(before) == identity(after) == identity(current))
        return data
    finally:
        os.close(descriptor)


def validate_payload_bytes(contents, metadata):
    """Verify bounded immutable bytes; filesystem/archive admission is the caller's job."""
    validate_metadata(metadata)
    names = asset_names(metadata["version"])
    _require(type(contents) is dict and set(contents) == set(names + ["candidate.json"]))
    _require(type(contents["candidate.json"]) is bytes and contents["candidate.json"] == encode_metadata(metadata))
    for item in metadata["assets"]:
        content = contents[item["name"]]
        _require(type(content) is bytes and len(content) == item["size"])
        _require(hashlib.sha256(content).hexdigest() == item["sha256"])
    expected = "".join(f"{item['sha256']}  {item['name']}\n" for item in metadata["assets"][:3]).encode("ascii")
    _require(contents["SHA256SUMS"] == expected)
    manifest = _json(contents["connected-manifest.json"])
    _require(type(manifest) is dict and manifest.get("schema") == "observer-connected-bundle/v2")
    for field, value in (("version", metadata["version"]), ("commit", metadata["source"]),
                         ("contract_sha256", metadata["contract_sha256"]), ("os", "linux"),
                         ("arch", "amd64"), ("profile", "ubuntu24.04-systemd255-amd64-canary")):
        _require(manifest.get(field) == value)


def validate_payload(directory, metadata):
    """Read five closed regular files through Linux descriptors, then verify bytes."""
    validate_metadata(metadata)
    descriptor = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        names = asset_names(metadata["version"])
        _require(sorted(os.listdir(descriptor)) == sorted(names + ["candidate.json"]))
        contents = {"candidate.json": _read_at(descriptor, "candidate.json", MAX_METADATA_BYTES)}
        for item in metadata["assets"]:
            contents[item["name"]] = _read_at(descriptor, item["name"], item["size"])
        validate_payload_bytes(contents, metadata)
        _require(sorted(os.listdir(descriptor)) == sorted(names + ["candidate.json"]))
    except (OSError, TypeError, KeyError) as error:
        raise ValueError(INVALID) from error
    finally:
        os.close(descriptor)


def _gh_json(endpoint, missing=False):
    """Read fixed public provider records with the workflow's ephemeral token."""
    environment = dict(os.environ)
    for key in ("GH_DEBUG", "GH_FORCE_TTY", "GH_HOST", "GH_PAGER", "PAGER"):
        environment.pop(key, None)
    environment["GH_PROMPT_DISABLED"] = "1"
    try:
        completed = subprocess.run(
            ["gh", "api", "--hostname", "github.com", "--include", "-H", "Accept: application/vnd.github+json",
             "-H", "X-GitHub-Api-Version: 2026-03-10", endpoint],
            env=environment, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=30, check=False)
    except (OSError, subprocess.SubprocessError) as error:
        raise ValueError(INVALID) from error
    _require(0 < len(completed.stdout) <= 1024 * 1024)
    status = re.match(rb"HTTP/[0-9.]+ ([0-9]{3})[^\r\n]*\r?\n", completed.stdout)
    _require(status is not None)
    if missing and status[1] == b"404" and completed.returncode != 0:
        return None
    _require(completed.returncode == 0 and status[1] == b"200")
    separator = b"\r\n\r\n" if b"\r\n\r\n" in completed.stdout else b"\n\n"
    _require(separator in completed.stdout)
    return _json(completed.stdout.split(separator, 1)[1])


def authorize(version, environment, get=_gh_json):
    try:
        return _authorize(version, environment, get)
    except (AttributeError, TypeError, KeyError, OverflowError) as error:
        raise ValueError(INVALID) from error


def _authorize(version, environment, get):
    """Bind an owner dispatch to the provider's actual current main and attempt."""
    asset_names(version)
    _require(environment.get("GITHUB_REPOSITORY") == REPOSITORY)
    _require(environment.get("GITHUB_REPOSITORY_ID") == str(REPOSITORY_ID))
    _require(environment.get("GITHUB_REPOSITORY_OWNER_ID") == str(OWNER_ID))
    _require(environment.get("GITHUB_ACTOR") == "braidenm" and environment.get("GITHUB_TRIGGERING_ACTOR") == "braidenm")
    _require(environment.get("GITHUB_ACTOR_ID") == str(OWNER_ID))
    _require(environment.get("GITHUB_EVENT_NAME") == "workflow_dispatch" and environment.get("GITHUB_REF") == "refs/heads/main")
    _require(environment.get("GITHUB_WORKFLOW_REF") == f"{REPOSITORY}/{WORKFLOW_PATH}@refs/heads/main")
    source = environment.get("GITHUB_SHA", "")
    _require(SHA1.fullmatch(source) is not None)
    numeric = {}
    for name in ("GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT"):
        value = environment.get(name, "")
        _require(re.fullmatch(r"[1-9][0-9]{0,15}", value) is not None and _integer(int(value)))
        numeric[name] = int(value)
    repository = get(f"/repos/{REPOSITORY}")
    _require(repository.get("id") == REPOSITORY_ID and repository.get("owner", {}).get("id") == OWNER_ID)
    _require(repository.get("full_name") == REPOSITORY and repository.get("default_branch") == "main" and repository.get("visibility") == "public")
    branch = get(f"/repos/{REPOSITORY}/git/ref/heads/main")
    _require(branch.get("object") == {"type": "commit", "sha": source, "url": f"https://api.github.com/repos/{REPOSITORY}/git/commits/{source}"})
    workflow = get(f"/repos/{REPOSITORY}/actions/workflows/connected-linux-candidate.yml")
    _require(_integer(workflow.get("id")) and workflow.get("path") == WORKFLOW_PATH and workflow.get("state") == "active")
    run = get(f"/repos/{REPOSITORY}/actions/runs/{numeric['GITHUB_RUN_ID']}/attempts/{numeric['GITHUB_RUN_ATTEMPT']}")
    _require(run.get("id") == numeric["GITHUB_RUN_ID"] and run.get("run_attempt") == numeric["GITHUB_RUN_ATTEMPT"])
    _require(run.get("workflow_id") == workflow["id"] and run.get("path") == WORKFLOW_PATH)
    _require(run.get("head_sha") == source and run.get("head_branch") == "main" and run.get("event") == "workflow_dispatch")
    for actor in ("actor", "triggering_actor"):
        _require(run.get(actor, {}).get("id") == OWNER_ID and run.get(actor, {}).get("login") == "braidenm")
    for repo in ("repository", "head_repository"):
        _require(run.get(repo, {}).get("id") == REPOSITORY_ID and run.get(repo, {}).get("owner", {}).get("id") == OWNER_ID)
    _require(get(f"/repos/{REPOSITORY}/git/ref/tags/v{version}", missing=True) is None)
    _require(get(f"/repos/{REPOSITORY}/releases/tags/v{version}", missing=True) is None)
    return {"version": version, "source": source, "workflow_id": workflow["id"],
            "run_id": numeric["GITHUB_RUN_ID"], "run_attempt": numeric["GITHUB_RUN_ATTEMPT"]}


def validate_sbom(data, metadata, manifest_data, required_modules):
    """Refuse an empty/outer-only scan; bind all three ELF hashes to the manifest."""
    validate_metadata(metadata)
    _require(type(data) is bytes and 0 < len(data) <= 16 * 1024 * 1024)
    try:
        sbom, manifest = _json(data), _json(manifest_data)
        _require(type(sbom) is dict and sbom.get("spdxVersion") == "SPDX-2.3")
        files, packages = sbom.get("files"), sbom.get("packages")
        _require(type(files) is list and len(files) == 3 and type(packages) is list and 3 <= len(packages) <= 1024)
        roles = {"observer-connected-" + role for role in ("collector", "uploader", "install")}
        expected = {entry["name"]: entry["sha256"] for entry in manifest["files"] if entry["name"] in roles}
        _require(set(expected) == roles and {entry.get("fileName") for entry in files} == roles)
        def sha256_checksums(item):
            checksums = item.get("checksums")
            _require(type(checksums) is list)
            return [entry["checksumValue"] for entry in checksums if entry.get("algorithm") == "SHA256"]
        for entry in files:
            _require(sha256_checksums(entry) == [expected[entry["fileName"]]])
        archive = metadata["assets"][0]
        root = [entry for entry in packages if entry.get("name") == archive["name"]]
        _require(len(root) == 1 and root[0].get("versionInfo") == "sha256:" + archive["sha256"])
        _require(sha256_checksums(root[0]) == [archive["sha256"]])
        identities = [(entry.get("name"), entry.get("versionInfo")) for entry in packages]
        _require(identities.count(("stdlib", "go1.27.3")) == 3)
        _require(sum(name == "github.com/braidenm/home-lab-observer" for name, _ in identities) == 3)
        _require(type(required_modules) is dict and len(required_modules) == 3)
        for module, version in required_modules.items():
            _require((module, version) in identities)
    except (AttributeError, TypeError, KeyError) as error:
        raise ValueError(INVALID) from error


def create_metadata(directory, version, source, workflow_id, run_id, run_attempt, contract_sha256):
    names = asset_names(version)
    descriptor = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        _require(sorted(os.listdir(descriptor)) == sorted(names))
        assets = []
        for name in names:
            content = _read_at(descriptor, name, MAX_PAYLOAD_BYTES)
            assets.append({"name": name, "size": len(content), "sha256": hashlib.sha256(content).hexdigest()})
        metadata = dict(zip(FIELDS, (SCHEMA_VERSION, REPOSITORY_ID, OWNER_ID, WORKFLOW_PATH, workflow_id,
                                    "refs/heads/main", run_id, run_attempt, source, version, contract_sha256, assets)))
        content = encode_metadata(metadata)
        output = os.open("candidate.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC,
                         0o644, dir_fd=descriptor)
        with os.fdopen(output, "wb") as stream:
            stream.write(content)
            os.fchmod(stream.fileno(), 0o644)
            stream.flush()
            os.fsync(stream.fileno())
    except (OSError, TypeError, KeyError) as error:
        raise ValueError(INVALID) from error
    finally:
        os.close(descriptor)
    validate_payload(directory, metadata)
    return metadata


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
