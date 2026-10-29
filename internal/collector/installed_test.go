package collector

import (
	"context"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

func TestInstalledCollectorKeepsUnverifiedFilesystemCoveragePartial(t *testing.T) {
	cfg := DefaultConfig()
	clock := fixedClock{time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
	installed := NewInstalled(clock, fakeProvider{}, cfg).Collect(context.Background())
	local := New(clock, fakeProvider{}, cfg).Collect(context.Background())
	if installed.Filesystems.State != observation.Degraded || installed.Filesystems.ReasonCode != observation.ReasonPartialCollection || installed.Quality.State != observation.Partial {
		t.Fatal("installed namespace claimed complete host filesystem coverage")
	}
	if local.Filesystems.State != observation.Available || local.Quality.State != observation.Complete {
		t.Fatal("local collector behavior changed")
	}
	if installed.Processes.State != observation.Available || installed.Processes.Data == nil {
		t.Fatal("installed process collection missing")
	}
	if installed.Network.State != observation.Disabled || installed.Network.Data != nil || local.Network.State != observation.Available {
		t.Fatal("private-network counters were represented as host network totals")
	}
}
