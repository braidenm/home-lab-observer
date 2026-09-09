package series

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
)

type fakeReader struct {
	raw     map[history.MetricID][]history.Sample
	rollups map[history.MetricID][]history.Rollup
	err     error
}

func (r fakeReader) Samples(_ context.Context, metric history.MetricID, _, _ time.Time, _ int) ([]history.Sample, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.raw[metric], nil
}
func (r fakeReader) Rollups(_ context.Context, metric history.MetricID, _, _ time.Time, _ int) ([]history.Rollup, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.rollups[metric], nil
}

func TestBuildProducesBoundedGapsAndCurrentValues(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	from := now.Add(-time.Hour)
	reader := fakeReader{raw: map[history.MetricID][]history.Sample{
		history.CPUUtilization: {
			{Metric: history.CPUUtilization, At: from, Value: 10},
			{Metric: history.CPUUtilization, At: now.Add(-10 * time.Second), Value: 30},
		},
	}}
	response, err := Build(context.Background(), reader, now, Range1Hour, []history.MetricID{history.CPUUtilization})
	if err != nil {
		t.Fatal(err)
	}
	series := response.Series[0]
	if response.SchemaVersion != SchemaVersion || response.SampleIntervalSeconds != 15 || len(series.Points) != 241 || series.GapCount != 239 || series.CollectionState != "PARTIAL" || series.Freshness != "CURRENT" {
		t.Fatalf("unexpected response: %+v series=%+v", response, series)
	}
	var latest *float64
	for _, point := range series.Points {
		if point.Value != nil {
			latest = point.Value
		}
	}
	if series.Points[0].Value == nil || *series.Points[0].Value != 10 || latest == nil || *latest != 30 {
		t.Fatalf("observed values were not preserved: %+v", series.Points)
	}
}

func TestBuildReturnsSupportedNoSamplesWithoutInventingZero(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	response, err := Build(context.Background(), fakeReader{}, now, Range7Days, []history.MetricID{history.ProcessCount})
	if err != nil {
		t.Fatal(err)
	}
	series := response.Series[0]
	if series.CollectionState != "NOT_RUN" || series.Freshness != "UNKNOWN" || series.ReasonCode == nil || len(series.Points) != 0 {
		t.Fatalf("empty series invented data: %+v", series)
	}
}

func TestBuildUsesIntegerLastValueForProcessCountRollups(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	reader := fakeReader{rollups: map[history.MetricID][]history.Rollup{
		history.ProcessCount: {{Metric: history.ProcessCount, BucketStart: now.Add(-time.Hour), LastAt: now.Add(-55 * time.Minute), ResolutionSeconds: 300, Count: 2, Sum: 21, Last: 11}},
	}}
	response, err := Build(context.Background(), reader, now, Range24Hours, []history.MetricID{history.ProcessCount})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, point := range response.Series[0].Points {
		if point.Value != nil {
			found = true
			if *point.Value != 11 {
				t.Fatalf("process count=%v", *point.Value)
			}
		}
	}
	if !found {
		t.Fatal("expected process count point")
	}
}

func TestBuildRejectsInvalidRequestsAndReaderFailures(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		rangeValue Range
		metrics    []history.MetricID
	}{
		{"forever", []history.MetricID{history.CPUUtilization}},
		{Range1Hour, nil},
		{Range1Hour, []history.MetricID{"arbitrary"}},
		{Range1Hour, []history.MetricID{history.CPUUtilization, history.CPUUtilization}},
	} {
		if _, err := Build(context.Background(), fakeReader{}, now, test.rangeValue, test.metrics); err == nil {
			t.Fatalf("accepted range=%q metrics=%v", test.rangeValue, test.metrics)
		}
	}
	if _, err := Build(context.Background(), fakeReader{err: errors.New("read failed")}, now, Range1Hour, []history.MetricID{history.CPUUtilization}); err == nil {
		t.Fatal("reader failure was hidden")
	}
}
