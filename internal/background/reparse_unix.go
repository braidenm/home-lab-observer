//go:build !windows

package background

func rejectReparse(string) error        { return nil }
func validatePlatformPath(string) error { return nil }
