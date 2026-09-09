# Install and run the native preview

Home Lab Observer runs on your machine. It collects host/process readings and serves a private local dashboard; Docker
observations are optional. You do **not** need Docker, Go, Node, a GitHub account, or a GitHub token to run the native
package. Nothing is sent to Platform Demo by this preview.

## Choose your download

Open [GitHub Releases](https://github.com/braidenm/home-lab-observer/releases) and choose the explicitly labeled preview.
The first published version is [0.1.0-preview.1](https://github.com/braidenm/home-lab-observer/releases/tag/v0.1.0-preview.1).
The examples below target `0.1.0-preview.2`, which adds optional background operation. Confirm that version is listed
before running them; preview 1 supports foreground operation only. Always use an installer from the chosen release.
Choose your **host OS**, not Docker's virtual-machine OS:

| Machine | Archive suffix |
| --- | --- |
| Windows Intel/AMD | `windows_amd64.zip` |
| Windows ARM | `windows_arm64.zip` |
| Mac with Apple silicon | `darwin_arm64.tar.gz` |
| Mac with Intel | `darwin_amd64.tar.gz` |
| Linux Intel/AMD | `linux_amd64.tar.gz` |
| Linux ARM64 | `linux_arm64.tar.gz` |

Each archive contains the observer executable, a launch helper, `START-HERE.md`, and the MIT license. Match its filename
and SHA-256 to that release's `SHA256SUMS` **before** extracting/running. Do not accept checksums from an unrelated site.
The release manifest includes exact versioned URLs, sizes and hashes for all six archives.

Windows/macOS previews do not yet have publisher signing/notarization. A checksum checks bytes against a release you
trust; it does not establish publisher identity. Build attestations can separately verify the GitHub repository/workflow.
Do not disable Defender, Gatekeeper or other system-wide protection. If your policy requires a publisher-signed binary,
wait for a signed release; a source build is also available for technical owners.

## Simplest start: unpack and run

After verification, extract the archive into a folder you own. On Windows, open `Run-Observer.cmd`; on macOS, open
`Run-Observer.command` in Terminal; on Linux, run `bash ./run-observer.sh` from that folder. On macOS you can also run
`bash ./Run-Observer.command`. Or run `observer` (`observer.exe` on Windows) directly without arguments.

The foreground console prints the local dashboard address and the location of the private access-token file. Open
`http://127.0.0.1:9847`, read that file locally, and paste the token into **Unlock dashboard**. The token is generated on
your machine; it is not a GitHub token. Keep the console open while using the dashboard; Ctrl+C stops the observer.

For headless use, leave the observer running without opening a browser. To inspect the version without starting anything:

```sh
./observer version --json
```

On Windows, use `.\observer.exe version --json`. [Local service options](local-service.md) cover a different local port,
state directory and authentication. [Docker observations](container-observations.md) explains explicit local socket/pipe
configuration. The observer never discovers credentials or installs/enables Docker.

## Optional per-user installation helpers

Download `install.sh` or `install.ps1` from the same versioned release, verify its checksum, and inspect it before running.
The scripts download the matching native archive anonymously and verify its SHA-256 before extracting it. Use an explicit
version; there is no auto-update or moving `latest` execution target.

Linux/macOS:

```sh
bash ./install.sh --version 0.1.0-preview.2
```

Windows PowerShell:

```powershell
.\install.ps1 -Version 0.1.0-preview.2
```

The helper prints the installed launcher path. It does not start the observer, change your PATH, request administrator
access, register a service or open a browser. If script execution is restricted by your organization, use the verified
archive instead; do not weaken machine-wide policy.

| OS | Default program directory | Launcher |
| --- | --- | --- |
| Linux | `${XDG_DATA_HOME:-$HOME/.local/share}/home-lab-observer` | `bin/observer` |
| macOS | `$HOME/Applications/Home Lab Observer` | `bin/observer` |
| Windows | `%LOCALAPPDATA%\Programs\Home Lab Observer` | `bin\observer.cmd` |

An optional `--install-root PATH` / `-InstallRoot PATH` selects a dedicated program directory. Paths with spaces are
supported. Do not choose your home directory, a drive root, a shared software folder or the observation state directory.
An unmanaged nonempty directory is rejected rather than overwritten.

## Offline install, upgrade, rollback and removal

For an offline installation, provide the downloaded archive and its independently verified SHA-256:

```sh
bash ./install.sh --version 0.1.0-preview.2 --archive ./home-lab-observer_0.1.0-preview.2_linux_amd64.tar.gz --checksum YOUR_64_HEX_SHA256
```

PowerShell uses `-Version`, `-Archive`, and `-Checksum` with the same meaning. Replace the illustrative checksum; it is
not a token or secret. The helper rejects a mismatch or unsafe archive without switching the current version.

Stop the observer before upgrading. Install a new explicit version with the same helper/root; the previous version is
retained. To select a previously installed version, use `--rollback VERSION` or `-Rollback VERSION`. These are program
rollbacks, not database backups: check release compatibility notes before opening newer state with an older binary.

`--uninstall` / `-Uninstall` removes only recognized managed program files. Observation history and the local token are
preserved. Installation never enables background startup. If you explicitly enabled it afterward, use
`observer background disable --install-root PATH` before uninstalling or rolling back to a foreground-only version.
The installer refuses removal while a managed registration remains. See [background operation](background-operation.md)
for login limitations, token locations, safe shutdown and bounded product diagnostics.

## Verify provenance and report problems

Advanced owners with GitHub CLI installed can verify a downloaded archive's provenance:

```sh
gh attestation verify ./DOWNLOADED_ARCHIVE --repo braidenm/home-lab-observer
```

GitHub CLI is not required to run the observer. Review the repository/workflow identity, release tag and commit, not
merely a successful hash comparison. Keep the release's `observer.spdx.json`, manifest and checksums with offline copies.
See [GitHub's attestation guidance](https://docs.github.com/en/actions/concepts/security/artifact-attestations).

For a problem report, include `observer version --json`, OS/architecture, the selected package/version and a concise
description. Never post the local access token, raw machine logs, private network details or unreviewed screenshots.
