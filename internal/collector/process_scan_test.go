package collector

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

func TestProcessScanLimitsAttemptsBeforeDetailReads(t *testing.T) {
	ids := make([]int32, hardMaxProcessScan+23)
	for i := range ids {
		ids[i] = int32(i + 1)
	}
	calls := 0
	result, err := scanProcesses(context.Background(), func(context.Context) ([]int32, error) {
		return ids, nil
	}, func(_ context.Context, pid int32) (ProcessStat, error) {
		calls++
		if pid%2 == 0 {
			return ProcessStat{}, fs.ErrPermission
		}
		return ProcessStat{PID: pid, Name: "worker", MemoryBytes: 1}, nil
	})
	if err != nil || calls != hardMaxProcessScan || result.Scanned != hardMaxProcessScan || result.Discovered != len(ids) || !result.ScanTruncated {
		t.Fatal("detail acquisition was not bounded with explicit coverage")
	}
	if result.PermissionDenied != hardMaxProcessScan/2 || len(result.Processes) != hardMaxProcessScan/2 || cap(result.Processes) > hardMaxProcessScan {
		t.Fatal("failed attempts escaped budget or result allocation exceeded budget")
	}
}

func TestProcessScanPreservesFailureCategories(t *testing.T) {
	wantFailure := errors.New("synthetic detail failure")
	result, err := scanProcesses(context.Background(), func(context.Context) ([]int32, error) {
		return []int32{1, 2, 3, 4}, nil
	}, func(_ context.Context, pid int32) (ProcessStat, error) {
		switch pid {
		case 1:
			return ProcessStat{}, errors.Join(wantFailure, fs.ErrPermission)
		case 2:
			return ProcessStat{}, errors.ErrUnsupported
		case 3:
			return ProcessStat{}, wantFailure
		default:
			return ProcessStat{PID: pid, Name: "worker"}, nil
		}
	})
	if err != nil || result.ScanTruncated || result.Scanned != 4 || result.Discovered != 4 || result.PermissionDenied != 1 || result.Unsupported != 1 || result.Failed != 1 || len(result.Processes) != 1 {
		t.Fatal("process scan failure classification changed")
	}
}

func TestProcessScanCancellationAndDiscoveryFailure(t *testing.T) {
	t.Run("before discovery", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := scanProcesses(ctx, func(context.Context) ([]int32, error) {
			t.Fatal("discovery ran after cancellation")
			return nil, nil
		}, func(context.Context, int32) (ProcessStat, error) {
			t.Fatal("detail read ran after cancellation")
			return ProcessStat{}, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation was lost")
		}
	})
	t.Run("during scan", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		result, err := scanProcesses(ctx, func(context.Context) ([]int32, error) { return []int32{1, 2, 3}, nil }, func(_ context.Context, pid int32) (ProcessStat, error) {
			calls++
			cancel()
			return ProcessStat{PID: pid}, nil
		})
		if !errors.Is(err, context.Canceled) || calls != 1 || result.Scanned != 1 || result.Discovered != 3 {
			t.Fatal("scan continued after cancellation or lost progress")
		}
	})
	t.Run("during discovery", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result, err := scanProcesses(ctx, func(context.Context) ([]int32, error) {
			cancel()
			return []int32{1}, nil
		}, func(context.Context, int32) (ProcessStat, error) {
			t.Fatal("detail read ran after discovery canceled")
			return ProcessStat{}, nil
		})
		if !errors.Is(err, context.Canceled) || result.Scanned != 0 || result.Discovered != 1 {
			t.Fatal("discovery cancellation lost progress or was ignored")
		}
	})
	t.Run("during final detail read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result, err := scanProcesses(ctx, func(context.Context) ([]int32, error) { return []int32{1}, nil }, func(context.Context, int32) (ProcessStat, error) {
			cancel()
			return ProcessStat{PID: 1}, nil
		})
		if !errors.Is(err, context.Canceled) || result.Scanned != 1 {
			t.Fatal("final detail cancellation was reported as success")
		}
	})
	t.Run("discovery fails", func(t *testing.T) {
		_, err := scanProcesses(context.Background(), func(context.Context) ([]int32, error) { return nil, fs.ErrPermission }, func(context.Context, int32) (ProcessStat, error) {
			t.Fatal("detail read ran after discovery failure")
			return ProcessStat{}, nil
		})
		if !errors.Is(err, fs.ErrPermission) {
			t.Fatal("discovery error was lost")
		}
	})
	t.Run("empty", func(t *testing.T) {
		result, err := scanProcesses(context.Background(), func(context.Context) ([]int32, error) { return nil, nil }, func(context.Context, int32) (ProcessStat, error) {
			t.Fatal("empty discovery made a detail read")
			return ProcessStat{}, nil
		})
		if err != nil || result.Processes == nil || result.Scanned != 0 || result.Discovered != 0 || result.ScanTruncated {
			t.Fatal("healthy empty scan misrepresented")
		}
	})
}

func TestProcessScanBudgetMarksCollectorPartial(t *testing.T) {
	result := ProcessResult{Processes: []ProcessStat{{PID: 7, Name: "worker", State: "running", MemoryBytes: 1}}, Discovered: hardMaxProcessScan + 1, Scanned: hardMaxProcessScan, ScanTruncated: true}
	provider := fakeProvider{processResult: &result}
	section := New(nil, provider, DefaultConfig()).Collect(context.Background()).Processes
	if section.State != observation.Degraded || section.ReasonCode != observation.ReasonPartialCollection || !section.Quality.Truncated || section.Quality.Errors != 0 || section.Quality.Total != hardMaxProcessScan+1 {
		t.Fatal("scan cutoff was represented as complete or an acquisition failure")
	}
}

func TestProcessScanBudgetPreservesDeniedCoverage(t *testing.T) {
	result := ProcessResult{Discovered: hardMaxProcessScan + 1, Scanned: hardMaxProcessScan, PermissionDenied: hardMaxProcessScan, ScanTruncated: true}
	section := New(nil, fakeProvider{processResult: &result}, DefaultConfig()).Collect(context.Background()).Processes
	if section.State != observation.Degraded || section.ReasonCode != observation.ReasonPartialCollection || section.Data == nil || len(*section.Data) != 0 || !section.Quality.Truncated || section.Quality.Total != hardMaxProcessScan+1 || section.Quality.Errors != hardMaxProcessScan {
		t.Fatal("denied bounded scan lost its actual coverage or failures")
	}
}
