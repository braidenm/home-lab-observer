// connectedpack is a release build tool, not an installed service or downloader.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

func main() {
	if run(os.Args[1:]) != nil {
		fmt.Fprintln(os.Stderr, "CONNECTED_PACK_FAILED")
		os.Exit(1)
	}
}

func run(args []string) error {
	f := flag.NewFlagSet("connectedpack", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	mode := f.String("mode", "", "identity or pack")
	role := f.String("role", "", "connected role")
	version := f.String("version", "", "prerelease version")
	commit := f.String("commit", "", "source commit")
	binaries := f.String("binaries", "", "exact binary input directory")
	output := f.String("output", "", "new output directory")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return connectedbundle.ErrInvalid
	}
	i := connectedidentity.Identity{Role: *role, Version: *version, Commit: *commit, OS: "linux", Arch: "amd64", ContractSHA256: connectedcompat.Digest()}
	if *mode == "identity" {
		if *binaries != "" || *output != "" {
			return connectedbundle.ErrInvalid
		}
		record, err := connectedidentity.Encode(i)
		if err != nil {
			return err
		}
		fmt.Print(record)
		return nil
	}
	if *mode != "pack" || *role != "" || *binaries == "" || *output == "" {
		return connectedbundle.ErrInvalid
	}
	i.Role = "install"
	if i.Validate() != nil {
		return connectedbundle.ErrInvalid
	}
	if err := os.Mkdir(*output, 0o755); err != nil {
		return connectedbundle.ErrInvalid
	}
	directory := filepath.Join(*output, "home-lab-observer-connected_"+*version+"_linux_amd64")
	if os.Mkdir(directory, 0o755) != nil {
		return connectedbundle.ErrInvalid
	}
	if ownerfs.ValidateDedicatedDirectory(*binaries) != nil {
		return connectedbundle.ErrInvalid
	}
	input, err := os.OpenRoot(*binaries)
	if err != nil {
		return connectedbundle.ErrInvalid
	}
	defer input.Close()
	listing, err := input.Open(".")
	if err != nil {
		return connectedbundle.ErrInvalid
	}
	entries, readErr := listing.ReadDir(4)
	closeErr := listing.Close()
	if (readErr != nil && readErr != io.EOF) || closeErr != nil || len(entries) != 3 {
		return connectedbundle.ErrInvalid
	}
	for _, name := range []string{"observer-connected-collector", "observer-connected-uploader", "observer-connected-install"} {
		info, err := ownerfs.ValidateRegular(filepath.Join(*binaries, name), connectedidentity.MaxFileBytes)
		if err != nil || info.Size() <= 0 {
			return connectedbundle.ErrInvalid
		}
		in, err := input.Open(name)
		if err != nil {
			return connectedbundle.ErrInvalid
		}
		out, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if err != nil {
			in.Close()
			return connectedbundle.ErrInvalid
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, connectedidentity.MaxFileBytes+1))
		modeErr := out.Chmod(0o755)
		syncErr := out.Sync()
		inErr := in.Close()
		outErr := out.Close()
		if copyErr != nil || modeErr != nil || syncErr != nil || inErr != nil || outErr != nil || n != info.Size() {
			return connectedbundle.ErrInvalid
		}
	}
	for name, data := range connectedunits.Resources() {
		if writeNew(filepath.Join(directory, name), data) != nil {
			return connectedbundle.ErrInvalid
		}
	}
	manifest, err := connectedbundle.CreateManifest(directory, *version, *commit)
	if err != nil {
		return err
	}
	if writeNew(filepath.Join(directory, connectedbundle.ManifestName), manifest) != nil {
		return connectedbundle.ErrInvalid
	}
	digest := sha256.Sum256(manifest)
	manifestSHA := hex.EncodeToString(digest[:])
	archiveName := filepath.Base(directory) + ".tar.gz"
	archive, err := os.OpenFile(filepath.Join(*output, archiveName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return connectedbundle.ErrInvalid
	}
	hash := sha256.New()
	packErr := connectedbundle.WriteArchive(io.MultiWriter(archive, hash), directory, manifestSHA)
	syncErr := archive.Sync()
	closeErr = archive.Close()
	if packErr != nil || syncErr != nil || closeErr != nil {
		return connectedbundle.ErrInvalid
	}
	if writeNew(filepath.Join(*output, connectedbundle.ManifestName), manifest) != nil {
		return connectedbundle.ErrInvalid
	}
	checksums := hex.EncodeToString(hash.Sum(nil)) + "  " + archiveName + "\n" + manifestSHA + "  " + connectedbundle.ManifestName + "\n"
	if writeNew(filepath.Join(*output, "SHA256SUMS"), []byte(checksums)) != nil {
		return connectedbundle.ErrInvalid
	}
	fmt.Println("CONNECTED_PACK_OK")
	return nil
}

func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(data)
	modeErr := f.Chmod(0o644)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || modeErr != nil || syncErr != nil || closeErr != nil || n != len(data) {
		return connectedbundle.ErrInvalid
	}
	return nil
}
