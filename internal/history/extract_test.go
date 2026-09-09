package history

import (
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

func TestExtractUsesOnlyAllowlistedAggregates(t *testing.T) {
	previousAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	at := previousAt.Add(15 * time.Second)
	cpu := observation.CPU{UsagePercent: 25}
	memory := observation.Memory{UsagePercent: 50}
	filesystems := []observation.Filesystem{{TotalBytes: 100, UsedBytes: 20}, {TotalBytes: 300, UsedBytes: 180}}
	processes := []observation.Process{{Name: "secret-process"}}
	network := observation.Network{BytesRecv: 1300, BytesSent: 2600}
	previousNetwork := observation.Network{BytesRecv: 1000, BytesSent: 2000}
	current := observation.Snapshot{ObservedAt: at, CPU: observation.Section[observation.CPU]{State: observation.Available, Data: &cpu}, Memory: observation.Section[observation.Memory]{State: observation.Available, Data: &memory}, Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Available, Data: &filesystems}, Processes: observation.Section[[]observation.Process]{State: observation.Available, Quality: observation.SectionQuality{Total: 12}, Data: &processes}, Network: observation.Section[observation.Network]{State: observation.Available, Data: &network}}
	previous := observation.Snapshot{ObservedAt: previousAt, Network: observation.Section[observation.Network]{State: observation.Available, Data: &previousNetwork}}
	samples := Extract(current, &previous)
	if len(samples) != 6 {
		t.Fatalf("samples=%+v", samples)
	}
	for _, sample := range samples {
		if _, ok := allowedMetrics[sample.Metric]; !ok {
			t.Fatalf("unexpected metric %q", sample.Metric)
		}
	}
}

func TestExtractOmitsNetworkReset(t *testing.T) {
	at := time.Now().UTC()
	currentNet := observation.Network{BytesRecv: 1}
	previousNet := observation.Network{BytesRecv: 2}
	current := observation.Snapshot{ObservedAt: at, Network: observation.Section[observation.Network]{State: observation.Available, Data: &currentNet}}
	previous := observation.Snapshot{ObservedAt: at.Add(-time.Second), Network: observation.Section[observation.Network]{State: observation.Available, Data: &previousNet}}
	if got := Extract(current, &previous); len(got) != 0 {
		t.Fatalf("samples=%+v", got)
	}
}
