//go:build !linux

package actionengine

import "context"

func OpenGuarded(context.Context, SocketWitness) (*GuardedEngine, error) {
	return nil, ErrUnsupported
}
