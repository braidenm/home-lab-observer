package numerichost

import (
	"context"
	"errors"
	"time"
)

var ErrNoData = errors.New("collector returned no data")

type Clock interface{ Now() time.Time }
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type MemoryStat struct {
	Total, Used, Available uint64
	UsedPercent            float64
}
type SwapStat struct{ Total, Used uint64 }
type Partition struct{ Mountpoint, Type string }
type UsageStat struct {
	Total, Used, Free uint64
	UsedPercent       float64
}

// Provider deliberately excludes process, network, log and container capabilities.
type Provider interface {
	LogicalCPUCount(context.Context) (int, error)
	CPUPercent(context.Context, time.Duration) (float64, error)
	Memory(context.Context) (MemoryStat, error)
	Swap(context.Context) (SwapStat, error)
	Partitions(context.Context) ([]Partition, error)
	Usage(context.Context, string) (UsageStat, error)
	Uptime(context.Context) (uint64, error)
}
