package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/braidenm/home-lab-observer/internal/releasepack"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("releasepack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	version := flags.String("version", "", "SemVer prerelease without a leading v")
	schema := flags.String("schema-version", releasepack.SchemaVersion, "closed release schema (observer-release/v1 or observer-release/v2)")
	commit := flags.String("commit", "", "exact 40-character source commit SHA")
	binaries := flags.String("binaries", "", "directory containing six staged observer binaries")
	output := flags.String("output", "", "new directory for final releasepack outputs")
	resources := flags.String("resources", "", "directory containing fixed archive resources")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "releasepack: unexpected positional arguments")
		return 2
	}
	if *version == "" || *commit == "" || *binaries == "" || *output == "" || *resources == "" {
		fmt.Fprintln(stderr, "releasepack: --version, --commit, --binaries, --output, and --resources are required")
		return 2
	}
	manifest, err := releasepack.Build(releasepack.Config{SchemaVersion: *schema, Version: *version, Commit: *commit, BinariesDir: *binaries, OutputDir: *output, ResourcesDir: *resources})
	if err != nil {
		fmt.Fprintf(stderr, "releasepack failed: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Created %d verified native archives for %s in %s\n", len(manifest.Assets), manifest.Tag, *output)
	return 0
}
