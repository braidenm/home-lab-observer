package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
	"github.com/braidenm/home-lab-observer/internal/releasepack"
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
	var diagnostics strings.Builder
	flags.SetOutput(&diagnostics)
	asJSON := flags.Bool("json", false, "write the observer-build/v1 description")
	releaseSchema := flags.Bool("release-schema", false, "write the required release manifest schema without collection")
	manifest := flags.String("release-manifest", "", "verify a bounded staged release manifest without starting the observer")
	archiveHash := flags.String("archive-sha256", "", "already verified archive SHA-256 (manifest verification only)")
	archiveSize := flags.Int64("archive-size", 0, "already verified archive byte size (manifest verification only)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = io.WriteString(stderr, diagnostics.String())
			return 0
		}
		fmt.Fprintln(stderr, "invalid version options")
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
	if *releaseSchema {
		if *asJSON || *manifest != "" || *archiveHash != "" || *archiveSize != 0 {
			fmt.Fprintln(stderr, "invalid version options")
			return 2
		}
		schema := releasepack.SchemaVersion
		if releaseIdentity != "" {
			schema = releasepack.SchemaVersionV2
		}
		if _, err := fmt.Fprintln(stdout, schema); err != nil {
			return 1
		}
		return 0
	}
	if *manifest != "" || *archiveHash != "" || *archiveSize != 0 {
		if *manifest == "" || *archiveHash == "" || *archiveSize <= 0 || *asJSON {
			fmt.Fprintln(stderr, "invalid version options")
			return 2
		}
		executable, err := os.Executable()
		expectedSchema := releasepack.SchemaVersion
		if releaseIdentity != "" {
			expectedSchema = releasepack.SchemaVersionV2
		}
		if err != nil || releasepack.VerifyInstalled(releasepack.Installed{ExpectedSchemaVersion: expectedSchema, ManifestPath: *manifest, ExecutablePath: executable, Identity: identity, ArchiveSHA256: *archiveHash, ArchiveSize: *archiveSize}) != nil {
			fmt.Fprintln(stderr, "INSTALLED_RELEASE_INVALID")
			return 1
		}
		if _, err := fmt.Fprintln(stdout, "RELEASE_MANIFEST_VERIFIED"); err != nil {
			return 1
		}
		return 0
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
