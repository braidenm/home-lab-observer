# Connected build identity declaration

Status: implementation slice under Spec 012 and ADR 022. Identity establishes
internal consistency only; it is not a signature, publisher attestation,
package-acceptance decision, or permission to activate workers.

The separate connected canary has closed `collector`, `uploader` and `install`
roles on Linux/amd64. V1 retains its exact five-field marker and has no
recognized compatibility contract. V2 uses distinct framing and a sixth
lowercase SHA-256 field that must equal the compiled code-owned descriptor.
Unknown, empty, malformed, mixed and duplicate records refuse. The scanner
reads at most 200 MiB with one fixed chunk plus record overlap; it accepts
exactly one canonical marker from an already bounded regular file. Runtime
resolution additionally checks the executable's role and current OS/arch.

Tests cover both generations, chunk boundaries, wrong role/target, malformed
semver and digest, duplicate/mixed markers and scanner bounds. No installer,
filesystem, network, credential, worker-start, or bundle-selection operation
is added by this slice. A candidate package still needs trusted release
digest, exact files, three matching binary identities, two-release state
compatibility and installed VM acceptance before code selection or rollback.
