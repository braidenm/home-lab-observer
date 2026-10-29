# Docker observation data plane — source layer 1 of 3

Date: 2026-10-29. Status: implemented and independently reviewed GO; exact-head CI pending.

## Scope and delivery sequence

Review size warrants three implemented, independently testable layers, not one large source change or a chain of
contract-only placeholders. This first layer implements bounded operations in `workloadprojection`, `sharedhandoff`,
`uploadstate` and `uploadledger`. Layer 2 will compose the three dormant fixed workers. Layer 3 will add their complete
privileged installation and four-executable packaging. All layers form one [installed acceptance target](installable-docker-observation.md).
The [accepted process-only VM evidence](../../docs/acceptance/connected-rich-processes-2026-10-29.md) remains prerequisite
evidence, not Docker acceptance. No Docker installation, worker, socket access, enrollment or release is exposed here.

## Executable boundaries

- Project and validate an opaque sanitized container section, retaining nullable measurements, source timestamps,
  explicit unavailable/healthy-empty states and existing privacy rules; combine it with the process section without
  importing Docker acquisition or host collection into the pure projection boundary.
- Implement a typed latest/staging Docker frame, not a listener or arbitrary file API: 131,072 bytes per record,
  130,560 bytes per section, at most 512 bytes of closed metadata, two slots, 45-second age and 5-second future skew.
  Bind server, immutable definition and policy digest. These hashes are not authentication: Linux ownership, private
  modes, no-follow dirfd operations, single-link files, stable identity and the lifetime lock enforce the local boundary.
  Preserve the section's real observation time; stale/malformed input cannot become fresh or healthy empty.
- Add only closed upload kind `2` and exact SQLite `user_version=3` for the separately approved future
  `RICH_PROCESSES_DOCKER_V1` profile. Both process and container groups must be enabled. Existing numeric kind/schema
  `0/1` and process-only `1/2` remain exact wrappers and retain their existing meanings and SQL.
- Reuse one global sequence, exact pending bytes/digest, full-record CAS and uncertainty recovery. Preserve the
  294,912-byte body cap, 4,096-byte pages, 128-page/512-KiB database and 2-MiB DB/journal/lock envelope. Reject wrong
  kind/version before SQLite recovery, never adopt or migrate state, and prove the third kind in the real bounded
  private ENOSPC fixture. A digest or eligible payload never grants a deployed credential a new profile.

## Acceptance and limits

Run pure projection/privacy/size/freshness tests; real SQLite kind/header/retry/CAS/crash/peak tests; native Linux and
Windows full Go tests/vet; focused race tests; existing Node contracts, schema and foundation checks. Run the explicit
root/DAC frame fixture with separate synthetic UIDs/GIDs and a private chroot, including ACL/link/mode/replacement,
large-frame and foreign-reader negatives. Run the isolated ENOSPC fixture, not a full host disk. Record actual commands
and results below; ordinary non-root tests do not substitute for root-only checks.

No installed unit/resource/profile definition or current worker is changed by this layer. Later source composition
must still prove root-only startup admission, actual packaged Ubuntu/Docker isolation and lifecycle/resource behavior,
independent server approval, reader-before-writer rollback floors, immutable artifacts and owner/delegate acceptance.
Public activation and the owner installation remain off. Spec 011 controls and Kubernetes remain separate.

## Evidence

- The 22 implementation/test files are byte-identical Git blobs to the independently reviewed full-composition source
  `70cadaf9b6bfcfd8542291385571ffa50ef8ce5b`, now isolated on merged main `4a849204cba0be47b298ed9f90ea867b2cff6fef`.
  The process-only acceptance documents and old installed definitions/resources/commands remain unchanged.
- Go 1.27.3 Windows and native Ubuntu WSL `go test -p=2 ./...` and `go vet -p=2 ./...` pass. Focused Linux race checks
  cover `workloadprojection`, `sharedhandoff`, `uploadstate` and `uploadledger`; use `go test -race -p=2` with those paths.
- Separately compiled native Linux test binaries pass as root with `OBSERVER_SHARED_ACCEPTANCE=1` for
  `TestOwnedCrossPrincipalFixture` and `HLO_LEDGER_PRESSURE=1` for `TestStoragePressureFixture`. The former runs real
  separate-UID/GID/chroot frame ACL/link/mode/stale/future/large-frame checks; the latter runs numeric/process/Docker
  ENOSPC cases in their own bounded private mount namespaces. No host accounts, services or Docker daemon are touched.
- Node 22.22.3 `npm ci --ignore-scripts --offline` and `npm test` pass: 80 contract tests, seven schemas, 17 valid/13
  invalid fixtures and real Windows collect-once validation. Foundation validation (43 required files), diff hygiene
  and the exact source-copy checks pass. No dependency, schema fixture or lockfile changed.

Earlier full-composition build/review evidence is not this smaller layer's independent review, exact-head CI or packaged
acceptance. Docker installed/VM/release/hosted/owner gates remain open.

Independent review of the isolated layer verified all22 non-document source/test
blobs match the reviewed composition, with no P0–P2. Focused tests were repeated;
the final Linux race run, including the real SQLite ledger, also passed.
