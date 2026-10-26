"""Fixed publisher-format extraction, not a general tar implementation."""
import base64
import gzip
import hashlib
import json
import os
import stat
import sys
import zlib


def extract(archive, directory, members, check=lambda: None):
    check()
    directory_fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        info = os.fstat(directory_fd)
        if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700 or os.listdir(directory_fd):
            raise ValueError("directory")
        with gzip.open(archive, "rb") as stream:
            for member in members:
                check()
                # Exact Go-generated canonical USTAR header comparison includes
                # checksum, type, names, metadata, magic/version and all padding.
                if stream.read(512) != base64.b64decode(member["header"], validate=True):
                    raise ValueError("header")
                name = member["name"]
                if not name or name in (".", "..") or "/" in name or "\\" in name:
                    raise ValueError("name")
                output = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=directory_fd)
                try:
                    digest = hashlib.sha256()
                    remaining = member["size"]
                    while remaining:
                        check()
                        chunk = stream.read(min(65536, remaining))
                        if not chunk:
                            raise ValueError("truncated")
                        digest.update(chunk)
                        remaining -= len(chunk)
                        view = memoryview(chunk)
                        while view:
                            check()
                            written = os.write(output, view)
                            if written <= 0:
                                raise ValueError("write")
                            view = view[written:]
                    if digest.hexdigest() != member["sha256"]:
                        raise ValueError("digest")
                    padding = (-member["size"]) % 512
                    if stream.read(padding) != bytes(padding):
                        raise ValueError("padding")
                    os.fchmod(output, member["mode"])
                    os.fsync(output)
                finally:
                    os.close(output)
            if stream.read(1024) != bytes(1024) or stream.read(1):
                raise ValueError("trailing")
        check()
        os.fsync(directory_fd)
    finally:
        os.close(directory_fd)
