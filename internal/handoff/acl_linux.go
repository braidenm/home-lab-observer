//go:build linux

package handoff

import "os"

// Linux POSIX ACL effective group/named-user permissions are bounded by the
// group-class mode mask, which privateHandle already requires to be zero.
func noExtendedACL(*os.File) error { return nil }
