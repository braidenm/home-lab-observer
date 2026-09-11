# Linux ledger storage-pressure fixture

Scope: follow-up acceptance for the unused D1 ledger. No production code, service installation or credential use.

## Contract and plan

Exercise actual kernel ENOSPC against a disposable 1 MiB tmpfs in a new private mount namespace, never by filling
the owner's disk. A root-only, explicitly opted-in Linux test subprocess makes mount propagation private, creates
its own temporary directory, and mounts only that directory. No host users, services or existing mounts are changed.
All observations, binding values and files are synthetic. The mount disappears with the child namespace.

1. Provision a ledger and durably admit a nonzero pending sequence before exhausting the fixture filesystem.
2. Fill a separate fixture file with bounded writes; require actual ENOSPC rather than an injected Go error.
3. Attempt the next ledger commit. Require a fixed recovery failure and refusal of further operations on that instance.
4. Release only the owned fill file, close/reopen the ledger, and validate that the prior complete record remains;
   never accept recreated zero state or changed bytes under an allocated sequence.
5. Bound subprocess time and memory/disk input. Default unit tests skip this privileged fixture explicitly; opt-in
   execution must fail (not skip) when required root/namespace/mount capabilities are missing.

This proves behavior under a real bounded-filesystem allocation failure. Tmpfs does **not** prove physical-device
sync, VM power-loss durability, persistent-filesystem behavior or the installed worker security boundary. Those
acceptance gates remain open. The fixture adds no dependencies or automatic elevation.

A path-filtered, read-only-token GitHub-hosted Ubuntu job runs this synthetic fixture, with a five-minute job limit.
It never uses a private/self-hosted runner or production credentials. Manual dispatch is available for later validation.
The documented local command requires the owner's explicit `sudo` invocation; library code never elevates itself.

## Tasks

- [x] Implement opt-in namespace-isolated fixture with fixed output and bounded cleanup.
- [x] Execute under owned Linux test environment and independently review safety and assertions (2026-09-11, no blockers).
- [ ] Run ordinary tests and normal CI; document exactly which storage guarantees remain untested.

## Running the fixture

Run only on an owned Linux test environment with root mount-namespace capability. It uses no production data or
credentials and makes no network calls. Explicit opt-in failure is a failed test, not unsupported-as-success.

```bash
fixture_dir=$(mktemp -d)
go test -c -o "$fixture_dir/uploadledger.test" ./internal/uploadledger
sudo env HLO_LEDGER_PRESSURE=1 "$fixture_dir/uploadledger.test" \
  -test.run='^TestStoragePressureFixture$' -test.count=3
```

The binary remains in the displayed temporary directory for inspection/removal; mounted synthetic data is cleaned
up by the fixture. Initial execution on 2026-09-11 passed three times under Ubuntu WSL. This is allocation-failure
evidence only, not persistent-disk or power-loss certification.
