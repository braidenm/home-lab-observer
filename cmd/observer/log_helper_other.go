//go:build !windows || (!amd64 && !arm64)

package main

import "io"

func dispatchPrivateLogHelper(_ []string, _ io.Reader, _ io.Writer) (bool, int) {
	return false, 0
}
