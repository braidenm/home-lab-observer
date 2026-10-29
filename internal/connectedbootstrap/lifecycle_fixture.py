"""Unprivileged, owned-process tests of the exact embedded supervisor source."""
import contextlib
import io
import errno
import os
import pathlib
import pty
import signal
import subprocess
import sys
import tempfile
import struct
import time
from unittest import mock


def wait_file(path):
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        if path.exists():
            return
        time.sleep(0.01)
    raise AssertionError("fixture readiness deadline")


CHILD = '''import os, pathlib, signal, sys, time
root = pathlib.Path(sys.argv[1])
def finish(signum, frame):
    (root / "child-finished").write_text("done")
    sys.exit(0)
signal.signal(signal.SIGTERM, finish)
signal.signal(signal.SIGINT, finish)
# Publish readiness only after the complete PID is closed. Opening the final
# name first lets the parent signal us between create and write, leaving it empty.
(root / "child-pid.pending").write_text(str(os.getpid()))
(root / "child-pid.pending").replace(root / "child-pid")
while True: time.sleep(0.02)
'''


def worker(root, phase):
    # The production source is included verbatim; only code-owned test children
    # and their temporary directories differ from download/installer commands.
    return SOURCE + '''
import pathlib, time
root = pathlib.Path(sys.argv[1])
life = Lifecycle()
signal.signal(signal.SIGCHLD, signal.SIG_DFL)
for sig in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
    signal.signal(sig, life.cancel)
try:
    if sys.argv[2] == "extract":
        (root / "ready").touch()
        while True:
            life.check()
            time.sleep(0.01)
    else:
        life.run([sys.executable, "-I", "-c", ''' + repr(CHILD) + ''', str(root)])
except InterruptedError:
    pass
finally:
    if sys.argv[2] != "extract":
        assert (root / "child-finished").exists(), "cleanup before child finish"
        pid = int((root / "child-pid").read_text())
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            pass
        else:
            raise AssertionError("cleanup before child join")
    (root / "cleanup").touch()
'''


for phase in ("download", "extract", "install"):
    for sig in (signal.SIGTERM, signal.SIGHUP, signal.SIGINT):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            child = subprocess.Popen([sys.executable, "-I", "-c", worker(root, phase), directory, phase], start_new_session=True)
            try:
                wait_file(root / ("ready" if phase == "extract" else "child-pid"))
                if sig == signal.SIGINT:
                    os.killpg(child.pid, sig)  # real foreground-group Ctrl-C shape
                else:
                    child.send_signal(sig)   # direct wrapper TERM/HUP
                assert child.wait(timeout=3) == 0
                assert (root / "cleanup").exists()
            finally:
                # Only this fixture's own isolated process group is eligible.
                if child.poll() is None:
                    os.killpg(child.pid, signal.SIGKILL)
                    child.wait(timeout=3)

# Spawn/handle gap cancellation, natural exit before forwarding, late signal
# after handle teardown, and pre-cancel refusal without launching anything.
namespace = {"__name__": "fixture"}
exec(SOURCE, namespace)
namespace["require_fd_xattrs"]()
for failure in (OSError(errno.EOPNOTSUPP, "synthetic"), TypeError("synthetic"), OSError(errno.EACCES, "synthetic")):
    with mock.patch.object(os, "removexattr", side_effect=failure):
        try:
            namespace["require_fd_xattrs"]()
        except (OSError, ValueError):
            pass
        else:
            raise AssertionError("unsupported fd xattrs admitted")

# Real valid inherited access/default ACLs, plus a path swap after open. Ordinary
# tests own only their private directory; only root UID/GID metadata is faked.
# Actual root metadata/admission is separately proved by the disposable VM.
clear_acl = namespace["clear_private_acl"]
original_fstat = os.fstat
def root_metadata(fd):
    fields = list(original_fstat(fd))
    fields[4] = fields[5] = 0
    return os.stat_result(fields)
acl = struct.pack("<I", 2) + b"".join(struct.pack("<HHI", tag, permission, identity) for tag, permission, identity in (
    (1, 7, 0xffffffff), (2, 7, 100123), (4, 0, 0xffffffff), (16, 0, 0xffffffff), (32, 0, 0xffffffff)))
with tempfile.TemporaryDirectory() as directory:
    root = pathlib.Path(directory)
    os.setxattr(root, "system.posix_acl_default", acl)
    target = root / "new-private-directory"
    target.mkdir(mode=0o700)
    assert set(namespace["ACL_NAMES"]).issubset(os.listxattr(target))
    os.setxattr(target, "user.hlo-fixture", b"keep")
    moved = root / "anchored-original"
    original_remove = os.removexattr
    def replace_path_then_remove(fd, name):
        assert type(fd) is int
        if not moved.exists():
            target.rename(moved)
            target.mkdir(mode=0o700)
        original_remove(fd, name)
    with mock.patch.object(os, "fstat", side_effect=root_metadata), mock.patch.object(os, "removexattr", side_effect=replace_path_then_remove):
        clear_acl(target)
    assert not set(namespace["ACL_NAMES"]).intersection(os.listxattr(moved))
    assert os.getxattr(moved, "user.hlo-fixture") == b"keep"
    assert set(namespace["ACL_NAMES"]).issubset(os.listxattr(target))
    assert moved.stat().st_mode & 0o7777 == 0o700

with tempfile.TemporaryDirectory() as directory:
    target = pathlib.Path(directory)
    for error in (errno.EOPNOTSUPP, errno.EPERM, errno.EINVAL):
        with mock.patch.object(os, "fstat", side_effect=root_metadata), mock.patch.object(os, "removexattr", side_effect=OSError(error, "synthetic")):
            try:
                clear_acl(target)
            except OSError:
                pass
            else:
                raise AssertionError("ACL syscall failure ignored")
    with mock.patch.object(os, "fstat", side_effect=root_metadata):
        clear_acl(target)  # ENODATA is the only accepted absence condition.
    os.chmod(target, 0o755)
    with mock.patch.object(os, "fstat", side_effect=root_metadata), mock.patch.object(os, "removexattr") as remove:
        try:
            clear_acl(target)
        except ValueError:
            pass
        else:
            raise AssertionError("wrong directory mode admitted")
        remove.assert_not_called()
    os.chmod(target, 0o700)
    for uid, gid in ((1, 0), (0, 1)):
        def foreign_metadata(fd):
            fields = list(root_metadata(fd))
            fields[4], fields[5] = uid, gid
            return os.stat_result(fields)
        with mock.patch.object(os, "fstat", side_effect=foreign_metadata), mock.patch.object(os, "removexattr") as remove:
            try:
                clear_acl(target)
            except ValueError:
                pass
            else:
                raise AssertionError("foreign owner/group admitted")
            remove.assert_not_called()
    def changed_metadata(fd, name):
        os.fchmod(fd, 0o755)
        raise OSError(errno.ENODATA, "synthetic")
    with mock.patch.object(os, "fstat", side_effect=root_metadata), mock.patch.object(os, "removexattr", side_effect=changed_metadata):
        try:
            clear_acl(target)
        except ValueError:
            pass
        else:
            raise AssertionError("post-removal mode drift admitted")
    os.chmod(target, 0o700)
    with mock.patch.object(os, "fstat", side_effect=root_metadata), mock.patch.object(os, "listxattr", return_value=[namespace["ACL_NAMES"][0]]):
        try:
            clear_acl(target)
        except ValueError:
            pass
        else:
            raise AssertionError("remaining ACL admitted")

Lifecycle = namespace["Lifecycle"]
life = Lifecycle()
original_open = os.pidfd_open
def cancelled_open(pid, flags):
    life.cancel(signal.SIGTERM, None)
    return original_open(pid, flags)
os.pidfd_open = cancelled_open
try:
    try:
        life.run([sys.executable, "-I", "-c", "import time; time.sleep(10)"])
    except InterruptedError:
        pass
    else:
        raise AssertionError("spawn-gap cancellation lost")
finally:
    os.pidfd_open = original_open
assert life.active_fd is None
life.cancel(signal.SIGHUP, None)
try:
    life.run(["/nonexistent-must-not-spawn"])
except InterruptedError:
    pass
else:
    raise AssertionError("pre-cancelled spawn")
life = Lifecycle()
life.run([sys.executable, "-I", "-c", "pass"])
life.cancel(signal.SIGTERM, None)
assert life.active_fd is None

# A child can already be a zombie when the pidfd is published; forwarding is
# harmless and mainline still joins. Inject at the actual handle acquisition.
life = Lifecycle()
def exited_open(pid, flags):
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        status = pathlib.Path("/proc", str(pid), "stat").read_text()
        if status.split(")", 1)[1].split()[0] == "Z":
            break
        time.sleep(0.01)
    else:
        raise AssertionError("natural-exit deadline")
    life.cancel(signal.SIGTERM, None)
    return original_open(pid, flags)
os.pidfd_open = exited_open
try:
    try:
        life.run([sys.executable, "-I", "-c", "pass"])
    except InterruptedError:
        pass
    else:
        raise AssertionError("late cancellation lost")
finally:
    os.pidfd_open = original_open

# Cancellation while the joined handle is being closed must never address a
# stale/reused descriptor, and must still refuse success.
life = Lifecycle()
original_close = os.close
def cancelled_close(fd):
    assert life.active_fd is None
    life.cancel(signal.SIGHUP, None)
    original_close(fd)
os.close = cancelled_close
try:
    try:
        life.run([sys.executable, "-I", "-c", "pass"])
    except InterruptedError:
        pass
    else:
        raise AssertionError("teardown cancellation lost")
finally:
    os.close = original_close

# Main must not announce success if the install operation's cleanup fails or
# receives cancellation. The operation is stubbed, never any real root path.
namespace["MEMBERS"] = []
sys.argv = ["fixture", "srv_" + "a" * 32]
for kind in ("cleanup-cancel", "cleanup-failure", "complete"):
    def operation(lifecycle, members, server_id):
        if kind == "cleanup-cancel":
            lifecycle.cancel(signal.SIGTERM, None)
        if kind == "cleanup-failure":
            raise OSError("synthetic cleanup failure")
    namespace["install"] = operation
    output = io.StringIO()
    with contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
        result = namespace["main"]()
    assert result == (0 if kind == "complete" else 22)
    assert ("Installed and stopped" in output.getvalue()) == (kind == "complete")

# Original terminal survives source-on-stdin exec and reaches child stdin.
master, slave = pty.openpty()
try:
    code = SOURCE + '\nLifecycle().run([sys.executable, "-I", "-c", "import os; assert os.isatty(0)"], stdin=3)\n'
    # Bash mirrors the generated entrypoint's FD3 redirection exactly.
    script = 'exec /usr/bin/python3 -I - 3<&0 <<\'FIXED\'\n' + code + '\nFIXED\n'
    child = subprocess.Popen(["/bin/bash", "-c", script], stdin=slave)
    try:
        assert child.wait(timeout=3) == 0
    finally:
        if child.poll() is None:
            child.kill()
            child.wait(timeout=3)
finally:
    os.close(master)
    os.close(slave)
print("supervisor fixtures passed")
