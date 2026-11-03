//go:build !linux

package main

import "os"

// Native action control is not supported by another platform's observation
// package. No platform or command fallback may expose a writable provider.
func main() { os.Exit(22) }
