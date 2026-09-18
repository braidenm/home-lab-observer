# Quality-aware native producer codec

Implementation slice, 2026-09-17; no runtime wiring or activation. This implements
the pure producer portion of the separately reviewed quality-aware producer plan.
`EncodeNative(Snapshot, Identity)` and `ValidateNative(bytes, expectedServerID)`
use `home-lab-native-host-metrics/v1`, matching Platform Demo's actual
`HomeLabNativeHostMetricsParser`. Existing `Encode`/`Validate` are unchanged.

## Closed mapping

CPU/uptime emit values only for available, complete data. Partial or missing
eligible data becomes unavailable/collection-failed. Memory's explicit degraded
partial-collection state retains valid RAM but emits null swap; other incomplete
memory is unavailable. Invalid eligible numeric values reject with fixed errors.

Unavailable reasons map only collection-failed, deadline-exceeded, canceled and
no-data to their fixed uppercase wire constants; unrecognized local text becomes
collection-failed. Disabled, permission-denied and unsupported map to their own
fixed reasons. Unknown/unrecognized states become unknown/not-yet-observed. No
local reason, source, path, process, network or filesystem name is copied.

Filesystems available with zero errors, no truncation and exact sample/total
counts is the collector's trusted complete state. Only that state emits a total.
Degraded samples always emit null total; collection-local totals are not whole-host
coverage. Truncation is exclusively the collector's configured-cap proof, and
requires total greater than returned samples (the cap can be smaller than 16).
Per-row failures alone must not set truncation. More than 16 rows is rejected,
not silently truncated. Empty degraded data becomes unavailable/no-data.
Unavailable/denied/unsupported sections carry no wire inventory: zero returned,
null total, false truncation and empty items, as required by the receiver. A local
cap with no usable samples remains local quality evidence, not an inventory claim.

The receiver's degraded null-total/truncated combination requires its separately
reviewed relaxation. The producer validator accepts only canonical bytes from
this producer, not every semantically valid receiver document. Ordinal filesystem
aliases are assigned afresh per observation. Unknown/duplicate/missing fields,
wrong binding, invalid nullable pairs, numeric bounds and quality combinations
are rejected; maximum output is 16 KiB, duration 10 seconds, integers 2^53-1.

## Verification and remaining integration

Pure tests cover complete/degraded/unavailable metrics, nullable zero, privacy,
all OS labels, numeric bounds, malformed/canonical documents and cap-versus-row
failure distinctions. Shared emitted fixtures must also pass the actual JVM
parser. Producer tests alone are not receiver, pending-ledger, handoff, transport,
installed or live acceptance; those integrations remain separate gated work.

Local Go 1.27.1 projection tests and vet passed. Shared complete and partial-cap
JSON fixtures are checked byte-for-byte against emitted canonical documents;
actual JVM fixture acceptance remains a separate integration check.
