//go:build !linux

package main

import "os"

var releaseIdentity string

func main() { os.Exit(22) }
