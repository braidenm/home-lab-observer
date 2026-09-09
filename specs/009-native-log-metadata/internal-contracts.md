# Spec 009 internal Go contract proposal

Status: Proposed for cross-slice acceptance after Spec 008. These are ports and invariants, not implementation.

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
    ObservedAt      *time.Time // last successful useful collection, not latest attempt
    AttemptedAt     *time.Time // latest attempted collection
    CoverageThrough *time.Time // latest caught-up query-start watermark, not contiguous proof
    ReasonCode      *ReasonCode
}
```

`Event.Validate` enforces exact source/severity/code forms and UTC. Native codes are only `WIN_<uint32>`,
`WIN_<32-lowercase-hex-guid>_<uint32>`, `SYSTEMD_<32-lowercase-hex-id>`, or `SYSTEMD_PRIORITY_<0..7>`.
Counters reject increments beyond 9007199254740991 before persistence.

## Checkpoint and bounded reader

```go
const (
    MaxSources                   = 2
    MaxAcceptedEvents            = 512
    MaxExaminedEvents            = 513 // all accepted/discarded rows plus optional deferred lookahead
    MaxSourceBytes               = 2 << 20
    MaxJournalLineBytes          = 4 << 10
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
evidence. `Opaque` and both optional timestamps are deep-cloned at every port boundary. Standalone validation requires
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

Adapter construction receives one validated observer-owned state root, not a request-selected path. Linux cursor
staging uses fixed per-source bounded owner-only files beneath one dedicated staging directory, removes stale known
filenames before an attempt, and cleans them after success, failure, cancellation, and restart. It never creates
arbitrary/random cursor filenames that can accumulate after a crash, exposes cursor contents in argv, or returns a
staging path through these ports. The Windows fixed helper continues to use bounded stdin/stdout pipes and discarded
stderr, not staging files.

Every batch has `ExaminedCount <= 513`; the cap applies to all examined rows, not accepted rows alone. For a normal
batch, `len(Events) + DiscardedCount <= 512`, `DiscardedCount == sum(Discards.Count)`, and `ExaminedCount` equals that
sum plus one only when a lookahead was examined and `Deferred` is true. The 513th row can only be that deferred
sentinel—never another accepted or discarded row. After 512 discarded rows, at most one sentinel may be examined even
when no event was accepted. The sentinel is absent from events/discards and `NextOpaque` must cause it to be read again.
A successful normal batch may advance the cursor; only `CaughtUp` may advance the persisted coverage-through watermark
to `QueryStartedAt`. That watermark is status, never a start from which positive historical coverage is inferred.

`Batch.Validate` requires all times to be non-zero UTC with `QueryStartedAt <= StartedAt <= FinishedAt`, validates
every event/discard and safe counter sum, and requires `CaughtUp == false` for failed/reset batches. Events, discards,
reason pointers, and `NextOpaque` are deep-cloned before caching or persistence. Store additionally checks under the
CAS transaction that source/revision equal the loaded checkpoint, a normal batch is not accepted while reset is
pending, and each reset transition is legal. No caller-owned slice or timestamp pointer is retained.

Reset recovery happens within the attempt that proves the ordinary checkpoint stale/invalid, not in an unconditional
extra cycle. The adapter immediately switches to one bounded metadata-only newest-record tail probe outside the
initial-read five-minute filter: Windows performs its fixed reverse-channel query for at most one event; Linux uses
fixed `journalctl -n 1` selected fields and its automatic cursor. It requests no body; a returned record makes
`ExaminedCount` one but contributes no captured or discarded count. If a cursor is proved, `RESET_ESTABLISHED`
atomically commits that `NextOpaque`, clears `ResetPending`, records `CHECKPOINT_RESET` plus the bounded attempt-window
gap, and advances no caught-up coverage. If the source is empty or the expected probe cannot prove a cursor,
`RESET_PENDING` atomically clears the stale opaque value, keeps `ResetPending`, records the bounded latest state/gap
and zero counts. A later 60-second attempt with
`ResetPending` repeats only this same tail probe until it returns `RESET_ESTABLISHED`; normal after-cursor reading starts
on the following cycle. Any checkpoint with nil `Opaque` and `ResetPending == false` performs a `NORMAL` fixed
five-minute read regardless of `Revision`; revision is CAS state, not an initialization/reset signal.

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

`history.Store` implements this port on the existing single SQLite connection and shared retention budget. The log
collector borrows it and never closes it. `CommitBatch` validates first, then atomically CASes the source checkpoint
at `ExpectedRevision`, writes minute source/severity rollups, discard-attribution counts, latest attempt, coalesced
coverage intervals and the next revision. Zero CAS rows returns `ErrRevisionConflict`. A failed write advances nothing.
After an ambiguous commit error, the single-flight collector reloads: revision `expected+1` means applied, unchanged
revision permits one normal retry, and any other revision is a conflict/reload; it never blindly increments twice.

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

`Config.Sources` is unique and in fixed `system,application` order. `Store` is required. `Reader` may be nil only when
the source list is empty, in which case the collector is a valid disabled cache and never calls native code or the log
Store methods. `Collector.Summary` supplies its configured sources to `Store.QuerySummary`; neither local API query
parameters nor callers can substitute source names.

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
- Prove projection never calls native code, never emits a body/private checkpoint, applies `log_limit` 0..200, and keeps
  a stale ring after failure.
- Prove summary fixed grids and independent count/coverage states, 256KiB bound, strict known fields with additive client
  compatibility, unsupported/permission/zero/gap/overflow fixtures, and secret/body/path canaries.
