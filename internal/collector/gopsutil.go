package collector

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	netio "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

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
		out = append(out, Partition{p.Mountpoint})
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
func (GopsutilProvider) Network(ctx context.Context) (NetStat, error) {
	v, e := netio.IOCountersWithContext(ctx, false)
	if e != nil {
		return NetStat{}, e
	}
	if len(v) == 0 {
		return NetStat{}, ErrNoData
	}
	n := v[0]
	return NetStat{n.BytesSent, n.BytesRecv, n.PacketsSent, n.PacketsRecv, n.Errin, n.Errout, n.Dropin, n.Dropout}, nil
}
func (GopsutilProvider) Uptime(ctx context.Context) (uint64, error) {
	return host.UptimeWithContext(ctx)
}
func (GopsutilProvider) Processes(ctx context.Context) ([]ProcessStat, int, error) {
	ps, e := process.ProcessesWithContext(ctx)
	if e != nil {
		return nil, 0, e
	}
	out := make([]ProcessStat, 0, len(ps))
	skipped := 0
	for _, p := range ps {
		if ctx.Err() != nil {
			return out, skipped, ctx.Err()
		}
		name, e1 := p.NameWithContext(ctx)
		cpuPct, e2 := p.CPUPercentWithContext(ctx)
		mi, e3 := p.MemoryInfoWithContext(ctx)
		ct, e4 := p.CreateTimeWithContext(ctx)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			skipped++
			continue
		}
		out = append(out, ProcessStat{p.Pid, name, cpuPct, mi.RSS, ct})
	}
	return out, skipped, nil
}
