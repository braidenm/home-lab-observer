package collector

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

const hardMaxProcesses = 200
const hardMaxFilesystems = 64
const hardMaxProcessScan = 4096

type Config struct {
	CollectorVersion             string
	CPUSampleDuration            time.Duration
	MaxProcesses, MaxFilesystems int
	CollectProcesses             bool
}

func DefaultConfig() Config { return Config{"dev", 200 * time.Millisecond, 25, 16, true} }

type Collector struct {
	clock    Clock
	provider SystemProvider
	config   Config
}

func New(clock Clock, provider SystemProvider, cfg Config) *Collector {
	if clock == nil {
		clock = RealClock{}
	}
	if cfg.CollectorVersion == "" {
		cfg.CollectorVersion = "dev"
	}
	if cfg.MaxProcesses < 1 {
		cfg.MaxProcesses = 25
	}
	if cfg.MaxProcesses > hardMaxProcesses {
		cfg.MaxProcesses = hardMaxProcesses
	}
	if cfg.MaxFilesystems < 1 {
		cfg.MaxFilesystems = 16
	}
	if cfg.MaxFilesystems > hardMaxFilesystems {
		cfg.MaxFilesystems = hardMaxFilesystems
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
	s.CPU = c.cpu(ctx)
	s.Memory = c.memory(ctx)
	s.Filesystems = c.filesystems(ctx)
	s.Network = c.network(ctx)
	s.Uptime = c.uptime(ctx)
	s.Processes = c.processes(ctx)
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

func (c *Collector) cpu(ctx context.Context) observation.Section[observation.CPU] {
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
func (c *Collector) memory(ctx context.Context) observation.Section[observation.Memory] {
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
func (c *Collector) filesystems(ctx context.Context) observation.Section[[]observation.Filesystem] {
	start := c.clock.Now()
	ps, e := c.provider.Partitions(ctx)
	if e != nil {
		st, r := classify(e)
		return observation.Section[[]observation.Filesystem]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, 1)}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Mountpoint < ps[j].Mountpoint })
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
		out = append(out, observation.Filesystem{ID: formatID("filesystem", i), DisplayName: formatName("Filesystem", i), TotalBytes: u.Total, UsedBytes: u.Used, FreeBytes: u.Free, UsagePercent: u.UsedPercent})
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
	if errs > 0 {
		st = observation.Degraded
		r = observation.ReasonPartialCollection
	}
	return observation.Section[[]observation.Filesystem]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), len(out), errs), Data: &out}
}
func (c *Collector) network(ctx context.Context) observation.Section[observation.Network] {
	start := c.clock.Now()
	n, e := c.provider.Network(ctx)
	if e != nil {
		st, r := classify(e)
		return observation.Section[observation.Network]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, 1)}
	}
	d := observation.Network{BytesSent: n.BytesSent, BytesRecv: n.BytesRecv, PacketsSent: n.PacketsSent, PacketsRecv: n.PacketsRecv, ErrorsIn: n.ErrorsIn, ErrorsOut: n.ErrorsOut, DropsIn: n.DropsIn, DropsOut: n.DropsOut}
	return observation.Section[observation.Network]{State: observation.Available, Quality: sq(c.clock.Now().Sub(start), 1, 0), Data: &d}
}
func (c *Collector) uptime(ctx context.Context) observation.Section[observation.Uptime] {
	start := c.clock.Now()
	v, e := c.provider.Uptime(ctx)
	if e != nil {
		st, r := classify(e)
		return observation.Section[observation.Uptime]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, 1)}
	}
	d := observation.Uptime{Seconds: v}
	return observation.Section[observation.Uptime]{State: observation.Available, Quality: sq(c.clock.Now().Sub(start), 1, 0), Data: &d}
}
func (c *Collector) processes(ctx context.Context) observation.Section[[]observation.Process] {
	if !c.config.CollectProcesses {
		return observation.Section[[]observation.Process]{State: observation.Disabled, ReasonCode: observation.ReasonDisabled}
	}
	start := c.clock.Now()
	ps, skipped, e := c.provider.Processes(ctx)
	if e != nil {
		st, r := classify(e)
		return observation.Section[[]observation.Process]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, 1)}
	}
	now := c.clock.Now().UnixMilli()
	out := make([]observation.Process, 0, len(ps))
	errs := skipped
	if len(ps) > hardMaxProcessScan {
		ps = ps[:hardMaxProcessScan]
		errs++
	}
	for _, p := range ps {
		if ctx.Err() != nil {
			errs++
			break
		}
		if p.PID <= 0 || !percentLoose(p.CPUPercent) {
			errs++
			continue
		}
		age := uint64(0)
		if p.CreateTimeMS > 0 && now > p.CreateTimeMS {
			age = uint64((now - p.CreateTimeMS) / 1000)
		}
		out = append(out, observation.Process{PID: p.PID, Name: safeName(p.Name), CPUPercent: p.CPUPercent, MemoryBytes: p.MemoryBytes, UptimeSeconds: age})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MemoryBytes != out[j].MemoryBytes {
			return out[i].MemoryBytes > out[j].MemoryBytes
		}
		if out[i].CPUPercent != out[j].CPUPercent {
			return out[i].CPUPercent > out[j].CPUPercent
		}
		return out[i].PID < out[j].PID
	})
	if len(out) > c.config.MaxProcesses {
		out = out[:c.config.MaxProcesses]
	}
	st, r := observation.Available, observation.ReasonCode("")
	if errs > 0 {
		st = observation.Degraded
		r = observation.ReasonPartialCollection
	}
	return observation.Section[[]observation.Process]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), len(out), errs), Data: &out}
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
func percent(v float64) bool      { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 100 }
func percentLoose(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func safeName(v string) string {
	v = strings.ReplaceAll(v, "\\", "/")
	if i := strings.LastIndexByte(v, '/'); i >= 0 {
		v = v[i+1:]
	}
	v = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(v, "")))
	if v == "" {
		return "process"
	}
	for len(v) > 128 {
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
