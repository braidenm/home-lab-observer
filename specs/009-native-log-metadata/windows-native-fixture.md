# Owned Windows native event fixture proposal

Status: PROPOSED, unexecuted prototype. The test-only manifest, publisher, EVTX seam and guarded workflow exist on
the prototype branch, but no registration workflow has been pushed or run and no native support claim exists yet.
Activation requires a separate review and successful hosted evidence.

## Purpose and boundary

The fixture should prove that the fixed Windows binding works against real WEVTAPI handles without reading,
exporting, writing, or clearing a developer, production, `System`, `Application`, or `Security` log. Existing unit
tests remain the evidence that public sources map only to the literal `System` and `Application` channel names and
that callers cannot provide a path, query, session, host, flag, or field. Native fixture code must not weaken that
production interface.

Microsoft supports querying either a channel or a log file, with mutually exclusive flags. The prototype should add
an unexported `_test.go`-only factory that accepts one already-validated owned EVTX path and uses
`EvtQueryFilePath`; production continues to use only `EvtQueryChannelPath`.
[EvtQuery](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtquery),
[query flags](https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_query_flags)

The test fixture is generated only on an ephemeral GitHub-hosted Windows runner. A checked-in synthetic manifest
defines one fixed provider GUID and two owned custom channels representing the fixture's logical system and
application cases. A tiny test-only publisher registers that provider with `EventRegister` and writes fixed event
descriptors with `EventWrite`. Manifest installation and removal use `wevtutil im` and `wevtutil um`. The manifest,
publisher and resource generation must follow Microsoft's supported manifest-provider process; no hand-authored EVTX
writer or binary fixture copied from a real machine is acceptable.
[developing a provider](https://learn.microsoft.com/en-us/windows/win32/wes/developing-a-provider),
[writing manifest events](https://learn.microsoft.com/en-us/windows/win32/etw/writing-manifest-based-events),
[wevtutil](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/wevtutil)

The generator exports exact synthetic provider/event-ID selections into private EVTX files before and after clearing
only an owned fixture channel. `EvtExportLog` is non-destructive and supports exact XPath selection; its target must
be a new absolute path. Exported files are never uploaded or committed.
[EvtExportLog](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtexportlog),
[saving event logs](https://learn.microsoft.com/en-us/windows/win32/wes/saving-events-to-a-log-file)

## Required native evidence

The prototype must fail unless all of the following are proved using the real owned fixture:

1. The binding calls `EvtCreateRenderContext` with the existing five exact value paths and `EvtRenderContextValues`.
   It must not substitute `EvtRenderContextSystem`, full event XML, `EvtFormatMessage`, EventData, UserData, provider
   names, computer identity, user identity, or message bodies. The returned variants must have the exact native
   types expected by the implementation: TimeCreated is `EvtVarTypeFileTime`, Level is `EvtVarTypeByte`, EventID is
   `EvtVarTypeUInt16`, the manifest provider GUID is `EvtVarTypeGuid`, and EventRecordID is `EvtVarTypeUInt64`.
   Their fixed synthetic values and owned-copy behavior must be asserted. This is native evidence, not an inference
   from the broader system render context.
   [rendering selected values](https://learn.microsoft.com/en-us/windows/desktop/WES/rendering-events),
   [EVT_VARIANT](https://learn.microsoft.com/en-us/windows/win32/api/winevt/ns-winevt-evt_variant),
   [system property identifiers](https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_system_property_id)
2. Forward queries return the owned records oldest-first; reverse queries return the owned tail first. End of file is
   accepted only as `ERROR_NO_MORE_ITEMS`, never timeout or another native error.
3. A bookmark is created, updated from an owned event, rendered to the private bounded anchor, recreated after every
   handle is closed, and used for strict offset-zero seek. The next record must be the bookmarked record and the
   selected anchor tuple must match. Microsoft documents this create/update/render/recreate sequence.
   [bookmarking events](https://learn.microsoft.com/en-us/windows/win32/wes/bookmarking-events)
4. Seeking the first snapshot's bookmark in a later snapshot that does not contain that event must fail strict seek
   rather than clamp to a nearby record. If clearing the owned channel reuses a RecordID, a different fixed event ID,
   timestamp or provider GUID must make anchor comparison false. The full `eventreader.Reader` path must then return
   reset-established with zero captured/discarded counts and the later owned tail checkpoint. `EvtSeekStrict` is the
   required missing-event behavior; non-strict seek is deliberately forbidden.
   [seek flags](https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_seek_flags),
   [Windows Event Log errors](https://learn.microsoft.com/en-us/windows/win32/wes/windows-event-log-error-constants)
5. A test wrapper records the native thread ID for query, render-context creation, next, seek, bookmark, render and
   close. One reader attempt must use one locked OS thread through the final close. Every returned native handle must
   be closed on success, reset, error and cancellation. Microsoft requires the query handle to remain on its creating
   thread.
6. The native test runs as a child with a 30-second external deadline and confirmed kill/reap fallback. A cooperative
   Go context is not evidence that a blocked WEVTAPI call stopped.

The same required test runs without skip on the supported GitHub-hosted Windows x64 and ARM64 runners. A single
architecture or mocked syscall test is not sufficient for a two-architecture support claim. The fixture job should
remain below the repository's 15-minute CI limit.

## Authority and cleanup guardrails

The generator must fail closed unless all of these conditions hold:

- `GITHUB_ACTIONS=true` and `RUNNER_ENVIRONMENT=github-hosted` are present, the OS is Windows, and the owned root is a
  newly created canonical directory beneath `RUNNER_TEMP`.
- The process has the authority required to install and remove the synthetic provider. Lack of authority is a failed
  required job, not a skip or a reason to broaden permissions.
- The fixed provider GUID/name and both fixed custom channel names are absent before installation. The test never
  adopts, overwrites, clears, or removes a pre-existing registration.
- Every export path is a new regular file beneath the owned root, with a fixed count and size cap. Native or command
  output is bounded and must not include event XML, bookmark XML, payloads, account names, host names or raw errors.
- No test API can name `System`, `Application`, `Security`, an arbitrary channel, an arbitrary EVTX outside the owned
  root, a remote session, or caller-provided XPath. Production code receives no file-path seam.

Cleanup targets are exact: close all WEVTAPI handles; stop and reap the test publisher; uninstall only the fixture
manifest/provider and its two custom channel registrations using `wevtutil um`; and remove only the newly created
`RUNNER_TEMP` root containing the manifest copy, generated resource/publisher files, marker and fixed EVTX snapshots.
The test must not delete Event Log service files or registry keys directly and must never clear `System`,
`Application`, `Security`, or any channel it did not create. Cleanup runs in `finally`; an unconfirmed unregister,
child reap or owned-directory cleanup fails the job and must not be reported as native acceptance.

## Honest coverage gaps and prototype gate

This fixture would prove the real Windows ABI, selected render types, bookmark mechanics, strict seek/reset behavior,
thread lifetime and private-file continuation. Because it deliberately never queries production channels, it does
not natively prove that a particular user's live `System` or `Application` permissions contain readable events;
those remain runtime availability/permission states. Exact fixed-channel names and flags remain independently checked
by closed unit tests.

Microsoft does not document a stable channel-generation identifier or promise that log creation metadata changes on
clear. The accepted anchor can detect normal missing records and reused IDs whose selected tuple differs, but cannot
mathematically exclude recreation of an identical RecordID, timestamp, EventID and provider GUID. The fixture must not
claim otherwise.

There is no supported offline EVTX writer in the selected API surface. The manifest/resource/publisher setup is thus
the first prototype gate. It must demonstrate real GUID-bearing events, exact selected-context variant types and
reliable cleanup on both hosted architectures before the fixture or Windows native source can be called verified.
`eventcreate` may be useful for an exploratory null-GUID comparison, but cannot replace the required GUID path.
