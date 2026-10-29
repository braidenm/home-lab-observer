// Package containerobs isolates local, read-only container observations from host metrics and transports.
package containerobs

import (
	"time"

	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
)

const SchemaVersion = model.SchemaVersion
const MaxContainers = model.MaxContainers

const (
	SupportSupported        = model.SupportSupported
	SupportDisabled         = model.SupportDisabled
	SupportUnavailable      = model.SupportUnavailable
	SupportPermissionDenied = model.SupportPermissionDenied
	SupportUnsupported      = model.SupportUnsupported

	CollectionOK      = model.CollectionOK
	CollectionPartial = model.CollectionPartial
	CollectionNotRun  = model.CollectionNotRun

	FreshnessCurrent = model.FreshnessCurrent
	FreshnessUnknown = model.FreshnessUnknown

	MetricsAvailable   = model.MetricsAvailable
	MetricsPartial     = model.MetricsPartial
	MetricsUnavailable = model.MetricsUnavailable
	MetricsNotRunning  = model.MetricsNotRunning

	StateUnknown    = model.StateUnknown
	StateCreated    = model.StateCreated
	StateRunning    = model.StateRunning
	StatePaused     = model.StatePaused
	StateRestarting = model.StateRestarting
	StateRemoving   = model.StateRemoving
	StateExited     = model.StateExited
	StateDead       = model.StateDead
)

// Aliases preserve the collector API while projections use the adapter-free model.
type Policy = model.Policy
type Container = model.Container
type Inventory = model.Inventory

func Disabled() Inventory {
	reason := "DOCKER_NOT_CONFIGURED"
	return Inventory{SchemaVersion: SchemaVersion, SupportState: SupportDisabled, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown, ReasonCode: &reason, Items: []Container{}, Policy: dataPolicy()}
}

func dataPolicy() Policy {
	return Policy{ReadOnly: true, DataClassification: "LOCAL_SENSITIVE", RemoteUploadEligible: false}
}

func cloneInventory(in Inventory) Inventory {
	out := in
	out.ReasonCode = cloneString(in.ReasonCode)
	out.ObservedAt = cloneTime(in.ObservedAt)
	out.Items = make([]Container, len(in.Items))
	for index := range in.Items {
		out.Items[index] = in.Items[index]
		out.Items[index].CPUPercent = cloneFloat(in.Items[index].CPUPercent)
		out.Items[index].MemoryBytes = cloneUint64(in.Items[index].MemoryBytes)
		out.Items[index].ReasonCode = cloneString(in.Items[index].ReasonCode)
	}
	return out
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
