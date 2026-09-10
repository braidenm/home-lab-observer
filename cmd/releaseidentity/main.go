// releaseidentity is a build-time formatter, not part of the installed observer.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, diagnostics io.Writer) int {
	flags := flag.NewFlagSet("releaseidentity", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	var value buildidentity.Identity
	flags.StringVar(&value.Role, "role", "", "observer or journal-helper")
	flags.StringVar(&value.Version, "version", "", "prerelease version")
	flags.StringVar(&value.Commit, "commit", "", "full source commit")
	flags.StringVar(&value.OS, "os", "", "target operating system")
	flags.StringVar(&value.Arch, "arch", "", "target architecture")
	flags.StringVar(&value.HelperSHA256, "helper-sha256", "", "Linux observer's paired helper hash")
	scan := flags.String("scan", "", "inspect one bounded release binary")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return 2
	}
	if *scan != "" {
		if value != (buildidentity.Identity{}) {
			fmt.Fprintln(diagnostics, "BUILD_IDENTITY_INVALID")
			return 1
		}
		return scanIdentity(*scan, out, diagnostics)
	}
	record, err := buildidentity.Encode(value)
	if err != nil {
		fmt.Fprintln(diagnostics, "BUILD_IDENTITY_INVALID")
		return 1
	}
	if _, err := fmt.Fprintln(out, record); err != nil {
		return 1
	}
	return 0
}

func scanIdentity(path string, out, diagnostics io.Writer) int {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > buildidentity.MaxFileBytes {
		fmt.Fprintln(diagnostics, "BUILD_IDENTITY_INVALID")
		return 1
	}
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(diagnostics, "BUILD_IDENTITY_INVALID")
		return 1
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		fmt.Fprintln(diagnostics, "BUILD_IDENTITY_INVALID")
		return 1
	}
	identity, err := buildidentity.Scan(file, info.Size())
	if err != nil {
		fmt.Fprintln(diagnostics, "BUILD_IDENTITY_INVALID")
		return 1
	}
	record, err := buildidentity.Encode(identity)
	if err != nil {
		fmt.Fprintln(diagnostics, "BUILD_IDENTITY_INVALID")
		return 1
	}
	if _, err := fmt.Fprintln(out, record); err != nil {
		return 1
	}
	return 0
}
