# C1 receiver evidence and pure transition contract

Accepted for C1. Inspected Platform Demo baseline `3a9f54c58073e35339db84cb32625395b15f4a8b` on 2026-09-11:
`HomeLabEnrollmentEndpoint.kt` / `HomeLabEnrollmentService.ingest`, `HomeLabEnrolledSnapshotStore.kt`, and
`HomeLabEnrollmentApiTest.kt`. These are actual receiver implementation/tests, not a proposed new server protocol.

| Receiver fact | C1 transport result |
| --- | --- |
| PUT `/v1/connectors/home-lab/servers/{serverId}/snapshot`; `Authorization: Connector <credential>`; positive signed-long `X-Connector-Sequence` | Request contains exact binding/sequence/body; credential remains adapter-owned |
| HTTP 200 body has `server_id` and `sequence` | ACK only after both match the pending request; mismatched/malformed ACK is ambiguous retry, never success |
| Higher sequence replaces; equal sequence returns success without comparing old body | Persist immutable pending body before send; retry exact bytes |
| Lower sequence returns 409 | Persist terminal CONFLICT; no automatic sequence reset |
| Invalid snapshot/sequence or collection age greater than 300 seconds returns 400; age checked before duplicate lookup | Persist terminal REJECTED; proactively retire locally expired observations without sequence reuse |
| Invalid/revoked/cross-server credential returns 401 | Persist terminal CREDENTIAL_REJECTED |
| Rate limit returns 429 with Retry-After 60 | RATE_LIMITED; retain exact pending state and require the later worker to wait at least 60 seconds |
| Oversized request returns 413 (receiver limit 1 MiB) | REJECTED; C1 already caps profile at 16 KiB |
| Transport error, unexpected response, 5xx or uncertain delivery | RETRY with identical pending state |

The existing API test demonstrates equal-sequence success, lower-sequence conflict, 301-second expiry, unauthorized
cross-server use and revocation. Age-before-duplicate and absence of duplicate body comparison are code-inspection facts.
`internal/uploadstate/testdata/receiver-cases.json` records synthetic translated outcomes and drives C1 tests. This is
not a new execution of the JVM integration suite or an HTTP parser proof; those remain D2 acceptance requirements.

The machine performs one serialized Step, at most one send and at most three ledger commits (retire, admit, acknowledge).
Every ledger Commit is compare-and-swap against the complete previous record and reports success only after durable
commit; all commit errors are uncertain, latch RECOVERY_REQUIRED and prohibit subsequent work on that machine. New
machine creation loads/validates the authoritative ledger; it never initializes missing enrollment state. A loader error
also latches recovery. Persist terminal outcomes. Ledger records and request/source bytes are cloned at interface boundaries.
Cancellation leaves any durably admitted request pending; a canceled/failed acknowledgement commit requires recovery.

A failed source read reports fixed `SOURCE_UNAVAILABLE`, never `IDLE` or the underlying error. If the same step
already retired an expired pending request, `EXPIRED_DELIVERY_UNKNOWN` takes precedence; the next step can report
source unavailability. `IDLE` means an already acknowledged latest digest, not a failed handoff read.
Storage adapters share `ValidateRecord(record, expectedBinding)`, which checks the expected binding and complete
logical record invariants and returns only `ErrRecovery` on failure. `CloneRecord` detaches the pending body;
it does not validate or authenticate state. Adapters must bound their input and validate before cloning untrusted records.

C1 includes no credential acquisition, HTTP parsing, disk format, scheduler sleeps/jitter, worker or automatic recovery.
Send receives a child deadline capped at five seconds. Source/ledger receive the caller's context; their eventual adapters
and worker must impose bounded I/O/step deadlines. Adapters must honor cancellation; the synchronous primitive cannot
forcibly join a malicious/blocking adapter. Pending
expiry preserves the allocation watermark and unknown-delivery semantics. Restoring old external backups still requires
explicit re-enrollment, because this receiver cannot authenticate historical body equality for reused sequence numbers.

D2a freezes the unused snapshot adapter in [transport.md](transport.md). The transport and state machine never sleep or
retry internally. `RATE_LIMITED` is a distinct non-terminal outcome so a later worker cannot accidentally use the ordinary
1-second retry floor after the receiver explicitly requires 60 seconds.
