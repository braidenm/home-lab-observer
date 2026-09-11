# Manual systemd primitive acceptance

Run the **Connected profile primitive acceptance** workflow manually on its
GitHub-hosted Ubuntu 24.04 VM, or explicitly run the script as root on a disposable
Ubuntu 24.04/systemd255 amd64 VM:

```sh
sudo env -i PATH=/usr/bin:/bin bash scripts/fixtures/connected-primitives/run.sh
```

This creates only three uniquely named transient services and a guarded owned `/tmp`
fixture. It uses the existing nobody/nogroup identity, not new accounts. Both syscall
invocations have fresh IPC namespaces; synthetic shared memory is removed immediately
and namespace destruction is a cleanup backstop. No socket binds/connects/sends,
real credentials, host observations, global policy changes or permanent units occur.
Runtime/timeouts and exact cleanup are bounded. Output is fixed result codes only.

The baseline must actually permit the tested operations; a host that already denies
io_uring does not provide this differential proof and fails rather than changing its
global policy. The hardened invocation denies AF_UNIX, socketpair, shmget and
io_uring_setup while preserving AF_INET/AF_INET6 socket creation. It does not prove
packet filtering, abstract-peer connections, every alternate syscall, final worker
mounts, Go/SQLite compatibility, enrollment, reboot or power-loss behavior.

The empty LoadCredential fixture checks root ownership, exact mode and named-worker
ACL/readability inside RootDirectory. Its Python-only public runtime mounts are
deliberately broader than the final uploader profile, so it is not a confidentiality
acceptance test for that profile.

Root's original synthetic WSL systemd255.4 probes passed these syscall and credential
semantics on 2026-09-11. This checked-in adaptation and hosted workflow require their
own execution evidence; neither is claimed passed merely from that earlier probe.
