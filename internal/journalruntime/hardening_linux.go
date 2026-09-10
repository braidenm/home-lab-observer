package journalruntime

import "golang.org/x/sys/unix"

// Harden disables core-file generation and process dumpability, then verifies
// both settings. Call only in the dedicated helper, before reading private input
// or opening journals. Failure must stop helper work. This irreversibly lowers
// the current process's hard core limit; it does not change host-wide policy and
// does not protect against a privileged host administrator.
func Harden() error {
	return establishPolicy(policyCalls{
		setCoreLimit: func(soft, hard uint64) error {
			return unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: soft, Max: hard})
		},
		coreLimit: func() (uint64, uint64, error) {
			var limit unix.Rlimit
			err := unix.Getrlimit(unix.RLIMIT_CORE, &limit)
			return limit.Cur, limit.Max, err
		},
		setDumpable: func(value int) error {
			return unix.Prctl(unix.PR_SET_DUMPABLE, uintptr(value), 0, 0, 0)
		},
		dumpable: func() (int, error) {
			return unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
		},
	})
}
