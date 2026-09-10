package main

import (
	"runtime"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
)

var version = "dev"
var commit = "unknown"
var journalHelperSHA256 = ""
var releaseIdentity = ""

func main() {
	identity, err := buildidentity.Resolve(releaseIdentity, version, commit, "observer", runtime.GOOS, runtime.GOARCH)
	println(identity.Version, err, journalHelperSHA256)
}
