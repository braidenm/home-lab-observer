// Package logobs defines the transport-neutral, privacy-bounded contracts for
// native log metadata. It intentionally contains no native payload or message
// body type.
package logobs

import (
	"context"
	"errors"
	"time"
)

const (
	MaxSources                          = 2
	MaxAcceptedEvents                   = 512
	MaxExaminedEvents                   = 513
	MaxProbeEvents                      = 2
	MaxSourceBytes                      = 2 << 20
	MaxNativeFieldBytes                 = 4 << 10
	MaxCheckpointBytes                  = 16 << 10
	MaxRecentEvents                     = 200
	MaxCoverageSegmentsPerCommit        = 2
	MaxSafeInteger               uint64 = 9_007_199_254_740_991
)

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
	SupportSupported        SupportState = "SUPPORTED"
	SupportDisabled         SupportState = "DISABLED"
	SupportUnavailable      SupportState = "UNAVAILABLE"
	SupportPermissionDenied SupportState = "PERMISSION_DENIED"
	SupportUnsupported      SupportState = "UNSUPPORTED"
)

type CollectionState string

const (
	CollectionOK      CollectionState = "OK"
	CollectionPartial CollectionState = "PARTIAL"
	CollectionFailed  CollectionState = "FAILED"
	CollectionNotRun  CollectionState = "NOT_RUN"
)

type Freshness string

const (
	FreshnessCurrent Freshness = "CURRENT"
	FreshnessStale   Freshness = "STALE"
	FreshnessUnknown Freshness = "UNKNOWN"
)

type ReasonCode string

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
	ReasonLogStorageUnavailable ReasonCode = "LOG_STORAGE_UNAVAILABLE"
	ReasonNoVisibleJournal      ReasonCode = "NO_VISIBLE_JOURNAL"
	ReasonLogHelperUnavailable  ReasonCode = "LOG_HELPER_UNAVAILABLE"
	ReasonLogHelperMismatch     ReasonCode = "LOG_HELPER_MISMATCH"
	ReasonPlatformUnsupported   ReasonCode = "PLATFORM_UNSUPPORTED"
	ReasonLogSourcesDisabled    ReasonCode = "LOG_SOURCES_DISABLED"
	ReasonSourcePartial         ReasonCode = "SOURCE_PARTIAL"
)

type Event struct {
	ObservedAt time.Time
	Source     Source
	Severity   Severity
	EventCode  string
}

type Status struct {
	SupportState    SupportState
	CollectionState CollectionState
	Freshness       Freshness
	ObservedAt      *time.Time
	AttemptedAt     *time.Time
	CoverageThrough *time.Time
	ReasonCode      *ReasonCode
}

type Checkpoint struct {
	Revision          uint64
	ResetPending      bool
	Opaque            []byte `json:"-"`
	PreviousAttemptAt *time.Time
	CoverageThrough   *time.Time
}

type ReadRequest struct {
	Source         Source
	Checkpoint     Checkpoint
	QueryStartedAt time.Time
}

type BatchKind string

const (
	BatchNormal           BatchKind = "NORMAL"
	BatchResetPending     BatchKind = "RESET_PENDING"
	BatchResetEstablished BatchKind = "RESET_ESTABLISHED"
)

type DiscardCount struct {
	At    time.Time
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
	ProbeCount       uint32 `json:"-"`
	DiscardedCount   uint32
	Deferred         bool
	CaughtUp         bool
	NextOpaque       []byte `json:"-"`
}

type Reader interface {
	Read(context.Context, ReadRequest) (Batch, error)
}

var ErrRevisionConflict = errors.New("log checkpoint revision conflict")

type SummaryQuery struct {
	WindowStart    time.Time
	WindowEnd      time.Time
	BucketInterval time.Duration
	BucketCount    int
}

type Counts struct {
	Captured  uint64
	Discarded uint64
}

type SeverityCounts struct {
	Trace    uint64
	Debug    uint64
	Info     uint64
	Warn     uint64
	Error    uint64
	Critical uint64
	Unknown  uint64
}

type BucketCounts struct {
	Counts
	Severity SeverityCounts
}

type CoverageState string

const (
	CoverageFull     CoverageState = "FULL"
	CoveragePartial  CoverageState = "PARTIAL"
	CoverageGapState CoverageState = "GAP"
	CoverageUnknown  CoverageState = "UNKNOWN"
)

type SummaryBucket struct {
	At             time.Time
	CoverageState  CoverageState
	CoveredSeconds uint32
	ReasonCode     *ReasonCode
	Counts         *BucketCounts
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

type Snapshot struct {
	Status     Status
	TotalCount int
	Events     []Event
}
