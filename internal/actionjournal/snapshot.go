package actionjournal

import (
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

// AdmissionSnapshot is bounded private v1 evidence, not a full remote receipt,
// permission, admission or a capability to resume an existing action.
type AdmissionSnapshot struct {
	Binding    Binding
	ObservedAt time.Time
	Records    []AdmissionRecord
}

type AdmissionRecord struct {
	Key        actioncore.Key
	HasIntent  bool
	Code       actioncore.Code
	TerminalAt time.Time
	Active     bool
}
