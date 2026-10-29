package workloadprojection

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	containerobs "github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
)

// Validate accepts canonical producer bytes only. It proves neither consent nor
// freshness/authentication, and does not extend the connected upload schema set.
func Validate(b []byte, source string) error {
	if len(b) == 0 || len(b) > MaxBytes || !utf8.Valid(b) {
		return ErrValidation
	}
	var d document
	if json.Unmarshal(b, &d) != nil || !validDocument(d, source) {
		return ErrValidation
	}
	canonical, err := json.Marshal(d)
	if err != nil || !bytes.Equal(canonical, b) {
		return ErrValidation
	}
	return nil
}

func validTime(value string) bool {
	t, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 && t.UTC().Format(time.RFC3339Nano) == value
}

func percent(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 100 }
func processPercent(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 409600
}
func textValid(v string, limit int) bool {
	if v == "" || len(v) > limit || !utf8.ValidString(v) || sensitive.MatchString(v) {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}
func validQuality(v quality) bool {
	reason := ""
	if v.Reason != nil {
		reason = *v.Reason
	}
	want := q(v.State, reason)
	return v.State == want.State && ((v.Reason == nil && want.Reason == nil) || (v.Reason != nil && want.Reason != nil && *v.Reason == *want.Reason))
}
func validSection[T any](s section[T], enabled bool, rows int) bool {
	if !validQuality(s.Quality) || s.Items == nil || len(s.Items) > rows || s.Returned != len(s.Items) || (s.Total != nil && (*s.Total > maxExactInteger || *s.Total < uint64(s.Returned))) || (s.ObservedAt != nil && !validTime(*s.ObservedAt)) {
		return false
	}
	if !enabled && s.Quality.State != observation.Disabled {
		return false
	}
	if s.Total != nil && (*s.Total > uint64(s.Returned)) != s.Truncated {
		return false
	}
	switch s.Quality.State {
	case observation.Available:
		if s.ObservedAt == nil || s.Total == nil || *s.Total != uint64(s.Returned) || s.Truncated {
			return false
		}
	case observation.Degraded:
		if s.ObservedAt == nil || (s.Truncated && s.Total != nil && *s.Total <= uint64(s.Returned)) {
			return false
		}
	default:
		if len(s.Items) != 0 || s.Total != nil || s.Truncated {
			return false
		}
		if (s.Quality.State == observation.Disabled || s.Quality.State == observation.Unknown) && s.ObservedAt != nil {
			return false
		}
	}
	b, err := json.Marshal(s)
	return err == nil && len(b) <= MaxSectionBytes
}
func validDocument(d document, source string) bool {
	if d.Schema != SchemaVersion || d.Profile != Profile || !sourcePattern.MatchString(source) || d.Source != source || len(d.Version) > 40 || !versionPattern.MatchString(d.Version) || !validTime(d.At) || d.Policy.Classification != "SENSITIVE" || !d.Policy.ReadOnly || d.Processes.CPUBasis != "LIFETIME_SINGLE_CORE" || !validSection(d.Processes, d.Policy.Processes, MaxProcesses) || !validContainersSection(d.Containers, d.Policy.Containers) {
		return false
	}
	seenPID := make(map[int32]bool, len(d.Processes.Items))
	for i, v := range d.Processes.Items {
		if v.PID <= 0 || seenPID[v.PID] || !textValid(v.Name, 128) || strings.ContainsAny(v.Name, "/\\") || processState(v.State) != v.State || v.CPU == nil || !processPercent(*v.CPU) || v.Memory == nil || *v.Memory > maxExactInteger || (v.Elapsed != nil && (*v.Elapsed == 0 || *v.Elapsed > maxExactInteger)) {
			return false
		}
		if i > 0 {
			prior := d.Processes.Items[i-1]
			if *prior.Memory < *v.Memory || (*prior.Memory == *v.Memory && (*prior.CPU < *v.CPU || (*prior.CPU == *v.CPU && prior.PID > v.PID))) {
				return false
			}
		}
		seenPID[v.PID] = true
	}
	return true
}

func validContainersSection(s section[containerRow], enabled bool) bool {
	if s.CPUBasis != "ENGINE_HOST_CAPACITY" || !validSection(s, enabled, MaxContainers) {
		return false
	}
	seenAlias := make(map[string]bool, len(s.Items))
	for i, v := range s.Items {
		if seenAlias[v.Alias] || !validContainer(v) || (i > 0 && s.Items[i-1].Alias > v.Alias) {
			return false
		}
		if (v.MetricsState == containerobs.MetricsPartial || v.MetricsState == containerobs.MetricsUnavailable) && s.Quality.State != observation.Degraded {
			return false
		}
		seenAlias[v.Alias] = true
	}
	return true
}
func containerState(v string) bool {
	switch v {
	case "unknown", "created", "running", "paused", "restarting", "removing", "exited", "dead":
		return true
	}
	return false
}
func metricReason(raw *string) string {
	if raw != nil {
		switch *raw {
		case "PERMISSION_DENIED", "DEADLINE_EXCEEDED", "COLLECTION_CANCELED":
			return *raw
		case "ENGINE_OS_UNSUPPORTED", "API_VERSION_UNSUPPORTED":
			return "UNSUPPORTED"
		case "METRIC_COUNTER_INVALID", "INVALID_COUNTER":
			return "INVALID_COUNTER"
		}
	}
	return "METRIC_UNAVAILABLE"
}
func validContainer(v containerRow) bool {
	if !aliasPattern.MatchString(v.Alias) || !textValid(v.Name, 128) || !textValid(v.Image, 240) || !containerState(v.State) || (v.CPU != nil && !percent(*v.CPU)) || (v.Memory != nil && *v.Memory > maxExactInteger) {
		return false
	}
	if v.State != "running" {
		return v.MetricsState == containerobs.MetricsNotRunning && v.CPU == nil && v.Memory == nil && v.Reason != nil && *v.Reason == "NOT_RUNNING"
	}
	switch v.MetricsState {
	case containerobs.MetricsAvailable:
		return v.CPU != nil && v.Memory != nil && v.Reason == nil
	case containerobs.MetricsPartial:
		return (v.CPU == nil) != (v.Memory == nil) && v.Reason != nil && *v.Reason == "INVALID_COUNTER"
	case containerobs.MetricsUnavailable:
		if v.CPU != nil || v.Memory != nil || v.Reason == nil {
			return false
		}
		switch *v.Reason {
		case "METRIC_UNAVAILABLE", "INVALID_COUNTER", "PERMISSION_DENIED", "UNSUPPORTED", "DEADLINE_EXCEEDED", "COLLECTION_CANCELED":
			return true
		}
	}
	return false
}
