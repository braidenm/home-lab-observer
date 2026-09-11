package connectedprofile

import "strings"

// CheckEnvironment rejects manager-inherited provider, proxy and trust-store
// overrides. Accepted manager metadata is bounded and never logged or executed.
func CheckEnvironment(environ []string) error {
	if len(environ) > 32 {
		return ErrUnsafe
	}
	seen := map[string]bool{}
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || seen[key] || len(entry) > 4096 {
			return ErrUnsafe
		}
		seen[key] = true
		switch key {
		case "GODEBUG":
			if value != "netdns=go" {
				return ErrUnsafe
			}
		case "PATH", "USER", "LOGNAME", "HOME", "SHELL", "LANG", "LC_ALL", "INVOCATION_ID", "SYSTEMD_EXEC_PID", "CREDENTIALS_DIRECTORY":
		default:
			return ErrUnsafe
		}
	}
	if !seen["GODEBUG"] {
		return ErrUnsafe
	}
	return nil
}
