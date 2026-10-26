"""One transient lifecycle owner; Go alone owns installed state and enrollment."""
import re
import errno
import signal
import subprocess
import tempfile


ACL_NAMES = ("system.posix_acl_access", "system.posix_acl_default")
INVALID_LINUX_FD = (1 << 31) - 1


def require_fd_xattrs():
    # Python 3.12 accepts xattr FDs but does not list these functions in
    # os.supports_fd. Probe an impossible Linux FD, never a real path/inode.
    for function, args in ((os.listxattr, (INVALID_LINUX_FD,)),
                           (os.removexattr, (INVALID_LINUX_FD, ACL_NAMES[0]))):
        try:
            function(*args)
        except OSError as error:
            if error.errno == errno.EBADF:
                continue
            raise
        except (TypeError, NotImplementedError, OverflowError):
            raise ValueError("fd xattrs required") from None
        raise ValueError("fd xattrs required")


def clear_private_acl(path):
    directory = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        def validate():
            info = os.fstat(directory)
            if not stat.S_ISDIR(info.st_mode) or info.st_uid or info.st_gid or stat.S_IMODE(info.st_mode) != 0o700:
                raise ValueError("private directory")
        validate()
        for name in ACL_NAMES:
            try:
                os.removexattr(directory, name)
            except OSError as error:
                if error.errno != errno.ENODATA:
                    raise
        if set(os.listxattr(directory)).intersection(ACL_NAMES):
            raise ValueError("acl remains")
        validate()
    finally:
        os.close(directory)


class Lifecycle:
    def __init__(self):
        self.cancelled = False
        self.active_fd = None

    def cancel(self, signum, frame):
        # Never throw asynchronously into staging, spawn or cleanup operations.
        self.cancelled = True
        if self.active_fd is not None:
            try:
                signal.pidfd_send_signal(self.active_fd, signal.SIGTERM)
            except OSError:
                pass

    def check(self):
        if self.cancelled:
            raise InterruptedError("cancelled")

    def run(self, argv, stdin=subprocess.DEVNULL, capture=False, interactive=False):
        self.check()
        child = subprocess.Popen(argv, stdin=stdin,
                                 stdout=subprocess.PIPE if capture else (None if interactive else subprocess.DEVNULL),
                                 stderr=None if interactive else subprocess.DEVNULL,
                                 env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL": "C"},
                                 close_fds=True, shell=False)
        handle = None
        try:
            # SIGCHLD remains default, and only this mainline reaps the child.
            # Even an already-exited child retains its PID until wait below.
            handle = os.pidfd_open(child.pid, 0)
            self.active_fd = handle
            if self.cancelled:
                self.cancel(signal.SIGTERM, None)
            output, _ = child.communicate()
        finally:
            # No cleanup is allowed until the child is joined, including when
            # handle acquisition fails. There is deliberately no PID fallback.
            child.wait()
            self.active_fd = None
            if handle is not None:
                os.close(handle)
        self.check()
        if child.returncode:
            raise ValueError("child refused")
        return output


def cleanup(stage, members):
    # Only fixed public names in this fresh private directory, never recursion.
    directory = os.open(stage, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        try:
            bundle = os.open("bundle", os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=directory)
        except FileNotFoundError:
            bundle = None
        if bundle is not None:
            try:
                for member in members:
                    try:
                        os.unlink(member["name"], dir_fd=bundle)
                    except FileNotFoundError:
                        pass
            finally:
                os.close(bundle)
            os.rmdir("bundle", dir_fd=directory)
        try:
            os.unlink("archive.tar.gz", dir_fd=directory)
        except FileNotFoundError:
            pass
    finally:
        os.close(directory)
    os.rmdir(stage)


def install(lifecycle, members, server_id):
    if sys.version_info[:2] != (3, 12) or os.geteuid() != 0 or os.getegid() != 0 or not os.isatty(3):
        raise ValueError("unsupported host")
    if not re.fullmatch(r"srv_[a-f0-9]{32}", server_id):
        raise ValueError("server reference")
    # Test both pidfd operations before children or filesystem changes.
    handle = os.pidfd_open(os.getpid(), 0)
    try:
        signal.pidfd_send_signal(handle, 0)
    finally:
        os.close(handle)
    require_fd_xattrs()
    version = lifecycle.run(["/usr/bin/curl", "-q", "--version"], capture=True)
    match = re.match(rb"curl ([0-9]+)\.([0-9]+)\.", version)
    if not match or tuple(map(int, match.groups())) < (8, 4):
        raise ValueError("curl 8.4 required")
    for parent in ("/", "/var", "/var/lib"):
        info = os.lstat(parent)
        if not stat.S_ISDIR(info.st_mode) or info.st_uid or info.st_gid or info.st_mode & 0o022:
            raise ValueError("staging ancestor")
    lifecycle.check()
    stage = tempfile.mkdtemp(prefix="home-lab-observer-download.", dir="/var/lib")
    try:
        clear_private_acl(stage)
        bundle = stage + "/bundle"
        os.mkdir(bundle, 0o700)
        clear_private_acl(bundle)
        archive = stage + "/archive.tar.gz"
        lifecycle.run(["/usr/bin/curl", "-q", "--fail", "--remove-on-error", "--show-error", "--silent",
                       "--proxy", "", "--location", "--proto", "=https", "--proto-redir", "=https",
                       "--max-redirs", "5", "--connect-timeout", "10", "--max-time", "180",
                       "--max-filesize", "@ARCHIVE_SIZE@",
                       "https://github.com/braidenm/home-lab-observer/releases/download/v@VERSION@/home-lab-observer-connected_@VERSION@_linux_amd64.tar.gz",
                       "--output", archive])
        lifecycle.check()
        if os.stat(archive).st_size != int("@ARCHIVE_SIZE@"):
            raise ValueError("archive size")
        digest = hashlib.sha256()
        with open(archive, "rb") as stream:
            while chunk := stream.read(65536):
                lifecycle.check()
                digest.update(chunk)
        if digest.hexdigest() != "@ARCHIVE_SHA256@":
            raise ValueError("archive digest")
        extract(archive, bundle, members, lifecycle.check)
        lifecycle.run([bundle + "/observer-connected-install", "install", bundle,
                       "@MANIFEST_SHA256@", server_id], stdin=3, interactive=True)
    finally:
        try:
            cleanup(stage, members)
        except OSError:
            print("CONNECTED_BOOTSTRAP_STAGING_RETAINED: inspect the owned download staging; installed state was not removed.", file=sys.stderr)
            raise


def main():
    lifecycle = Lifecycle()
    signal.signal(signal.SIGCHLD, signal.SIG_DFL)
    for signum in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
        signal.signal(signum, lifecycle.cancel)
    try:
        install(lifecycle, MEMBERS, sys.argv[1])
        lifecycle.check()
    except (OSError, EOFError, ValueError, zlib.error, AttributeError):
        print("CONNECTED_BOOTSTRAP_REFUSED: installation interrupted or preflight/download/verification failed; installed state was not removed.", file=sys.stderr)
        return 22
    print("Installed and stopped. Start explicitly, then check for a received snapshot in the app. Startup after reboot is manual.")
    for action in ("start", "status", "stop"):
        print("sudo -- /opt/home-lab-observer-connected/releases/@MANIFEST_SHA256@/observer-connected-install " + action)
    return 0


if __name__ == "__main__":
    sys.exit(main())
