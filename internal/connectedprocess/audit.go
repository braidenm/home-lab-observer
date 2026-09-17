// Package connectedprocess performs a root-side, read-only startup descriptor
// audit. The caller holds the installation lease and keeps the reviewed worker
// behind an absent-request barrier: no network, credentials or ledger access.
// This is not a sandbox for hostile code or permission to activate a worker.
package connectedprocess

import "errors"

var ErrUnsafe = errors.New("connected_process_refused")

// Identity is diagnostic correlation data, not reusable activation authority.
type Identity struct {
	PID            int
	StartTimeTicks uint64
}
