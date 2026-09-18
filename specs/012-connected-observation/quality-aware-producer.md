# Quality-aware connected host observations

Status: locally implemented and independently reviewed; receiver/deployment and
packaged acceptance pending. No runtime activation or compatibility claim. This
adds to slice A without changing its legacy wire contract.

## Outcome and evidence

An owner must still see valid CPU, memory and uptime observations when filesystem
coverage cannot be proved. The current numeric collector deliberately leaves
filesystem coverage unverified in confinement. The legacy `numeric-host/v1`
projection consequently removes the entire overview, even when those other
sections succeeded. Do not solve this by asserting complete namespace coverage.

Platform Demo's `HomeLabNativeHostMetricsParser` already accepts the separate
`home-lab-native-host-metrics/v1` upload and builds a quality-aware read model.
The inspected receiver is in Platform Demo commit `d04e0005`; implementation
must pin shared synthetic fixtures to the actual parser, not just duplicate its
intended schema in Go tests.

## Required behavior

- Preserve legacy `Encode`/`Validate` and legacy connector payloads unchanged.
  Add an explicit quality-aware producer/validator; do not silently reinterpret
  persisted legacy pending bytes or upgrade an installed compatibility epoch.
- Emit only schema, enrolled server ID, verified collector version, UTC collection
  time, bounded duration and host CPU/memory/uptime/filesystem sections. Existing
  numeric bounds and 16 KiB envelope limit remain. No hostname, path, account,
  process, container, log, address, environment or local source identifier leaves
  this projection. All filesystem aliases remain observation-local ordinals.
- Each metric has explicit quality and nullable values. Unsupported, denied,
  unavailable, disabled and unknown are distinct from zero. Map reasons through
  closed constants; never copy arbitrary local reason text.
- CPU and uptime publish values only when complete and available. Memory may
  publish valid RAM with degraded quality and null swap when swap collection
  failed. Invalid eligible numeric values reject the whole document with fixed
  errors and no rejected data.
- Filesystem samples may be degraded without suppressing other sections. Never
  present the collector's namespace-visible partition count as a proved whole-host
  total. Unknown coverage uses null total. Truncation independently means the
  output cap omitted enumerated rows, not that a selected row failed collection.
  The receiver and first-party decoder must accept degraded null-total/truncated
  samples before activation. Never fabricate a total or lose a truncation flag
  on a published partial sample. If every selected row fails, retain cap metadata
  locally but publish the existing unavailable shape (no inventory claim). A
  uniform failure preserves its closed reason; mixed failures use collection
  failed. This does not misrepresent an unavailable inventory as complete.
- Canonical validation rejects unknown/duplicate fields, malformed quality/value
  combinations, noncanonical bytes, cross-server binding and oversized input.
- The shared handoff, pending-state validation and HTTP transport must agree on
  the admitted closed schema set. Existing pending bytes and their sequence are
  immutable through retry. No schema change may reset ledger sequence, credentials
  or owner binding. Test old pending records alongside new admissions.
- Keep local collection/UI independent. Remote containers, processes, logs and
  delegated controls remain separate work; no new remote collection consent is
  inferred from this host-only improvement.

## Acceptance and delivery

1. Freeze exact quality mapping and filesystem coverage semantics against actual
   producer and receiver code, then independently review this specification.
2. Add privacy, bounds, canonical roundtrip and per-section failure fixtures;
   pass the same emitted bytes through the actual JVM parser.
3. Join collector handoff, uploader admission and transport without changing
   legacy public entrypoints. Test immutable legacy retries and new-profile
   admission through the complete path, including failures and revocation.
4. Include this explicit schema set in the code-owned installed compatibility
   descriptor before implementing compatible code selection. Existing v1 bundle
   identity is not evidence of compatibility with the new profile.
5. Packaged isolated Linux acceptance must demonstrate useful hosted CPU/RAM
   with unverified disk coverage. Only then can an owner canary claim a working
   remote metrics dashboard rather than a successful heartbeat.

These are internal engineering gates, not missing product decisions or external
prerequisites. This document records pending work, not completed behavior.

## Local implementation evidence (2026-09-17)

- `EncodeNative`/`ValidateNative` use closed DTOs; legacy entrypoints are unchanged.
  Exact complete and partial/capped producer fixtures are checked into
  `schemas/remote/fixtures`. Receiver execution is a separate gate below.
- `ValidateUpload`/`UploadCollectionTime` dispatch the two exact schemas. Shared
  handoff, C1 pending validation and HTTP transport use the same validator. The
  ledger format is unchanged; schema identity is already in immutable body bytes.
- Windows `go test ./...` and `go vet ./...` pass. The new tests prove both schema
  transition directions retain an in-flight body/sequence before admitting the
  next sample, and revoked native credentials remain terminal.
- Linux cross-build/vet passes. Real SQLite close/reopen tests and the explicit
  separate-UID shared-handoff fixture each pass three repetitions in local WSL.
  The fixture includes native publication by collector UID and native read by
  uploader UID; this is not an installed service or network activation test.
- Independent review of producer and integration found two empty-filesystem
  quality bugs; the collector now preserves local cap metadata on all-read
  failure and retains uniform permission-denied reasons. The added regression
  passes; mixed failures deliberately map to collection failed.

- Platform candidate `2e62dbc4` executes both byte-identical fixtures through the
  actual JVM parser and upload schema, plus the partial sample's read projection.
  The 11 parser/schema tests pass, as do 10 focused first-party client/UI tests and
  typechecking. Fixture SHA-256 values are `150e1a1dae8b2e2da313b25fc2836b42e51d2550345e7c249291245e5b400961`
  and `73f1017cedd1c0d875772ae879b31df425e05812ddd5c12e25d5cadb8c85c40b`.
  Platform PR 372 subsequently passed required hosted CI run `35293008907`,
  including an executed (not cached) `homeLabNativeMetricsContractTest` against
  the real database. It merged as `851f66082ed76bdd2bdb58ade7c1c867e2c2c7a4`.
  This proves authenticated upload/detail compatibility, not installed transport
  or installed transport proof. Platform Docker Publish run `35293688120` then
  succeeded from exact source `851f66082ed76bdd2bdb58ade7c1c867e2c2c7a4`;
  infrastructure Deploy Service run `35294425625` reported successful exact-source
  reconciliation. This establishes deployed receiver bytes, not a connected
  collector sending a live sample.

Remaining: packaged activation/rollback acceptance and the owner canary. Do not advertise
the draft connected collector as a supported installed profile yet.
