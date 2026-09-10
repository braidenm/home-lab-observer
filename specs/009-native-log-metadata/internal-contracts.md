# Spec 009 internal Go contract proposal

Status: Accepted design for executable contract validation. Spec 008 is merged; these are ports and invariants.

## Package boundary

`internal/logobs` owns the sanitized domain types, validation, reader/store ports, bounded current cache, and independent
collector loop. Native adapters and `internal/history` may import `logobs`; `logobs` imports neither history,
projection, localapi, nor OS adapter packages. Projection and localapi consume read-only logobs views. This prevents a
history/projection cycle and keeps native payload types outside shared contracts.

No type below contains a message, native XML/JSON, provider name, identity, path, PID, unit, executable, hash, or
arbitrary attribute. Opaque checkpoints are private persistence/native-reader inputs only and must never implement
JSON/text formatting or reach API, UI, diagnostics, errors, or logs.

## Sanitized values

```go
package logobs

type Source string
const (
    SourceSystem      Source = "system"
    SourceApplication Source = "application"
)

type Severity string
const (
    SeverityTrace    Severity = "TRACE"
    SeverityDebug    Severity = "DEBUG"
    SeverityInfo     Severity = "INFO"
    SeverityWarn     Severity = "WARN"
    SeverityError    Severity = "ERROR"
    SeverityCritical Severity = "CRITICAL"
    SeverityUnknown  Severity = "UNKNOWN"
)

type SupportState string
const (
    SupportSupported SupportState = "SUPPORTED"
    SupportDisabled SupportState = "DISABLED"
    SupportUnavailable SupportState = "UNAVAILABLE"
    SupportPermissionDenied SupportState = "PERMISSION_DENIED"
    SupportUnsupported SupportState = "UNSUPPORTED"
)
type CollectionState string
const (
    CollectionOK CollectionState = "OK"
    CollectionPartial CollectionState = "PARTIAL"
    CollectionFailed CollectionState = "FAILED"
    CollectionNotRun CollectionState = "NOT_RUN"
)
type Freshness string
const (
    FreshnessCurrent Freshness = "CURRENT"
    FreshnessStale Freshness = "STALE"
    FreshnessUnknown Freshness = "UNKNOWN"
)
type ReasonCode string         // code-owned ^[A-Z0-9_]{1,64}$

type Event struct {
    ObservedAt time.Time // non-zero UTC
    Source     Source
    Severity   Severity
    EventCode  string    // one accepted native code form, <=64 bytes
}
func (Event) Validate() error

type Status struct {
    SupportState    SupportState
    CollectionState CollectionState
    Freshness       Freshness
    ObservedAt      *time.Time // query start of latest committed captured-or-caught-up success; includes caught-up empty
    AttemptedAt     *time.Time // latest attempted collection
    CoverageThrough *time.Time // latest caught-up query-start watermark, not contiguous proof
    ReasonCode      *ReasonCode
}
```

`Event.Validate` enforces exact source/severity/code forms and UTC. Native codes are only `WIN_<uint32>`,
`WIN_<32-lowercase-hex-guid>_<uint32>`, `SYSTEMD_<32-lowercase-hex-id>`, or `SYSTEMD_PRIORITY_<0..7>`.
Counters reject increments beyond 9007199254740991 before persistence.

Historical gap reduction accepts this closed `ReasonCode` precedence, highest first:

```go
const (
    ReasonCheckpointReset       ReasonCode = "CHECKPOINT_RESET"
    ReasonPermissionDenied      ReasonCode = "PERMISSION_DENIED"
    ReasonDeadlineExceeded      ReasonCode = "DEADLINE_EXCEEDED"
    ReasonInvalidResponse       ReasonCode = "INVALID_RESPONSE"
    ReasonResponseTooLarge      ReasonCode = "RESPONSE_TOO_LARGE"
    ReasonReaderFailed          ReasonCode = "READER_FAILED"
    ReasonBacklogDeferred       ReasonCode = "BACKLOG_DEFERRED"
    ReasonMissedCollection      ReasonCode = "MISSED_COLLECTION"
    ReasonNotYetObserved        ReasonCode = "NOT_YET_OBSERVED"
    ReasonLogStorageUnavailable ReasonCode = "LOG_STORAGE_UNAVAILABLE" // status only
)

func HistoricalReasonRank(ReasonCode) (rank uint8, ok bool)
```

The first eight reuse established code meanings. `NOT_YET_OBSERVED` is the new projection-only fallback for a fixed
historical cell with neither positive coverage nor persisted gap evidence; it is not stored as a gap. For a bucket with
multiple persisted gap reasons, the first present code wins. `HistoricalReasonRank` returns `ok` only for those nine
historical codes and a lower rank wins. There is no lexical/arbitrary fallback. This precedence does not replace the
latest-status aggregation matrix. `LOG_STORAGE_UNAVAILABLE` is a separate new status-only reason for a definite
persistence-write failure visible in the current process; it is never a historical gap reason.

Additional closed source-status reasons are `NO_VISIBLE_JOURNAL` (no visible initial journal evidence),
`LOG_HELPER_UNAVAILABLE` (missing helper/loader/library/required symbol), and `LOG_HELPER_MISMATCH`
(digest/build/protocol identity mismatch). Their committed attempt gaps use `READER_FAILED`; latest status preserves
the specific code. `PLATFORM_UNSUPPORTED` likewise maps to `READER_FAILED` if an unavailable attempt is committed.
`LOG_SOURCES_DISABLED` and `SOURCE_PARTIAL` are aggregate/cache codes, never persisted gap reasons. Endpoint absence
uses `LOG_SUMMARY_UNAVAILABLE` Problem Details, not a fabricated historical response. No raw native error is exposed.

## Checkpoint and bounded reader

```go
const (
    MaxSources                   = 2
    MaxAcceptedEvents            = 512
    MaxExaminedEvents            = 513 // all accepted/discarded rows plus optional deferred lookahead
    MaxProbeEvents               = 2   // reset-only maximum; NORMAL permits at most one
    MaxSourceBytes               = 2 << 20
    MaxNativeFieldBytes          = 4 << 10
    MaxCheckpointBytes           = 16 << 10
    MaxRecentEvents              = 200
    MaxCoverageSegmentsPerCommit = 2
)

type Checkpoint struct {
    Revision          uint64
    ResetPending      bool
    Opaque            []byte     // deep-cloned, <=16KiB; nil while reset is pending
    PreviousAttemptAt *time.Time // every last durably committed attempt, success or failure
    CoverageThrough   *time.Time // latest caught-up query-start watermark; status only
}
func (Checkpoint) Validate() error

type ReadRequest struct {
    Source         Source
    Checkpoint     Checkpoint
    QueryStartedAt time.Time // collector-owned UTC clock captured before Read
}

type BatchKind string
const (
    BatchNormal           BatchKind = "NORMAL"
    BatchResetPending     BatchKind = "RESET_PENDING"
    BatchResetEstablished BatchKind = "RESET_ESTABLISHED"
)

type DiscardCount struct {
    At    time.Time // valid event time, or QueryStartedAt when timestamp is missing/invalid
    Count uint32
}

type Batch struct {
    Kind             BatchKind
    Source           Source
    ExpectedRevision uint64
    QueryStartedAt   time.Time
    StartedAt        time.Time
    FinishedAt       time.Time
    SupportState     SupportState
    CollectionState  CollectionState
    ReasonCode       *ReasonCode
    Events           []Event
    Discards         []DiscardCount
    ExaminedCount    uint32
    ProbeCount       uint32 // internal-only cursor/tail visits; excluded from public JSON
    DiscardedCount   uint32
    Deferred         bool
    CaughtUp         bool
    NextOpaque       []byte
}
func (Batch) Validate() error

type Reader interface {
    Read(context.Context, ReadRequest) (Batch, error)
}
```

`Checkpoint.Validate` accepts `Revision > 0` with nil `Opaque`; that is a valid initialized empty source, not reset
evidence, but every `Revision > 0` checkpoint requires non-nil `PreviousAttemptAt` because every committed attempt
advances both together. Revision zero has no opaque value, reset state or attempt/coverage timestamps. `Opaque` and
both optional timestamps are deep-cloned at every port boundary. Standalone validation requires
each non-nil timestamp to be non-zero UTC, requires `CoverageThrough <= PreviousAttemptAt`, and rejects
`CoverageThrough` when `PreviousAttemptAt` is nil. `ReadRequest` validation additionally requires both prior timestamps
to be no later than its UTC `QueryStartedAt` and requires `QueryStartedAt > PreviousAttemptAt` when the latter exists;
Store applies the same comparison between a batch and the checkpoint loaded under CAS. `PreviousAttemptAt` is the
query-start time of every durably committed attempt, including a failed or reset attempt. `CoverageThrough` changes
only after a `NORMAL` caught-up proof and is retained across failure/reset; it is the latest successful watermark shown
in status, never the start of a positive coverage interval.

Fixed adapters, not callers, enforce the two-second per-source deadline and byte/line/native-handle bounds. The
collector wraps both enabled sources in a four-second overall deadline. Expected platform, permission, deadline,
query, malformed-input and reset outcomes return a closed validated `Batch` with a code-owned reason, so they can be
CAS-committed as attempts. A non-nil error means no trustworthy batch escaped. Unless the collector's parent context
was cancelled for shutdown, the collector replaces it with a fixed `UNAVAILABLE/FAILED/READER_FAILED` batch with no
events, discards, cursor advance, or caught-up proof and never exposes the error text. Its kind is `RESET_PENDING` iff
the loaded checkpoint was already pending, otherwise `NORMAL`; therefore a generic reader error cannot accidentally
clear reset state or become a normal success. Shutdown cancellation joins the reader/native child and commits nothing,
so it cannot race the shared Store closing.

Adapter construction receives only validated code-owned configuration, never a request-selected executable/path.
Linux uses ADR 010's separately bundled helper and Windows its fixed same-binary helper. Both use bounded stdin/stdout
pipes and discarded stderr, not cursor staging files. Private request framing is limited to 32 KiB, complete output to
2 MiB including protocol overhead. Native helper identity and environment checks precede execution. Missing Linux
loader/library/helper degrades only logs. No cursor contents enter argv or environment.

Linux acquisition has an independent two-MiB cumulative native-metadata budget in addition to the encoded-response
bound: charge each visited bounded cursor, eight bytes per successful realtime value, and selected priority/message-ID
values. Reserve worst-case headroom before the next visit (16-KiB cursor, realtime, two 4-KiB fields). If another visit
cannot fit, a valid processed prefix returns `PARTIAL/RESPONSE_TOO_LARGE`, `Deferred=false`, `CaughtUp=false`, preserving
its cursor so repeated attempts make progress. A no-prefix failure advances nothing. Oversized selected fields use a
no-payload `ErrFieldTooLarge` native result and become known discards, never fallback events; conservatively charge their
full field allowance. Initial realtime seek rounds its exact five-minute lower bound upward to native microsecond
precision, never including a pre-window row by rounding down or inventing a discard for that precision adjustment.

Every batch has `ExaminedCount <= 513`; the cap applies to every native record visit, not accepted rows alone.
`ProbeCount` is internal accounting, excluded from public JSON and limited to one for `NORMAL` or two for a reset
transition. It counts cursor-presence and metadata-only tail probes that contribute neither captured nor discarded
rows. For a normal batch, `len(Events) + DiscardedCount <= 512`, `DiscardedCount == sum(Discards.Count)`, and
`ExaminedCount == len(Events) + DiscardedCount + ProbeCount + bool(Deferred)`. The optional deferred lookahead is a
visited sentinel—never another accepted or discarded row—and `NextOpaque` must cause it to be read again. A normal
adapter that needs a continuation probe reserves capacity for both that probe and a possible sentinel before reading:
it processes at most 511 rows before one lookahead. A backend may return 512 processed rows plus one probe as caught up
only when it has independent end-of-source proof that does not visit another record. Any normal batch with a probe or
an accepted/discarded row requires non-empty `NextOpaque`; the continuation probe may return the original cursor.
A successful normal batch may advance the cursor; only `CaughtUp` may advance the persisted coverage-through watermark
to `QueryStartedAt`. That watermark is status, never a start from which positive historical coverage is inferred.

`CaughtUp` is true only when the adapter explicitly proves the selected source exhausted/current as of the query and
the attempt has no deferred lookahead or byte, line, row, deadline, cancellation, malformed-protocol, or other
truncation. Known discarded rows do not prevent caught-up when the complete selected source was examined. A caught-up
empty commit sets `ObservedAt` to `QueryStartedAt` as a successful collection time; individual events always retain
their own native event time.

`Batch.Validate` requires all times to be non-zero UTC with `QueryStartedAt <= StartedAt <= FinishedAt`, validates
every event/discard and safe counter sum, and requires `CaughtUp == false` for failed/reset batches. Events, discards,
reason pointers, and `NextOpaque` are deep-cloned before caching or persistence. Store additionally checks under the
CAS transaction that source/revision equal the loaded checkpoint, a normal batch is not accepted while reset is
pending, and each reset transition is legal. No caller-owned slice or timestamp pointer is retained.

The native-reader batch state/reason vocabulary is closed. `SUPPORTED/OK` has no reason. A normal
`SUPPORTED/PARTIAL` uses only `INVALID_RESPONSE`, `DEADLINE_EXCEEDED`, `RESPONSE_TOO_LARGE` or
`BACKLOG_DEFERRED`; a supported failed batch uses only the first three failure codes or `READER_FAILED`.
`UNAVAILABLE/FAILED` uses `READER_FAILED`; `UNAVAILABLE/NOT_RUN` uses `NO_VISIBLE_JOURNAL`,
`LOG_HELPER_UNAVAILABLE`, `LOG_HELPER_MISMATCH` or `READER_FAILED`. Permission-denied and unsupported batches are
respectively `PERMISSION_DENIED/NOT_RUN/PERMISSION_DENIED` and
`UNSUPPORTED/NOT_RUN/PLATFORM_UNSUPPORTED`. A disabled source never produces a reader batch.
`CHECKPOINT_RESET` is accepted only on a supported partial reset transition. `MISSED_COLLECTION`,
`NOT_YET_OBSERVED`, `LOG_STORAGE_UNAVAILABLE`, `LOG_SOURCES_DISABLED` and `SOURCE_PARTIAL` are derived
store/projection/cache reasons and cannot enter through a native batch.

`OK` requires explicit caught-up proof and zero discarded rows. A caught-up partial normal batch is allowed only as
`SUPPORTED/PARTIAL/INVALID_RESPONSE` with at least one known discarded selected-field row. Here
`INVALID_RESPONSE` means an individual selected metadata record fell outside the accepted normalized domain while the
reader continued to a proved end of source; malformed helper framing/protocol produces no trustworthy batch.
`DEADLINE_EXCEEDED`, `RESPONSE_TOO_LARGE`, `BACKLOG_DEFERRED` and every other truncation always clear `CaughtUp`.
The validator rejects more than 512 discard groups before iterating them, as well as more than 512 total accepted plus
discarded rows.

Reset recovery happens within the attempt that proves the ordinary checkpoint stale/invalid, not in an unconditional
extra cycle. The adapter immediately switches to one bounded metadata-only newest-record tail probe outside the
initial-read five-minute filter: Windows performs its fixed reverse-channel query for at most one event; Linux uses
native seek-tail/previous/get-cursor after seek/next/test-cursor exactness validation. It requests no body. Reset
batches contain no ingested rows and require `ExaminedCount == ProbeCount`; reset establishment requires one or two
probes, while pending reset permits zero to two. If a cursor is proved, `RESET_ESTABLISHED`
atomically commits that `NextOpaque`, clears `ResetPending`, records `CHECKPOINT_RESET` plus the bounded attempt-window
gap, and advances no caught-up coverage. If the source is empty or the expected probe cannot prove a cursor,
`RESET_PENDING` atomically clears the stale opaque value, keeps `ResetPending`, records the bounded latest state/gap
and zero counts. A later 60-second attempt with
`ResetPending` repeats only this same tail probe until it returns `RESET_ESTABLISHED`; normal after-cursor reading starts
on the following cycle. Any checkpoint with nil `Opaque` and `ResetPending == false` performs a `NORMAL` fixed
five-minute read regardless of `Revision`; revision is CAS state, not an initialization/reset signal.
The initial empty-window path may use one metadata-only tail probe to prove a continuation cursor without adding
counts. A row whose selected metadata can be discarded still requires a valid cursor; failure to acquire a bounded
cursor for any visited row rejects the whole attempt rather than committing counts that could replay. Helper protocol
identity and framing use a separate closed private DTO; adapters never serialize `Batch` as their wire protocol.

## Store port and CAS

```go
var ErrRevisionConflict = errors.New("log checkpoint revision conflict")

type SummaryQuery struct {
    WindowStart    time.Time
    WindowEnd      time.Time
    BucketInterval time.Duration
    BucketCount    int
}

type Counts struct { Captured, Discarded uint64 }
type SeverityCounts struct { Trace, Debug, Info, Warn, Error, Critical, Unknown uint64 }

type BucketCounts struct {
    Counts
    Severity SeverityCounts
}

type CoverageState string
const (
    CoverageFull CoverageState = "FULL"
    CoveragePartial CoverageState = "PARTIAL"
    CoverageGapState CoverageState = "GAP"
    CoverageUnknown CoverageState = "UNKNOWN"
)

type SummaryBucket struct {
    At             time.Time
    CoverageState  CoverageState
    CoveredSeconds uint32
    ReasonCode     *ReasonCode
    Counts         *BucketCounts // nil only with no known records and zero coverage
}

type SourceSummary struct {
    Source         Source
    Status         Status
    CoverageState  CoverageState
    CoveredSeconds uint64
    Counts         *Counts
    Buckets        []SummaryBucket
}

type Summary struct {
    WindowStart    time.Time
    WindowEnd      time.Time
    BucketInterval time.Duration
    BucketCount    int
    Sources        []SourceSummary
}

type Store interface {
    LoadCheckpoint(context.Context, Source) (Checkpoint, error)
    CommitBatch(context.Context, Batch) error
    QuerySummary(context.Context, []Source, SummaryQuery) (Summary, error)
}
```

Configured `SourceSummary.Status` values use a source-specific closed matrix even though the reusable `Status` type
also represents aggregate snapshots. A source is never `DISABLED`. Supported partial reasons are only
`CHECKPOINT_RESET`, `INVALID_RESPONSE`, `DEADLINE_EXCEEDED`, `RESPONSE_TOO_LARGE` or `BACKLOG_DEFERRED`; supported
failed reasons are only the three fixed failure reasons, `READER_FAILED` or the volatile
`LOG_STORAGE_UNAVAILABLE`. Unavailable not-run reasons are only `NO_VISIBLE_JOURNAL`, `LOG_HELPER_UNAVAILABLE`,
`LOG_HELPER_MISMATCH` or `READER_FAILED`; unavailable failed is only `READER_FAILED` or
`LOG_STORAGE_UNAVAILABLE`. Permission-denied and unsupported remain their matching not-run reasons.
`LOG_SOURCES_DISABLED` and `SOURCE_PARTIAL` are aggregate-only and never appear in a source status.

`history.Store` implements this port on the existing single SQLite connection and shared retention budget. The log
collector borrows it and never closes it. `CommitBatch` validates first, then atomically CASes the source checkpoint
at `ExpectedRevision`, writes minute source/severity rollups, discard-attribution counts, latest attempt, coalesced
coverage intervals and the next revision. Zero CAS rows returns `ErrRevisionConflict`. A failed write advances nothing.
`ErrRevisionConflict` is a definite rejection: the collector does not reload, retry, or append that rejected batch to
the process cache. After any other ambiguous commit error, the single-flight collector reloads: revision `expected+1` means applied, unchanged
revision permits one retry of the exact same immutable validated `Batch` (including its kind, times, cursor and counts),
and any other revision is a conflict/reload. It never reruns `Reader`, substitutes a new batch, clears reset state, or
blindly increments twice.

Coverage is derived by `CommitBatch`, not by the native `Reader`. Each commit derives at most
`MaxCoverageSegmentsPerCommit` coalesced, half-open UTC `[start,end)` segments. Let `q = Batch.QueryStartedAt`,
`p = prior Checkpoint.PreviousAttemptAt`, and `floor = q-7d`; every derived start is clamped to at least `floor`, so a
healthy resume after a shutdown longer than retention records only bounded retained evidence rather than failing:

- A first (`p == nil`) `NORMAL` caught-up batch proves only `[q-5m,q)` covered. A later caught-up normal batch proves
  only `[max(p,q-60s),q)` covered. When `p < q-60s`, Store first persists
  `[max(p,floor),q-60s)` as `GAP/MISSED_COLLECTION`; therefore a ten-minute missed poll leaves nine minutes of gap and
  only the final minute covered.
- A failed or non-caught-up normal attempt proves no positive coverage. Store persists a gap over `[q-5m,q)` when
  `p == nil`, otherwise `[max(p,floor),q)`, with only its code-owned reason. Counts captured from backlog remain
  attributed by event time but do not upgrade that gap.
- `RESET_PENDING` and `RESET_ESTABLISHED` prove no positive coverage and persist the same bounded attempt-window gap
  using the batch's code-owned reason (`CHECKPOINT_RESET` for the stale/tail transition, or the fixed underlying
  failure code for an unsuccessful pending probe). They do not change `CoverageThrough`.
- Only explicit persisted gap evidence is sticky and wins overlap with later positive coverage. `UNKNOWN` is the
  absence of evidence in the fixed grid and may become covered after later proof. Store coalesces adjacent intervals
  only when state and reason match; it does not delete a gap because a later interval overlaps it. The summary reducer
  therefore reports `PARTIAL` when one API bucket contains both gap and covered evidence.

The existing seven-day/250-MiB history retention applies to these segments, and every summary returns only its fixed
grid (at most 168 buckets per source and two sources). Thus neither persistence nor a port return grows without the
accepted age/size/count bounds.

In that same successful transaction Store increments `Revision`, sets `PreviousAttemptAt = q` for every batch, and
sets `CoverageThrough = q` only for a `NORMAL` caught-up batch. It validates/rejects inverted, non-UTC, zero, or
unbounded interval input/state before writing. A committed expected source failure therefore advances attempt metadata
and durable gap evidence without advancing the native cursor or coverage watermark.

Log tables have dedicated log-schema metadata while the existing core SQLite `user_version` remains 2. Incompatible or
failed log-table initialization makes only the log Store port unavailable with a fixed reason; it does not quarantine
or replace an otherwise valid host-history database. Physical database corruption continues through existing
whole-store recovery. The optional log collector/API can therefore fail without blanking host snapshots or metric
history.

`QuerySummary` is read-only, accepts only the collector-owned fixed source order and exact contract-checkpoint grids,
never a source list supplied by an HTTP request, and returns one fixed
ascending grid for every configured source, and retains historical buckets despite a later failed, permission-denied,
or unsupported status. Captured values use event time. Invalid/missing-time discards use `QueryStartedAt`; their totals
therefore are not described as exclusively event-time. Counts and coverage remain independent exactly as specified in
`contract-checkpoint.md`.

Every internal timestamp is UTC, non-zero and within RFC 3339's representable year range 1 through 9999. Fixed summary
grid boundaries additionally have zero sub-second component and exact UTC-epoch alignment. The log store must choose a
representation that round-trips that accepted range; this contract does not require Unix-nanosecond storage.

### Storage precision and conservative seconds

Persist coverage endpoints without losing sub-second precision. The integer public `covered_seconds` counts only
complete UTC-aligned one-second cells wholly covered after sticky-gap precedence and interval coalescing. A fractional
gap therefore cannot round away into `FULL`. Adjacent identical intervals coalesce before this projection. A bucket
with less than one full covered second and no explicit gap is `UNKNOWN/NOT_YET_OBSERVED`; with explicit gap evidence it
is `GAP` using the highest-precedence overlapping gap reason. Known positive record counts remain independent in both
cases. This is conservative precision reduction, not evidence that collection was absent during every nanosecond.

Store independently rejects event/discard timestamps later than `q+2s`, the fixed native acquisition horizon. Native
readers classify such timestamps as invalid and attribute their known discard to `q`; valid old backlog retains event
time and is pruned by ordinary retention. Reject a derived attempt interval outside the non-zero RFC 3339 year range
before changing state; never cast overflowing dates into Unix nanoseconds or unsigned native time.

## Collector and cache ownership

```go
type Clock interface { Now() time.Time }
type Ticker interface { C() <-chan time.Time; Stop() }

type Config struct {
    Sources   []Source
    Reader    Reader
    Store     Store
    Clock     Clock
    NewTicker func(time.Duration) Ticker // test seam; production is fixed at 60s
}

func New(Config) (*Collector, error)
func (*Collector) Start(context.Context) error
func (*Collector) Stop(context.Context) error
func (*Collector) Current() Snapshot
func (*Collector) Summary(context.Context, SummaryQuery) (Summary, error)
func AggregateStatus([]Status) (Status, error)
func StatusAfterBatch(Batch, Status) Status // validated batch/previous-status inputs

type Snapshot struct {
    Status     Status
    TotalCount int     // current-process/session ring length, 0..200
    Events     []Event // newest first, deep-cloned, <=200
}
```

`Start` performs one immediate collection, then uses an independent fixed 60-second cadence; it never runs inside the
15-second host collector. Source calls are single-flight per source. Confirmed commits update latest status and append
events to the bounded memory ring. A failed or ambiguous store write keeps prior events, marks the cache stale/failed
with a fixed reason, and does not claim persistence. `Current` and `Summary` return deep clones.

The collector loads every enabled source checkpoint before it starts the four-second context shared only by the
native reads, then begins persistence after all native reads have returned. Checkpoint load and the complete
commit/revision-resolution sequence each receive their own fixed two-second Store context per source; summary Store
reads also receive a two-second context. The fresh commit context intentionally remains available after a native
deadline so a typed deadline batch can durably record its gap, while neither SQLite access nor ambiguity resolution
can block lifecycle indefinitely. One commit context covers the initial write, checkpoint reread, optional exact-batch
retry and final revision reread; retries do not reset that budget.

`Collector.Summary` overlays that process-current cached source status onto the matching configured source returned by
`Store.QuerySummary`, then recomputes only the top-level support/collection/freshness/reason from the overlaid statuses.
It never changes Store-derived buckets, counts, coverage state/seconds, or the last durable `CoverageThrough`. A definite
persistence failure sets `CollectionState = FAILED`, `ReasonCode = LOG_STORAGE_UNAVAILABLE`, and `AttemptedAt` to the
failed query start. It chooses a support candidate from the validated attempted batch when available, otherwise prior
cached status, otherwise `UNAVAILABLE`; it never promotes that candidate to `SUPPORTED`. A `SUPPORTED` candidate remains
`SUPPORTED/FAILED`. Any non-`SUPPORTED` candidate maps to `UNAVAILABLE/FAILED` in this summary-only overlay because the
existing current-snapshot and metric-series schemas require their non-`SUPPORTED` states to be `NOT_RUN/UNKNOWN`; those
producer contracts remain unchanged and never receive an invalid `UNSUPPORTED/FAILED` combination.

The overlay preserves prior successful `ObservedAt` and `CoverageThrough`; freshness is `STALE` with a prior success and
`UNKNOWN` otherwise. It fabricates no durable gap or `PreviousAttemptAt`, and may disappear after restart. The next
successful commit derives its retained-window gap from the last durable `PreviousAttemptAt`. If `Store.QuerySummary`
itself fails, no overlay is fabricated and the API returns its fixed unavailable Problem. Overlay and cloning occur
under collector synchronization, reject a source mismatch, and keep all projected status timestamps no later than the
collector clock snapshot used for that overlay.

`Config.Sources` is unique and in fixed `system,application` order. `Store` is required. `Reader` may be nil only when
the source list is empty, in which case the collector is a valid disabled cache and never calls native code or the log
Store methods. `Collector.Summary` supplies its configured sources to `Store.QuerySummary`; neither local API query
parameters nor callers can substitute source names.
Store summaries must also exactly match the requested window, interval and count. A process-current storage overlay is
applied only when its attempted time is strictly newer than the durable source status, preventing an older overlay
from replacing a commit that became readable just before the collector cleared that overlay.

`Stop` cancels and joins the loop and every native child/read before returning. It does not close `Store`. Serve shutdown
must call log collector `Stop` before the existing host scheduler `Stop`, because the latter owns and closes the shared
history store. Brief bounded transaction serialization with host metric writes is expected; this design adds no second
database writer and makes no zero-contention claim.

## Projection and local API consumers

The host scheduler receives an optional narrow `interface{ Current() logobs.Snapshot }`. After ordinary host collection
it projects that already-cached snapshot into the existing current log section; it never calls `Reader`. Projection
emits only observed time, fixed source/severity/event code and `{state:"OMITTED"}`. The ring count is `total_count`,
`returned_count=min(log_limit,total_count)`, and `truncated=returned_count<total_count`, including `log_limit=0`.

Local API config receives an optional narrow summary source:

```go
type LogSummarySource interface {
    Summary(context.Context, logobs.SummaryQuery) (logobs.Summary, error)
}
```

`internal/localapi/logs.go` alone maps the fixed `range` to a `SummaryQuery`, projects the returned domain summary into
the closed wire DTO (schema version, aggregate quality/counts, limits and privacy), enforces 262144 encoded bytes, and
supports GET/HEAD. The handler never reaches the Store or Reader directly. No configured source means a valid disabled
response; an absent optional source keeps legacy clients/current snapshots unchanged and returns the code-owned
summary-unavailable Problem response.

## Contract tests before implementation

- Table-test every value validator, batch count/lookahead/reset invariant, checkpoint deep clone and safe-integer edge.
- Prove disabled calls no reader, immediate-first plus 60-second single-flight cadence, cancellation/reaping, and Stop
  before shared Store close using deterministic fakes.
- Prove CAS success/conflict/ambiguous reread, same-attempt stale-to-tail proof, repeated empty/failed reset-pending
  probes with zero captured/discarded counts, expected failure commit versus shutdown-cancel no-commit, failed-write no
  progress, minute attribution, coalesced half-open UTC coverage, retained history after latest failure, retention and
  shared-store serialization. Coverage fixtures prove initial empty caught-up success is five minutes `FULL`, an
  ordinary success proves only the latest 60 seconds, a ten-minute missed poll leaves nine minutes `GAP` plus one
  covered minute, failure followed by overlapping success remains `PARTIAL`, backlog counts do not upgrade a gap, and
  unknown cells may be resolved by later positive proof. A prior attempt older than seven days is clamped to the
  retention floor without rejecting or inventing pre-retention coverage.
- Prove the exact historical-reason ordering with pairwise/mixed fixtures, `NOT_YET_OBSERVED` only for absent coverage
  evidence, no arbitrary fallback, and volatile `LOG_STORAGE_UNAVAILABLE` status overlay without bucket/checkpoint
  mutation; restart drops only the volatile status and the next commit derives the durable gap. An unsupported or
  permission-denied attempted batch followed by Store failure must never become `SUPPORTED`; the summary overlay is
  `UNAVAILABLE/FAILED/LOG_STORAGE_UNAVAILABLE` and legacy current-snapshot combinations remain unchanged.
- Prove caught-up requires explicit exhaustion and every truncation/deadline path clears it; caught-up empty updates
  `ObservedAt`, and ambiguous commit retry submits the identical batch without a second native read.
- Prove projection never calls native code, never emits a body/private checkpoint, applies `log_limit` 0..200, and keeps
  a stale ring after failure.
- Prove summary fixed grids and independent count/coverage states, 256KiB bound, strict known fields with additive client
  compatibility, unsupported/permission/zero/gap/overflow fixtures, and secret/body/path canaries.
