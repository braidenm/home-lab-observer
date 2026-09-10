// Package journalruntime establishes process-local policy for the optional Linux
// journal helper. The main observer must not call this package.
package journalruntime

import "errors"

// ErrUnavailable deliberately excludes native errors and process information.
var ErrUnavailable = errors.New("LOG_HELPER_UNAVAILABLE")

type policyCalls struct {
	setCoreLimit func(soft, hard uint64) error
	coreLimit    func() (soft, hard uint64, err error)
	setDumpable  func(value int) error
	dumpable     func() (int, error)
}

func establishPolicy(calls policyCalls) error {
	if err := calls.setCoreLimit(0, 0); err != nil {
		return ErrUnavailable
	}
	if err := calls.setDumpable(0); err != nil {
		return ErrUnavailable
	}
	soft, hard, err := calls.coreLimit()
	if err != nil || soft != 0 || hard != 0 {
		return ErrUnavailable
	}
	dumpable, err := calls.dumpable()
	if err != nil || dumpable != 0 {
		return ErrUnavailable
	}
	return nil
}
