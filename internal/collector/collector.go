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

	"github.com/braidenm/home-lab-observer/internal/numerichost"
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
	numeric  *numerichost.Collector
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
	// Preserve the rich observer's existing local-view semantics. This does not
	// establish namespace evidence for an installed isolated numeric worker.
	return &Collector{clock: clock, provider: provider, config: cfg, numeric: numerichost.New(clock, provider, numerichost.Config{CollectorVersion: cfg.CollectorVersion, CPUSampleDuration: cfg.CPUSampleDuration, MaxFilesystems: cfg.MaxFilesystems, FilesystemCoverageVerified: true})}
}

func (c *Collector) Collect(ctx context.Context) observation.Snapshot {
	started := c.clock.Now().UTC()
	s := observation.Snapshot{SchemaVersion: observation.SchemaVersion, CollectorVersion: c.config.CollectorVersion, Source: observation.Source{Kind: "HOST", ID: "local"}, ObservedAt: started}
	s.CPU = c.numeric.CPU(ctx)
	s.Memory = c.numeric.Memory(ctx)
	s.Filesystems = c.numeric.Filesystems(ctx)
	s.Network = c.network(ctx)
	s.Uptime = c.numeric.Uptime(ctx)
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
func (c *Collector) processes(ctx context.Context) observation.Section[[]observation.Process] {
	if !c.config.CollectProcesses {
		return observation.Section[[]observation.Process]{State: observation.Disabled, ReasonCode: observation.ReasonDisabled}
	}
	start := c.clock.Now()
	result, e := c.provider.Processes(ctx)
	if e != nil {
		st, r := classify(e)
		return observation.Section[[]observation.Process]{State: st, ReasonCode: r, Quality: sq(c.clock.Now().Sub(start), 0, 1)}
	}
	if len(result.Processes) == 0 && result.Scanned > 0 {
		q := sq(c.clock.Now().Sub(start), 0, result.PermissionDenied+result.Unsupported+result.Failed)
		q.Total = result.Discovered
		switch {
		case result.PermissionDenied == result.Scanned:
			return observation.Section[[]observation.Process]{State: observation.PermissionDenied, ReasonCode: observation.ReasonPermissionDenied, Quality: q}
		case result.Unsupported == result.Scanned:
			return observation.Section[[]observation.Process]{State: observation.Unsupported, ReasonCode: observation.ReasonUnsupported, Quality: q}
		default:
			return observation.Section[[]observation.Process]{State: observation.Unavailable, ReasonCode: observation.ReasonCollectionFailed, Quality: q}
		}
	}
	ps := result.Processes
	now := c.clock.Now().UnixMilli()
	out := make([]observation.Process, 0, len(ps))
	errs := result.PermissionDenied + result.Unsupported + result.Failed
	if len(ps) > hardMaxProcessScan {
		ps = ps[:hardMaxProcessScan]
		errs++
	}
	for _, p := range ps {
		if ctx.Err() != nil {
			errs++
			break
		}
		if p.PID <= 0 || !percent(p.CPUPercent) {
			errs++
			continue
		}
		age := uint64(0)
		if p.CreateTimeMS > 0 && now > p.CreateTimeMS {
			age = uint64((now - p.CreateTimeMS) / 1000)
		}
		out = append(out, observation.Process{PID: p.PID, Name: safeName(p.Name), State: safeToken(p.State, "unknown", 32), CPUPercent: p.CPUPercent, MemoryBytes: p.MemoryBytes, UptimeSeconds: age})
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
	q := sq(c.clock.Now().Sub(start), len(out), errs)
	q.Total, q.Truncated = result.Discovered, result.Discovered > len(out)
	return observation.Section[[]observation.Process]{State: st, ReasonCode: r, Quality: q, Data: &out}
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
func sq(d time.Duration, samples, errs int) observation.SectionQuality {
	return observation.SectionQuality{DurationMS: max(0, d.Milliseconds()), Samples: samples, Errors: errs}
}
func percent(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 100 }
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
