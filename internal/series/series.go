package series

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
)

const SchemaVersion = "observer-metric-series/v1"
const MaxPointsPerSeries = 2048
const MaxResponseBytes = 1_048_576

type Reader interface {
	Samples(context.Context, history.MetricID, time.Time, time.Time, int) ([]history.Sample, error)
	Rollups(context.Context, history.MetricID, time.Time, time.Time, int) ([]history.Rollup, error)
}

type Range string

const (
	Range1Hour   Range = "1h"
	Range6Hours  Range = "6h"
	Range24Hours Range = "24h"
	Range7Days   Range = "7d"
)

type Limits struct {
	MaxMetrics         int `json:"max_metrics"`
	MaxPointsPerSeries int `json:"max_points_per_series"`
	MaxResponseBytes   int `json:"max_response_bytes"`
}

type Privacy struct {
	DataClassification   string `json:"data_classification"`
	ContainsProcessID    bool   `json:"contains_process_identity"`
	ContainsLogBodies    bool   `json:"contains_log_bodies"`
	RemoteUploadEligible bool   `json:"remote_upload_eligible"`
}

type Point struct {
	At    time.Time `json:"at"`
	Value *float64  `json:"value"`
}

type MetricSeries struct {
	MetricID        history.MetricID `json:"metric_id"`
	Unit            string           `json:"unit"`
	SupportState    string           `json:"support_state"`
	CollectionState string           `json:"collection_state"`
	Freshness       string           `json:"freshness"`
	ReasonCode      *string          `json:"reason_code"`
	PointCount      int              `json:"point_count"`
	GapCount        int              `json:"gap_count"`
	Truncated       bool             `json:"truncated"`
	Points          []Point          `json:"points"`
}

type Response struct {
	SchemaVersion         string             `json:"schema_version"`
	Range                 Range              `json:"range"`
	GeneratedAt           time.Time          `json:"generated_at"`
	WindowStart           time.Time          `json:"window_start"`
	WindowEnd             time.Time          `json:"window_end"`
	SampleIntervalSeconds int                `json:"sample_interval_seconds"`
	Limits                Limits             `json:"limits"`
	Privacy               Privacy            `json:"privacy"`
	RequestedMetrics      []history.MetricID `json:"requested_metrics"`
	Series                []MetricSeries     `json:"series"`
}

type rangeConfig struct {
	duration   time.Duration
	resolution time.Duration
}

func Build(ctx context.Context, reader Reader, now time.Time, requestedRange Range, metrics []history.MetricID) (Response, error) {
	if reader == nil {
		return Response{}, errors.New("series reader is required")
	}
	config, ok := ranges[requestedRange]
	if !ok {
		return Response{}, errors.New("series range is not allowlisted")
	}
	if len(metrics) < 1 || len(metrics) > 6 {
		return Response{}, errors.New("one to six metrics are required")
	}
	seen := make(map[history.MetricID]struct{}, len(metrics))
	for _, metric := range metrics {
		if _, ok := metricMetadata[metric]; !ok {
			return Response{}, history.ErrMetricNotAllowed
		}
		if _, exists := seen[metric]; exists {
			return Response{}, errors.New("duplicate metric")
		}
		seen[metric] = struct{}{}
	}

	now = now.UTC()
	windowStart := now.Truncate(config.resolution).Add(-config.duration)
	response := Response{
		SchemaVersion: SchemaVersion, Range: requestedRange, GeneratedAt: now, WindowStart: windowStart, WindowEnd: now,
		SampleIntervalSeconds: int(config.resolution / time.Second),
		Limits:                Limits{MaxMetrics: 6, MaxPointsPerSeries: MaxPointsPerSeries, MaxResponseBytes: MaxResponseBytes},
		Privacy:               Privacy{DataClassification: "PUBLIC_METADATA", ContainsProcessID: false, ContainsLogBodies: false, RemoteUploadEligible: false},
		RequestedMetrics:      append([]history.MetricID(nil), metrics...), Series: make([]MetricSeries, 0, len(metrics)),
	}
	for _, metric := range metrics {
		built, err := buildMetric(ctx, reader, metric, windowStart, now, config.resolution)
		if err != nil {
			return Response{}, fmt.Errorf("build %s series: %w", metric, err)
		}
		response.Series = append(response.Series, built)
	}
	return response, nil
}

var ranges = map[Range]rangeConfig{
	Range1Hour:   {duration: time.Hour, resolution: 15 * time.Second},
	Range6Hours:  {duration: 6 * time.Hour, resolution: time.Minute},
	Range24Hours: {duration: 24 * time.Hour, resolution: 5 * time.Minute},
	Range7Days:   {duration: 7 * 24 * time.Hour, resolution: 30 * time.Minute},
}

type metadata struct{ unit string }

var metricMetadata = map[history.MetricID]metadata{
	history.CPUUtilization:        {unit: "percent"},
	history.MemoryUtilization:     {unit: "percent"},
	history.FilesystemUtilization: {unit: "percent"},
	history.NetworkReceiveRate:    {unit: "bytes_per_second"},
	history.NetworkTransmitRate:   {unit: "bytes_per_second"},
	history.ProcessCount:          {unit: "count"},
}

type bucket struct {
	count int64
	sum   float64
	last  float64
	at    time.Time
}

func buildMetric(ctx context.Context, reader Reader, metric history.MetricID, from, to time.Time, resolution time.Duration) (MetricSeries, error) {
	result := MetricSeries{MetricID: metric, Unit: metricMetadata[metric].unit, SupportState: "SUPPORTED", CollectionState: "NOT_RUN", Freshness: "UNKNOWN", Points: []Point{}}
	raw, err := reader.Samples(ctx, metric, from, to, 10000)
	if err != nil {
		return MetricSeries{}, err
	}
	rollups, err := reader.Rollups(ctx, metric, from, to, 10000)
	if err != nil {
		return MetricSeries{}, err
	}
	if len(raw) == 0 && len(rollups) == 0 {
		result.ReasonCode = stringPointer("NO_SAMPLES")
		return result, nil
	}

	bucketCount := int(to.Sub(from)/resolution) + 1
	if bucketCount > MaxPointsPerSeries {
		return MetricSeries{}, errors.New("series point bound exceeded")
	}
	buckets := make([]bucket, bucketCount)
	add := func(at, lastAt time.Time, count int64, sum, last float64) {
		index := int(at.Sub(from) / resolution)
		if index < 0 || index >= len(buckets) || count < 1 {
			return
		}
		candidate := &buckets[index]
		candidate.count += count
		candidate.sum += sum
		if candidate.at.IsZero() || lastAt.After(candidate.at) {
			candidate.last, candidate.at = last, lastAt
		}
	}
	latest := time.Time{}
	for _, sample := range raw {
		add(sample.At, sample.At, 1, sample.Value, sample.Value)
		if sample.At.After(latest) {
			latest = sample.At
		}
	}
	for _, rollup := range rollups {
		add(rollup.BucketStart, rollup.LastAt, rollup.Count, rollup.Sum, rollup.Last)
		if rollup.LastAt.After(latest) {
			latest = rollup.LastAt
		}
	}

	result.Points = make([]Point, 0, bucketCount)
	for index, item := range buckets {
		point := Point{At: from.Add(time.Duration(index) * resolution)}
		if item.count == 0 {
			result.GapCount++
		} else {
			value := item.sum / float64(item.count)
			if metric == history.ProcessCount {
				value = math.Round(item.last)
			}
			point.Value = &value
		}
		result.Points = append(result.Points, point)
	}
	result.PointCount = len(result.Points)
	result.Truncated = len(raw) == 10000 || len(rollups) == 10000
	result.CollectionState = "OK"
	if result.GapCount > 0 || result.Truncated {
		result.CollectionState = "PARTIAL"
		result.ReasonCode = stringPointer("COLLECTION_GAPS")
	}
	result.Freshness = "CURRENT"
	if to.Sub(latest) > 45*time.Second {
		result.Freshness = "STALE"
		if result.ReasonCode == nil {
			result.ReasonCode = stringPointer("LATEST_SAMPLE_STALE")
		}
	}
	return result, nil
}

func stringPointer(value string) *string { return &value }
