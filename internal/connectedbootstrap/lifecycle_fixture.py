"""Unprivileged, owned-process tests of the exact embedded supervisor source."""
import contextlib
import io
import os
import pathlib
import pty
import signal
import subprocess
import sys
import tempfile
import time


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
(root / "child-pid").write_text(str(os.getpid()))
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
