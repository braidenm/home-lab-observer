package collector

import (
	"context"
	"errors"
	"io/fs"

	"github.com/shirou/gopsutil/v4/process"
)

// scanProcesses caps detail attempts, including failures, before constructing
// process handles. ID enumeration remains OS-owned; the sample is not a claim
// about the largest processes among IDs outside the scan budget.
func scanProcesses(ctx context.Context, discover func(context.Context) ([]int32, error), read func(context.Context, int32) (ProcessStat, error)) (ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return ProcessResult{}, err
	}
	ids, err := discover(ctx)
	if err != nil {
		return ProcessResult{}, err
	}
	limit := min(len(ids), hardMaxProcessScan)
	result := ProcessResult{Processes: make([]ProcessStat, 0, limit), Discovered: len(ids), ScanTruncated: len(ids) > limit}
	for _, pid := range ids[:limit] {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Scanned++
		item, err := read(ctx, pid)
		if err == nil {
			result.Processes = append(result.Processes, item)
			continue
		}
		switch {
		case errors.Is(err, fs.ErrPermission):
			result.PermissionDenied++
		case errors.Is(err, errors.ErrUnsupported):
			result.Unsupported++
		default:
			result.Failed++
		}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, nil
}

func readGopsutilProcess(ctx context.Context, pid int32) (ProcessStat, error) {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return ProcessStat{}, err
	}
	name, e1 := p.NameWithContext(ctx)
	status, e2 := readProcessStatus(ctx, p)
	cpuPct, e3 := p.CPUPercentWithContext(ctx)
	mi, e4 := p.MemoryInfoWithContext(ctx)
	ct, e5 := p.CreateTimeWithContext(ctx)
	if err := errors.Join(e1, e2, e3, e4, e5); err != nil {
		return ProcessStat{}, err
	}
	if mi == nil {
		return ProcessStat{}, ErrNoData
	}
	return ProcessStat{PID: p.Pid, Name: name, State: status, CPUPercent: cpuPct, MemoryBytes: mi.RSS, CreateTimeMS: ct}, nil
}
