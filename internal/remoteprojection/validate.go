package remoteprojection

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

// ErrValidation deliberately omits rejected data and parser diagnostics.
var ErrValidation = errors.New("remote_projection_invalid_document")

// Validate accepts only the canonical private-file bytes produced by Encode for
// expectedServerID. It is not a general legacy HTTP document validator or auth check.
func Validate(data []byte, expectedServerID string) error {
	if len(data) == 0 || len(data) > MaxBytes || !sourcePattern.MatchString(expectedServerID) {
		return ErrValidation
	}
	var decoded document
	// encoding/json cannot allocate an embedded pointer to an unexported type.
	// Allocate it explicitly; absent data is never inferred from this allocation.
	decoded.Sections.Overview.host = &host{}
	if json.Unmarshal(data, &decoded) != nil || decoded.SourceID != expectedServerID {
		return ErrValidation
	}
	at, err := time.Parse(time.RFC3339Nano, decoded.CollectedAt)
	if err != nil {
		return ErrValidation
	}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at,
		Quality: observation.Quality{DurationMS: decoded.DurationMS}}
	identity := Identity{SourceID: expectedServerID, Version: decoded.CollectorVersion, OS: "linux"}
	if decoded.Sections.Overview.Available {
		h := decoded.Sections.Overview.host
		if h == nil {
			return ErrValidation
		}
		identity.OS = map[string]string{"Linux": "linux", "Windows": "windows", "macOS": "darwin"}[h.OperatingSystem]
		raw.CPU = observation.Section[observation.CPU]{State: observation.Available,
			Data: &observation.CPU{LogicalCPUs: h.LogicalCPUCount, UsagePercent: h.CPUUsagePercent}}
		raw.Memory = observation.Section[observation.Memory]{State: observation.Available,
			Data: &observation.Memory{TotalBytes: h.MemoryTotalBytes, UsedBytes: h.MemoryUsedBytes,
				SwapTotalBytes: h.SwapTotalBytes, SwapUsedBytes: h.SwapUsedBytes}}
		raw.Uptime = observation.Section[observation.Uptime]{State: observation.Available,
			Data: &observation.Uptime{Seconds: h.UptimeSeconds}}
		fs := make([]observation.Filesystem, len(h.Filesystems))
		for i, item := range h.Filesystems {
			fs[i] = observation.Filesystem{TotalBytes: item.TotalBytes, UsedBytes: item.UsedBytes}
		}
		raw.Filesystems = observation.Section[[]observation.Filesystem]{State: observation.Available,
			Data: &fs, Quality: observation.SectionQuality{Samples: len(fs), Total: len(fs)}}
	}
	canonical, err := Encode(raw, identity)
	if err != nil || !bytes.Equal(data, canonical) {
		return ErrValidation
	}
	return nil
}
