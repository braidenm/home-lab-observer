//go:build !linux

package journalruntime

// Harden is unavailable outside the separately packaged Linux helper runtime.
// It does not modify process or host policy on other platforms.
func Harden() error { return ErrUnavailable }
