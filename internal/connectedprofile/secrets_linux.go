//go:build linux

package connectedprofile

import "golang.org/x/sys/unix"

// HardenSecretProcess must precede reading a grant or connector credential.
// These settings affect only this process. RLIMIT_CORE alone is insufficient
// when the host has configured a piped crash handler; nondumpable is mandatory.
func HardenSecretProcess() error {
	if unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}) != nil || unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) != nil {
		return ErrUnsafe
	}
	var limit unix.Rlimit
	dumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err != nil || dumpable != 0 || unix.Getrlimit(unix.RLIMIT_CORE, &limit) != nil || limit.Cur != 0 || limit.Max != 0 {
		return ErrUnsafe
	}
	return nil
}
