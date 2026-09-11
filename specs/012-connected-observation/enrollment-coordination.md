# D2b one-shot enrollment coordination

Status: implementation authorized for an unused pure coordination kernel only. Exchange HTTP, private filesystem
credentials, installed service wiring, worker activation and owner canary remain separately reviewed gates.

## Outcome and scope

Coordinate one explicit owner enrollment without ever treating a lost exchange response as retryable success. The kernel
reads a private one-use grant through a narrow port, generates one stable connector instance ID, durably records an attempt,
performs at most one exchange, exclusively persists the returned binding/credential, provisions the new upload ledger, and
marks the installation READY only after every prior operation reports durable success.

The kernel contains no HTTP client, filesystem implementation, CLI flags, service manager, collector, uploader loop or
automatic retry. It returns fixed outcomes and errors only. Dependency errors, grants, connector credentials, paths,
receiver bodies and URLs are never returned or logged.

## Exact ports and ordering

- `GrantSource.Read(ctx)` returns a detached bounded in-memory record containing the expected Platform `server_id` and the
  one-use `hle_` secret. The source must obtain it from a later reviewed no-echo prompt or owner-only opened handle, never
  argv, environment, URL/query, public config, diagnostics or hosted browser artifacts. No expiry field is required because
  the current owner UI does not provide a stable local input contract for it.
- The coordinator validates `srv_` plus 32 lowercase hex and `hle_` plus 43 base64url characters before any durable or
  network work. It generates `agent_` plus 32 lowercase hex from 16 cryptographically random bytes.
- `AttemptStore.Begin(ctx, binding)` exclusively and durably records only the expected server/connector binding and fixed
  `ATTEMPTED` state before `Exchanger.Exchange` is called. It contains no grant, grant hash or connector credential.
- `Exchanger.Exchange(ctx, request)` receives the fixed binding and one-use grant and performs at most one receiver request.
  The later adapter owns the fixed HTTPS route and decoding policy. The kernel never invokes it twice.
- On accepted exchange, validate exact response server correlation and `hlc_` plus 43 base64url characters. Then call
  `CredentialStore.Create(ctx, credential)` exclusively. The store must durably bind the secret to both server and connector
  IDs and refuse any existing, linked, malformed or mismatched entry without repair or overwrite.
- Only after credential persistence succeeds, call `LedgerProvisioner.Provision(ctx, binding)`. Its later D1 adapter must
  create a proven-new ledger, validate the zero-watermark binding, close it cleanly and never reset/open arbitrary state.
- Only after both calls succeed may `AttemptStore.MarkReady(ctx, binding)` durably change the same exact attempt to `READY`.
  A later worker must independently load READY, validate the credential binding and use D1 `OpenExisting`; it never calls
  Provision and never infers readiness from a present credential or database alone.

All boundary buffers are exactly length-checked before cloning and detached between ports. Clearing coordinator-owned byte
slices is defense-in-depth hygiene, not a guarantee that the Go runtime has erased every compiler/runtime copy.
Implementations honor cancellation and return code-owned errors. The coordinator is single-use and serial: cancellation or
any dependency failure after Begin returns `RECOVERY_REQUIRED`, performs no cleanup, and cannot be retried on that instance.
If `Begin` itself returns an error, no receiver request was made, but the store may have durably written the marker before
reporting failure; that local root is still preserved and must not be automatically reused or repaired.

## Receiver outcomes and ambiguity

Platform Demo PR 367 proves the raw snake-case exchange response and snapshot acknowledgement. Current receiver behavior:

- 200 with bounded valid `{server_id, connector_secret}` is accepted only after exact expected-server correlation.
- 429 with `Retry-After: 60` is the only explicit `RETRYABLE` exchange result because admission occurs before grant lookup
  and activation. The coordinator does not wait or retry. An owner may explicitly start a new one-shot attempt with the
  still-private grant after at least 60 seconds; same-root marker cleanup/provisioning is a later installed workflow.
- 400 is `REJECTED`; local validation should prevent it and no retry is inferred.
- 401 is `RECOVERY_REQUIRED`, not a reusable invalid-token result. The receiver maps aggregate activation failures to 401,
  and activation may already have committed before the response failed.
- Redirects, TLS/network/deadline errors, 5xx, unexpected statuses, invalid/mismatched/oversized 200 responses and process
  loss after Begin are all `RECOVERY_REQUIRED`. D2b deliberately does not use `httptrace.WroteRequest` to relax this rule.

An interrupted setup without a fully validated READY marker is not resumed, even when a matching credential or ledger is
present. This trades convenience for a small auditable first release: preserve the evidence, do not re-exchange or repair,
and require explicit owner recovery.

## Fixed HTTPS exchange adapter

The unused `platformtransport.EnrollmentTransport` implements the coordinator's `Exchanger` port without adding a second
destination policy. It accepts only the trusted installer-selected canonical HTTPS origin and reuses the snapshot
transport's production-owned TLS, proxy, redirect, compression, connection, header and five-second deadline policy. Tests
may supply only an owned TLS root through a package-private constructor. There is no exported client, proxy, certificate,
path or retry override.

Each `Exchange` validates the exact server, connector and enrollment-secret lengths and syntax before allocating the
bounded JSON body or opening a connection. It issues one logical POST to the fixed
`/v1/connectors/home-lab/enrollments:exchange` path with only JSON accept/content headers; the one-use grant is never placed
in a URL, authorization header, cookie, referer, user agent or error. The request body is non-rewindable and `GetBody` is
nil, and the adapter performs no application retry. Standard TLS/TCP/HTTP protocol recovery below the logical request is
not described as a second enrollment attempt or as durable receiver proof.

A successful response is at most 16 KiB and must be JSON with exactly one known `server_id` and `connector_secret` of the
fixed lengths and syntax. Duplicate known or unknown keys, trailing data, BOM, compression and malformed known fields are
rejected; bounded additive unknown fields are ignored and discarded. The adapter checks exact expected-server correlation
before returning the secret, and the coordinator checks it again before persistence. A 429 is `RATE_LIMITED` only with one
exact `Retry-After: 60` header; 400 is `REJECTED`; 401, 5xx, other statuses, malformed success, invalid framing, redirect,
TLS/network/deadline failure and any unexpected condition are ambiguous. Non-200 response bodies are not decoded and cannot
change status classification. Exact length checks precede secret cloning. Byte-slice clearing is best-effort memory hygiene,
not a guarantee that transport, TLS, compiler or runtime copies have been erased.

## Owner recovery and credential lifecycle

The current receiver exposes no connector-side exchange-status, credential retrieval or reissue route. For an ambiguous or
partial setup, the owner uses the authenticated Platform server page/API to revoke that server and create a fresh enrollment,
then uses a fresh local provisioning root. The observer never receives the owner's user session and cannot revoke on their
behalf. Old local credentials/state remain private and untouched until a later stop/join plus explicit removal operation.

The connector credential is a server-scoped snapshot bearer and remains replayable until owner revocation. It is not a user
session, enrollment grant or action credential. Stronger sender constraint/rotation is a separately versioned receiver and
installer migration. Local-only Observer operation is unchanged and does not require enrollment.

## Acceptance

Pure synthetic tests must prove exact call order; one exchange maximum; invalid input/random/Begin failure before exchange;
all response classifications; server/credential correlation; no credential/ledger/READY work after retryable, rejected or
ambiguous exchange; credential-before-ledger-before-READY; every crash/failure boundary returning recovery without cleanup;
single-use/concurrent refusal; buffer detachment; and canary secrets/errors absent from attempt records and public results.

The later adapter tests must separately prove private grant input, exclusive durable credential files, D1 integration and
the raw PR 367 HTTPS exchange. Installed acceptance must prove owner revoke/fresh enrollment and that no authenticated trace,
screenshot, CI artifact, command line, environment or diagnostic contains either secret. No public-PR code contacts a
private receiver.
