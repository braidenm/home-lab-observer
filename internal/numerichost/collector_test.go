package numerichost_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/collector"
	"github.com/braidenm/home-lab-observer/internal/numerichost"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

type clock struct{}

func (clock) Now() time.Time { return time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC) }

type numericProvider struct {
	calls  []string
	fail   string
	err    error
	cpu    float64
	mounts int
}

func (p *numericProvider) call(name string) error {
	p.calls = append(p.calls, name)
	if p.fail == name {
		return p.err
	}
	return nil
}
func (p *numericProvider) LogicalCPUCount(context.Context) (int, error) { return 8, p.call("count") }
func (p *numericProvider) CPUPercent(context.Context, time.Duration) (float64, error) {
	return p.cpu, p.call("cpu")
}
func (p *numericProvider) Memory(context.Context) (numerichost.MemoryStat, error) {
	return numerichost.MemoryStat{Total: 100, Used: 40, Available: 60, UsedPercent: 40}, p.call("memory")
}
func (p *numericProvider) Swap(context.Context) (numerichost.SwapStat, error) {
	return numerichost.SwapStat{Total: 10, Used: 2}, p.call("swap")
}
func (p *numericProvider) Partitions(context.Context) ([]numerichost.Partition, error) {
	count := p.mounts
	if count == 0 {
		count = 1
	}
	result := make([]numerichost.Partition, count)
	for i := range result {
		result[i] = numerichost.Partition{Mountpoint: "/synthetic-private/" + string(rune('a'+i)), Type: "syntheticfs"}
	}
	return result, p.call("partitions")
}
func (p *numericProvider) Usage(context.Context, string) (numerichost.UsageStat, error) {
	return numerichost.UsageStat{Total: 200, Used: 50, Free: 150, UsedPercent: 25}, p.call("usage")
}
func (p *numericProvider) Uptime(context.Context) (uint64, error) { return 123, p.call("uptime") }

var _ numerichost.Provider = (*numericProvider)(nil) // No excluded method is implemented.

func config() numerichost.Config {
	return numerichost.Config{CollectorVersion: "0.1.0", FilesystemCoverageVerified: true}
}
func project(t *testing.T, s observation.Snapshot) bool {
	t.Helper()
	data, err := remoteprojection.Encode(s, remoteprojection.Identity{SourceID: "srv_0123456789abcdef0123456789abcdef", Version: "0.1.0", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic-private") || strings.Contains(string(data), "syntheticfs") {
		t.Fatal("source metadata escaped projection")
	}
	var doc struct {
		Sections struct {
			Overview struct {
				Available bool `json:"available"`
			} `json:"overview"`
		} `json:"sections"`
	}
	if json.Unmarshal(data, &doc) != nil {
		t.Fatal("invalid projection")
	}
	return doc.Sections.Overview.Available
}

func TestOnlyNumericOperationsAndSafeProjection(t *testing.T) {
	p := &numericProvider{cpu: 12.5}
	s := numerichost.New(clock{}, p, config()).Collect(context.Background())
	if !reflect.DeepEqual(p.calls, []string{"count", "cpu", "memory", "swap", "partitions", "usage", "uptime"}) {
		t.Fatal("unexpected sampling calls")
	}
	if s.Network.State != observation.Disabled || s.Processes.State != observation.Disabled || s.Network.Data != nil || s.Processes.Data != nil {
		t.Fatal("excluded section sampled")
	}
	if s.Quality.State != observation.Complete || !project(t, s) {
		t.Fatal("numeric data missing")
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUnprovenNamespaceCannotPublishCompleteOverview(t *testing.T) {
	s := numerichost.New(clock{}, &numericProvider{}, numerichost.Config{}).Collect(context.Background())
	if s.Filesystems.State != observation.Degraded || s.Quality.State != observation.Partial || project(t, s) {
		t.Fatal("unproven namespace advertised complete")
	}
}

func TestFailuresRemainUnavailableOrPartial(t *testing.T) {
	for _, tc := range []struct {
		name, fail string
		err        error
		state      observation.SupportState
	}{
		{"permission", "memory", fs.ErrPermission, observation.PermissionDenied},
		{"unsupported", "memory", errors.ErrUnsupported, observation.Unsupported},
		{"cancelled", "memory", context.Canceled, observation.Unavailable},
		{"deadline", "memory", context.DeadlineExceeded, observation.Unavailable},
		{"swap", "swap", errors.New("synthetic-private-error"), observation.Degraded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := numerichost.New(clock{}, &numericProvider{fail: tc.fail, err: tc.err}, config()).Collect(context.Background())
			if s.Memory.State != tc.state || project(t, s) {
				t.Fatal("failure state was hidden")
			}
		})
	}
	s := numerichost.New(clock{}, &numericProvider{cpu: math.NaN()}, config()).Collect(context.Background())
	if s.CPU.State != observation.Unavailable || project(t, s) {
		t.Fatal("invalid numeric value accepted")
	}
}

func TestTruncatedFilesystemsCannotPublishCompleteOverview(t *testing.T) {
	cfg := config()
	cfg.MaxFilesystems = 2
	p := &numericProvider{mounts: 3}
	s := numerichost.New(clock{}, p, cfg).Collect(context.Background())
	if len(*s.Filesystems.Data) != 2 || !s.Filesystems.Quality.Truncated || project(t, s) {
		t.Fatal("truncated inventory advertised complete")
	}
	count := 0
	for _, call := range p.calls {
		if call == "usage" {
			count++
		}
	}
	if count != 2 {
		t.Fatal("filesystem bound exceeded")
	}
}

type richProvider struct{ *numericProvider }

func (*richProvider) Network(context.Context) (collector.NetStat, error) {
	return collector.NetStat{}, nil
}
func (*richProvider) Processes(context.Context) (collector.ProcessResult, error) {
	return collector.ProcessResult{}, nil
}

func TestRichCollectorPreservesNumericSections(t *testing.T) {
	for _, fail := range []string{"", "memory", "swap", "usage", "partitions", "uptime", "cpu"} {
		left := &numericProvider{cpu: 12.5, fail: fail, err: fs.ErrPermission}
		right := &numericProvider{cpu: 12.5, fail: fail, err: fs.ErrPermission}
		rich := collector.New(clock{}, &richProvider{left}, collector.DefaultConfig()).Collect(context.Background())
		numeric := numerichost.New(clock{}, right, config()).Collect(context.Background())
		if !reflect.DeepEqual(rich.CPU, numeric.CPU) || !reflect.DeepEqual(rich.Memory, numeric.Memory) || !reflect.DeepEqual(rich.Filesystems, numeric.Filesystems) || !reflect.DeepEqual(rich.Uptime, numeric.Uptime) {
			t.Fatal("rich numeric semantics changed")
		}
	}
}

func TestProductionDependencyClosure(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("crosscompiled test runner: dependency proof runs with Go in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "go", "list", "-deps", "github.com/braidenm/home-lab-observer/internal/numerichost").Output()
	if err != nil {
		t.Fatal("dependency graph unavailable")
	}
	for _, dependency := range strings.Fields(string(output)) {
		const internalPrefix = "github.com/braidenm/home-lab-observer/internal/"
		if strings.HasPrefix(dependency, internalPrefix) && dependency != internalPrefix+"numerichost" && dependency != internalPrefix+"observation" {
			t.Fatal("numeric collection gained an unapproved internal dependency", dependency)
		}
		for _, forbidden := range []string{"/gopsutil/v4/process", "/gopsutil/v4/net", "net/http"} {
			if strings.HasSuffix(dependency, forbidden) {
				t.Fatal("numeric collection gained excluded dependency", dependency)
			}
		}
	}
}

func TestOtherNativePlatformsFailExplicitly(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("other-platform contract")
	}
	s := numerichost.New(clock{}, numerichost.GopsutilProvider{}, config()).Collect(context.Background())
	if s.CPU.State != observation.Unsupported || s.Memory.State != observation.Unsupported || s.Filesystems.State != observation.Unsupported || s.Uptime.State != observation.Unsupported {
		t.Fatal("unsupported adapter fabricated observations")
	}
}
