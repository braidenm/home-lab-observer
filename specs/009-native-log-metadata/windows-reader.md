# Windows Event Log reader kernel

Status: Windows amd64/arm64 WEVTAPI binding over the accepted synthetic kernel. The owner accepted the documented
private-anchor collision limitation on 2026-09-10; no host-log fixture read, helper entrypoint or activation is supplied. The
same-binary hidden helper and its enforced kill/reap deadline remain separate requirements; a cooperative context
cannot interrupt a stalled native call.

## Primary API evidence and fixed seams

Microsoft requires query handles to stay on their creating thread and to be closed; therefore each reader attempt
locks one OS thread before opening anything and retains it through every record/query close.
[EvtQuery](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtquery)

The factory exposes only `OpenInitial(source, lowerFileTime)`, `OpenContinuation(source)` and `OpenTail(source)`.
The source is validated `system` or `application`, mapped by the binding to local System or Application
channel constants. Initial uses a code-owned time predicate, continuation an unfiltered forward channel query,
and tail a fixed reverse query. There is no caller path, XML, XPath, session, flags, remote host or command.

Continuation seeks offset zero relative to the bookmark with strict behavior, then visits and verifies that exact
record before moving forward. Offset one is not used for the presence proof: being at the current tail does not
mean the bookmarked row was lost. The API documentation describes strict failure when the target is absent and
also record-ID-relative behavior when a bookmark is outside a filtered result. The unfiltered exactness seam must
not infer identity from mere seek success.
[EvtSeek](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtseek),
[seek flags](https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_seek_flags)

`Query.Next` returns one owned record, or nil only for native `ERROR_NO_MORE_ITEMS`. Timeout is not exhaustion.
Every returned record is closed even if returned alongside an error or cancellation. Tail reads at most one record.
[EvtNext](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtnext)

`Record` exposes only TimeCreated FILETIME, Level, EventID, optional provider GUID, private bookmark/exactness, and
close. The binding renders only those selected values using a fixed render context, never all system fields,
full event XML, formatted messages or provider names. Native null/wrong type/oversized values have separate bounded
errors. No borrowed native buffer crosses a subsequent native call. Level is uint8, EventID uint16 (widened into the
existing uint32 code grammar), GUID is canonical UUID-byte order, and FILETIME is uint64.
[render context](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtcreaterendercontext),
[selected rendering](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender),
[property types](https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_system_property_id)

## Bookmark proof and intentionally unresolved binding work

The persisted private bookmark must include a bounded anchor, at least channel, record identity and original event
timestamp, plus available generation evidence. `BookmarkMatches` compares the saved anchor with the bounded anchor
already acquired by `Bookmark`; it must acquire no further native data. XML byte equality or RecordID alone is not
proof after clearing/reusing IDs. Bookmark construction, canonical private format, size-before-copy checks and exact
native error mapping require a separately reviewed binding and synthetic native fixture, not assumptions here.
[bookmark API workflow](https://learn.microsoft.com/en-us/windows/win32/wes/bookmarking-events)

### Accepted private anchor v1 (operational, not forensic)

The native binding uses a closed binary envelope, never public JSON: eight-byte `HLOWEV1\0` magic; one-byte fixed
source (`system=1`, `application=2`); one-byte flags with only TimeCreated-present, EventID-present and
provider-GUID-present bits 0..2; two reserved zero bytes; big-endian EventRecordID, TimeCreated FILETIME and EventID;
sixteen canonical UUID-order provider-GUID bytes; a big-endian 32-bit UTF-8 bookmark-XML length; that exact
WEVTAPI-rendered private bookmark;
and a 32-byte SHA-256 integrity digest over every preceding byte. The complete envelope is nonempty and at most
16 KiB. Decode rejects unknown versions/flags, nonzero reserved bytes, wrong source, zero RecordID, invalid
UTF-8/NUL, length/trailing-data mismatch and digest mismatch before calling `EvtCreateBookmark`. The XML is needed only
to reconstruct WEVTAPI's bookmark handle; it is never logged, rendered publicly, or compared for exactness.

EventRecordID is mandatory and a missing, malformed or out-of-range value fails the attempt. A native Null
TimeCreated, EventID or provider GUID has its present bit clear and canonical value bytes zero; exactness compares
both presence and value. This lets a validly bookmarkable row with a missing selected public field be discarded and
advanced without inventing that field. A wrong native type, range, pointer or attributed oversize for any identity
field fails the attempt because equating different malformed raw values is not proof; malformed native bytes are
never persisted. Level is deliberately absent from the identity anchor: it affects severity normalization but does
not strengthen the fixed record identity. An overall render/protocol/combined-size failure also fails the attempt.

After strict offset-zero seek, the verification record first creates its own bounded anchor. Exactness compares the
saved versus newly selected fixed source, EventRecordID, presence-qualified TimeCreated, EventID and provider GUID; it
does not compare bookmark XML bytes. This detects normal record-ID reuse with different selected identity. Microsoft
documents no stable channel-generation identifier, however, and does not state that `EvtLogCreationTime` changes on
`EvtClearLog`. Therefore a cleared channel recreating an identical tuple can evade reset detection. On 2026-09-10
the owner accepted this documented limitation for an operational dashboard, not forensic or tamper-evident auditing.
The binding must not claim absolute generation proof or substitute creation/last-write/count/oldest-record heuristics.
This decision does not waive native fixture, process isolation, packaging or release acceptance requirements.

Only `ErrBookmarkStale` during strict positioning or false exactness establishes reset. A missing probe row after a
successful seek is a failed attempt, not independent stale proof. Generic `ERROR_EVT_QUERY_RESULT_STALE`, timeout,
invalid position or permission errors do not establish bookmark loss: Microsoft describes query-result staleness
as a result-set problem requiring recreation. Next attempts reopen the query without silently dropping progress.
[Windows event errors](https://learn.microsoft.com/en-us/windows/win32/debug/system-error-codes--12000-15999-)

A proved stale bookmark closes the forward query and performs the same-attempt reverse tail probe. Tail success
establishes a new checkpoint with zero counts and no coverage; empty tail stays reset-pending. Pending requests do
only that probe. Initial empty windows also need visible tail evidence; a completely invisible/empty channel returns
`UNAVAILABLE/NOT_RUN/NO_VISIBLE_JOURNAL`, reusing the accepted native-source reason without claiming healthy zero.
These are accessible-channel observations, not complete audit or malicious-host/forensic guarantees.

Initial tail proof additionally reads the selected TimeCreated value and requires it strictly before the exact q−5m
lower bound. A newly arrived in-window tail after filtered-query EOF must not become a skipped checkpoint and false
zero. Missing/malformed/in-window tail time fails the whole attempt without advancing or inventing discard counts.
This probe reserves an additional 4-KiB field allowance and charges the successful 8-byte FILETIME; reset tails need no
timestamp selection beyond their bounded private anchor. Native fixture validation of query/tail races remains required.

## Bounds and normalization

The fixed two-second context covers all opens, positioning, reads and closes on one thread. Parent shutdown returns
no batch. Every visited event, including probes and sentinel, needs a nonempty owned private bookmark at most 16 KiB;
failure rejects the whole attempt. There are at most 513 visits; initial ingestion permits 512 rows plus sentinel,
continuation 511 plus verification probe plus sentinel. Captured and discarded rows share the ingestion quota.

An independent 2 MiB native budget charges returned bookmark/anchor bytes and successful typed selected values
(8-byte FILETIME, 1-byte Level, 2-byte EventID, 16-byte GUID). The bookmark bound includes the complete private anchor;
the binding must not acquire unaccounted anchor data. Reserve 16 KiB plus four 4-KiB field allowances before
each ingestion visit; ordinary probes reserve 16 KiB. A size-probe `ErrFieldTooLarge` returns no buffer, charges 4 KiB and discards
the row. Byte-limited valid prefixes return PARTIAL/RESPONSE_TOO_LARGE with their cursor and no extra visit/EOF claim.
The separate encoded helper-response 2 MiB cap remains independent.

FILETIME counts 100-ns ticks from 1601-01-01 UTC. Conversion divides before scaling, avoiding signed nanosecond overflow.
Initial q−5m rounds upward to the next 100-ns tick; a lower bound before 1601 fails closed without unsigned wrap.
Representable old backlog retains its event time for Store retention; missing, malformed or later-than-q+2s timestamps
are discarded at q. No pre-window row is included by rounding down.
[FILETIME](https://learn.microsoft.com/en-us/windows/win32/api/minwinbase/ns-minwinbase-filetime)

Level 1→CRITICAL, 2→ERROR, 3→WARN, 4→INFO, 5→TRACE; zero/missing/unknown→UNKNOWN. A native Null TimeCreated or
EventID is bookmarkable with explicit absence and then discards; a wrong-type/range identity field fails the attempt
because its exact continuation identity is unprovable. Valid EventID plus an absent GUID uses WIN_id, or a valid GUID
uses WIN_32lowerhex_id. An attributed oversized non-identity field discards the whole row; identity or combined-render
oversize fails the attempt rather than accepting fallback. Native operation errors reject the tentative prefix with fixed reasons.
No arbitrary native error, bookmark, name or body enters public events, errors, or diagnostics.

## Verification

Synthetic tests precede implementation: both fixed sources, forbidden source refusal, initial precise lower bound,
exact/nearest/reused-anchor outcomes, strict stale versus generic failures, current-tail continuation, reset-only
tail, empty initial proof, per-record/query ownership including cancellation, normalization and private canaries,
FILETIME/field/bookmark/row/byte edges, multiple byte-limited batches without replay, and cloned outputs.
Synthetic binding tests cover strict flags/error mapping, selected render types, canonical GUID byte order, fixed
queries and per-handle ownership. Owned native fixture tests for clear/reuse, permissions and thread affinity, plus
the helper's process kill/reap proof, remain separate and must precede activation.
