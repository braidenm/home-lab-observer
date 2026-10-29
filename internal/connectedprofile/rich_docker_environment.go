package connectedprofile

import "strings"

// CheckRichProcessesDockerReaderEnvironment admits the fixed resource knobs
// only for the Docker reader; every other inherited value retains the original
// installed-worker checks. This is not a configurable environment allowlist.
func CheckRichProcessesDockerReaderEnvironment(environ []string) error {
	if len(environ) > 32 {
		return ErrUnsafe
	}
	rest := make([]string, 0, len(environ))
	seenCPU, seenMemory := false, false
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || len(entry) > 4096 {
			return ErrUnsafe
		}
		switch key {
		case "GOMAXPROCS":
			if seenCPU || value != "2" {
				return ErrUnsafe
			}
			seenCPU = true
		case "GOMEMLIMIT":
			if seenMemory || value != "64MiB" {
				return ErrUnsafe
			}
			seenMemory = true
		default:
			rest = append(rest, entry)
		}
	}
	if !seenCPU || !seenMemory {
		return ErrUnsafe
	}
	return CheckEnvironment(rest)
}
