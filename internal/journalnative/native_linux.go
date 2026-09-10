//go:build linux && (amd64 || arm64)

// Package journalnative provides the fixed libsystemd binding used only by the
// separately packaged Linux journal helper.
package journalnative

import (
	"bytes"
	"sync"
	"unsafe"

	"github.com/braidenm/home-lab-observer/internal/journalreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"golang.org/x/sys/unix"
)

const (
	systemJournalFlags = int32(1<<0 | 1<<2) // SD_JOURNAL_LOCAL_ONLY | SD_JOURNAL_SYSTEM
	maximumFieldObject = logobs.MaxNativeFieldBytes + len("MESSAGE_ID=")
	dataThreshold      = maximumFieldObject + 1
)

type nativeCalls struct {
	open             func(*uintptr, int32) int32
	close            func(uintptr)
	next             func(uintptr) int32
	previous         func(uintptr) int32
	seekRealtime     func(uintptr, uint64) int32
	seekCursor       func(uintptr, string) int32
	seekTail         func(uintptr) int32
	getRealtime      func(uintptr, *uint64) int32
	setDataThreshold func(uintptr, uintptr) int32
	getData          func(uintptr, string, *unsafe.Pointer, *uintptr) int32
	getCursor        func(uintptr, *unsafe.Pointer) int32
	testCursor       func(uintptr, string) int32
	free             func(unsafe.Pointer)
}

type Factory struct {
	mu     sync.Mutex
	calls  nativeCalls
	close  func() error
	active int
	closed bool
}

var _ journalreader.Factory = (*Factory)(nil)

// NewFactory loads only the fixed systemd SONAME and exact symbol allowlist.
func NewFactory() (*Factory, error) {
	return newFactoryFromLoader(loadNativeCalls)
}

func newFactoryFromLoader(loader func() (nativeCalls, func() error, error)) (*Factory, error) {
	if loader == nil {
		return nil, journalreader.ErrUnavailable
	}
	calls, closeLibrary, err := loader()
	if err != nil {
		return nil, journalreader.ErrUnavailable
	}
	return newFactory(calls, closeLibrary), nil
}

func newFactory(calls nativeCalls, closeLibrary func() error) *Factory {
	if closeLibrary == nil {
		closeLibrary = func() error { return nil }
	}
	return &Factory{calls: calls, close: closeLibrary}
}

// OpenSystem implements journalreader.Factory. It has no configurable path,
// namespace, matches, or flags.
func (f *Factory) OpenSystem() (journalreader.Journal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, journalreader.ErrUnavailable
	}
	var handle uintptr
	result := f.calls.open(&handle, systemJournalFlags)
	if result < 0 || handle == 0 {
		if handle != 0 {
			f.calls.close(handle)
		}
		return nil, classifyOpen(result)
	}
	if result = f.calls.setDataThreshold(handle, uintptr(dataThreshold)); result < 0 {
		f.calls.close(handle)
		return nil, classifyGeneral(result)
	}
	f.active++
	return &journal{factory: f, calls: f.calls, handle: handle, ownerThread: unix.Gettid()}, nil
}

// Close unloads libsystemd after every journal handle has been closed.
func (f *Factory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	if f.active != 0 {
		return journalreader.ErrReadFailed
	}
	f.closed = true
	if f.close() != nil {
		return journalreader.ErrUnavailable
	}
	return nil
}

func (f *Factory) release() {
	f.mu.Lock()
	if f.active > 0 {
		f.active--
	}
	f.mu.Unlock()
}

type journal struct {
	factory     *Factory
	calls       nativeCalls
	handle      uintptr
	ownerThread int
	closed      bool
}

var _ journalreader.Journal = (*journal)(nil)

func (j *journal) valid() bool {
	return j != nil && !j.closed && j.handle != 0 && unix.Gettid() == j.ownerThread
}

func (j *journal) SeekRealtime(microseconds uint64) error {
	if !j.valid() {
		return journalreader.ErrReadFailed
	}
	return classifyGeneral(j.calls.seekRealtime(j.handle, microseconds))
}

func (j *journal) SeekCursor(cursor []byte) error {
	if !j.valid() {
		return journalreader.ErrReadFailed
	}
	value, ok := cursorString(cursor)
	if !ok {
		return journalreader.ErrInvalidCursor
	}
	return classifyCursor(j.calls.seekCursor(j.handle, value))
}

func (j *journal) SeekTail() error {
	if !j.valid() {
		return journalreader.ErrReadFailed
	}
	return classifyGeneral(j.calls.seekTail(j.handle))
}

func (j *journal) Next() (bool, error) {
	if !j.valid() {
		return false, journalreader.ErrReadFailed
	}
	return classifyStep(j.calls.next(j.handle))
}

func (j *journal) Previous() (bool, error) {
	if !j.valid() {
		return false, journalreader.ErrReadFailed
	}
	return classifyStep(j.calls.previous(j.handle))
}

func (j *journal) TestCursor(cursor []byte) (bool, error) {
	if !j.valid() {
		return false, journalreader.ErrReadFailed
	}
	value, ok := cursorString(cursor)
	if !ok {
		return false, journalreader.ErrInvalidCursor
	}
	result := j.calls.testCursor(j.handle, value)
	if result < 0 {
		return false, classifyCursor(result)
	}
	return result > 0, nil
}

func (j *journal) RealtimeMicros() (uint64, error) {
	if !j.valid() {
		return 0, journalreader.ErrReadFailed
	}
	var result uint64
	if code := j.calls.getRealtime(j.handle, &result); code < 0 {
		return 0, classifyGeneral(code)
	}
	return result, nil
}

func (j *journal) Priority() ([]byte, error) { return j.field("PRIORITY") }

func (j *journal) MessageID() ([]byte, error) { return j.field("MESSAGE_ID") }

func (j *journal) field(name string) ([]byte, error) {
	if !j.valid() {
		return nil, journalreader.ErrReadFailed
	}
	var pointer unsafe.Pointer
	var size uintptr
	if code := j.calls.getData(j.handle, name, &pointer, &size); code < 0 {
		return nil, classifyField(code)
	}
	prefix := []byte(name + "=")
	maximum := uintptr(len(prefix) + logobs.MaxNativeFieldBytes)
	if pointer == nil || size < uintptr(len(prefix)) {
		return nil, journalreader.ErrReadFailed
	}
	if size > maximum {
		return nil, journalreader.ErrFieldTooLarge
	}
	data := unsafe.Slice((*byte)(pointer), int(size))
	if !bytes.Equal(data[:len(prefix)], prefix) {
		return nil, journalreader.ErrReadFailed
	}
	return append([]byte(nil), data[len(prefix):]...), nil
}

func (j *journal) Cursor() ([]byte, error) {
	if !j.valid() {
		return nil, journalreader.ErrReadFailed
	}
	var pointer unsafe.Pointer
	if code := j.calls.getCursor(j.handle, &pointer); code < 0 {
		if pointer != nil {
			j.calls.free(pointer)
		}
		return nil, classifyGeneral(code)
	}
	if pointer == nil {
		return nil, journalreader.ErrReadFailed
	}
	defer j.calls.free(pointer)
	for size := 0; size <= logobs.MaxCheckpointBytes; size++ {
		value := *(*byte)(unsafe.Add(pointer, size))
		if value == 0 {
			if size == 0 {
				return nil, journalreader.ErrReadFailed
			}
			data := unsafe.Slice((*byte)(pointer), size)
			return append([]byte(nil), data...), nil
		}
	}
	return nil, journalreader.ErrReadFailed
}

func (j *journal) Close() {
	if !j.valid() {
		return
	}
	j.calls.close(j.handle)
	j.handle = 0
	j.closed = true
	j.factory.release()
}

func cursorString(cursor []byte) (string, bool) {
	if len(cursor) == 0 || len(cursor) > logobs.MaxCheckpointBytes || bytes.IndexByte(cursor, 0) >= 0 {
		return "", false
	}
	return string(cursor), true
}

func classifyOpen(code int32) error {
	if code >= 0 {
		return journalreader.ErrReadFailed
	}
	if isPermission(code) {
		return journalreader.ErrPermissionDenied
	}
	return journalreader.ErrUnavailable
}

func classifyGeneral(code int32) error {
	if code >= 0 {
		return nil
	}
	if isPermission(code) {
		return journalreader.ErrPermissionDenied
	}
	return journalreader.ErrReadFailed
}

func classifyCursor(code int32) error {
	if code >= 0 {
		return nil
	}
	if code == -int32(unix.EINVAL) || code == -int32(unix.ESTALE) {
		return journalreader.ErrInvalidCursor
	}
	return classifyGeneral(code)
}

func classifyField(code int32) error {
	switch code {
	case -int32(unix.ENOENT):
		return journalreader.ErrFieldMissing
	case -int32(unix.E2BIG), -int32(unix.ENOBUFS):
		return journalreader.ErrFieldTooLarge
	default:
		return classifyGeneral(code)
	}
}

func classifyStep(code int32) (bool, error) {
	if code < 0 {
		return false, classifyGeneral(code)
	}
	return code > 0, nil
}

func isPermission(code int32) bool {
	return code == -int32(unix.EACCES) || code == -int32(unix.EPERM)
}
