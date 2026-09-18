# Code-owned connected compatibility descriptor

Status: implementation slice under Spec 012 and ADR 022. Recognition is not
package authenticity, transition, activation, rollback or migration authority.

The compiled `observer-connected-compatibility/v1` JSON descriptor freezes the
reviewed current wire, upload-state, ledger, credential, installed-config,
enrollment, status, handoff, worker and resource contracts. Its SHA-256 covers
the exact compact bytes including the final LF. No file path, network source,
caller-supplied accepted-version list, or mutable registry can replace it.
`Descriptor` returns detached bytes; `Known` accepts only the compiled digest.

The four embedded unit/schema resources must hash to the exact values in this
descriptor. Tests pin the descriptor digest, actual resource bytes, current
ledger SQL hash, upload-state limits and remote wire constants. A change to
any of these is a deliberate compatibility revision with cross-version tests,
not an implicit acceptance of a candidate package's self-description.

`transition_protocol` remains `not-implemented` and
`durable_filesystem_identity` remains `not-asserted`. Current stage-file
primitives do not change those claims. Trusted release digests, exact package
verification, two-release state tests, installed/reboot/recovery acceptance,
and independently reviewed activation remain open gates.
