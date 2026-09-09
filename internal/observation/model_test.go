package observation

import (
	"testing"
	"time"
)

func TestValidateRejectsInvalidContract(t *testing.T) {
	s := Snapshot{SchemaVersion: SchemaVersion, ObservedAt: time.Now(), Capabilities: make([]Capability, 6)}
	if s.Validate() == nil {
		t.Fatal("expected non-UTC time to fail")
	}
}
