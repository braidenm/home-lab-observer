# D2a snapshot HTTPS transport

Status: implementation authorized for the unused snapshot adapter and upload-state result only. Enrollment exchange,
credential persistence, worker scheduling, installation and network activation remain out of scope.

## Boundary

Implement one `internal/platformtransport` adapter for the existing `uploadstate.Transport` port. The constructor accepts
one installer-selected canonical HTTPS origin and a credential provider. It derives only the fixed receiver path
`/v1/connectors/home-lab/servers/{server_id}/snapshot`; callers cannot provide paths, clients, proxies, transports, TLS
settings or redirect behavior. Production uses the normal platform CA roots. A package-private test seam may add only the
certificate pool for an owned `httptest` TLS server.

The credential provider receives the expected `uploadstate.Binding` and returns the scoped connector credential. The
adapter validates `hlc_` plus 43 base64url characters before constructing the request. Credential bytes never enter the
state machine, ledger, request DTO, URL, query, error, diagnostic or log. The provider is a port for later private storage;
D2a does not implement that storage or enrollment.

## Request and response contract

- Every direct `Send` call validates the binding, positive signed-64-bit sequence, canonical numeric-host body, body/source
  identity and 16 KiB request cap before loading a credential or opening a connection. It enforces a five-second child
  deadline even outside `uploadstate.Machine`.
- Send exactly `PUT`, `Accept: application/json`, `Content-Type: application/json`, `Authorization: Connector <credential>`
  and decimal `X-Connector-Sequence`. Do not add cookies, referers or origin-dependent headers.
- Disable inherited proxy configuration and response compression. Verify normal HTTPS certificates, reject redirects, and
  cap response headers and body. A redirect is an ambiguous retry and its target receives neither body nor credential.
- A 200 response is acknowledged only when its media type is JSON and one bounded JSON object contains valid `server_id`
  and positive `sequence` fields matching the pending request. Reject a BOM, trailing JSON, duplicate keys (including an
  escaped spelling of the same key), missing/wrong known fields, unsafe numbers and oversized bodies. Bounded additive
  unknown fields are ignored and never retained, logged or reflected.
- Map 400 and 413 to `REJECTED`, 401 to `CREDENTIAL_REJECTED`, 409 to `CONFLICT`, and 429 to `RATE_LIMITED`. Map malformed
  or mismatched 200, 408, redirects, other unexpected statuses, 5xx, TLS/network/deadline failures and invalid response
  framing to retry. Response bodies cannot override status classes.
- Return fixed errors only. Never expose raw network/parser errors, response bodies, URLs, headers or credentials.

`RATE_LIMITED` is non-terminal. The machine retains the exact pending body and performs no acknowledgement or terminal
commit. The later worker must wait at least the receiver's fixed 60 seconds before another send; neither the transport nor
state machine sleeps or retries internally.

## Enrollment boundary reserved for D2b

The one-use enrollment exchange is deliberately not implemented here. It will use the same fixed-origin, TLS, proxy,
redirect and response-bound policy, but has different ambiguity semantics: once request bytes may have been written, a
lost/invalid response cannot be replayed safely because the receiver may already have consumed the grant. Such an outcome
must stop for explicit owner revocation and a fresh enrollment. Only a failure proven before any request write may retry.
The uploader must durably establish the returned binding and private credential before a worker starts.

## Acceptance

Synthetic TLS tests prove exact method/path/headers/body, normal certificate rejection and package-private owned-root trust,
proxy-environment isolation, redirect refusal, response header/body limits, caller and adapter deadlines, cancellation,
source validation before credential/network access, every status mapping, strict acknowledgement correlation, duplicate and
escaped duplicate rejection, trailing/BOM rejection, and additive unknown-field discard. State-machine tests prove 429
returns `RATE_LIMITED`, preserves the pending record, performs no extra ledger commit and does not become terminal.

No test contacts a real receiver, reads a host observation, installs a credential or changes local/remote runtime behavior.
Platform Demo's raw JSON receiver proof and the disposable cross-repository HTTPS acceptance remain later gates.
