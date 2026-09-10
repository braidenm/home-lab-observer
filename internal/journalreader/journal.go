// Package journalreader implements the bounded Linux journal read state machine
// behind fixed native seams. It does not load libraries or launch processes.
package journalreader

import (
	"errors"
	"time"
)

var (
	ErrInvalidRequest = errors.New("INVALID_LOG_READ_REQUEST")
	ErrReadFailed     = errors.New("LOG_READER_FAILED")
	// Native bindings classify only these conditions; arbitrary error text never escapes.
	ErrUnavailable      = errors.New("JOURNAL_UNAVAILABLE")
	ErrPermissionDenied = errors.New("JOURNAL_PERMISSION_DENIED")
	ErrInvalidCursor    = errors.New("JOURNAL_INVALID_CURSOR")
	ErrFieldMissing     = errors.New("JOURNAL_FIELD_MISSING")
)

// Factory always opens the caller-accessible local system journal. It accepts no
// path, query, channel, library or flags from the caller.
type Factory interface{ OpenSystem() (Journal, error) }

// Journal belongs to the creating locked OS thread until Close. Field methods
// return only the named field value, without its fixed native field-name prefix.
// The native implementation must bound copies before returning and release its
// native allocations; the reader rechecks bounds and takes owned copies.
type Journal interface {
	SeekRealtime(microseconds uint64) error
	SeekCursor(cursor []byte) error
	SeekTail() error
	Next() (bool, error)
	Previous() (bool, error)
	TestCursor(cursor []byte) (bool, error)
	RealtimeMicros() (uint64, error)
	Priority() ([]byte, error)
	MessageID() ([]byte, error)
	Cursor() ([]byte, error)
	Close()
}

// Config is constructed by the fixed helper, not by an API request. Now is a
// deterministic clock seam; production defaults to the local UTC clock.
type Config struct {
	Factory Factory
	Now     func() time.Time
}

type Reader struct {
	factory Factory
	now     func() time.Time
}

func New(config Config) (*Reader, error) {
	if config.Factory == nil {
		return nil, ErrInvalidRequest
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Reader{factory: config.Factory, now: config.Now}, nil
}
