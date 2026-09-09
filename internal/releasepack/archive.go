package releasepack

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

var (
	tarTime = time.Unix(0, 0).UTC()
	zipTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
)

type archiveEntry struct {
	name   string
	mode   os.FileMode
	source *sourceFile
	data   []byte
}

func buildArchive(stage string, in inputs, target target) (Asset, error) {
	root := "home-lab-observer_" + in.config.Version + "_" + target.os + "_" + target.arch
	filename := root + "." + target.format
	path := filepath.Join(stage, filename)
	binaryArchiveName := "observer"
	if target.os == "windows" {
		binaryArchiveName = "observer.exe"
	}
	entries := []archiveEntry{
		{name: binaryArchiveName, mode: 0o755, source: sourcePointer(in.binaries[target.binaryName])},
		{name: "LICENSE", mode: 0o644, data: in.license},
		{name: "START-HERE.md", mode: 0o644, data: in.start},
		{name: target.helperName, mode: 0o755, data: in.helpers[target.helperSource]},
	}
	var err error
	if target.format == "zip" {
		err = writeZIP(path, root, entries)
	} else {
		err = writeTarGzip(path, root, entries)
	}
	if err != nil {
		return Asset{}, errors.New("ARCHIVE_WRITE_FAILED")
	}
	hash, size, err := hashFileBounded(path, maxArchiveSize)
	if err != nil {
		return Asset{}, errors.New("ARCHIVE_HASH_FAILED")
	}
	if size <= 0 || size > maxArchiveSize {
		return Asset{}, errors.New("ARCHIVE_SIZE_INVALID")
	}
	return Asset{
		OS: target.os, Arch: target.arch, Filename: filename, SHA256: hash, SizeBytes: size, Format: target.format,
		DownloadURL: "https://github.com/" + Repository + "/releases/download/v" + in.config.Version + "/" + filename,
	}, nil
}

func sourcePointer(source sourceFile) *sourceFile { return &source }

func writeTarGzip(path, root string, entries []archiveEntry) (returnErr error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); returnErr == nil {
			returnErr = err
		}
	}()
	gzipWriter, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzipWriter.Header.ModTime = tarTime
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	tarClosed := false
	gzipClosed := false
	defer func() {
		if !tarClosed {
			_ = tarWriter.Close()
		}
		if !gzipClosed {
			_ = gzipWriter.Close()
		}
	}()
	if err := tarWriter.WriteHeader(&tar.Header{Name: root + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: tarTime, Format: tar.FormatUSTAR}); err != nil {
		return err
	}
	for _, entry := range entries {
		size, err := entrySize(entry)
		if err != nil {
			return err
		}
		header := &tar.Header{Name: root + "/" + entry.name, Typeflag: tar.TypeReg, Mode: int64(entry.mode.Perm()), Size: size, ModTime: tarTime, Format: tar.FormatUSTAR}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if err := copyEntry(tarWriter, entry); err != nil {
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	tarClosed = true
	if err := gzipWriter.Close(); err != nil {
		return err
	}
	gzipClosed = true
	return nil
}

func writeZIP(path, root string, entries []archiveEntry) (returnErr error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); returnErr == nil {
			returnErr = err
		}
	}()
	writer := zip.NewWriter(file)
	zipClosed := false
	defer func() {
		if !zipClosed {
			_ = writer.Close()
		}
	}()
	directory := &zip.FileHeader{Name: root + "/", Method: zip.Store}
	directory.SetModTime(zipTime)
	directory.SetMode(os.ModeDir | 0o755)
	if _, err := writer.CreateHeader(directory); err != nil {
		return err
	}
	for _, entry := range entries {
		header := &zip.FileHeader{Name: root + "/" + entry.name, Method: zip.Deflate}
		header.SetModTime(zipTime)
		header.SetMode(entry.mode)
		destination, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if err := copyEntry(destination, entry); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	zipClosed = true
	return nil
}

func entrySize(entry archiveEntry) (int64, error) {
	if entry.source == nil {
		return int64(len(entry.data)), nil
	}
	return entry.source.info.Size(), nil
}

func copyEntry(destination io.Writer, entry archiveEntry) error {
	if entry.source == nil {
		_, err := destination.Write(entry.data)
		return err
	}
	source, err := os.Open(entry.source.path)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !os.SameFile(entry.source.info, info) || info.Size() != entry.source.info.Size() {
		return errors.New("source file changed")
	}
	if _, err := io.CopyN(destination, source, info.Size()); err != nil {
		return err
	}
	var extra [1]byte
	count, err := source.Read(extra[:])
	if count != 0 || !errors.Is(err, io.EOF) {
		return errors.New("source file changed")
	}
	return nil
}
