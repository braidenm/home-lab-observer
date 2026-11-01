// Package actionjournal is an unused private action store. It adds no authority,
// transport, installed role or automatic uncertainty reconciliation.
package actionjournal

import "errors"

type Binding struct{ ServerID, InstallationID, OwnerID string }

var (
	ErrUnsupported = errors.New("action_journal_unsupported")
	ErrUnsafe      = errors.New("action_journal_unsafe")
	ErrBusy        = errors.New("action_journal_busy")
	ErrFull        = errors.New("action_journal_full")
	ErrRecovery    = errors.New("action_journal_recovery_required")
)

const (
	databaseName  = "action.sqlite"
	lockName      = ".action-lock"
	journalName   = "action.sqlite-journal"
	databaseBytes = 12 * 1024 * 1024
	journalBytes  = (databaseBytes/4096)*(4096+8) + 2*65536
	totalBytes    = 32 * 1024 * 1024
	maxRecords    = 1000
)
