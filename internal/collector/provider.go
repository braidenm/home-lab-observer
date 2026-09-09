package collector

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
type NetStat struct{ BytesSent, BytesRecv, PacketsSent, PacketsRecv, ErrorsIn, ErrorsOut, DropsIn, DropsOut uint64 }
type ProcessStat struct {
	PID          int32
	Name         string
	State        string
	CPUPercent   float64
	MemoryBytes  uint64
	CreateTimeMS int64
}

type SystemProvider interface {
	LogicalCPUCount(context.Context) (int, error)
	CPUPercent(context.Context, time.Duration) (float64, error)
	Memory(context.Context) (MemoryStat, error)
	Swap(context.Context) (SwapStat, error)
	Partitions(context.Context) ([]Partition, error)
	Usage(context.Context, string) (UsageStat, error)
	Network(context.Context) (NetStat, error)
	Uptime(context.Context) (uint64, error)
	Processes(context.Context) ([]ProcessStat, int, error)
}
