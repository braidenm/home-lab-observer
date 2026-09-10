# ADR 011: Windows log continuation with an explicit collision limitation

Status: Accepted (owner approved on 2026-09-10).

## Context

Windows bookmarks position a reader within an event result set. Positioning alone does not establish that an event
retains the same identity after a channel is cleared and record IDs are reused. The documented log metadata includes
creation time, write time and record counts, but does not promise a clear-operation generation identifier. We do not
infer that creation time changes on every clear.

## Decision

For the fixed System and Application sources, seek strictly to offset zero at the private bookmark, then compare
the observed record's source, record ID, timestamp, event ID and provider GUID with the saved anchor. Optional identity
fields include explicit presence bits; malformed identity fields fail the attempt rather than fabricate equivalence.
Only a proved missing bookmark or a differing anchor establishes a reset. Generic native errors preserve progress
and report failure. A reset contributes no healthy coverage or invented event counts.

The owner accepts that clearing a channel and recreating an identical selected tuple can evade reset detection.
This is an operational dashboard, not a forensic, tamper-evident or complete audit system. We will document the
limitation in log-history help when activating the feature. We will not add undocumented generation heuristics or
expand collection to messages, provider names or other sensitive fields to imply stronger guarantees.

## Consequences

- Windows history can proceed without resetting on every poll, while normal identity changes still produce gaps.
- A hostile or unusually reconstructed event history can defeat the continuation proof; counts are observations,
  not evidence of completeness against such a host.
- This decision does not waive isolated helper deadlines, native fixture tests, privacy checks, packaged-runtime
  acceptance or explicit source opt-in. It does not authorize remote control or elevated log access.
- Stronger future audit requirements need a separate specification and an ADR superseding this decision.

## Alternatives

Deferring Windows log history avoids this limitation but does not meet the owner's current operational goal.
Resetting every poll discards useful continuity. Creation/write-time and count heuristics lack the required documented
generation guarantee and can incorrectly classify normal activity.

## Primary references

- [EvtSeek](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtseek)
- [Windows log metadata properties](https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_log_property_id)
- [Detailed reader contract](../../specs/009-native-log-metadata/windows-reader.md)
