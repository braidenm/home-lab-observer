package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/braidenm/home-lab-observer/internal/buildidentity"
	"io"
	"runtime"
)

// Authoritative for v2 releases; empty only for legacy builds and development.
var releaseIdentity = ""

type buildVersion struct {
	SchemaVersion string `json:"schema_version"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	GoVersion     string `json:"go_version"`
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "write the observer-build/v1 description")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "invalid version options")
		return 2
	}
	identity, identityErr := buildidentity.Resolve(releaseIdentity, version, commit, "observer", runtime.GOOS, runtime.GOARCH)
	if identityErr != nil {
		fmt.Fprintln(stderr, "BUILD_IDENTITY_INVALID")
		return 1
	}
	info := buildVersion{SchemaVersion: "observer-build/v1", Version: identity.Version, Commit: identity.Commit, OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version()}
	var err error
	if *asJSON {
		err = json.NewEncoder(stdout).Encode(info)
	} else {
		_, err = fmt.Fprintf(stdout, "Home Lab Observer %s (%s/%s, commit %s)\n", info.Version, info.OS, info.Arch, info.Commit)
	}
	if err != nil {
		fmt.Fprintln(stderr, "could not write version information")
		return 1
	}
	return 0
}
