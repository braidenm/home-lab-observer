// Package uploadledger persists upload admission state independently of history.
// It does not enroll, read credentials, run workers or perform network requests.
package uploadledger

import "errors"

var (
	ErrUnsupported = errors.New("upload_ledger_unsupported")
	ErrUnsafe      = errors.New("upload_ledger_unsafe_storage")
	ErrBusy        = errors.New("upload_ledger_busy")
	ErrRecovery    = errors.New("upload_ledger_recovery_required")
)

const (
	databaseName     = "upload.sqlite"
	lockName         = ".upload-lock"
	journalName      = databaseName + "-journal"
	maxDatabaseBytes = 4096 * 64
	maxTotalBytes    = 1024 * 1024
)
