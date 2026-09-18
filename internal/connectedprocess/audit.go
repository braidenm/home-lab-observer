// Package connectedprocess performs a read-only startup descriptor audit.
// Root holds the installation lease and keeps an uploader behind its absent-
// request barrier; the collector audits itself before opening host observations.
// This is not a sandbox for hostile code or permission to activate a worker.
package connectedprocess

import "errors"

var ErrUnsafe = errors.New("connected_process_refused")

// Identity is diagnostic correlation data, not reusable activation authority.
type Identity struct {
	PID            int
	StartTimeTicks uint64
}
