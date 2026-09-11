"""Synthetic primitives only: no host observations, connections, or credentials."""
import ctypes
import errno
import os
import socket
import stat
import struct
import sys


def checked(operation, denied):
    try:
        operation()
    except OSError as error:
        if denied and error.errno in (errno.EPERM, errno.EACCES, errno.EAFNOSUPPORT):
            return
        raise
    if denied:
        raise AssertionError("expected denial")


def network_socket(family):
    # Never bind, listen, connect, send or receive.
    with socket.socket(family, socket.SOCK_STREAM):
        pass


def unix_pair():
    first, second = socket.socketpair(socket.AF_UNIX, socket.SOCK_DGRAM)
    first.close()
    second.close()


libc = ctypes.CDLL(None, use_errno=True)
libc.syscall.restype = ctypes.c_long


def shared_memory():
    # Runner gives every invocation a fresh IPC namespace. Even an unexpected
    # cleanup failure cannot leave a host-visible IPC object after service exit.
    identifier = libc.shmget(0, 4096, 0o1000 | 0o600)
    if identifier < 0:
        raise OSError(ctypes.get_errno(), "synthetic shmget failed")
    if libc.shmctl(identifier, 0, None) != 0:
        raise OSError(ctypes.get_errno(), "synthetic shmctl cleanup failed")


def uring():
    # Linux x86_64 __NR_io_uring_setup=425; runner refuses other architectures.
    params = ctypes.create_string_buffer(120)
    descriptor = libc.syscall(425, 2, ctypes.byref(params))
    if descriptor < 0:
        raise OSError(ctypes.get_errno(), "synthetic io_uring setup failed")
    os.close(descriptor)


def credential():
    directory = os.environ["CREDENTIALS_DIRECTORY"]
    if not directory.startswith("/run/credentials/") or os.getuid() == 0:
        raise AssertionError("unexpected credential context")
    for path, mode, permission, is_directory in (
        (directory, 0o550, 5, True),
        (directory + "/connector.json", 0o440, 4, False),
    ):
        metadata = os.lstat(path)
        if metadata.st_uid != 0 or metadata.st_gid != 0 or stat.S_IMODE(metadata.st_mode) != mode:
            raise AssertionError("unexpected credential ownership/mode")
        if is_directory != stat.S_ISDIR(metadata.st_mode):
            raise AssertionError("unexpected credential kind")
        acl = os.getxattr(path, "system.posix_acl_access")
        expected = struct.pack("<I", 2) + b"".join(
            struct.pack("<HHI", tag, rights, identity)
            for tag, rights, identity in (
                (1, permission, 0xFFFFFFFF), (2, permission, os.getuid()),
                (4, 0, 0xFFFFFFFF), (16, permission, 0xFFFFFFFF), (32, 0, 0xFFFFFFFF),
            )
        )
        if acl != expected:
            raise AssertionError("unexpected credential ACL")
    # Fixture source is deliberately empty; no real credential is ever read.
    with open(directory + "/connector.json", "rb") as file:
        if file.read(1) != b"":
            raise AssertionError("nonempty synthetic credential")


def main():
    if len(sys.argv) != 2 or sys.argv[1] not in ("baseline", "denied", "credential"):
        raise AssertionError("invalid fixture mode")
    mode = sys.argv[1]
    if mode == "credential":
        credential()
        print("CONNECTED_CREDENTIAL_PRIMITIVE_OK", flush=True)
        return
    denied = mode == "denied"
    checked(lambda: network_socket(socket.AF_INET), False)
    checked(lambda: network_socket(socket.AF_INET6), False)
    checked(lambda: network_socket(socket.AF_UNIX), denied)
    checked(unix_pair, denied)
    checked(shared_memory, denied)
    checked(uring, denied)
    print("CONNECTED_" + mode.upper() + "_PRIMITIVE_OK", flush=True)


if __name__ == "__main__":
    try:
        main()
    except BaseException:
        # No raw native exceptions, addresses, IDs, paths or credential bytes.
        print("CONNECTED_PRIMITIVE_FAILED", file=sys.stderr, flush=True)
        sys.exit(1)
