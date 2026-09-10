# Windows binding review — 2026-09-10

The owner accepted the operational continuation limitation in ADR 011. The native binding was independently reviewed
for fixed channels and selected fields, bounded private anchors, strict offset-zero continuation, scalar parsing,
pointer bounds, native ownership and closed errors.

The review identified and corrected three issues before submission:

- Byte/UInt16 values now read only the active union member; inactive upper storage is not treated as value overflow.
- Failed EvtNext calls close a returned nonzero handle; contradictory EOF output cannot claim exhaustion.
- Render size-probe permission and unavailable errors retain their fixed classification; allocation still requires
  the expected insufficient-buffer response and a bounded positive size.

Synthetic syscall-result regressions exercise these paths without querying native logs. Root independently reviewed
the fixes and reran the Windows eventnative/eventreader suites ten times, full Go tests, vet and repository policy;
all passed. Windows arm64 test binaries compiled before the review fixes; CI remains responsible for updated target
validation. No developer or production logs were read or cleared.

This slice does not activate collection. Actual owned native fixtures, same-binary helper composition, packaged
runtime checks and final release acceptance remain required; passing these synthetic tests does not satisfy them.
