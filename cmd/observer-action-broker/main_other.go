//go:build !linux

package main

import "os"

// Other platforms' observation support is not writable container authority.
func main() { os.Exit(22) }
