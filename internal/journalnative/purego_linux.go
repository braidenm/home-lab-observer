//go:build linux && (amd64 || arm64)

package journalnative

import (
	"errors"

	"github.com/ebitengine/purego"
)

const systemdSONAME = "libsystemd.so.0"

var errNativeUnavailable = errors.New("JOURNAL_NATIVE_UNAVAILABLE")

type binding struct {
	name   string
	target any
}

func loadNativeCalls() (calls nativeCalls, closeLibrary func() error, err error) {
	library, openErr := purego.Dlopen(systemdSONAME, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if openErr != nil {
		return nativeCalls{}, nil, openErr
	}
	closeLibrary = func() error { return purego.Dlclose(library) }

	bindings := []binding{
		{"sd_journal_open", &calls.open},
		{"sd_journal_close", &calls.close},
		{"sd_journal_next", &calls.next},
		{"sd_journal_previous", &calls.previous},
		{"sd_journal_seek_realtime_usec", &calls.seekRealtime},
		{"sd_journal_seek_cursor", &calls.seekCursor},
		{"sd_journal_seek_tail", &calls.seekTail},
		{"sd_journal_get_realtime_usec", &calls.getRealtime},
		{"sd_journal_set_data_threshold", &calls.setDataThreshold},
		{"sd_journal_get_data", &calls.getData},
		{"sd_journal_get_cursor", &calls.getCursor},
		{"sd_journal_test_cursor", &calls.testCursor},
		{"free", &calls.free},
	}
	addresses := make([]uintptr, len(bindings))
	for index, item := range bindings {
		addresses[index], err = purego.Dlsym(library, item.name)
		if err != nil || addresses[index] == 0 {
			_ = closeLibrary()
			return nativeCalls{}, nil, errNativeUnavailable
		}
	}
	if !registerBindings(bindings, addresses) {
		_ = closeLibrary()
		return nativeCalls{}, nil, errNativeUnavailable
	}
	return calls, closeLibrary, nil
}

// Purego validates neither the C signature nor target shape. The addresses are
// resolved first, and this recovery converts only local registration failures
// to a fixed unavailable result at the exported boundary.
func registerBindings(bindings []binding, addresses []uintptr) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	for index := range bindings {
		purego.RegisterFunc(bindings[index].target, addresses[index])
	}
	return true
}
