// Package sharedhandoff implements a fixed Linux collector-to-uploader handoff.
// It is separate from owner-private handoff and contains no worker or installer.
package sharedhandoff

import "errors"

type Policy struct {
	CollectorUID uint32
	SharedGID    uint32
	UploaderUID  uint32
	ServerID     string
}

var (
	ErrUnsupported = errors.New("shared_handoff_unsupported")
	ErrUnsafe      = errors.New("shared_handoff_unsafe")
	ErrMissing     = errors.New("shared_handoff_missing")
	ErrUnavailable = errors.New("shared_handoff_unavailable")
	ErrInvalid     = errors.New("shared_handoff_invalid")
	ErrBusy        = errors.New("shared_handoff_busy")
)

const latestName = "snapshot.json"
const stageName = ".snapshot-next"
const lockName = ".writer-lock"
