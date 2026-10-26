# Linux connected monitoring operations

The connected Linux package is under acceptance and is separate from the local
Preview 3 downloads. Supported first target: Ubuntu 24.04, systemd 255, amd64.
Release/canary acceptance must pass before these commands are offered to owners.

After installing a verified connected bundle, use its installed management binary:

```sh
sudo /opt/home-lab-observer-connected/releases/<manifest-sha256>/observer-connected-install start
sudo /opt/home-lab-observer-connected/releases/<manifest-sha256>/observer-connected-install status
sudo /opt/home-lab-observer-connected/releases/<manifest-sha256>/observer-connected-install stop
```

Use the manifest digest from the verified package, not an archive checksum or a
guessed release directory. The command verifies its installed identity. Start
requires stopped workers and repeats the current invocation's sandbox checks.
It reports `WAITING_FIRST_UPLOAD` after startup; the first acknowledged upload
normally needs at least one minute plus collection and network time. Status
distinguishes starting, stopped, partial/failed workers, and fresh/stale receiver
acknowledgement. The uploaded profile is numeric host monitoring only.

A refused start includes a fixed phase: `LEASE` means another installer operation
owns the lock, `INSTALLED_STATE` means files or ownership differ, `HOST_PROFILE`
means the supported OS/manager prerequisites differ, `PRINCIPALS` means dedicated
service accounts differ, `SERVICE_STATE` means the pair is not safely stopped,
and `RUNTIME_GATE` means the fresh sandbox/identity/handshake checks failed.
Preserve state for investigation; do not broaden permissions or reset the ledger.
Unknown failures retain the generic refusal and never include raw OS errors.

Stop joins both workers and retains credentials and delivery history. A recovery
diagnostic means preserve the installation for inspection; reinstalling or copying
back an older ledger is not recovery. Revocation belongs to the owner server page.

This manual lifecycle does not yet enable boot startup, automatic crash recovery,
endpoint refresh, code rollback or automatic uninstall. The final headless release
must provide and validate those operator paths before claiming unattended use.
The legacy collector/uploader can remain active during the separate canary.

See [Spec 012](../specs/012-connected-observation/activation-lifecycle.md) for the
activation boundary and [the first-install acceptance record](../specs/012-connected-observation/first-install-acceptance.md)
for the earlier stopped-install proof.
