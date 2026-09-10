# Linux journal reader kernel

Status: Bounded implementation slice behind synthetic native seams. No systemd binding, helper entrypoint, process
launch, host journal query or release package is supplied by this slice.

`internal/journalreader` owns one read attempt on one fixed local system-journal handle. `Factory.OpenSystem` and the
returned `Journal` expose only realtime/cursor/tail positioning, next/previous, exact cursor testing, realtime,
priority, message ID, bounded cursor and close. There is no generic field, query, path, environment or command input.
The eventual native adapter fixes local/system flags and strips the code-owned field prefix before returning a field.
It must copy bounded native data before invalidating borrowed pointers and free native cursor allocations itself.

The reader locks its OS thread before open and retains that thread through close. No native handle escapes an attempt.
It checks a maximum two-second child context before and after native calls. These cooperative checks cannot interrupt
a stalled C call; the separate helper parent's enforced deadline/kill/reap remains mandatory before shipping.

Initial reads seek to query-start minus five minutes. Continuation seeks/visits/tests the exact saved cursor before
unseen rows; only an explicit invalid-cursor result or a false exactness test enters reset. Other native failures do
not invent stale-cursor evidence. Reset-pending requests perform only one tail probe. Stale continuation may consume
one verification probe plus one tail probe; establishment returns no counts or coverage. An initial empty window
requires an older visible tail/cursor before returning caught-up zero, otherwise `NO_VISIBLE_JOURNAL`.

Every visited row, including a probe and deferred sentinel, requires a nonempty cursor of at most 16 KiB. Cursor
acquisition failure rejects the complete attempt with no usable batch. Normal ingestion reserves the sentinel: at
most 512 rows without a continuation probe or 511 with one, and at most 513 total visits. Deferred rows are never
counted as captured/discarded and retain the previous committed candidate cursor. Only an actual end-of-source result
sets caught-up. Returned event/cursor bytes are owned copies, and expected failures expose code-owned reasons only.

Priority and message ID are the only selected string fields and each is bounded to 4 KiB. Valid MESSAGE_ID produces
the fixed SYSTEMD code; otherwise a valid priority permits the fixed priority fallback. Missing/unrecognized priority
with a valid message ID uses UNKNOWN severity; when neither field supplies a valid code the row is a known discard.
Oversized selected fields are malformed rows, not permission to request bodies. Native operation errors remain
separate from a missing selected field and never become healthy zero.

Native realtime is unsigned microseconds. Representable old backlog times are retained unchanged for Store retention
filtering. Times later than query-start plus the fixed two-second acquisition horizon, or outside RFC3339 years
1..9999, are invalid timestamps: discard the row at query-start after acquiring its cursor. Do not fabricate an event
code or severity for this invalid row. Small within-horizon timestamps retain their native time. Store independently
enforces the future horizon before persistence. An initial seek lower bound before the Unix epoch is an invalid request,
never an unsigned wrap. The fixed native realtime seek must be verified against synthetic journal fixtures before binding;
the kernel does not invent discard counts for native seek positioning outside the requested initial window.

Verification uses synthetic handles only: exact/nearest/empty cursors, reset persistence, initial tail evidence,
source refusal, selected-field canaries, all bounds and mapping cases, native-error versus stale behavior, cancellation,
close-once ownership, thread identity, and whole-attempt rejection after cursor failure. Real libsystemd compatibility,
native fixture journals, executable identity, private pipes and static-core packaging remain separate required slices.
