package releasepack

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
)

func verifyArchive(path string, asset Asset) error {
	invalid := errors.New("ARCHIVE_CONTENT_INVALID")
	root := strings.TrimSuffix(asset.Filename, "."+asset.Format)
	binary, launcher := "observer", "run-observer.sh"
	if asset.OS == "darwin" {
		launcher = "Run-Observer.command"
	}
	if asset.OS == "windows" {
		binary, launcher = "observer.exe", "Run-Observer.cmd"
	}
	want := map[string]int64{root + "/" + binary: maxBinarySize, root + "/LICENSE": 1 << 20, root + "/START-HERE.md": 1 << 20, root + "/" + launcher: 1 << 20}
	if asset.JournalHelper != nil {
		want[root+"/observer-journal-helper"] = maxBinarySize
	}
	seen := map[string]bool{}
	var total int64
	check := func(name string, size int64, reader io.Reader) error {
		limit, ok := want[name]
		if !ok || seen[name] || size <= 0 || size > limit || total > maxArchiveSize-size {
			return invalid
		}
		seen[name] = true
		total += size
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(reader, size+1))
		if err != nil || n != size {
			return invalid
		}
		if strings.HasSuffix(name, "/observer-journal-helper") {
			if asset.JournalHelper == nil || asset.JournalHelper.SizeBytes != n || hex.EncodeToString(h.Sum(nil)) != asset.JournalHelper.SHA256 {
				return invalid
			}
		}
		return nil
	}
	if asset.Format == "zip" {
		archive, err := zip.OpenReader(path)
		if err != nil {
			return invalid
		}
		defer archive.Close()
		if len(archive.File) != len(want)+1 {
			return invalid
		}
		rootSeen := false
		for _, entry := range archive.File {
			if entry.Name == root+"/" {
				if rootSeen || !entry.Mode().IsDir() || entry.UncompressedSize64 != 0 {
					return invalid
				}
				rootSeen = true
				continue
			}
			if !entry.Mode().IsRegular() || entry.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 || entry.UncompressedSize64 > uint64(maxBinarySize) {
				return invalid
			}
			reader, err := entry.Open()
			if err != nil {
				return invalid
			}
			err = check(entry.Name, int64(entry.UncompressedSize64), reader)
			closeErr := reader.Close()
			if err != nil || closeErr != nil {
				return invalid
			}
		}
		if !rootSeen {
			return invalid
		}
	} else {
		file, err := os.Open(path)
		if err != nil {
			return invalid
		}
		defer file.Close()
		gz, err := gzip.NewReader(file)
		if err != nil {
			return invalid
		}
		defer gz.Close()
		limited := &io.LimitedReader{R: gz, N: maxArchiveSize + 1}
		archive := tar.NewReader(limited)
		rootSeen := false
		for {
			entry, err := archive.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return invalid
			}
			if entry.Format != tar.FormatUSTAR || entry.Linkname != "" || len(entry.PAXRecords) != 0 || entry.Mode&06000 != 0 {
				return invalid
			}
			if entry.Name == root+"/" {
				if rootSeen || entry.Typeflag != tar.TypeDir || entry.Size != 0 {
					return invalid
				}
				rootSeen = true
				continue
			}
			if entry.Typeflag != tar.TypeReg {
				return invalid
			}
			if err := check(entry.Name, entry.Size, archive); err != nil {
				return invalid
			}
		}
		// Do not allow an ignored second tar stream or unbounded gzip padding.
		var buffer [4096]byte
		for {
			n, err := limited.Read(buffer[:])
			for _, b := range buffer[:n] {
				if b != 0 {
					return invalid
				}
			}
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return invalid
			}
		}
		if !rootSeen || limited.N <= 0 {
			return invalid
		}
	}
	if len(seen) != len(want) {
		return invalid
	}
	return nil
}
