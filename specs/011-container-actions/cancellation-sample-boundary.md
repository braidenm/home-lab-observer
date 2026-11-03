# Cancellation observed during a native timing sample

2026-11-03. Narrow review correction to the existing paired timing guard; no new permission, limit or API.

`CheckExecution`, `CheckMutation` and `CheckRecovery` must refuse when their original context is canceled
during the native clock capture, not only when it was canceled before capture. Recheck `ctx.Err()` after
capture in their shared private `checked` function, consistently with the existing admin/delivery/report
guards. Keep original timing origins, native continuity and equality deadlines unchanged.

Prove all three consumers with the existing private clock fixture: construct a valid real execution or
recovery context, prove its positive check, then cancel that same context inside the next capture. Each
check must return `ErrUnavailable`. Demonstrate RED before the one-condition correction, then run the
budget regression/race and affected guarded-engine/journal checks without larger timeouts or skips.

This closes an already-observed cancellation window. It does not make check-plus-system-call atomic,
prevent cancellation immediately after the check, undo a committed ACK or prove a daemon did not act.
Post-commit uncertainty remains retained evidence, never automatic retry or permission to dispatch.
