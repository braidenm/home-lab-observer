#!/usr/bin/env python3
"""Owner-operated exact-byte publication; never execute candidate payloads."""

import argparse
import contextlib
import datetime as dt
import io
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

sys.dont_write_bytecode = True
import connected_candidate as candidate
import connected_promotion_policy as policy

# Registration is a separate owner operation. There is deliberately no CLI override.
WORKFLOW_ID = 397507617
REPO = candidate.REPOSITORY
PREFIX = "/repos/" + REPO
INVALID = "connected_promotion_failed"
JSON_LIMIT = 1024 * 1024
ROOT = Path(__file__).resolve().parent.parent


def require(condition):
    if not condition:
        raise ValueError(INVALID)


def environment(source):
    """Use the owner's stored gh credential, never ambient token/debug overrides."""
    result = {key: value for key, value in source.items()
              if not key.upper().startswith(("GH_", "GITHUB_", "GIT_"))
              and key.upper() not in ("PAGER", "LESS", "BROWSER", "SSH_ASKPASS")}
    result.update(GH_HOST="github.com", GH_PROMPT_DISABLED="1", GH_PAGER="cat",
                  GIT_TERMINAL_PROMPT="0", NO_COLOR="1")
    return result


def bounded_process(argv, cwd, env, limit=JSON_LIMIT, timeout=120):
    """Drain only bounded stdout; discard private diagnostics and join on all exits."""
    process = subprocess.Popen(argv, cwd=cwd, env=env, shell=False,
                               stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                               stderr=subprocess.DEVNULL)
    chunks, length, overflow = [], [0], threading.Event()

    def read():
        try:
            while True:
                data = process.stdout.read(min(65536, limit + 1 - length[0]))
                if not data:
                    return
                chunks.append(data)
                length[0] += len(data)
                if length[0] > limit:
                    overflow.set()
                    process.kill()
                    return
        except OSError:
            overflow.set()

    reader = threading.Thread(target=read, daemon=True)
    reader.start()
    try:
        code = process.wait(timeout=timeout)
    except BaseException:
        process.kill()
        process.wait(timeout=10)
        raise
    finally:
        reader.join(timeout=10)
        process.stdout.close()
    require(not reader.is_alive() and not overflow.is_set())
    return code, b"".join(chunks)


def decode_response(data, missing=False, conflict=False):
    require(isinstance(data, bytes) and len(data) <= JSON_LIMIT)
    # gh --include can include intermediate redirect responses. JSON API methods
    # here do not redirect; refuse rather than accidentally parse another body.
    parts = data.split(b"\r\n\r\n", 1) if b"\r\n\r\n" in data else data.split(b"\n\n", 1)
    require(len(parts) == 2)
    headers, body = parts
    first = headers.splitlines()[0]
    match = re.fullmatch(rb"HTTP/[^ ]+ ([0-9]{3})(?: .*)?", first)
    require(match is not None)
    status = int(match.group(1))
    if status == 404 and missing:
        return None
    if status == 422 and conflict:
        return CONFLICT
    require(200 <= status < 300)
    return bounded_json(body)


def bounded_json(data):
    require(isinstance(data, bytes) and 0 < len(data) <= JSON_LIMIT)

    def unique(pairs):
        value = {}
        for key, item in pairs:
            require(key not in value)
            value[key] = item
        return value
    return json.loads(data, object_pairs_hook=unique,
                      parse_constant=lambda _: require(False))


CONFLICT = object()


class Provider:
    def __init__(self, root=ROOT, run=bounded_process, source_environment=None):
        self.root = Path(root)
        self.env = environment(os.environ if source_environment is None else source_environment)
        self.run = run
        self.gh = shutil.which("gh", path=self.env.get("PATH"))
        self.git = shutil.which("git", path=self.env.get("PATH"))
        require(self.gh is not None and self.git is not None)
        self.gh, self.git = str(Path(self.gh).resolve()), str(Path(self.git).resolve())

    def git_bytes(self, *args, limit=JSON_LIMIT):
        code, output = self.run([self.git, "--no-replace-objects", "--no-pager", "-c", "core.fsmonitor=false",
                                 "-c", "core.untrackedCache=false", "-C", str(self.root), *args],
                                self.root, self.env, limit=limit, timeout=30)
        require(code == 0)
        return output

    def json(self, endpoint, method="GET", data=None, missing=False, conflict=False):
        require(endpoint.startswith(PREFIX + "/") or endpoint in (PREFIX, "/user"))
        args = [self.gh, "api", "--hostname", "github.com", "--method", method,
                "--include", "-H", "Accept: application/vnd.github+json",
                "-H", "X-GitHub-Api-Version: 2026-03-10", endpoint]
        with contextlib.ExitStack() as stack:
            if data is not None:
                file = stack.enter_context(tempfile.NamedTemporaryFile(mode="wb", delete=False))
                name = Path(file.name)
                stack.callback(lambda: name.unlink(missing_ok=True))
                file.write(json.dumps(data, separators=(",", ":")).encode("ascii"))
                file.close()  # Windows gh opens the path itself.
                args += ["--input", str(name)]
            code, output = self.run(args, self.root, self.env, limit=JSON_LIMIT, timeout=120)
        value = decode_response(output, missing, conflict)
        require(code == 0 or value is None or value is CONFLICT)
        return value

    def binary(self, endpoint, limit):
        require(re.fullmatch(re.escape(PREFIX) + r"/(?:actions/artifacts/[1-9][0-9]*/zip|releases/assets/[1-9][0-9]*)", endpoint))
        code, value = self.run([self.gh, "api", "--hostname", "github.com", "--method", "GET",
                               "-H", "Accept: application/octet-stream", endpoint],
                              self.root, self.env, limit=limit, timeout=120)
        require(code == 0 and 0 < len(value) <= limit)
        return value

    def attest(self, path, source, predicate_type):
        code, value = self.run([self.gh, "attestation", "verify", str(path), "--repo", REPO,
                               "--hostname", "github.com", "--signer-workflow",
                               "github.com/" + REPO + "/" + candidate.WORKFLOW_PATH,
                               "--cert-identity", policy.WORKFLOW_URI,
                               "--source-digest", source, "--signer-digest", source,
                               "--source-ref", "refs/heads/main",
                               "--deny-self-hosted-runners", "--predicate-type",
                               predicate_type, "--format", "json"],
                              self.root, self.env, limit=JSON_LIMIT, timeout=120)
        require(code == 0)
        return bounded_json(value)

    def upload(self, version, path):
        # No --clobber: a race can only fail and be revalidated by the caller.
        code, _ = self.run([self.gh, "release", "upload", "v" + version, str(path),
                            "--repo", REPO], self.root, self.env, limit=4096, timeout=120)
        return code == 0

    def anonymous(self, version, name, expected):
        url = f"https://github.com/{REPO}/releases/download/v{version}/{name}"
        return anonymous_download(url, expected["size"])


class ReleaseRedirect(urllib.request.HTTPRedirectHandler):
    def __init__(self):
        super().__init__()
        self.count = 0

    def redirect_request(self, request, fp, code, msg, headers, newurl):
        self.count += 1
        require(self.count <= 5)
        validate_download_url(newurl)
        return super().redirect_request(request, fp, code, msg, headers, newurl)


def validate_download_url(url):
    value = urllib.parse.urlsplit(url)
    require(len(url) <= 16384 and value.scheme == "https" and value.port in (None, 443)
            and value.hostname in ("github.com", "release-assets.githubusercontent.com")
            and not value.username and not value.password and not value.fragment)


def anonymous_download(url, limit):
    validate_download_url(url)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), ReleaseRedirect())
    deadline = time.monotonic() + 120
    request = urllib.request.Request(url, headers={"User-Agent": "home-lab-observer-release-verifier"})
    with opener.open(request, timeout=30) as response:
        require(response.status == 200)
        pieces, size = [], 0
        while True:
            require(time.monotonic() < deadline)
            piece = response.read(min(65536, limit + 1 - size))
            if not piece:
                return b"".join(pieces)
            pieces.append(piece)
            size += len(piece)
            require(size <= limit)


def trusted_main(provider):
    grafts = Path(provider.git_bytes("rev-parse", "--path-format=absolute", "--git-path",
                                    "info/grafts").decode("utf-8").strip())
    require(grafts.is_absolute() and not os.path.lexists(grafts))
    require(provider.git_bytes("status", "--porcelain=v1", "--untracked-files=all") == b"")
    head = provider.git_bytes("rev-parse", "--verify", "HEAD").decode("ascii").strip()
    require(re.fullmatch(r"[a-f0-9]{40}", head))
    owner = provider.json("/user")
    require(owner["id"] == candidate.OWNER_ID and owner["login"] == "braidenm")
    repo = provider.json(PREFIX)
    require(repo["id"] == candidate.REPOSITORY_ID and repo["owner"]["id"] == candidate.OWNER_ID
            and repo["full_name"] == REPO and repo["default_branch"] == "main"
            and repo["visibility"] == "public")
    ref = provider.json(PREFIX + "/git/ref/heads/main")
    require(ref["object"]["type"] == "commit" and ref["object"]["sha"] == head)
    return head


def ensure_same_main(provider, head):
    require(trusted_main(provider) == head)


def read_metadata(packed):
    require(0 < len(packed) <= policy.MAX_ARTIFACT_BYTES)
    with zipfile.ZipFile(io.BytesIO(packed)) as archive:
        entries = [item for item in archive.infolist() if item.filename == "candidate.json"]
        require(len(entries) == 1 and 0 < entries[0].file_size <= candidate.MAX_METADATA_BYTES)
        with archive.open(entries[0]) as stream:
            return candidate.decode_metadata(stream.read(candidate.MAX_METADATA_BYTES + 1))


def artifact_record(info, metadata, metadata_bytes):
    require(info["expired"] is False and isinstance(info["digest"], str)
            and info["digest"].startswith("sha256:"))
    return {"candidate": metadata, "candidate_sha256": policy.digest(metadata_bytes),
            "artifact": {"id": info["id"], "name": info["name"],
                         "sha256": policy.sha(info["digest"][7:]),
                         "size": policy.number(info["size_in_bytes"]),
                         "expires_at": info["expires_at"]}}


def verify_candidate(provider, record, packed, workflow_id, directory, now):
    metadata = candidate.validate_metadata(record["candidate"], workflow_id)
    require(policy.utc(record["artifact"]["expires_at"]) > now)
    workflow = provider.json(PREFIX + "/actions/workflows/" + str(workflow_id))
    # Latest run, not just /attempts/N: a rerun invalidates old attempt evidence.
    run = provider.json(PREFIX + "/actions/runs/" + str(metadata["run_id"]))
    policy.validate_run(run, workflow, record)
    info = provider.json(PREFIX + "/actions/artifacts/" + str(record["artifact"]["id"]))
    policy.validate_artifact(info, record)
    payload = policy.unpack_candidate(packed, record)
    candidate.validate_payload_bytes(payload, metadata)
    directory.mkdir(mode=0o700)
    for name, content in payload.items():
        path = directory / name
        with path.open("xb") as stream:
            stream.write(content)
        path.chmod(0o600)
        for predicate_type in ("https://slsa.dev/provenance/v1", "https://spdx.dev/Document"):
            policy.validate_provenance(provider.attest(path, metadata["source"], predicate_type),
                                       metadata, name, policy.digest(content), predicate_type)
    return payload


def prepare(provider, run_id, artifact_id, output, workflow_id, now):
    require(type(workflow_id) is int and workflow_id > 0)
    policy.number(run_id)
    policy.number(artifact_id)
    head = trusted_main(provider)
    info = provider.json(PREFIX + "/actions/artifacts/" + str(artifact_id))
    require(type(info["id"]) is int and info["id"] == artifact_id
            and type(info["size_in_bytes"]) is int and 0 < info["size_in_bytes"] <= policy.MAX_ARTIFACT_BYTES)
    packed = provider.binary(PREFIX + f"/actions/artifacts/{artifact_id}/zip", info["size_in_bytes"])
    metadata = read_metadata(packed)
    require(metadata["run_id"] == run_id and metadata["source"] == head)
    record = artifact_record(info, metadata, candidate.encode_metadata(metadata))
    output.mkdir(mode=0o700)  # Exclusive new directory; existing paths are never adopted.
    verify_candidate(provider, record, packed, workflow_id, output / "payload", now)
    ensure_same_main(provider, head)
    with (output / "candidate-identity.json").open("xb") as stream:
        stream.write((json.dumps(record, separators=(",", ":")) + "\n").encode("ascii"))
    return record


@contextlib.contextmanager
def version_lock(provider, version):
    candidate.asset_names(version)
    directory = Path(provider.git_bytes("rev-parse", "--path-format=absolute", "--git-common-dir").decode("utf-8").strip())
    require(directory.is_absolute() and directory.is_dir())
    path = directory / ("hlo-connected-" + version + ".lock")
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    identity = os.fstat(descriptor)
    try:
        yield
    finally:
        os.close(descriptor)
        current = path.lstat()
        require(stat.S_ISREG(current.st_mode) and (current.st_dev, current.st_ino) ==
                (identity.st_dev, identity.st_ino))
        path.unlink()


def release_snapshot(provider, record, acceptance):
    version = record["candidate"]["version"]
    tag = provider.json(PREFIX + "/git/ref/tags/v" + version, missing=True)
    # Fail closed if a complete single page cannot prove absence/uniqueness.
    releases = provider.json(PREFIX + "/releases?per_page=100&page=1")
    require(isinstance(releases, list) and len(releases) < 100)
    matching = [item for item in releases if item["tag_name"] == "v" + version]
    require(len(matching) <= 1)
    release = None
    if matching:
        ident = policy.number(matching[0]["id"])
        release = provider.json(PREFIX + "/releases/" + str(ident))
        require(release["id"] == ident)
    state, missing = policy.release_state(tag, release, record, acceptance)
    return tag, release, state, missing


def verify_assets(provider, release, record, anonymous=False):
    expected = {item["name"]: item for item in record["candidate"]["assets"]}
    for asset in release["assets"]:
        item = expected[asset["name"]]
        data = (provider.anonymous(record["candidate"]["version"], asset["name"], item) if anonymous else
                provider.binary(PREFIX + "/releases/assets/" + str(asset["id"]), item["size"]))
        require(len(data) == item["size"] and policy.digest(data) == item["sha256"])


def release_authority(release):
    """Provider download counters/timestamps are not publication authority."""
    if release is None:
        return None
    fields = ("id", "tag_name", "target_commitish", "name", "body", "draft", "prerelease")
    authority = {key: release[key] for key in fields}
    authority["immutable"] = release.get("immutable", False)
    asset_fields = ("id", "name", "state", "size", "digest")
    authority["assets"] = sorted(
        ({key: asset[key] for key in asset_fields} for asset in release["assets"]),
        key=lambda asset: asset["name"])
    return authority


def guard(provider, head, record, acceptance):
    ensure_same_main(provider, head)
    require(provider.json(PREFIX + "/immutable-releases")["enabled"] is True)
    c = record["candidate"]
    policy.validate_run(provider.json(PREFIX + "/actions/runs/" + str(c["run_id"])),
                        provider.json(PREFIX + "/actions/workflows/" + str(c["workflow_id"])), record)
    policy.validate_artifact(provider.json(PREFIX + "/actions/artifacts/" +
                                          str(record["artifact"]["id"])), record)
    require(policy.utc(record["artifact"]["expires_at"]) > dt.datetime.now(dt.timezone.utc))
    return release_snapshot(provider, record, acceptance)


def publish(provider, version, workflow_id, now):
    require(type(workflow_id) is int and workflow_id > 0)
    candidate.asset_names(version)
    head = trusted_main(provider)
    path = f"docs/acceptance/connected-linux/{version}.json"
    acceptance = provider.git_bytes("show", "HEAD:" + path, limit=policy.MAX_JSON_BYTES)
    record = policy.validate_acceptance(acceptance, workflow_id, now)
    require(record["candidate"]["version"] == version)
    source = record["candidate"]["source"]
    provider.git_bytes("merge-base", "--is-ancestor", source, "HEAD")
    policy.validate_evidence_diff(provider.git_bytes("diff", "--raw", "--no-abbrev", "--no-renames",
        "--no-ext-diff", "--no-textconv", "-z", source, "HEAD", "--"), record)
    packed = provider.binary(PREFIX + f"/actions/artifacts/{record['artifact']['id']}/zip", record["artifact"]["size"])
    with tempfile.TemporaryDirectory(prefix="hlo-publish-") as directory, version_lock(provider, version):
        staging = Path(directory) / "payload"
        verify_candidate(provider, record, packed, workflow_id, staging, now)
        for _ in range(12):  # Four uploads plus tag/draft/publication, bounded race recovery.
            _, release, state, missing = guard(provider, head, record, acceptance)
            if state == "VERIFY_PUBLISHED":
                verify_assets(provider, release, record, anonymous=True)
                ensure_same_main(provider, head)
                return release["id"]
            if release is not None:
                verify_assets(provider, release, record)
            # Re-observe every binding/asset identity after draft-byte downloads.
            before = guard(provider, head, record, acceptance)
            require(release_authority(before[1]) == release_authority(release)
                    and before[2] == state and before[3] == missing)
            if state == "CREATE_TAG":
                provider.json(PREFIX + "/git/refs", "POST",
                    {"ref": "refs/tags/v" + version, "sha": source}, conflict=True)
            elif state == "CREATE_DRAFT":
                provider.json(PREFIX + "/releases", "POST",
                    {"tag_name": "v" + version, "target_commitish": source,
                     "name": "Home Lab Observer " + version,
                     "body": policy.release_body(record, acceptance), "draft": True,
                     "prerelease": True, "make_latest": "false"}, conflict=True)
            elif state == "UPLOAD_MISSING":
                # Never adopt an upload response as authority; next loop reads IDs/digests/bytes.
                provider.upload(version, staging / missing[0])
            elif state == "PUBLISH":
                provider.json(PREFIX + "/releases/" + str(release["id"]), "PATCH",
                    {"draft": False, "prerelease": True, "make_latest": "false"})
            else:
                require(False)
        require(False)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--prepare", action="store_true")
    mode.add_argument("--publish", action="store_true")
    parser.add_argument("--version")
    parser.add_argument("--run-id", type=int)
    parser.add_argument("--artifact-id", type=int)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args(argv)
    try:
        require(type(WORKFLOW_ID) is int and WORKFLOW_ID > 0)
        provider = Provider()
        now = dt.datetime.now(dt.timezone.utc)
        if args.prepare:
            require(args.version is None and args.run_id and args.artifact_id and args.output is not None)
            record = prepare(provider, args.run_id, args.artifact_id, args.output, WORKFLOW_ID, now)
            print(f"CONNECTED_CANDIDATE_VERIFIED run={record['candidate']['run_id']} attempt={record['candidate']['run_attempt']} artifact={record['artifact']['id']}")
        else:
            require(args.version is not None and args.run_id is None and args.artifact_id is None and args.output is None)
            ident = publish(provider, args.version, WORKFLOW_ID, now)
            print(f"CONNECTED_RELEASE_VERIFIED version={args.version} release={ident}")
        return 0
    except (ValueError, OSError, KeyError, TypeError, AttributeError, IndexError,
            RecursionError, RuntimeError, NotImplementedError, UnicodeError, zipfile.BadZipFile,
            subprocess.SubprocessError, urllib.error.URLError):
        print("CONNECTED_PROMOTION_FAILED", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
