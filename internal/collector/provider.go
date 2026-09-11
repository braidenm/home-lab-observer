package collector

import (
	"context"
	"github.com/braidenm/home-lab-observer/internal/numerichost"
	"time"
)

var ErrNoData = numerichost.ErrNoData

type Clock interface{ Now() time.Time }
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type MemoryStat = numerichost.MemoryStat
type SwapStat = numerichost.SwapStat
type Partition = numerichost.Partition
type UsageStat = numerichost.UsageStat
type NetStat struct{ BytesSent, BytesRecv, PacketsSent, PacketsRecv, ErrorsIn, ErrorsOut, DropsIn, DropsOut uint64 }
type ProcessStat struct {
	PID          int32
	Name         string
	State        string
	CPUPercent   float64
	MemoryBytes  uint64
	CreateTimeMS int64
}

type ProcessResult struct {
	Processes        []ProcessStat
	Discovered       int
	Scanned          int
	PermissionDenied int
	Unsupported      int
	Failed           int
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
	Processes(context.Context) (ProcessResult, error)
}
