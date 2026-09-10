// Package eventreader implements fixed-source Windows Event Log acquisition
// behind synthetic/native seams. It loads no libraries and launches no process.
package eventreader

import (
	"errors"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

var (
	ErrInvalidRequest   = errors.New("INVALID_LOG_READ_REQUEST")
	ErrReadFailed       = errors.New("LOG_READER_FAILED")
	ErrUnavailable      = errors.New("EVENT_LOG_UNAVAILABLE")
	ErrPermissionDenied = errors.New("EVENT_LOG_PERMISSION_DENIED")
	ErrBookmarkStale    = errors.New("EVENT_LOG_BOOKMARK_STALE")
	ErrFieldMissing     = errors.New("EVENT_LOG_FIELD_MISSING")
	ErrFieldInvalid     = errors.New("EVENT_LOG_FIELD_INVALID")
	ErrFieldTooLarge    = errors.New("EVENT_LOG_FIELD_TOO_LARGE")
)

// Factory maps only the validated source aliases to fixed local System and
// Application channels. It exposes no path, query text, flags or remote session.
type Factory interface {
	OpenInitial(source logobs.Source, lowerFileTime uint64) (Query, error)
	OpenContinuation(source logobs.Source) (Query, error)
	OpenTail(source logobs.Source) (Query, error)
}

// Query and its records belong to the creating locked OS thread. Next returns
// nil,nil only for ERROR_NO_MORE_ITEMS. Any returned record must be closed, even
// when an error accompanies it. Seek uses strict offset-zero bookmark semantics;
// ErrBookmarkStale is reserved for explicit absence, not generic query staleness.
type Query interface {
	SeekBookmark(bookmark []byte) error
	Next() (Record, error)
	Close()
}

// Record selects no bodies or provider names. Typed getters reject wrong native
// types before conversion. Oversized fields return ErrFieldTooLarge without a
// buffer. GUID is canonical UUID byte order, not Windows in-memory mixed endian.
// Bookmark includes the complete bounded private anchor. BookmarkMatches uses
// that already-acquired anchor only, without further native reads. Its native
// implementation requires separate exactness proof; XML byte equality is not it.
type Record interface {
	TimeCreated() (uint64, error)
	Level() (uint8, error)
	EventID() (uint16, error)
	ProviderGUID() ([16]byte, error)
	Bookmark() ([]byte, error)
	BookmarkMatches(saved []byte) (bool, error)
	Close()
}

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
