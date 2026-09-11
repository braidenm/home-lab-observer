package connectedbundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"
	"time"
)

// WriteArchive emits a deterministic flat tar.gz of exactly the verified bundle.
// It rehashes the actual copied bytes so source replacement cannot produce a
// successful archive result after a previous directory verification.
func WriteArchive(w io.Writer, directory, expectedManifestSHA256 string) error {
	if w == nil {
		return ErrInvalid
	}
	m, err := VerifyDirectory(directory, expectedManifestSHA256)
	if err != nil {
		return ErrInvalid
	}
	root, err := openRoot(directory)
	if err != nil {
		return ErrInvalid
	}
	defer root.Close()
	manifest, err := Encode(m)
	if err != nil {
		return ErrInvalid
	}
	entries := append([]File(nil), m.Files...)
	entries = append(entries, File{Name: ManifestName, SHA256: expectedManifestSHA256, Size: int64(len(manifest)), Mode: 0o644})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for _, entry := range entries {
		f, err := openFile(root, directory, entry.Name, entry.Size)
		if err != nil {
			return ErrInvalid
		}
		h := &tar.Header{Name: entry.Name, Mode: int64(entry.Mode), Size: entry.Size, Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}
		if tw.WriteHeader(h) != nil {
			f.Close()
			return ErrInvalid
		}
		hash := sha256.New()
		n, copyErr := io.CopyN(io.MultiWriter(tw, hash), f, entry.Size)
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil || n != entry.Size || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return ErrInvalid
		}
	}
	if tw.Close() != nil || gz.Close() != nil {
		return ErrInvalid
	}
	return nil
}
