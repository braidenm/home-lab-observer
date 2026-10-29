package workloadprojection

import (
	"bytes"
	"encoding/json"
	"time"
	"unicode/utf8"

	containerobs "github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
)

// ContainerSection is a detached, sanitized boundary value. Its zero value is
// invalid. It carries neither an Engine handle nor authority or local consent.
type ContainerSection struct{ value section[containerRow] }

// ProjectContainers applies the existing wire privacy and deterministic size
// rules before the Docker reader persists a section. No raw Inventory is saved.
func ProjectContainers(raw containerobs.Inventory) (ContainerSection, error) {
	s, err := projectContainers(raw)
	if err != nil {
		return ContainerSection{}, ErrData
	}
	s.CPUBasis = "ENGINE_HOST_CAPACITY"
	fitSection(&s, MaxContainers)
	if !validContainersSection(s, true) {
		return ContainerSection{}, ErrData
	}
	return ContainerSection{value: s}, nil
}

// ParseContainers accepts only canonical bounded producer bytes and revalidates
// all row/quality/privacy rules. It does not establish freshness or consent.
func ParseContainers(b []byte) (ContainerSection, error) {
	if len(b) == 0 || len(b) > MaxSectionBytes || !utf8.Valid(b) {
		return ContainerSection{}, ErrValidation
	}
	var s section[containerRow]
	if json.Unmarshal(b, &s) != nil || !validContainersSection(s, true) {
		return ContainerSection{}, ErrValidation
	}
	value := ContainerSection{value: s}
	canonical, err := value.Bytes()
	if err != nil || !bytes.Equal(canonical, b) {
		return ContainerSection{}, ErrValidation
	}
	return value, nil
}

// Bytes returns a new canonical copy, never an alias to caller-owned data.
func (s ContainerSection) Bytes() ([]byte, error) {
	if !validContainersSection(s.value, true) {
		return nil, ErrData
	}
	b, err := json.Marshal(s.value)
	if err != nil || len(b) > MaxSectionBytes {
		return nil, ErrData
	}
	return b, nil
}

// ObservedAt returns the section's original observation time, not publication
// time. Missing time is not a healthy-empty observation.
func (s ContainerSection) ObservedAt() (time.Time, bool) {
	if !validContainersSection(s.value, true) || s.value.ObservedAt == nil {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, *s.value.ObservedAt)
	return at, err == nil
}

// UnavailableContainers reports acquisition failure without a fabricated count,
// timestamp or observation. Detailed fixed causes remain local to the reader.
func UnavailableContainers() ContainerSection {
	s := empty[containerRow](observation.Unavailable, "COLLECTION_FAILED")
	s.CPUBasis = "ENGINE_HOST_CAPACITY"
	return ContainerSection{value: s}
}

// EncodeProcessesAndContainers composes approved process acquisition with a
// separately validated file section. Both groups are explicitly enabled here;
// callers must independently enforce the immutable installed policy.
func EncodeProcessesAndContainers(raw observation.Snapshot, id Identity, containers ContainerSection) ([]byte, error) {
	if !validContainersSection(containers.value, true) {
		return nil, ErrData
	}
	// Reuse the existing process projector unchanged rather than duplicating its
	// privacy/quality/CPU semantics. This bounded roundtrip stays inside the pure
	// projection package; it never persists or forwards an intermediate body.
	b, err := Encode(raw, containerobs.Inventory{}, id, Consent{Processes: true})
	if err != nil {
		return nil, err
	}
	var d document
	if json.Unmarshal(b, &d) != nil {
		return nil, ErrData
	}
	d.Policy.Containers, d.Containers = true, containers.value
	if !validDocument(d, id.SourceID) {
		return nil, ErrData
	}
	b, err = json.Marshal(d)
	if err != nil || len(b) > MaxBytes {
		return nil, ErrData
	}
	return b, nil
}
