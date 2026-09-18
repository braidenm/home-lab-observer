# Connected operational status boundary

Status: implementation slice under Spec 012 and ADR 020. This is disposable
diagnostic state, not delivery, enrollment, or activation authority.

## Record and privacy

- A status record has one fixed version, a closed state vocabulary, bounded
  counters and timestamps, and an optional lowercase invocation identifier.
  Canonical JSON is at most 1 KiB. Unknown fields, raw errors, credentials,
  arbitrary labels and malformed bytes refuse.
- Freshness reports the age of the most recently acknowledged observation; it
  is not the upload receiver's longer admission or duplicate-delivery window.
  Missing or stale acknowledgements must not be represented as success.

## Private Linux writer

- Open only an exact 0700 directory owned by the current worker without ACLs.
  Hold one nonblocking private lease. Accept only fixed regular 0600 files with
  one link; refuse unknown entries, links, foreign ownership and loose modes.
- Keep one atomic, synchronized `status.json` slot and one
  `activation-response.json` slot. The uploader must encode the canonical
  activation response before publication; the shared writer bounds and copies
  opaque bytes without importing the uploader protocol into the collector.
  A dependency-closure test enforces that separation.
  The manager must still match the live request and invocation, and the worker
  its retained challenge. Slot contents alone never authorize a worker.
- On reopen, remove only validated disposable staging files left by an
  interrupted write. Repeated writes cannot grow an unbounded history.

Tests cover canonical status refusal, a locked concurrent writer, repeated
bounded replacement, crash staging recovery, unsafe entries and response-size
limits. The uploader's activation tests cover the response schema. This
package does not install units, select code, access the upload ledger or start
services. Packaged VM acceptance remains an open gate.
