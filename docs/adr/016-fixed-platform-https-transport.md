# ADR 016: Fixed Platform HTTPS transport and separate enrollment ambiguity

Status: Accepted for the unused D2a snapshot adapter. Enrollment coordination and activation remain Proposed.
Date: 2026-09-11.

## Context

The connected-observation state machine deliberately owns durable sequence and acknowledgement decisions without owning a
credential or network client. The existing receiver has fixed exchange/upload routes, returns a correlated JSON
acknowledgement, accepts an equal sequence without comparing its previous body, rate-limits uploads for 60 seconds, and
consumes enrollment grants before returning the connector credential. A general URL/client injection surface would weaken
the reviewed destination, credential and redirect boundaries.

## Decision

Use a separate snapshot transport package with a narrow constructor: one trusted installer-selected canonical HTTPS origin
and one credential provider. Derive the upload path from the validated server binding. Production owns its HTTP transport,
uses normal CA verification, ignores environment proxies, disables compression, refuses every redirect and applies fixed
five-second, 16 KiB body and bounded-header limits. There is no exported arbitrary client, proxy, TLS override or request
path. Tests may privately add an owned TLS root and nothing else.

The credential provider is called with the complete expected server/connector binding. The adapter validates its returned
`hlc_` credential, uses it only in the connector authorization header, and returns fixed result/error classes. Credentials
never enter `uploadstate.Request`, the durable ledger, diagnostics or ordinary errors.

Decode a successful acknowledgement defensively while preserving additive compatibility: require and strictly validate the
two known fields and their correlation, reject duplicate/trailing/oversized framing, ignore unknown bounded fields, and do
not retain the decoded document. HTTP statuses are mapped exactly as frozen in the receiver contract. A 429 becomes a new
non-terminal `RATE_LIMITED` machine outcome; the future scheduler, not the adapter, waits at least 60 seconds.

Enrollment is a separate operation and state boundary. Snapshot retry safety comes from the durably immutable pending
sequence/body. Enrollment has no equivalent receiver recovery: after its request may have been written, failure is
ambiguous and automatic replay is prohibited. Later enrollment work must distinguish proven pre-write failure from ambiguous
delivery, durably commit binding/credential before worker start, and require explicit revocation plus a new grant after an
ambiguous exchange.

## Alternatives

| Alternative | Benefit | Rejected consequence |
| --- | --- | --- |
| Caller-provided `http.Client` or upload URL | Flexible tests/deployments | Proxy, redirect, TLS and destination policy become ambient configuration |
| Put credential in machine binding/request | Simple adapter | Secret enters durable/control state and crosses unrelated boundaries |
| Treat 429 as generic retry | No new outcome | Worker cannot enforce the receiver's explicit minimum delay |
| Automatically replay ambiguous enrollment | Convenient recovery | Can consume a second response from an already-consumed one-use grant and hide an orphaned credential |

## Consequences and proof

The adapter remains useful only with a later private credential provider and worker. It cannot target arbitrary compatible
servers or custom certificate authorities in production. Enterprise proxy/custom trust support requires a separately
reviewed destination policy rather than environment inheritance. Tests use only owned synthetic TLS servers and can prove
wire behavior, not receiver deployment compatibility. Actual raw receiver JSON tests, credential persistence, enrollment
recovery, installed isolation and disposable canary acceptance remain mandatory before activation.
