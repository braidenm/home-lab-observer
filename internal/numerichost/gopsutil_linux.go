//go:build linux

package numerichost

import (
	"context"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"time"
)

// GopsutilProvider performs only numeric calls; it has no broad provider capabilities.
type GopsutilProvider struct{}

func (GopsutilProvider) LogicalCPUCount(ctx context.Context) (int, error) {
	return cpu.CountsWithContext(ctx, true)
}
func (GopsutilProvider) CPUPercent(ctx context.Context, d time.Duration) (float64, error) {
	v, err := cpu.PercentWithContext(ctx, d, false)
	if err != nil {
		return 0, err
	}
	if len(v) == 0 {
		return 0, ErrNoData
	}
	return v[0], nil
}
func (GopsutilProvider) Memory(ctx context.Context) (MemoryStat, error) {
	v, e := mem.VirtualMemoryWithContext(ctx)
	if e != nil {
		return MemoryStat{}, e
	}
	return MemoryStat{v.Total, v.Used, v.Available, v.UsedPercent}, nil
}
func (GopsutilProvider) Swap(ctx context.Context) (SwapStat, error) {
	v, e := mem.SwapMemoryWithContext(ctx)
	if e != nil {
		return SwapStat{}, e
	}
	return SwapStat{v.Total, v.Used}, nil
}
func (GopsutilProvider) Partitions(ctx context.Context) ([]Partition, error) {
	v, e := disk.PartitionsWithContext(ctx, false)
	if e != nil {
		return nil, e
	}
	out := make([]Partition, 0, len(v))
	for _, p := range v {
		out = append(out, Partition{Mountpoint: p.Mountpoint, Type: p.Fstype})
	}
	return out, nil
}
func (GopsutilProvider) Usage(ctx context.Context, p string) (UsageStat, error) {
	v, e := disk.UsageWithContext(ctx, p)
	if e != nil {
		return UsageStat{}, e
	}
	return UsageStat{v.Total, v.Used, v.Free, v.UsedPercent}, nil
}
func (GopsutilProvider) Uptime(ctx context.Context) (uint64, error) {
	return host.UptimeWithContext(ctx)
}
