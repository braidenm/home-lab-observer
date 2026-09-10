package releasepack

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostileTarHeadersAreRejected(t *testing.T) {
	in, manifest, _ := v2Fixture(t)
	asset := manifest.Assets[0]
	root := strings.TrimSuffix(asset.Filename, ".tar.gz")
	helper, _ := os.ReadFile(in.binaries[journalInputName("amd64")].path)
	for _, mode := range []string{"symlink", "hardlink", "duplicate", "traversal", "setuid", "setgid", "second-stream", "device", "pax"} {
		t.Run(mode, func(t *testing.T) {
			var raw bytes.Buffer
			w := tar.NewWriter(&raw)
			if err := w.WriteHeader(&tar.Header{Name: root + "/", Typeflag: tar.TypeDir, Mode: 0755, Format: tar.FormatUSTAR}); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"observer", "LICENSE", "START-HERE.md", "run-observer.sh", "observer-journal-helper"} {
				data := []byte("synthetic")
				if name == "observer-journal-helper" {
					data = helper
				}
				h := &tar.Header{Name: root + "/" + name, Typeflag: tar.TypeReg, Mode: 0755, Size: int64(len(data)), Format: tar.FormatUSTAR}
				if name == "LICENSE" {
					switch mode {
					case "symlink":
						h.Typeflag = tar.TypeSymlink
						h.Linkname = "observer"
						h.Size = 0
						data = nil
					case "hardlink":
						h.Typeflag = tar.TypeLink
						h.Linkname = root + "/observer"
						h.Size = 0
						data = nil
					case "traversal":
						h.Name = root + "/../LICENSE"
					case "setuid":
						h.Mode = 04755
					case "setgid":
						h.Mode = 02755
					case "device":
						h.Typeflag = tar.TypeChar
						h.Size = 0
						data = nil
					case "pax":
						h.Format = tar.FormatPAX
						h.PAXRecords = map[string]string{"comment": "private-canary"}
					}
				}
				copies := 1
				if mode == "duplicate" && name == "LICENSE" {
					copies = 2
				}
				for i := 0; i < copies; i++ {
					if err := w.WriteHeader(h); err != nil {
						t.Fatal(err)
					}
					if _, err := w.Write(data); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if mode == "second-stream" {
				raw.Write([]byte("SECOND-TAR-STREAM-CANARY"))
			}
			path := filepath.Join(t.TempDir(), asset.Filename)
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(file)
			if _, err := gz.Write(raw.Bytes()); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			file.Close()
			if verifyArchive(path, asset) == nil {
				t.Fatal("hostile tar accepted")
			}
		})
	}
}

func TestHostileZipHeadersAreRejected(t *testing.T) {
	_, manifest, _ := v2Fixture(t)
	asset := manifest.Assets[4]
	root := strings.TrimSuffix(asset.Filename, ".zip")
	for _, mode := range []string{"symlink", "duplicate", "traversal", "setuid", "reparse", "directory-file"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), asset.Filename)
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			w := zip.NewWriter(file)
			defer file.Close()
			rootHeader := &zip.FileHeader{Name: root + "/"}
			rootHeader.SetMode(os.ModeDir | 0755)
			if _, err := w.CreateHeader(rootHeader); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"observer.exe", "LICENSE", "START-HERE.md", "Run-Observer.cmd"} {
				h := &zip.FileHeader{Name: root + "/" + name, Method: zip.Store}
				h.SetMode(0755)
				if name == "LICENSE" {
					switch mode {
					case "symlink":
						h.SetMode(os.ModeSymlink | 0755)
					case "traversal":
						h.Name = root + "/../LICENSE"
					case "setuid":
						h.SetMode(os.ModeSetuid | 0755)
					case "reparse":
						h.ExternalAttrs |= 0x400
					case "directory-file":
						h.SetMode(os.ModeDir | 0755)
					}
				}
				copies := 1
				if mode == "duplicate" && name == "LICENSE" {
					copies = 2
				}
				for i := 0; i < copies; i++ {
					copyHeader := *h
					destination, err := w.CreateHeader(&copyHeader)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := destination.Write([]byte("synthetic")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			file.Close()
			if verifyArchive(path, asset) == nil {
				t.Fatal("hostile zip accepted")
			}
		})
	}
}
