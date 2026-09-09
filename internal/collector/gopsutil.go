package collector

import (
	"context"
	"errors"
	"io/fs"
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
func (GopsutilProvider) Processes(ctx context.Context) (ProcessResult, error) {
	ps, e := process.ProcessesWithContext(ctx)
	if e != nil {
		return ProcessResult{}, e
	}
	result := ProcessResult{Processes: make([]ProcessStat, 0, len(ps)), Discovered: len(ps)}
	for _, p := range ps {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Scanned++
		name, e1 := p.NameWithContext(ctx)
		status, e2 := readProcessStatus(ctx, p)
		cpuPct, e3 := p.CPUPercentWithContext(ctx)
		mi, e4 := p.MemoryInfoWithContext(ctx)
		ct, e5 := p.CreateTimeWithContext(ctx)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil {
			joined := errors.Join(e1, e2, e3, e4, e5)
			switch {
			case errors.Is(joined, fs.ErrPermission):
				result.PermissionDenied++
			case errors.Is(joined, errors.ErrUnsupported):
				result.Unsupported++
			default:
				result.Failed++
			}
			continue
		}
		result.Processes = append(result.Processes, ProcessStat{PID: p.Pid, Name: name, State: status, CPUPercent: cpuPct, MemoryBytes: mi.RSS, CreateTimeMS: ct})
	}
	return result, nil
}

func normalizedProcessStatus(statuses []string) string {
	if len(statuses) == 0 || statuses[0] == "" {
		return "unknown"
	}
	return statuses[0]
}
