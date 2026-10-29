// Package workloadprojection defines an unused, explicit opt-in workload egress
// contract. It does not collect, persist, transmit, enroll, or authorize data.
package workloadprojection

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"time"

	containerobs "github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
)

const (
	SchemaVersion          = "home-lab-workload-inventory/v1"
	Profile                = "process-container/v1"
	MaxBytes               = 256 * 1024
	MaxSectionBytes        = (MaxBytes - 1024) / 2
	MaxProcesses           = 200
	MaxContainers          = 500
	maxInputRows           = 4096
	maxExactInteger uint64 = 1<<53 - 1
)

var (
	ErrEnvelope    = errors.New("workload_projection_invalid_envelope")
	ErrData        = errors.New("workload_projection_invalid_data")
	ErrValidation  = errors.New("workload_projection_invalid_bytes")
	sourcePattern  = regexp.MustCompile(`^srv_[a-f0-9]{32}$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)
	aliasPattern   = regexp.MustCompile(`^ctr_[a-f0-9]{16}$`)
)

// Identity is enrolled server identity and verified release metadata, not raw host identity.
type Identity struct{ SourceID, Version string }

// Consent is an explicit caller policy, default off. It is not proof of persisted
// local consent, authorization, or receiver support.
type Consent struct{ Processes, Containers bool }

type quality struct {
	State  observation.SupportState `json:"state"`
	Reason *string                  `json:"reason_code"`
}
type section[T any] struct {
	Quality    quality `json:"quality"`
	CPUBasis   string  `json:"cpu_measurement_basis"`
	ObservedAt *string `json:"observed_at"`
	Returned   int     `json:"returned_count"`
	Total      *uint64 `json:"total_count"`
	Truncated  bool    `json:"truncated"`
	Items      []T     `json:"items"`
}
type processRow struct {
	PID     int32    `json:"pid"`
	Name    string   `json:"name"`
	State   string   `json:"state"`
	CPU     *float64 `json:"cpu_percent"`
	Memory  *uint64  `json:"memory_rss_bytes"`
	Elapsed *uint64  `json:"elapsed_seconds"`
}
type containerRow struct {
	Alias        string   `json:"id_alias"`
	Name         string   `json:"name"`
	Image        string   `json:"image"`
	State        string   `json:"state"`
	CPU          *float64 `json:"cpu_percent"`
	Memory       *uint64  `json:"memory_bytes"`
	MetricsState string   `json:"metrics_state"`
	Reason       *string  `json:"reason_code"`
}
type policy struct {
	Classification string `json:"data_classification"`
	ReadOnly       bool   `json:"read_only"`
	Processes      bool   `json:"processes_enabled"`
	Containers     bool   `json:"containers_enabled"`
}
type document struct {
	Schema     string                `json:"schema_version"`
	Profile    string                `json:"projection_profile"`
	Source     string                `json:"source_id"`
	Version    string                `json:"collector_version"`
	At         string                `json:"collected_at"`
	Policy     policy                `json:"policy"`
	Processes  section[processRow]   `json:"processes"`
	Containers section[containerRow] `json:"containers"`
}

func p[T any](v T) *T { return &v }

func q(state observation.SupportState, reason string) quality {
	if state == observation.Available {
		return quality{State: state}
	}
	switch state {
	case observation.Degraded:
		reason = "PARTIAL_COLLECTION"
	case observation.Disabled:
		reason = "DISABLED_BY_CONFIGURATION"
	case observation.PermissionDenied:
		reason = "PERMISSION_DENIED"
	case observation.Unsupported:
		reason = "UNSUPPORTED"
	case observation.Unavailable:
		switch reason {
		case "DEADLINE_EXCEEDED", "COLLECTION_CANCELED", "NO_DATA":
		default:
			reason = "COLLECTION_FAILED"
		}
	default:
		state, reason = observation.Unknown, "NOT_YET_OBSERVED"
	}
	return quality{State: state, Reason: &reason}
}

func empty[T any](state observation.SupportState, reason string) section[T] {
	return section[T]{Quality: q(state, reason), Items: []T{}}
}

// Encode copies only explicitly allowed fields. Disabled groups are never read.
// Returned bytes have no relationship to a local snapshot's source or raw version.
func Encode(raw observation.Snapshot, docker containerobs.Inventory, id Identity, consent Consent) ([]byte, error) {
	if raw.SchemaVersion != observation.SchemaVersion || !validTime(raw.ObservedAt.UTC().Format(time.RFC3339Nano)) || raw.ObservedAt.IsZero() || !sourcePattern.MatchString(id.SourceID) || len(id.Version) > 40 || !versionPattern.MatchString(id.Version) {
		return nil, ErrEnvelope
	}
	d := document{Schema: SchemaVersion, Profile: Profile, Source: id.SourceID, Version: id.Version,
		At: raw.ObservedAt.UTC().Format(time.RFC3339Nano), Policy: policy{"SENSITIVE", true, consent.Processes, consent.Containers},
		Processes: empty[processRow](observation.Disabled, ""), Containers: empty[containerRow](observation.Disabled, "")}
	var err error
	if consent.Processes {
		d.Processes, err = projectProcesses(raw.Processes, d.At)
		if err != nil {
			return nil, ErrData
		}
	}
	if consent.Containers {
		d.Containers, err = projectContainers(docker)
		if err != nil {
			return nil, ErrData
		}
	}
	d.Processes.CPUBasis = "LIFETIME_SINGLE_CORE"
	d.Containers.CPUBasis = "ENGINE_HOST_CAPACITY"
	fitSection(&d.Processes, MaxProcesses)
	fitSection(&d.Containers, MaxContainers)
	if !validDocument(d, id.SourceID) {
		return nil, ErrData
	}
	b, err := json.Marshal(d)
	if err != nil || len(b) > MaxBytes {
		return nil, ErrData
	}
	return b, nil
}

func projectProcesses(raw observation.Section[[]observation.Process], at string) (section[processRow], error) {
	reason := map[observation.ReasonCode]string{observation.ReasonDeadlineExceeded: "DEADLINE_EXCEEDED", observation.ReasonCollectionStopped: "COLLECTION_CANCELED", observation.ReasonNoData: "NO_DATA"}[raw.ReasonCode]
	out := empty[processRow](raw.State, reason)
	if out.Quality.State != observation.Disabled && out.Quality.State != observation.Unknown {
		out.ObservedAt = &at
	}
	if raw.State != observation.Available && raw.State != observation.Degraded {
		return out, nil
	}
	if raw.Data == nil {
		return empty[processRow](observation.Unavailable, "NO_DATA"), nil
	}
	items, meta := *raw.Data, raw.Quality
	if len(items) > maxInputRows || meta.Samples != len(items) || meta.Total < len(items) || meta.Errors < 0 || uint64(meta.Total) > maxExactInteger || meta.Truncated != (meta.Total > len(items)) {
		return out, ErrData
	}
	seen := make(map[int32]bool, len(items))
	for _, v := range items {
		name, err := display(v.Name, 128, true)
		if err != nil || seen[v.PID] || v.PID <= 0 || !processPercent(v.CPUPercent) || v.MemoryBytes > maxExactInteger || v.UptimeSeconds > maxExactInteger {
			return out, ErrData
		}
		seen[v.PID] = true
		row := processRow{PID: v.PID, Name: name, State: processState(v.State), CPU: p(v.CPUPercent), Memory: p(v.MemoryBytes)}
		if v.UptimeSeconds > 0 {
			row.Elapsed = p(v.UptimeSeconds)
		}
		out.Items = append(out.Items, row)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if *a.Memory != *b.Memory {
			return *a.Memory > *b.Memory
		}
		if *a.CPU != *b.CPU {
			return *a.CPU > *b.CPU
		}
		return a.PID < b.PID
	})
	out.Returned, out.Truncated = len(out.Items), meta.Truncated
	if raw.State == observation.Available && meta.Errors == 0 {
		out.Total = p(uint64(meta.Total))
	}
	if raw.State == observation.Degraded || meta.Errors > 0 || meta.Truncated || meta.Total > len(items) {
		out.Quality = q(observation.Degraded, "")
	}
	return out, nil
}

func projectContainers(raw containerobs.Inventory) (section[containerRow], error) {
	states := map[string]observation.SupportState{containerobs.SupportDisabled: observation.Disabled, containerobs.SupportPermissionDenied: observation.PermissionDenied, containerobs.SupportUnsupported: observation.Unsupported, containerobs.SupportUnavailable: observation.Unavailable, containerobs.SupportSupported: observation.Available}
	state := states[raw.SupportState]
	reason := ""
	if raw.ReasonCode != nil {
		reason = *raw.ReasonCode
	}
	out := empty[containerRow](state, reason)
	if raw.SchemaVersion != containerobs.SchemaVersion {
		return out, ErrData
	}
	if state != observation.Available {
		return out, nil
	}
	if raw.Freshness != containerobs.FreshnessCurrent || raw.CollectionState == containerobs.CollectionNotRun {
		return empty[containerRow](observation.Unknown, ""), nil
	}
	if raw.ObservedAt == nil || raw.ObservedAt.IsZero() || !validTime(raw.ObservedAt.UTC().Format(time.RFC3339Nano)) || (raw.CollectionState != containerobs.CollectionOK && raw.CollectionState != containerobs.CollectionPartial) || len(raw.Items) > maxInputRows || raw.ReturnedCount != len(raw.Items) || raw.TotalCount < len(raw.Items) || uint64(raw.TotalCount) > maxExactInteger || raw.Truncated != (raw.TotalCount > len(raw.Items)) {
		return out, ErrData
	}
	out.ObservedAt = p(raw.ObservedAt.UTC().Format(time.RFC3339Nano))
	out.Total, out.Truncated = p(uint64(raw.TotalCount)), raw.Truncated
	seen := make(map[string]bool, len(raw.Items))
	for _, v := range raw.Items {
		name, err1 := display(v.Name, 128, false)
		image, err2 := display(v.Image, 240, false)
		if err1 != nil || err2 != nil || seen[v.IDAlias] || !aliasPattern.MatchString(v.IDAlias) || !containerState(v.State) {
			return out, ErrData
		}
		seen[v.IDAlias] = true
		row := containerRow{Alias: v.IDAlias, Name: name, Image: image, State: v.State, MetricsState: v.MetricsState}
		if v.CPUPercent != nil {
			row.CPU = p(*v.CPUPercent)
		}
		if v.MemoryBytes != nil {
			row.Memory = p(*v.MemoryBytes)
		}
		if row.MetricsState == containerobs.MetricsNotRunning {
			row.Reason = p("NOT_RUNNING")
		} else if row.MetricsState == containerobs.MetricsPartial {
			row.Reason = p("INVALID_COUNTER")
		} else if row.MetricsState != containerobs.MetricsAvailable {
			row.Reason = p(metricReason(v.ReasonCode))
		}
		if !validContainer(row) {
			return out, ErrData
		}
		if row.MetricsState == containerobs.MetricsPartial || row.MetricsState == containerobs.MetricsUnavailable {
			out.Quality = q(observation.Degraded, "")
		}
		out.Items = append(out.Items, row)
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Alias < out.Items[j].Alias })
	out.Returned = len(out.Items)
	if raw.CollectionState == containerobs.CollectionPartial || raw.Truncated || raw.TotalCount > len(raw.Items) {
		out.Quality = q(observation.Degraded, "")
	}
	return out, nil
}

// Bound the final encoded section, including JSON escapes and metadata. Binary
// search avoids repeatedly serializing every possible tail prefix.
func fitSection[T any](s *section[T], rows int) {
	if len(s.Items) > rows {
		s.Items = s.Items[:rows]
		s.Truncated, s.Quality = true, q(observation.Degraded, "")
	}
	s.Returned = len(s.Items)
	b, _ := json.Marshal(s)
	if len(b) <= MaxSectionBytes {
		return
	}
	s.Truncated, s.Quality = true, q(observation.Degraded, "")
	items := s.Items
	low, high := 0, len(items)
	for low < high {
		mid := (low + high + 1) / 2
		s.Items, s.Returned = items[:mid], mid
		b, _ = json.Marshal(s)
		if len(b) <= MaxSectionBytes {
			low = mid
		} else {
			high = mid - 1
		}
	}
	s.Items, s.Returned = items[:low], low
}
