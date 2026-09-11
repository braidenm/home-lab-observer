# ADR 012: Explicit numeric host egress projection

Status: Accepted for slice A of Spec 012; no network or credential activation.
Date: 2026-09-10.

## Context and decision

The owner approved native connectivity before separate Docker actions. Platform Demo ADR 044 selects a separate
collector/uploader boundary and the existing `home-lab-server-snapshot/v1` receiver. That receiver accepts unavailable
sections, but requires numeric overview/container values when present. Rich local snapshot types contain information
that is not approved for egress and cannot preserve missing container metrics through that legacy shape.

Build a separate package-private DTO by explicit assignments. The first producer profile exports numeric host overview
only, with generated filesystem aliases and unavailable workload sections. Reject invalid eligible values; suppress
incomplete host overview rather than misrepresenting unknown data. Cap exact JSON integers at 2^53-1 and bytes at 16 KiB.
Use an additive profile marker; do not modify legacy receiver semantics or claim support for rich remote inventory.

## Alternatives

| Option | Benefit | Consequence |
| --- | --- | --- |
| Serialize local DTO and redact strings | Fewer models | New fields can leak; strings are not reliably anonymized; incompatible receiver |
| New complete remote protocol now | Rich per-field quality | Couples security, receiver, UI and migration before basic egress is proven |
| Explicit narrow legacy-compatible profile | Small testable boundary; retains installed consumers | Conservative unavailable overview; richer views need a later contract |

Select the third option as a reversible foundation, not the final dashboard feature set. Do not infer privacy from a
successful schema check alone. Enrollment binds source identity and consent later; syntax validation is not proof.

## Evidence and interpretation

Accessed 2026-09-10; Go implementation uses the repository's Go 1.27 toolchain, without new dependencies.

- [RFC 8259 section 6](https://www.rfc-editor.org/rfc/rfc8259#section-6) identifies the interoperable exact integer range
  used by binary64 consumers. The producer's cap is intentionally narrower than the JVM's signed-long range.
- [Go encoding/json](https://pkg.go.dev/encoding/json) serializes exported struct fields and rejects non-finite numbers.
  Package-private DTOs with explicit scalar assignments avoid reflection-based copying of newly added local fields.
- [OWASP DTO/allowlist guidance](https://cheatsheetseries.owasp.org/cheatsheets/Mass_Assignment_Cheat_Sheet.html)
  concerns input binding; applying the explicit-field DTO principle to egress is our design inference, not a claim
  that this source specifies telemetry uploads.
- [Open-source comparison](../architecture-research/007-open-source-alignment.md) supports local-first operation,
  explicit remote association and separating observation from management. No upstream custom crypto is copied.

## Proof, limitations and rollback

Schema negatives, Go fixture equality and input-boundary tests prove only this producer profile. Actual receiver tests
are required before transport activation. Network counters/history and richer workloads remain follow-ups; kernel and
load are explicitly uncollected. Filesystem ordinals have no stable resource meaning. Existing local data is unchanged.
No credentials, network or side effects are added. Removing this unused projector requires no migration. Later uploader
activation requires separate reviewed isolation, bounded storage/retry, revocation, consent and release evidence.
