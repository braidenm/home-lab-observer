package collector

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

type fakeProvider struct {
	cpuErr, memoryErr, swapErr, partitionErr, networkErr, uptimeErr, processErr error
	usageErr                                                                    map[string]error
}

func (f fakeProvider) LogicalCPUCount(context.Context) (int, error) { return 8, f.cpuErr }
func (f fakeProvider) CPUPercent(context.Context, time.Duration) (float64, error) {
	return 12.5, f.cpuErr
}
func (f fakeProvider) Memory(context.Context) (MemoryStat, error) {
	return MemoryStat{100, 40, 60, 40}, f.memoryErr
}
func (f fakeProvider) Swap(context.Context) (SwapStat, error) { return SwapStat{10, 2}, f.swapErr }
func (f fakeProvider) Partitions(context.Context) ([]Partition, error) {
	return []Partition{{`C:\Users\secret`}, {`D:\private`}}, f.partitionErr
}
func (f fakeProvider) Usage(_ context.Context, p string) (UsageStat, error) {
	if e := f.usageErr[p]; e != nil {
		return UsageStat{}, e
	}
	return UsageStat{100, 50, 50, 50}, nil
}
func (f fakeProvider) Network(context.Context) (NetStat, error) {
	return NetStat{1, 2, 3, 4, 5, 6, 7, 8}, f.networkErr
}
func (f fakeProvider) Uptime(context.Context) (uint64, error) { return 99, f.uptimeErr }
func (f fakeProvider) Processes(context.Context) ([]ProcessStat, int, error) {
	return []ProcessStat{{3, "small", 99, 10, 0}, {2, `C:\Users\secret\big` + "\x00name", 1, 100, 0}, {1, "medium", 2, 50, 0}}, 0, f.processErr
}

func TestCollectDeterministicSafeAndBounded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxProcesses = 2
	s := New(fixedClock{time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)}, fakeProvider{}, cfg).Collect(context.Background())
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.Quality.State != observation.Complete {
		t.Fatalf("quality=%s", s.Quality.State)
	}
	if got := len(*s.Processes.Data); got != 2 {
		t.Fatalf("processes=%d", got)
	}
	if (*s.Processes.Data)[0].PID != 2 {
		t.Fatalf("not sorted by memory: %+v", *s.Processes.Data)
	}
	if got := (*s.Filesystems.Data)[0].ID; got != "filesystem-001" {
		t.Fatalf("id=%s", got)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, secret := range []string{`C:\Users\secret`, `D:\private`, `\u0000`, `argv`, `environment`, `username`, `executable_path`} {
		if strings.Contains(body, secret) {
			t.Fatalf("leaked %q: %s", secret, body)
		}
	}
}

func TestExplicitZeroDiffersFromUnavailable(t *testing.T) {
	s := New(fixedClock{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, fakeProvider{networkErr: errors.New("down")}, DefaultConfig()).Collect(context.Background())
	if s.Network.State != observation.Unavailable || s.Network.Data != nil {
		t.Fatalf("network=%+v", s.Network)
	}
	if s.Uptime.State != observation.Available || s.Uptime.Data == nil {
		t.Fatal("available data omitted")
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), `"network":{"state":"UNAVAILABLE","quality":{"duration_ms":0,"samples":0,"errors":1},"data"`) {
		t.Fatal("unavailable data must be absent")
	}
}

func TestPermissionAndPartialStates(t *testing.T) {
	f := fakeProvider{memoryErr: errors.New("ordinary"), partitionErr: errors.ErrUnsupported, processErr: errors.ErrUnsupported}
	f.cpuErr = errors.New("cpu")
	s := New(fixedClock{time.Now().UTC()}, f, DefaultConfig()).Collect(context.Background())
	if s.CPU.ReasonCode != observation.ReasonCollectionFailed {
		t.Fatalf("reason=%s", s.CPU.ReasonCode)
	}
	if s.Quality.State != observation.Partial {
		t.Fatalf("quality=%s", s.Quality.State)
	}
	f = fakeProvider{memoryErr: fs.ErrPermission}
	s = New(fixedClock{time.Now().UTC()}, f, DefaultConfig()).Collect(context.Background())
	if s.Memory.State != observation.PermissionDenied || s.Memory.ReasonCode != observation.ReasonPermissionDenied {
		t.Fatalf("memory=%+v", s.Memory)
	}
}

func TestFilesystemDegradesWithoutLeakingFailedPath(t *testing.T) {
	f := fakeProvider{usageErr: map[string]error{`D:\private`: errors.New("denied")}}
	s := New(fixedClock{time.Now().UTC()}, f, DefaultConfig()).Collect(context.Background())
	if s.Filesystems.State != observation.Degraded || len(*s.Filesystems.Data) != 1 {
		t.Fatalf("filesystems=%+v", s.Filesystems)
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "private") {
		t.Fatal("path leaked")
	}
}

func TestNoRawErrorsInJSON(t *testing.T) {
	s := New(fixedClock{time.Now().UTC()}, fakeProvider{networkErr: errors.New("token=do-not-leak")}, DefaultConfig()).Collect(context.Background())
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "do-not-leak") || s.Network.ReasonCode != observation.ReasonCollectionFailed {
		t.Fatalf("unsafe error contract: %s", b)
	}
}
