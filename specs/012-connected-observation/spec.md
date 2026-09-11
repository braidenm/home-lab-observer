# Spec 012: Connected native observation

Status: Phased implementation authorized by the owner; slice A is the bounded numeric-host projection below.
Network activation requires the subsequent credential, isolation and transport gates; none is implicitly enabled here.

## Outcome

An owner can eventually enroll a native installation and see its permitted observations in the hosted dashboard without
opening inbound ports. Local operation stays independent. Kubernetes is deferred; Docker actions belong to Spec 011.

## Slice A: explicit remote projection

As an owner, I need an enforceable distinction between rich local observations and data permitted to leave my machine.
As the receiving application, I need a stable document compatible with the existing server overview parser.

- R1: A pure `internal/remoteprojection` package encodes an explicit DTO into UTF-8 JSON. It has no network, credentials,
  filesystem operations, collector calls or runtime activation. Never serialize the local snapshot as an upload body.
- R2: Use `home-lab-server-snapshot/v1` with `projection_profile: numeric-host/v1`. Include source ID supplied by the
  enrollment exchange's `server_id` (`srv_` plus 32 lowercase hex digits, never the `agent_` connector instance), release
  version, UTC collection time, duration, and the four required legacy sections. This is a
  restricted producer profile, not a replacement schema for all legacy agents. Source syntax validation is not auth.
- R3: Only CPU, memory/swap, uptime and at most 16 filesystem capacities are eligible. OS is a fixed Linux/Windows/macOS
  value. Kernel is explicitly `not collected`; load averages remain null. Filesystem keys/titles are generated ordinal
  aliases, not source names, paths or stable resource identities. Do not use aliases for authorization or historical joins.
- R4: Processes, services and containers remain `available: false` with empty arrays and fixed `NOT_UPLOAD_ELIGIBLE`
  reason. No log fields, process/account IDs, commands, environment, source labels, raw reasons, hostnames, addresses,
  Docker metadata or local snapshot identifiers can be copied. New local fields do not become remote-eligible.
- R5: Overview is unavailable, with no numeric payload, unless all four required host sections are AVAILABLE with data
  and filesystem coverage is complete. Preserve absence, denied, unsupported, partial and truncated inputs as unavailable
  rather than zeros. This intentionally conservative legacy view cannot expose every local quality distinction.
- R6: Reject invalid envelope/eligible values with fixed errors and no rejected value. Bound duration to 0..10000 ms,
  logical CPUs to 1..4096, finite CPU percentage to 0..100, and integers to 0..2^53-1 for Go/JVM/browser compatibility.
  Used memory/swap/filesystem bytes cannot exceed totals. Output is at most 16 KiB. Invalid input produces no bytes.
- R7: Test exact fixture equality, strict schema validation, malicious local text exclusion, absent/partial data,
  numeric/time/envelope limits, maximum-size output, determinism and input independence. Do not inspect a real host.
- R8: Local APIs/CLI and current preview behavior remain unchanged. No token acquisition, upload or automatic opt-in.

## Subsequent gates (not complete)

1. Receiver contract proof using the actual Platform Demo parser and synthetic fixtures; retain legacy agents unchanged.
2. Reviewed OS-specific collector/uploader isolation, enrollment recovery, scoped revocable credentials, rotation,
   immutable artifact catalog, outbound TLS/redirect policy and sender-constrained credential decision.
3. Private bounded handoff/spool, monotonic upload identity, jittered retries, expiry/revocation, gap accounting and
   connection health. Keep ambiguous action retry semantics separate.
4. OS-aware install/enrollment UX and hosted owner/delegate views, privacy disclosure, packaged acceptance and canary.
5. Rich remote process/container/history views require separately versioned quality-aware contracts and explicit field
   policies. Do not force missing container metrics into mandatory numeric legacy fields.

No new product decision is needed for slice A. Exact authentication and privileged installation choices require reviewed
ADRs, not improvisation inside this pure projector. Signing/notarization remains a disclosed release prerequisite.
