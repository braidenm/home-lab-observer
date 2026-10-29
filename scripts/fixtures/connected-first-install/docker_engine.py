"""Real Docker fixture, exclusively inside the admitted disposable no-NIC VM."""
import grp
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import time

import assert_guest
import docker_scope
import preflight_probe
from rich_guest import need, record, run

ROOT = Path("/var/lib/hlo-first-install-fixture")
PREFIX = Path("/opt/hlo-fixture-docker")
PAYLOAD = Path("/mnt/hlo-first-install")
UNIT = "hlo-fixture-docker.service"
IMAGE = "hlo-fixture:local"
NAMES = ("hlo-fixture-running", "hlo-fixture-exited")
DAEMON_GROUP = "hlo-fixture-docker"
DAEMON_GID = 22003
CLI = str(PREFIX / "usr/bin/docker")
ENV = {"PATH": str(PREFIX / "usr/bin") + ":/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL": "C"}


def no_network():
    need({value.name for value in Path("/sys/class/net").iterdir()} == {"lo"}, "ENGINE_NETWORK")
    for family in ("-4", "-6"):
        need(preflight_probe.command("/usr/sbin/ip", family, "route", "show", "default", limit=1024) == b"", "ENGINE_NETWORK")


def daemon_command():
    return (str(PREFIX / "usr/bin/dockerd"), "--config-file=" + str(ROOT / "docker-daemon.json"),
            "--host=unix:///run/docker.sock", "--group=" + DAEMON_GROUP,
            "--data-root=" + str(ROOT / "docker-data"), "--exec-root=/run/hlo-fixture-docker", "--pidfile=/run/hlo-fixture-docker.pid",
            "--bridge=none", "--iptables=false", "--ip6tables=false", "--ip-forward=false", "--ip-masq=false", "--userland-proxy=false",
            "--storage-driver=vfs", "--log-driver=none", "--exec-opt=native.cgroupdriver=systemd")


def import_command():
    return ("image", "import", "--change", "USER 65532:65532", "--change", 'ENTRYPOINT ["/fixture"]', str(PAYLOAD / "fixture-rootfs.tar"), IMAGE)


def container_command(name):
    need(name in NAMES, "ENGINE")
    return ("run", "--detach", "--pull=never", "--name=" + name, "--network=none", "--read-only", "--cap-drop=ALL",
            "--security-opt=no-new-privileges=true", "--memory=32m", "--memory-swap=32m", "--cpus=0.25", "--pids-limit=32",
            "--restart=no", "--log-driver=none", "--env=HLO_FIXTURE_SECRET=hlo-docker-private-canary", "--label=hlo.fixture=hlo-docker-private-canary", IMAGE)


def cli(*args):
    # Fixed verified tools, private socket, no inherited Docker context/credentials.
    return preflight_probe.command(CLI, "--host=unix:///run/docker.sock", *args, limit=32768)


def check_version(value):
    def api(raw):
        need(isinstance(raw, str) and re.fullmatch(r"1\.[0-9]{1,3}", raw) is not None, "ENGINE_API")
        return int(raw.split(".")[1])
    need(value.get("Version") == "29.9.2" and value.get("Os") == "linux", "ENGINE_API")
    minimum, maximum = api(value.get("MinAPIVersion")), api(value.get("ApiVersion"))
    need(minimum <= maximum and max(41, minimum) <= min(45, maximum), "ENGINE_API")


def prerequisites():
    expected = {"libc6:amd64": "2.39-0ubuntu8.9", "libsystemd0:amd64": "255.4-1ubuntu8.17", "libseccomp2:amd64": "2.5.5-1ubuntu3.1",
                "iptables": "1.8.10-3ubuntu2", "nftables": "1.0.9-1ubuntu0.1", "apparmor": "4.0.1really4.0.1-0ubuntu0.24.04.7"}
    for package, version in expected.items():
        raw = preflight_probe.command("/usr/bin/dpkg-query", "--show", "--showformat=${db:Status-Abbrev} ${Version}", package, limit=1024)
        need(raw == ("ii  " + version).encode(), "ENGINE_DEPENDENCY")
    need(Path("/sys/module/apparmor/parameters/enabled").read_text().strip() == "Y", "ENGINE_SECURITY")
    need(preflight_probe.command("/usr/sbin/apparmor_parser", "--version", limit=4096).startswith(b"AppArmor parser version"), "ENGINE_SECURITY")


def prepare(first):
    need(os.geteuid() == 0, "ENGINE")
    no_network()
    prerequisites()
    if first:
        # lstat observes dangling entries too. Refuse every fixed collision
        # before the first mkdir/extraction/group/unit/configuration write.
        for target in (PREFIX, Path("/run/docker.sock"), Path("/etc/systemd/system", UNIT)):
            try:
                target.lstat()
            except FileNotFoundError:
                continue
            raise AssertionError("ENGINE")
        PREFIX.mkdir(mode=0o755)
        for name, (size, pin) in docker_scope.PACKAGES.items():
            source = PAYLOAD / name
            need(source.stat().st_size == size and assert_guest.digest(source) == pin, "ENGINE_INPUT")
            run("/usr/bin/dpkg-deb", "--extract", str(source), str(PREFIX))
        identity = record(PAYLOAD / "reviewed-docker-engine.json")
        need(assert_guest.digest(PAYLOAD / "fixture-rootfs.tar") == identity["rootfs_sha256"] and assert_guest.digest(PAYLOAD / "fixture") == identity["fixture_sha256"], "ENGINE_INPUT")
        docker_scope.validate_rootfs((PAYLOAD / "fixture-rootfs.tar").read_bytes(), (PAYLOAD / "fixture").read_bytes())
        run("/usr/sbin/groupadd", "--gid", str(DAEMON_GID), DAEMON_GROUP)
        (ROOT / "docker-daemon.json").write_text("{}\n", encoding="ascii")
        (ROOT / "docker-daemon.json").chmod(0o600)
        unit = "[Unit]\nDescription=Disposable offline Docker fixture only\n[Service]\nType=simple\nExecStart=" + " ".join(daemon_command()) + "\nEnvironment=PATH=" + ENV["PATH"] + "\nRestart=no\nStandardOutput=null\nStandardError=null\nTimeoutStartSec=40\nTimeoutStopSec=20\nTasksMax=256\nMemoryMax=768M\nCPUQuota=100%\n"
        (Path("/etc/systemd/system") / UNIT).write_text(unit, encoding="ascii")
        run("/usr/bin/systemctl", "daemon-reload")
    group = grp.getgrnam(DAEMON_GROUP)
    need(group.gr_gid == DAEMON_GID and group.gr_mem == [], "ENGINE")
    start()
    if first:
        cli(*import_command())
        image = json.loads(cli("image", "inspect", IMAGE))
        need(len(image) == 1 and re.fullmatch(r"sha256:[a-f0-9]{64}", image[0]["Id"]) is not None and image[0]["Config"]["User"] == "65532:65532" and image[0]["Config"]["Entrypoint"] == ["/fixture"], "ENGINE_IMAGE")
        for name in NAMES:
            cli(*container_command(name))
        cli("container", "stop", "--time=5", NAMES[1])
    else:
        cli("container", "start", NAMES[0])
    prove_containers()
    no_network()


def start():
    run("/usr/bin/systemctl", "start", "--", UNIT)
    deadline = time.monotonic() + 35
    while time.monotonic() < deadline:
        try:
            value = json.loads(cli("version", "--format={{json .Server}}"))
            check_version(value)
            info = Path("/run/docker.sock").lstat()
            need(stat.S_ISSOCK(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o660 and info.st_uid == 0 and info.st_gid == DAEMON_GID and info.st_nlink == 1, "ENGINE_SOCKET")
            no_network()
            return
        except (OSError, subprocess.SubprocessError, preflight_probe.Refused, ValueError):
            time.sleep(0.2)
    raise AssertionError("ENGINE_START")


def stop():
    run("/usr/bin/systemctl", "stop", "--", UNIT)
    fields = assert_guest.unit_properties(UNIT, ("ActiveState", "MainPID", "UnitFileState"))
    need(fields == {"ActiveState": "inactive", "MainPID": "0", "UnitFileState": "static"}, "ENGINE")
    no_network()


def prove_containers():
    rows = json.loads(cli("container", "inspect", *NAMES))
    need(len(rows) == 2, "ENGINE_CONTAINER")
    for row, name in zip(rows, NAMES):
        host, config = row["HostConfig"], row["Config"]
        need(row["Name"] == "/" + name and row["State"]["Status"] == ("running" if name == NAMES[0] else "exited"), "ENGINE_CONTAINER")
        need(config["User"] == "65532:65532" and config["Entrypoint"] == ["/fixture"] and row["Mounts"] == [], "ENGINE_CONTAINER")
        need(host["NetworkMode"] == "none" and host["ReadonlyRootfs"] is True and host["CapDrop"] == ["ALL"] and host["Privileged"] is False, "ENGINE_SECURITY")
        need(host["SecurityOpt"] == ["no-new-privileges=true"] and host["Memory"] == 32 * 1024 * 1024 and host["MemorySwap"] == host["Memory"] and host["PidsLimit"] == 32 and host["NanoCpus"] == 250000000, "ENGINE_SECURITY")
        need(host["RestartPolicy"]["Name"] == "no" and host["LogConfig"]["Type"] == "none" and row["AppArmorProfile"] == "docker-default", "ENGINE_SECURITY")
        if name == NAMES[0]:
            pid = int(row["State"]["Pid"])
            need(pid > 1, "ENGINE_SECURITY")
            status = Path(f"/proc/{pid}/status").read_text()
            need("\nSeccomp:\t2\n" in status and "\nNoNewPrivs:\t1\n" in status and "\nCapEff:\t0000000000000000\n" in status, "ENGINE_SECURITY")
    # Only metadata/booleans escape this function; raw IDs/env/labels stay in memory.
