# Installed composition implementation checkpoint

Updated 2026-09-17. Implementation and evidence are separate; this is not release or activation approval.

## Implemented surface

The connected bundle has separate collector, uploader and installer command roots; native-v2 packages and the
rich standalone dashboard remain unchanged. The fixed installer operations are:

- `install <verified-bundle-directory> <manifest-sha256> <server-id>`: checks the host/bundle before a no-echo
  one-use-grant prompt, creates isolated principals/root, gates private input on loaded online/offline manager
  policy, enrolls once and promotes the same ledger. Installed identity is published last. Success leaves services
  **stopped**, reporting `INSTALLED_PENDING_ACCEPTANCE`.
- `status`: bounded JSON, including PREPARING and interrupted-detach states. Acknowledgement time differs from
  capture freshness. The 45-second freshness window matches hosted status, not C1's five-minute admission limit.
  An installed identity is not process liveness or a fresh acknowledgement.
- `stop`: verifies exact owned artifacts/units, stops uploader then collector, checks inactive state and no main
  process. Registration and local state remain intact.
- `refresh`: validates fixed-host TLS endpoints and CA trust, records a bounded next-generation journal, stops
  and disables workers, replaces only exact known hosts/CA/unit/config files, verifies loaded endpoint policy and
  publishes a receipt bound to the exact journal. Services remain stopped; activation is a separate gate. A crash,
  refused write or missing/mismatched receipt reports `REFRESH_RECOVERY_REQUIRED`, not a ready mixed generation.
- `uninstall`: stops/disables only exact owned services, quarantines their unit files and retains credentials,
  ledger, accounts, artifacts and diagnostics. The owner must separately revoke registration in Platform Demo.
  A partial detach preserves evidence and reports recovery required.

No alternate origin, arbitrary command, proxy or filesystem-coverage override is accepted. Do not manually start
services to bypass outstanding packaged acceptance. Installer/uploader processes set nondumpable and both core
limits to zero before secret reads; no host crash-handler or system-wide setting changes.

## Evidence and remaining work

The actual three Linux/amd64 command roots compile. Focused synthetic tests cover bounded status, wrong-owner/
symlink/FIFO refusal, input flush/restoration, manager-output ownership, bounded subprocess pipe joins and freshness.
An approved WSL-root fixture uses only fresh owned temporary directories and synthetic records for publication,
no-replace behavior and the separate one-MiB CA bound. No installer test creates persistent accounts/services or a
real enrollment. Primitive systemd probes are documented separately and are not complete installed-profile proof.

At clean commit `33cef83`, Windows build tooling cross-built all three actual Linux/amd64 command roots with
distinct connected identities and successfully generated the exact manifest/resources/archive using `connectedpack`.
This proves actual command packaging, not Linux build-script execution, publication or installed runtime behavior.
The bounded private-input tests passed three Linux runs. Root temporary publication/replacement tests and focused
installer tests also passed three runs; they do not yet inject faults across the entire refresh transaction.

This checkpoint is **not completed F1**. Remaining implementation: activation/restart acceptance gate;
compatible-code rollback; broader transaction failure/held-input composition tests. Separate exact internal
dependency-closure regression tests now cover both worker commands. Remaining acceptance: loaded packaged profiles
under dedicated principals, full repository checks, independent final review and the explicitly unexecuted VM
reboot/power-loss gates. Do not substitute helper tests or static templates for these requirements.

Current numeric-host/v1 reports the **whole overview unavailable** without proven filesystem coverage. The initial
canary proves identity/freshness/delivery, not partial hosted charts. Never set the coverage flag just to get charts.

See [the approved composition](installed-composition.md), [acceptance evidence](installed-acceptance.md),
[bundle contract](connected-bundle.md), and [endpoint policy](endpoint-policy.md).
