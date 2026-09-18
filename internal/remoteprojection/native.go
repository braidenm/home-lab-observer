package remoteprojection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

const NativeSchemaVersion = "home-lab-native-host-metrics/v1"

type nativeQuality struct {
	State  observation.SupportState `json:"state"`
	Reason *string                  `json:"reason_code"`
}
type nativeCPU struct {
	Quality nativeQuality `json:"quality"`
	Count   *int          `json:"logical_cpu_count"`
	Usage   *float64      `json:"usage_percent"`
}
type nativeMemory struct {
	Quality   nativeQuality `json:"quality"`
	Total     *uint64       `json:"total_bytes"`
	Used      *uint64       `json:"used_bytes"`
	SwapTotal *uint64       `json:"swap_total_bytes"`
	SwapUsed  *uint64       `json:"swap_used_bytes"`
}
type nativeUptime struct {
	Quality nativeQuality `json:"quality"`
	Seconds *uint64       `json:"uptime_seconds"`
}
type nativeFilesystem struct {
	Alias string `json:"id_alias"`
	Total uint64 `json:"total_bytes"`
	Used  uint64 `json:"used_bytes"`
}
type nativeFilesystems struct {
	Quality   nativeQuality      `json:"quality"`
	Returned  int                `json:"returned_count"`
	Total     *uint64            `json:"total_count"`
	Truncated bool               `json:"truncated"`
	Items     []nativeFilesystem `json:"items"`
}
type nativeHost struct {
	OS          string            `json:"operating_system"`
	CPU         nativeCPU         `json:"cpu"`
	Memory      nativeMemory      `json:"memory"`
	Uptime      nativeUptime      `json:"uptime"`
	Filesystems nativeFilesystems `json:"filesystems"`
}
type nativeDocument struct {
	Schema   string     `json:"schema_version"`
	Source   string     `json:"source_id"`
	Version  string     `json:"collector_version"`
	At       string     `json:"collected_at"`
	Duration int64      `json:"duration_ms"`
	Host     nativeHost `json:"host"`
}

func ptr[T any](v T) *T { return &v }

// nativeState maps only closed constants; local reason text never leaves the host.
func nativeState(state observation.SupportState, reason observation.ReasonCode) nativeQuality {
	q := nativeQuality{State: state}
	switch state {
	case observation.Available:
		return q
	case observation.Degraded:
		q.Reason = ptr("PARTIAL_COLLECTION")
	case observation.Disabled:
		q.Reason = ptr("DISABLED_BY_CONFIGURATION")
	case observation.PermissionDenied:
		q.Reason = ptr("PERMISSION_DENIED")
	case observation.Unsupported:
		q.Reason = ptr("UNSUPPORTED")
	case observation.Unavailable:
		r := "COLLECTION_FAILED"
		switch reason {
		case observation.ReasonDeadlineExceeded:
			r = "DEADLINE_EXCEEDED"
		case observation.ReasonCollectionStopped:
			r = "COLLECTION_CANCELED"
		case observation.ReasonNoData:
			r = "NO_DATA"
		}
		q.Reason = &r
	default:
		q.State = observation.Unknown
		q.Reason = ptr("NOT_YET_OBSERVED")
	}
	return q
}

func failedNative() nativeQuality {
	return nativeState(observation.Unavailable, observation.ReasonCollectionFailed)
}
func completeNative(q observation.SectionQuality) bool {
	return q.Errors == 0 && !q.Truncated && q.Samples >= 0 && q.Total >= 0
}

// EncodeNative produces independent nullable host metrics. It never emits local
// identifiers, names, paths, processes or reasons outside the closed wire enum.
func EncodeNative(raw observation.Snapshot, identity Identity) ([]byte, error) {
	osName, ok := map[string]string{"linux": "Linux", "windows": "Windows", "darwin": "macOS"}[identity.OS]
	if !ok || !sourcePattern.MatchString(identity.SourceID) || len(identity.Version) > 40 || !versionPattern.MatchString(identity.Version) || raw.SchemaVersion != observation.SchemaVersion || raw.ObservedAt.IsZero() || raw.ObservedAt.UTC().Year() < 1 || raw.ObservedAt.UTC().Year() > 9999 || raw.Quality.DurationMS < 0 || raw.Quality.DurationMS > 10000 {
		return nil, ErrEnvelope
	}
	d := nativeDocument{Schema: NativeSchemaVersion, Source: identity.SourceID, Version: identity.Version, At: raw.ObservedAt.UTC().Format(time.RFC3339Nano), Duration: raw.Quality.DurationMS}
	d.Host.OS = osName
	c := &d.Host.CPU
	c.Quality = nativeState(raw.CPU.State, raw.CPU.ReasonCode)
	if raw.CPU.State == observation.Available && raw.CPU.Data != nil && completeNative(raw.CPU.Quality) {
		c.Count = ptr(raw.CPU.Data.LogicalCPUs)
		c.Usage = ptr(raw.CPU.Data.UsagePercent)
	} else if raw.CPU.State == observation.Available || raw.CPU.State == observation.Degraded {
		c.Quality = failedNative()
	}
	m := &d.Host.Memory
	m.Quality = nativeState(raw.Memory.State, raw.Memory.ReasonCode)
	if raw.Memory.State == observation.Available && raw.Memory.Data != nil && completeNative(raw.Memory.Quality) {
		v := raw.Memory.Data
		m.Total = ptr(v.TotalBytes)
		m.Used = ptr(v.UsedBytes)
		m.SwapTotal = ptr(v.SwapTotalBytes)
		m.SwapUsed = ptr(v.SwapUsedBytes)
	} else if raw.Memory.State == observation.Degraded && raw.Memory.ReasonCode == observation.ReasonPartialCollection && raw.Memory.Data != nil && !raw.Memory.Quality.Truncated && raw.Memory.Quality.Errors >= 0 {
		m.Total = ptr(raw.Memory.Data.TotalBytes)
		m.Used = ptr(raw.Memory.Data.UsedBytes)
	} else if raw.Memory.State == observation.Available || raw.Memory.State == observation.Degraded {
		m.Quality = failedNative()
	}
	u := &d.Host.Uptime
	u.Quality = nativeState(raw.Uptime.State, raw.Uptime.ReasonCode)
	if raw.Uptime.State == observation.Available && raw.Uptime.Data != nil && completeNative(raw.Uptime.Quality) {
		u.Seconds = ptr(raw.Uptime.Data.Seconds)
	} else if raw.Uptime.State == observation.Available || raw.Uptime.State == observation.Degraded {
		u.Quality = failedNative()
	}
	f := &d.Host.Filesystems
	f.Quality = nativeState(raw.Filesystems.State, raw.Filesystems.ReasonCode)
	f.Items = []nativeFilesystem{}
	if raw.Filesystems.State == observation.Available || raw.Filesystems.State == observation.Degraded {
		if raw.Filesystems.Data == nil {
			f.Quality = failedNative()
		} else {
			items, q := *raw.Filesystems.Data, raw.Filesystems.Quality
			if len(items) > MaxFilesystems || q.Samples != len(items) || q.Total < len(items) || q.Errors < 0 || (q.Truncated && q.Total <= len(items)) {
				return nil, ErrHostData
			}
			for i, v := range items {
				f.Items = append(f.Items, nativeFilesystem{fmt.Sprintf("filesystem-%02d", i+1), v.TotalBytes, v.UsedBytes})
			}
			f.Returned = len(items)
			if raw.Filesystems.State == observation.Available && completeNative(q) && q.Total == len(items) {
				f.Total = ptr(uint64(len(items)))
			} else if len(items) > 0 {
				f.Quality = nativeState(observation.Degraded, observation.ReasonPartialCollection)
				f.Truncated = q.Truncated
			} else {
				f.Quality = nativeState(observation.Unavailable, observation.ReasonNoData)
			}
		}
	}
	if !validNative(d, identity.SourceID) {
		return nil, ErrHostData
	}
	data, err := json.Marshal(d)
	if err != nil || len(data) > MaxBytes {
		return nil, ErrHostData
	}
	return data, nil
}

// ValidateNative accepts exactly canonical producer bytes, not arbitrary receiver
// JSON. Validation is schema/binding checking, never authorization or freshness.
func ValidateNative(data []byte, expectedServerID string) error {
	if len(data) == 0 || len(data) > MaxBytes {
		return ErrValidation
	}
	var d nativeDocument
	if json.Unmarshal(data, &d) != nil || !validNative(d, expectedServerID) {
		return ErrValidation
	}
	canonical, err := json.Marshal(d)
	if err != nil || !bytes.Equal(canonical, data) {
		return ErrValidation
	}
	return nil
}

func qualityValid(q nativeQuality) bool {
	if q.State == observation.Available {
		return q.Reason == nil
	}
	if q.Reason == nil {
		return false
	}
	if q.State == observation.Unavailable {
		switch *q.Reason {
		case "COLLECTION_FAILED", "DEADLINE_EXCEEDED", "COLLECTION_CANCELED", "NO_DATA":
			return true
		}
		return false
	}
	want := nativeState(q.State, "")
	return q.State == want.State && want.Reason != nil && *q.Reason == *want.Reason
}
func pairValid(total, used *uint64) bool {
	return (total == nil && used == nil) || (total != nil && used != nil && *total <= MaxExactInteger && *used <= *total)
}
func validNative(d nativeDocument, server string) bool {
	if d.Schema != NativeSchemaVersion || !sourcePattern.MatchString(server) || d.Source != server || len(d.Version) > 40 || !versionPattern.MatchString(d.Version) || d.Duration < 0 || d.Duration > 10000 {
		return false
	}
	at, err := time.Parse(time.RFC3339Nano, d.At)
	if err != nil || at.IsZero() || at.UTC().Year() < 1 || at.UTC().Year() > 9999 || at.UTC().Format(time.RFC3339Nano) != d.At {
		return false
	}
	if d.Host.OS != "Linux" && d.Host.OS != "Windows" && d.Host.OS != "macOS" {
		return false
	}
	c, m, u, f := d.Host.CPU, d.Host.Memory, d.Host.Uptime, d.Host.Filesystems
	if !qualityValid(c.Quality) || !qualityValid(m.Quality) || !qualityValid(u.Quality) || !qualityValid(f.Quality) {
		return false
	}
	if c.Quality.State == observation.Available {
		if c.Count == nil || c.Usage == nil || *c.Count < 1 || *c.Count > 4096 || math.IsNaN(*c.Usage) || math.IsInf(*c.Usage, 0) || *c.Usage < 0 || *c.Usage > 100 {
			return false
		}
	} else if c.Count != nil || c.Usage != nil || c.Quality.State == observation.Degraded {
		return false
	}
	if !pairValid(m.Total, m.Used) || !pairValid(m.SwapTotal, m.SwapUsed) {
		return false
	}
	switch m.Quality.State {
	case observation.Available:
		if m.Total == nil || m.SwapTotal == nil {
			return false
		}
	case observation.Degraded:
		if m.Total == nil || m.SwapTotal != nil {
			return false
		}
	default:
		if m.Total != nil || m.SwapTotal != nil {
			return false
		}
	}
	if u.Quality.State == observation.Available {
		if u.Seconds == nil || *u.Seconds > MaxExactInteger {
			return false
		}
	} else if u.Seconds != nil || u.Quality.State == observation.Degraded {
		return false
	}
	if f.Items == nil || len(f.Items) > MaxFilesystems || f.Returned != len(f.Items) {
		return false
	}
	for i, v := range f.Items {
		if v.Alias != fmt.Sprintf("filesystem-%02d", i+1) || v.Total > MaxExactInteger || v.Used > v.Total {
			return false
		}
	}
	switch f.Quality.State {
	case observation.Available:
		return f.Total != nil && *f.Total == uint64(f.Returned) && !f.Truncated
	case observation.Degraded:
		return f.Returned > 0 && f.Total == nil
	default:
		return f.Returned == 0 && f.Total == nil && !f.Truncated
	}
}
