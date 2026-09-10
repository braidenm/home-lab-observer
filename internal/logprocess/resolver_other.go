//go:build !linux && !windows

package logprocess

func resolvePlatformHelper(string, string) (commandSpec, error) {
	return commandSpec{}, ErrHelperUnavailable
}
