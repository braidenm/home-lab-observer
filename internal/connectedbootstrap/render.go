// Package connectedbootstrap renders a fixed release helper. It performs no
// download, installation, enrollment or service operation in the build process.
package connectedbootstrap

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
)

var ErrInvalid = errors.New("connected_bootstrap_invalid")
var versionPattern = regexp.MustCompile(`^0\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})-canary\.[1-9][0-9]{0,3}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

//go:embed bootstrap.sh.tmpl
var shellTemplate string

//go:embed extract.py
var extractionSource string

//go:embed coordinator.py
var coordinatorSource string

type member struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
	Header string `json:"header"`
}

type release struct {
	Version, Commit, ArchiveSHA256, ManifestSHA256, Extraction, Coordinator string
	ArchiveSize                                                             int64
	Members                                                                 []member
}

// Render accepts only a canonical V2 manifest and independently computed archive
// identity. The caller must first build/verify that archive with connectedbundle.
func Render(manifest []byte, archiveSHA256 string, archiveSize int64) ([]byte, error) {
	m, err := connectedbundle.Decode(manifest)
	if err != nil || !m.HasKnownContract() || !versionPattern.MatchString(m.Version) ||
		!digestPattern.MatchString(archiveSHA256) || archiveSize < 1 || archiveSize > 256<<20 {
		return nil, ErrInvalid
	}
	digest := sha256.Sum256(manifest)
	r := release{Version: m.Version, Commit: m.Commit, ArchiveSHA256: archiveSHA256,
		ArchiveSize: archiveSize, ManifestSHA256: hex.EncodeToString(digest[:])}
	files := append([]connectedbundle.File(nil), m.Files...)
	files = append(files, connectedbundle.File{Name: connectedbundle.ManifestName, SHA256: r.ManifestSHA256, Size: int64(len(manifest)), Mode: 0o644})
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	for _, file := range files {
		var header bytes.Buffer
		writer := tar.NewWriter(&header)
		if writer.WriteHeader(&tar.Header{Name: file.Name, Mode: int64(file.Mode), Size: file.Size,
			Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}) != nil || header.Len() != 512 {
			return nil, ErrInvalid
		}
		r.Members = append(r.Members, member{file.Name, file.SHA256, file.Size, file.Mode, base64.StdEncoding.EncodeToString(header.Bytes())})
	}
	encoded, err := json.Marshal(r.Members)
	if err != nil {
		return nil, ErrInvalid
	}
	r.Extraction = extractionSource + "\nMEMBERS = json.loads(base64.b64decode(\"" + base64.StdEncoding.EncodeToString(encoded) + "\", validate=True))\n"
	r.Coordinator = strings.NewReplacer("@VERSION@", r.Version, "@ARCHIVE_SIZE@", strconv.FormatInt(r.ArchiveSize, 10),
		"@ARCHIVE_SHA256@", r.ArchiveSHA256, "@MANIFEST_SHA256@", r.ManifestSHA256).Replace(coordinatorSource)
	tmpl, err := template.New("bootstrap").Parse(shellTemplate)
	if err != nil {
		return nil, ErrInvalid
	}
	var output bytes.Buffer
	if tmpl.Execute(&output, r) != nil || output.Len() > 64<<10 {
		return nil, ErrInvalid
	}
	return output.Bytes(), nil
}

func extractionProgram(members []byte) string {
	return extractionSource + "\ntry:\n    extract(sys.argv[1], sys.argv[2], json.loads(base64.b64decode(\"" +
		base64.StdEncoding.EncodeToString(members) + "\", validate=True)))\n" +
		"except (OSError, EOFError, ValueError, zlib.error):\n    print(\"CONNECTED_BOOTSTRAP_EXTRACTION_REFUSED\", file=sys.stderr)\n    sys.exit(22)\n"
}
