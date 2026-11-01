// Package actionengine is an unused fixed Docker adapter, not an installed
// broker, authorization mechanism, generic client or action listener.
package actionengine

import "errors"

var (
	ErrUnsafe      = errors.New("action_engine_unsafe")
	ErrUnsupported = errors.New("action_engine_unsupported")
	ErrUnavailable = errors.New("action_engine_unavailable")
	ErrInvalid     = errors.New("action_engine_invalid")
)

// SocketWitness comes from separately protected explicit local approval. It is
// node-drift evidence, NOT EngineGeneration, a grant or daemon authentication.
type SocketWitness struct {
	Device, Inode uint64
	GroupID       uint32
}

const (
	versionLimit = 64 * 1024
	inspectLimit = 256 * 1024
)

func validID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, b := range []byte(id) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
