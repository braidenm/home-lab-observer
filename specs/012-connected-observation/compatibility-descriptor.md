# Recognized connected compatibility descriptor

Implementation slice, 2026-09-17. This binds the current contracts to explicit
connected bundle/identity v2. It does not implement or authorize code selection,
rollback, filesystem migration, reboot recovery or activation. Native standalone
distribution v2 is unchanged.

## Code-owned authority and exact bytes

`internal/connectedcompat/descriptor.json` is a fixed canonical compact JSON object
with one final LF. SHA-256 covers those exact bytes, including LF. The standard-
library-only package returns detached bytes and the compiled digest; `Known`
compares only against that digest, not a caller's accepted-version list. There is
no descriptor file/path/network loader and no supplied-epoch migration hook.

Freeze current D1 SQLite application ID 1213156420, user version 1, exact SQL hash,
4096-byte pages, DELETE journal, EXTRA sync and bounded one-record layout. C1 uses
signed-64 sequence exhaustion, immutable binding/pending retry bytes, correlated
ACK before clearing, and preserved terminal states. Both legacy numeric-host and
native quality-aware documents are admitted; native is the current producer.
Admission age is 120 seconds with 30-second future skew; offline witness comparison
never expires pending data. Freeze credential/install/enrollment/status schemas,
shared handoff protocol, fixed worker isolation and actual template/schema hashes.
Scheduling fields explicitly name the uploader: its startup delay is 60 seconds
and polling interval 15 seconds. The collector starts collecting immediately;
the descriptor does not assign the uploader's delay to every worker role.

Activation request/response/commit v1 is recognized, but its per-invocation checks
are still required. Transition protocol is explicitly **not implemented** and
durable filesystem identity **not asserted**. Adding them requires a deliberate
descriptor revision/digest and reviewed package tests, not reusing this digest.

The descriptor describes compatibility, not program equivalence or publisher
authenticity. Trusted release digests, exact package validation, cross-version
state tests and installed/reboot/recovery gates remain mandatory.

## Explicit identity and bundle formats

Identity v1 retains its exact existing five fields and framing. Its contract is
empty and `HasKnownContract` is false. Identity v2 has distinct V2 framing and a
sixth lowercase contract SHA-256 field. Only the compiled known digest is accepted;
unknown, empty, self-asserted, mixed or duplicate records refuse. Both formats
retain the same closed three roles, prerelease version, Linux/amd64 target and
bounded scanner; neither format changes native standalone identity validation.

Bundle v1 retains exact original canonical keys and no contract field. Bundle v2
requires `contract_sha256` equal to the compiled digest and exact agreement with
each collector/uploader/install binary identity. All seven original payloads and
their exact hashes/modes remain required; no arbitrary descriptor payload or extra
files are accepted. Template resources must match the reviewed compiled resources.

The build tool emits v2 by explicitly binding the compiled digest; it accepts no
contract override. Read-only v1 parsing/verification remains available for diagnosis
and explicit existing-install handling, but v1 never gains a recognized contract
through its version, commit, filenames, or current build-tool version. No new
upgrade/activation operation is introduced. `HasKnownContract` is declaration
recognition only; callers still need successful actual package verification.

## Tests

- Pin exact descriptor bytes/digest, current resource hashes and D1 SQL; reject
  arbitrary digest values and ensure returned bytes cannot mutate authority.
- Roundtrip v1/v2 identities; bounded scanning across chunks, mixed/duplicate
  framing, wrong digest, malformed v2 and actual role mismatches refuse.
- Validate v1 nonauthority, v2 manifest-to-three-binary contract equality, exact
  files/resources, altered self-assertions, downgrade/mixed bundle combinations.
- Build tiny genuine Linux Go ELF fixtures (never execute), verify deterministic
  archive packaging, and retain separate worker dependency boundaries.

Actual two-release compatibility, installed activation, transition/reboot and
power-loss proof remain open. Structural validation does not close those gates.

## Local verification (2026-09-17)

- Go 1.27.1 focused tests/vet passed for descriptor, connected identity, bundle,
  build tool and separate worker dependency gates. Both workers gained only the
  standard-library-only `connectedcompat` dependency.
- Tests built genuine tiny Linux ELF fixtures for both identity generations and
  verified exact bundles, deterministic archives, mixed-contract rejection and
  tampered self-assertions with recomputed manifest digests. Inputs were never run.
- Linux-native descriptor and identity test binaries passed five repetitions.
- All three actual connected commands were built with Linux/amd64, CGO disabled,
  trimpath and v2 identity, then passed the build tool's exact verification and
  archive packaging. This working-tree test package used a synthetic prerelease
  label and is not a clean-source release, signed artifact or deployment input.
- No service, account, installed state, remote endpoint or standalone release was
  changed. Cross-version and installed acceptance gates above remain unexecuted.
