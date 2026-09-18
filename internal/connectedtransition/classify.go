package connectedtransition

import "bytes"

// Classification is private, read-only recovery evidence. Neither value is a
// worker startup permit, nor does it imply durable filesystem synchronization.
type Classification string

const (
	RecoveryRequired Classification = "RECOVERY_REQUIRED"
	Complete         Classification = "RECORD_COMPLETE"
)

type Observed struct {
	Resources Resources
	Ledger    Ledger
	// Nil means the exact receipt name is absent. A present empty file is invalid.
	Receipt []byte
}

func either(actual, previous, next string) bool {
	return actual == previous || actual == next
}

// Classify accepts only the exact recorded ledger and previous/next resource
// values. Callers must first validate ownership, paths, stopped workers,
// predecessor evidence and the actual file bytes behind every hash.
func Classify(record Record, observed Observed) (Classification, error) {
	canonical, err := Encode(record)
	if err != nil || observed.Ledger != record.Ledger {
		return "", ErrInvalid
	}
	a, before, after := observed.Resources, record.PreviousResources, record.NextResources
	if !either(a.CA, before.CA, after.CA) || !either(a.Hosts, before.Hosts, after.Hosts) ||
		!either(a.CollectorUnit, before.CollectorUnit, after.CollectorUnit) ||
		!either(a.UploaderUnit, before.UploaderUnit, after.UploaderUnit) ||
		!either(a.InstalledConfig, before.InstalledConfig, after.InstalledConfig) {
		return "", ErrInvalid
	}
	if observed.Receipt == nil {
		return RecoveryRequired, nil
	}
	receipt, err := Completion(canonical)
	if err != nil || !bytes.Equal(observed.Receipt, receipt) || a != after {
		return "", ErrInvalid
	}
	return Complete, nil
}
