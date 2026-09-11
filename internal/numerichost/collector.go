// Package numerichost samples only numeric host sections. It has no upload or service authority.
package numerichost

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

// Config describes collection bounds. FilesystemCoverageVerified is trusted
// installation evidence about namespace coverage, never an owner override.
type Config struct {
	CollectorVersion           string
	CPUSampleDuration          time.Duration
	MaxFilesystems             int
	FilesystemCoverageVerified bool
}

type Collector struct {
	clock    Clock
	provider Provider
	config   Config
}

func New(clock Clock, provider Provider, cfg Config) *Collector {
	if clock == nil {
		clock = RealClock{}
	}
	if cfg.CollectorVersion == "" {
		cfg.CollectorVersion = "dev"
	}
	if cfg.MaxFilesystems < 1 {
		cfg.MaxFilesystems = 16
	}
	if cfg.MaxFilesystems > 64 {
		cfg.MaxFilesystems = 64
	}
	if cfg.CPUSampleDuration < 0 {
		cfg.CPUSampleDuration = 0
	}
	if cfg.CPUSampleDuration > 5*time.Second {
		cfg.CPUSampleDuration = 5 * time.Second
	}
	return &Collector{clock, provider, cfg}
}
func (c *Collector) Collect(ctx context.Context) observation.Snapshot {
	started := c.clock.Now().UTC()
	s := observation.Snapshot{SchemaVersion: observation.SchemaVersion, CollectorVersion: c.config.CollectorVersion, Source: observation.Source{Kind: "HOST", ID: "local"}, ObservedAt: started}
	s.CPU = c.CPU(ctx)
	s.Memory = c.Memory(ctx)
	s.Filesystems = c.Filesystems(ctx)
	s.Network = observation.Section[observation.Network]{State: observation.Disabled, ReasonCode: observation.ReasonDisabled}
	s.Uptime = c.Uptime(ctx)
	s.Processes = observation.Section[[]observation.Process]{State: observation.Disabled, ReasonCode: observation.ReasonDisabled}
	states := []struct {
		name   observation.CapabilityName
		state  observation.SupportState
		reason observation.ReasonCode
	}{{observation.HostCPU, s.CPU.State, s.CPU.ReasonCode}, {observation.HostMemory, s.Memory.State, s.Memory.ReasonCode}, {observation.HostFilesystems, s.Filesystems.State, s.Filesystems.ReasonCode}, {observation.HostNetwork, s.Network.State, s.Network.ReasonCode}, {observation.HostUptime, s.Uptime.State, s.Uptime.ReasonCode}, {observation.HostProcesses, s.Processes.State, s.Processes.ReasonCode}}
	for _, v := range states {
		s.Capabilities = append(s.Capabilities, observation.Capability{Name: v.name, State: v.state, ReasonCode: v.reason})
		switch v.state {
		case observation.Available:
			s.Quality.AvailableSections++
		case observation.Degraded:
			s.Quality.DegradedSections++
		case observation.Disabled:
		default:
			s.Quality.UnavailableSections++
		}
	}
	if s.Quality.UnavailableSections == 0 && s.Quality.DegradedSections == 0 {
		s.Quality.State = observation.Complete
	} else if s.Quality.AvailableSections+s.Quality.DegradedSections > 0 {
		s.Quality.State = observation.Partial
	} else {
		s.Quality.State = observation.Failed
	}
	s.Quality.StartedAt = started
	s.Quality.FinishedAt = c.clock.Now().UTC()
	s.Quality.DurationMS = max(0, s.Quality.FinishedAt.Sub(started).Milliseconds())
	return s
}

func (c *Collector) CPU(ctx context.Context) observation.Section[observation.CPU] {
	start := c.clock.Now()
	n, e1 := c.provider.LogicalCPUCount(ctx)
	p, e2 := c.provider.CPUPercent(ctx, c.config.CPUSampleDuration)
	q := sq(c.clock.Now().Sub(start), 2, boolInt(e1 != nil)+boolInt(e2 != nil))
	if e1 != nil || e2 != nil || n < 1 || !percent(p) {
		st, r := classify(first(e1, e2))
		return observation.Section[observation.CPU]{State: st, ReasonCode: r, Quality: q}
	}
	d := observation.CPU{LogicalCPUs: n, UsagePercent: p}
	return observation.Section[observation.CPU]{State: observation.Available, Quality: q, Data: &d}
}
func (c *Collector) Memory(ctx context.Context) observation.Section[observation.Memory] {
	start := c.clock.Now()
	m, e := c.provider.Memory(ctx)
	if e != nil {
		st, r := classify(e)
		return observation.Section[observation.Memory]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, 1)}
	}
	sw, se := c.provider.Swap(ctx)
	d := observation.Memory{TotalBytes: m.Total, UsedBytes: m.Used, AvailableBytes: m.Available, UsagePercent: m.UsedPercent, SwapTotalBytes: sw.Total, SwapUsedBytes: sw.Used}
	st, r := observation.Available, observation.ReasonCode("")
	errs := 0
	if se != nil {
		st = observation.Degraded
		r = observation.ReasonPartialCollection
		errs = 1
	}
	if !percent(m.UsedPercent) {
		st = observation.Unavailable
		r = observation.ReasonCollectionFailed
		d = observation.Memory{}
		return observation.Section[observation.Memory]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, errs+1)}
	}
	return observation.Section[observation.Memory]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 1, errs), Data: &d}
}
func (c *Collector) Filesystems(ctx context.Context) observation.Section[[]observation.Filesystem] {
	start := c.clock.Now()
	ps, e := c.provider.Partitions(ctx)
	if e != nil {
		st, r := classify(e)
		return observation.Section[[]observation.Filesystem]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, 1)}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Mountpoint < ps[j].Mountpoint })
	total := len(ps)
	if len(ps) > c.config.MaxFilesystems {
		ps = ps[:c.config.MaxFilesystems]
	}
	out := make([]observation.Filesystem, 0, len(ps))
	errs := 0
	seen := map[string]bool{}
	for _, p := range ps {
		if seen[p.Mountpoint] {
			continue
		}
		seen[p.Mountpoint] = true
		if ctx.Err() != nil {
			errs++
			break
		}
		u, e := c.provider.Usage(ctx, p.Mountpoint)
		if e != nil || !percent(u.UsedPercent) {
			errs++
			continue
		}
		i := len(out) + 1
		out = append(out, observation.Filesystem{ID: formatID("filesystem", i), DisplayName: formatName("Filesystem", i), Type: safeToken(p.Type, "unknown", 32), TotalBytes: u.Total, UsedBytes: u.Used, FreeBytes: u.Free, UsagePercent: u.UsedPercent})
	}
	if len(out) == 0 {
		if errs == 0 {
			return observation.Section[[]observation.Filesystem]{State: observation.Unavailable, ReasonCode: observation.ReasonNoData, Quality: sq(c.clock.Now().Sub(start), 0, 0)}
		}
		st, r := classify(e)
		if errs > 0 && e == nil {
			st = observation.Unavailable
			r = observation.ReasonCollectionFailed
		}
		return observation.Section[[]observation.Filesystem]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, errs)}
	}
	st, r := observation.Available, observation.ReasonCode("")
	if errs > 0 || !c.config.FilesystemCoverageVerified {
		st = observation.Degraded
		r = observation.ReasonPartialCollection
	}
	q := sq(c.clock.Now().Sub(start), len(out), errs)
	q.Total, q.Truncated = total, total > len(out)
	return observation.Section[[]observation.Filesystem]{State: st, ReasonCode: r, Quality: q, Data: &out}
}
func (c *Collector) Uptime(ctx context.Context) observation.Section[observation.Uptime] {
	start := c.clock.Now()
	v, e := c.provider.Uptime(ctx)
	if e != nil {
		st, r := classify(e)
		return observation.Section[observation.Uptime]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, 1)}
	}
	d := observation.Uptime{Seconds: v}
	return observation.Section[observation.Uptime]{State: observation.Available, Quality: sq(c.clock.Now().Sub(start), 1, 0), Data: &d}
}
func classify(e error) (observation.SupportState, observation.ReasonCode) {
	if errors.Is(e, fs.ErrPermission) {
		return observation.PermissionDenied, observation.ReasonPermissionDenied
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return observation.Unavailable, observation.ReasonDeadlineExceeded
	}
	if errors.Is(e, context.Canceled) {
		return observation.Unavailable, observation.ReasonCollectionStopped
	}
	if errors.Is(e, ErrNoData) {
		return observation.Unavailable, observation.ReasonNoData
	}
	if errors.Is(e, errors.ErrUnsupported) {
		return observation.Unsupported, observation.ReasonUnsupported
	}
	return observation.Unavailable, observation.ReasonCollectionFailed
}
func first(es ...error) error {
	for _, e := range es {
		if e != nil {
			return e
		}
	}
	return errors.New("invalid data")
}
func sq(d time.Duration, samples, errs int) observation.SectionQuality {
	return observation.SectionQuality{DurationMS: max(0, d.Milliseconds()), Samples: samples, Errors: errs}
}
func percent(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 100 }
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func safeToken(v, fallback string, limit int) string {
	v = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(v, "")))
	if v == "" {
		return fallback
	}
	for len(v) > limit {
		_, n := utf8.DecodeLastRuneInString(v)
		v = v[:len(v)-n]
	}
	return v
}
func formatID(prefix string, n int) string   { return prefix + "-" + threeDigits(n) }
func formatName(prefix string, n int) string { return prefix + " " + itoa(n) }
func threeDigits(n int) string {
	if n < 10 {
		return "00" + itoa(n)
	}
	if n < 100 {
		return "0" + itoa(n)
	}
	return itoa(n)
}
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := ""
	for n > 0 {
		b = string(rune('0'+n%10)) + b
		n /= 10
	}
	return b
}
