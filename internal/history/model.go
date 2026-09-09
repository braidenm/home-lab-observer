package history

import (
	"errors"
	"math"
	"time"
)

const SchemaVersion = 1
const MinimumSQLiteVersion = "3.51.3"
const ExpectedSQLiteVersion = "3.53.4"

type MetricID string

const (
	CPUUtilization        MetricID = "cpu.utilization_percent"
	MemoryUtilization     MetricID = "memory.utilization_percent"
	FilesystemUtilization MetricID = "filesystem.aggregate_utilization_percent"
	NetworkReceiveRate    MetricID = "network.receive_bytes_per_second"
	NetworkTransmitRate   MetricID = "network.transmit_bytes_per_second"
	ProcessCount          MetricID = "process.count"
)

var allowedMetrics = map[MetricID]struct{}{
	CPUUtilization: {}, MemoryUtilization: {}, FilesystemUtilization: {},
	NetworkReceiveRate: {}, NetworkTransmitRate: {}, ProcessCount: {},
}

var ErrMetricNotAllowed = errors.New("metric is not allowlisted")

type Sample struct {
	Metric MetricID
	At     time.Time
	Value  float64
}

func (s Sample) validate() error {
	if _, ok := allowedMetrics[s.Metric]; !ok {
		return ErrMetricNotAllowed
	}
	if s.At.IsZero() || math.IsNaN(s.Value) || math.IsInf(s.Value, 0) || s.Value < 0 {
		return errors.New("invalid metric sample")
	}
	return nil
}

type Rollup struct {
	Metric            MetricID
	BucketStart       time.Time
	ResolutionSeconds int64
	Count             int64
	Minimum           float64
	Maximum           float64
	Sum               float64
	Last              float64
}

type Health struct {
	State               string
	ReasonCode          string
	QuarantinePath      string
	SQLiteVersion       string
	DatabaseBytes       int64
	AgeDropped          uint64
	SizeDropped         uint64
	RollupInputRows     uint64
	CheckpointCount     uint64
	CheckpointFailures  uint64
	WriteFailures       uint64
	SequenceFailures    uint64
	MaintenanceFailures uint64
}

func (h Health) DroppedCount() uint64 { return h.AgeDropped + h.SizeDropped }
