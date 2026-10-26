# Linux connected monitoring operations

[Connected beta 0.1.0-canary.1](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-canary.1)
is published for **Ubuntu 24.04, systemd 255, amd64**. Its immutable assets passed
provenance/SBOM checks and both exact-artifact installed VM scopes; see the
[canonical acceptance record](acceptance/connected-linux/0.1.0-canary.1.json).
Real owner-server installation and sharing acceptance remain separate rollout
work. This is an unsigned, explicit-start beta, not unattended or GA software.

This is not the rich [Windows/macOS/Linux local preview](install.md), and it does
not replace Platform Demo's legacy Compose collector/uploader. Keep existing
connectors running during a separate native canary. Native downloads require no
GitHub account/PAT, Docker, Go or Node. The installer requires a sudo-authorized
terminal, Bash/coreutils, curl 8.4 or newer and Python 3.12 on the supported host.
It does not install prerequisite packages; ambient download proxies are unsupported.

## Install, connect, check and share

1. Sign in to the hosting app and open **My servers**. Choose **Connect a Linux
   server**, then the native Linux beta, name the server and **Create enrollment**.
   If that option is absent, ask the site operator whether the connected release
   catalog is enabled and available; a local preview is not a substitute. Registration binds this
   server to your signed-in user ID, not to a platform admin role.
2. Keep the enrollment page open. Copy its **Verified native install** command to
   the server terminal. It verifies the version-pinned public helper before sudo;
   do not use `curl | sh` or mix release files. At the hidden prompt paste **only**
   the separate `hle_` one-use enrollment value shown by the app. It expires after
   ten minutes and is not a SHA-256 checksum, GitHub token, command or local
   dashboard token. Never put it in shell arguments, environment, scripts or logs.
3. Installation uses root to provision separate restricted collector/uploader
   workers and scoped credentials, then leaves both services disabled and stopped.
   Run the exact **Start native observer** command printed by the app/installer.
   Repeat explicit Start after reboot; do not bypass the gate with `systemctl enable`
   or direct service starts. It can run headless without an open browser or inbound port.
4. Wait at least a minute, then use **Check connection** and **View dashboard**.
   Confirm fresh CPU, memory and uptime readings; storage capacity is shown where
   permitted and available. Process startup or a received-but-stale/partial snapshot
   is not a useful, acknowledged connection. Use **Native status** when diagnosing.
5. When site sharing is enabled, open the owned server's **Sharing** panel. Ask
   the existing recipient to copy their user ID from **My servers**, then **Grant
   read access**. The recipient signs into their own account to view this server;
   no platform admin role or connector token is needed. Revoke the specific viewer
   grant when finished. This grants viewing only, not re-sharing, connector changes
   or container actions. These are the supported flow and remaining live acceptance
   steps, not a claim that owner/recipient testing is already complete.

Uploaded observations are limited to numeric CPU, memory/swap, uptime and bounded
permitted filesystem capacities, plus required source/release/time and quality
metadata. The snapshot profile excludes process/container details, hostnames,
addresses, logs/log metadata, command lines and environment variables. Rich local
observations remain a different product surface; enrollment does not enable
remote upload of those fields.

## Start, status and stop

After installing a verified connected bundle, use its installed management binary:

```sh
sudo /opt/home-lab-observer-connected/releases/<manifest-sha256>/observer-connected-install start
sudo /opt/home-lab-observer-connected/releases/<manifest-sha256>/observer-connected-install status
sudo /opt/home-lab-observer-connected/releases/<manifest-sha256>/observer-connected-install stop
```

Use the exact commands printed by the installer/app. The placeholder is the
verified manifest digest, not an archive checksum or a guessed release directory.
The command verifies its installed identity. Start
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
back an older ledger is not recovery. If installation reports partial setup,
preserve it and revoke that registration before a new enrollment; do not delete
credentials, reset delivery sequences or repeatedly rerun the installer.
**Revoke connector** on the owner server page prevents further authenticated
uploads; it does not uninstall software. Revoking a viewer grant is a separate
access operation, not connector revocation.

This manual lifecycle does not yet enable boot startup, automatic crash recovery,
endpoint refresh, automated upgrade/code rollback or automatic uninstall. A crash
does not automatically restart workers. Preserve uncertain state for explicit
recovery; there is no supported automatic repair or removal command in this beta.
Connected Windows/macOS, rich remote containers/processes/logs, Docker controls
and Kubernetes remain separate future slices.

See [Spec 012](../specs/012-connected-observation/activation-lifecycle.md) for the
activation boundary. The earlier
[first-install acceptance record](../specs/012-connected-observation/first-install-acceptance.md)
is historical evidence; the canonical versioned record above binds this release's
bootstrap and activation results to its own exact candidate bytes.
