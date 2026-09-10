package localapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const (
	logSummarySchemaVersion = "observer-log-summary/v1"
	logSummaryResponseLimit = 256 * 1024
)

type LogSummarySource interface {
	Summary(context.Context, logobs.SummaryQuery) (logobs.Summary, error)
}

type logSummaryResponse struct {
	SchemaVersion         string                     `json:"schema_version"`
	GeneratedAt           time.Time                  `json:"generated_at"`
	Range                 string                     `json:"range"`
	WindowStart           time.Time                  `json:"window_start"`
	WindowEnd             time.Time                  `json:"window_end"`
	BucketIntervalSeconds int64                      `json:"bucket_interval_seconds"`
	ExpectedBucketCount   int                        `json:"expected_bucket_count"`
	SupportState          logobs.SupportState        `json:"support_state"`
	CollectionState       logobs.CollectionState     `json:"collection_state"`
	Freshness             logobs.Freshness           `json:"freshness"`
	ObservedAt            *time.Time                 `json:"observed_at"`
	ReasonCode            *logobs.ReasonCode         `json:"reason_code"`
	CoverageState         logobs.CoverageState       `json:"coverage_state"`
	Counts                *logSummaryCounts          `json:"counts"`
	Limits                logSummaryLimits           `json:"limits"`
	Privacy               logSummaryPrivacy          `json:"privacy"`
	Sources               []logSummarySourceResponse `json:"sources"`
}

type logSummaryCounts struct {
	Captured  uint64 `json:"captured"`
	Discarded uint64 `json:"discarded"`
}

type logSummarySeverityCounts struct {
	Trace    uint64 `json:"trace"`
	Debug    uint64 `json:"debug"`
	Info     uint64 `json:"info"`
	Warn     uint64 `json:"warn"`
	Error    uint64 `json:"error"`
	Critical uint64 `json:"critical"`
	Unknown  uint64 `json:"unknown"`
}

type logSummaryBucketCounts struct {
	Captured  uint64                   `json:"captured"`
	Discarded uint64                   `json:"discarded"`
	Severity  logSummarySeverityCounts `json:"severity"`
}

type logSummaryBucket struct {
	At             time.Time               `json:"at"`
	CoverageState  logobs.CoverageState    `json:"coverage_state"`
	CoveredSeconds uint32                  `json:"covered_seconds"`
	ReasonCode     *logobs.ReasonCode      `json:"reason_code"`
	Counts         *logSummaryBucketCounts `json:"counts"`
}

type logSummarySourceStatus struct {
	SupportState    logobs.SupportState    `json:"support_state"`
	CollectionState logobs.CollectionState `json:"collection_state"`
	Freshness       logobs.Freshness       `json:"freshness"`
	ObservedAt      *time.Time             `json:"observed_at"`
	AttemptedAt     *time.Time             `json:"attempted_at"`
	CoverageThrough *time.Time             `json:"coverage_through"`
	ReasonCode      *logobs.ReasonCode     `json:"reason_code"`
}

type logSummarySourceResponse struct {
	Source         logobs.Source          `json:"source"`
	Status         logSummarySourceStatus `json:"status"`
	CoverageState  logobs.CoverageState   `json:"coverage_state"`
	CoveredSeconds uint64                 `json:"covered_seconds"`
	Counts         *logSummaryCounts      `json:"counts"`
	Buckets        []logSummaryBucket     `json:"buckets"`
}

type logSummaryLimits struct {
	MaxSources          int `json:"max_sources"`
	MaxBucketsPerSource int `json:"max_buckets_per_source"`
	MaxResponseBytes    int `json:"max_response_bytes"`
}

type logSummaryPrivacy struct {
	DataClassification   string `json:"data_classification"`
	ContainsLogBodies    bool   `json:"contains_log_bodies"`
	ContainsEventCodes   bool   `json:"contains_event_codes"`
	ContainsIdentity     bool   `json:"contains_identity_fields"`
	RemoteUploadEligible bool   `json:"remote_upload_eligible"`
}

func (h *handler) logSummary(w http.ResponseWriter, r *http.Request) {
	query, err := parseLogSummaryQuery(r.URL.RawQuery)
	if err != nil {
		writeRequestProblem(w, r, http.StatusBadRequest, "INVALID_QUERY", "Invalid query")
		return
	}
	if h.config.LogSummarySource == nil {
		writeRequestProblem(w, r, http.StatusServiceUnavailable, "LOG_SUMMARY_UNAVAILABLE", "Log summary unavailable")
		return
	}

	queryAt, err := summaryNow(h.config.Now)
	if err != nil {
		writeRequestProblem(w, r, http.StatusInternalServerError, "LOG_SUMMARY_UNAVAILABLE", "Log summary unavailable")
		return
	}
	for attempt := 0; attempt < 2; attempt++ {
		domainQuery := buildLogSummaryDomainQuery(queryAt, query)
		summary, sourceErr := h.config.LogSummarySource.Summary(r.Context(), domainQuery)
		if sourceErr != nil {
			writeRequestProblem(w, r, http.StatusServiceUnavailable, "LOG_SUMMARY_UNAVAILABLE", "Log summary unavailable")
			return
		}
		generatedAt, nowErr := summaryNow(h.config.Now)
		if nowErr != nil {
			writeRequestProblem(w, r, http.StatusInternalServerError, "LOG_SUMMARY_UNAVAILABLE", "Log summary unavailable")
			return
		}
		if !generatedAt.Truncate(query.interval).Equal(domainQuery.WindowEnd) {
			if attempt == 0 {
				queryAt = generatedAt
				continue
			}
			writeRequestProblem(w, r, http.StatusServiceUnavailable, "LOG_SUMMARY_UNAVAILABLE", "Log summary unavailable")
			return
		}
		response, projectionErr := projectLogSummary(summary, domainQuery, query.rangeValue, generatedAt)
		if projectionErr != nil {
			writeRequestProblem(w, r, http.StatusInternalServerError, "LOG_SUMMARY_UNAVAILABLE", "Log summary unavailable")
			return
		}
		h.writeJSON(w, r, http.StatusOK, response, logSummaryResponseLimit)
		return
	}
}

func summaryNow(now func() time.Time) (time.Time, error) {
	value := now().UTC()
	if value.IsZero() || value.Year() < 1 || value.Year() > 9999 {
		return time.Time{}, errors.New("summary clock is outside the contract range")
	}
	return value, nil
}

func buildLogSummaryDomainQuery(now time.Time, query logSummaryQuery) logobs.SummaryQuery {
	windowEnd := now.Truncate(query.interval)
	return logobs.SummaryQuery{
		WindowStart:    windowEnd.Add(-time.Duration(query.bucketCount) * query.interval),
		WindowEnd:      windowEnd,
		BucketInterval: query.interval,
		BucketCount:    query.bucketCount,
	}
}

func projectLogSummary(summary logobs.Summary, query logobs.SummaryQuery, rangeValue string, generatedAt time.Time) (logSummaryResponse, error) {
	if !summary.WindowStart.Equal(query.WindowStart) || !summary.WindowEnd.Equal(query.WindowEnd) ||
		summary.BucketInterval != query.BucketInterval || summary.BucketCount != query.BucketCount {
		return logSummaryResponse{}, errors.New("summary does not match requested grid")
	}
	if err := summary.Validate(); err != nil {
		return logSummaryResponse{}, err
	}
	statuses := make([]logobs.Status, len(summary.Sources))
	for index := range summary.Sources {
		statuses[index] = summary.Sources[index].Status
	}
	quality, err := logobs.AggregateStatus(statuses)
	if err != nil {
		return logSummaryResponse{}, err
	}
	if err := validateLogSummaryAsOf(summary, quality, generatedAt); err != nil {
		return logSummaryResponse{}, err
	}

	response := logSummaryResponse{
		SchemaVersion: logSummarySchemaVersion, GeneratedAt: generatedAt, Range: rangeValue,
		WindowStart: query.WindowStart, WindowEnd: query.WindowEnd,
		BucketIntervalSeconds: int64(query.BucketInterval / time.Second), ExpectedBucketCount: query.BucketCount,
		SupportState: quality.SupportState, CollectionState: quality.CollectionState, Freshness: quality.Freshness,
		ObservedAt: cloneSummaryTime(quality.ObservedAt), ReasonCode: cloneSummaryReason(quality.ReasonCode),
		CoverageState: aggregateSummaryCoverage(summary.Sources), Counts: aggregateSummaryCounts(summary.Sources),
		Limits:  logSummaryLimits{MaxSources: logobs.MaxSources, MaxBucketsPerSource: 168, MaxResponseBytes: logSummaryResponseLimit},
		Privacy: logSummaryPrivacy{DataClassification: "LOCAL_SENSITIVE"},
		Sources: make([]logSummarySourceResponse, len(summary.Sources)),
	}
	for index, source := range summary.Sources {
		response.Sources[index] = projectLogSummarySource(source)
	}
	return response, nil
}

func validateLogSummaryAsOf(summary logobs.Summary, quality logobs.Status, generatedAt time.Time) error {
	if generatedAt.Before(summary.WindowEnd) {
		return errors.New("summary generation precedes window")
	}
	for _, status := range append([]logobs.Status{quality}, summaryStatuses(summary)...) {
		for _, value := range []*time.Time{status.ObservedAt, status.AttemptedAt, status.CoverageThrough} {
			if value != nil && value.After(generatedAt) {
				return errors.New("summary status is newer than response")
			}
		}
	}
	return nil
}

func summaryStatuses(summary logobs.Summary) []logobs.Status {
	result := make([]logobs.Status, len(summary.Sources))
	for index := range summary.Sources {
		result[index] = summary.Sources[index].Status
	}
	return result
}

func projectLogSummarySource(source logobs.SourceSummary) logSummarySourceResponse {
	result := logSummarySourceResponse{
		Source: source.Source,
		Status: logSummarySourceStatus{
			SupportState: source.Status.SupportState, CollectionState: source.Status.CollectionState, Freshness: source.Status.Freshness,
			ObservedAt: cloneSummaryTime(source.Status.ObservedAt), AttemptedAt: cloneSummaryTime(source.Status.AttemptedAt),
			CoverageThrough: cloneSummaryTime(source.Status.CoverageThrough), ReasonCode: cloneSummaryReason(source.Status.ReasonCode),
		},
		CoverageState: source.CoverageState, CoveredSeconds: source.CoveredSeconds, Counts: projectLogSummaryCounts(source.Counts),
		Buckets: make([]logSummaryBucket, len(source.Buckets)),
	}
	for index, bucket := range source.Buckets {
		result.Buckets[index] = logSummaryBucket{
			At: bucket.At, CoverageState: bucket.CoverageState, CoveredSeconds: bucket.CoveredSeconds,
			ReasonCode: cloneSummaryReason(bucket.ReasonCode), Counts: projectLogSummaryBucketCounts(bucket.Counts),
		}
	}
	return result
}

func aggregateSummaryCoverage(sources []logobs.SourceSummary) logobs.CoverageState {
	if len(sources) == 0 {
		return logobs.CoverageUnknown
	}
	allFull, allUnknown, anyGap, anyCoverage := true, true, false, false
	for _, source := range sources {
		allFull = allFull && source.CoverageState == logobs.CoverageFull
		allUnknown = allUnknown && source.CoverageState == logobs.CoverageUnknown
		anyGap = anyGap || source.CoverageState == logobs.CoverageGapState
		anyCoverage = anyCoverage || source.CoveredSeconds > 0
	}
	if allFull {
		return logobs.CoverageFull
	}
	if allUnknown {
		return logobs.CoverageUnknown
	}
	if !anyCoverage && anyGap {
		return logobs.CoverageGapState
	}
	return logobs.CoveragePartial
}

func aggregateSummaryCounts(sources []logobs.SourceSummary) *logSummaryCounts {
	var result *logSummaryCounts
	for _, source := range sources {
		if source.Counts == nil {
			continue
		}
		if result == nil {
			result = &logSummaryCounts{}
		}
		result.Captured += source.Counts.Captured
		result.Discarded += source.Counts.Discarded
	}
	return result
}

func projectLogSummaryCounts(value *logobs.Counts) *logSummaryCounts {
	if value == nil {
		return nil
	}
	return &logSummaryCounts{Captured: value.Captured, Discarded: value.Discarded}
}

func projectLogSummaryBucketCounts(value *logobs.BucketCounts) *logSummaryBucketCounts {
	if value == nil {
		return nil
	}
	return &logSummaryBucketCounts{
		Captured: value.Captured, Discarded: value.Discarded,
		Severity: logSummarySeverityCounts{
			Trace: value.Severity.Trace, Debug: value.Severity.Debug, Info: value.Severity.Info, Warn: value.Severity.Warn,
			Error: value.Severity.Error, Critical: value.Severity.Critical, Unknown: value.Severity.Unknown,
		},
	}
}

func cloneSummaryTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func cloneSummaryReason(value *logobs.ReasonCode) *logobs.ReasonCode {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
