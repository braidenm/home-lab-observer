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
