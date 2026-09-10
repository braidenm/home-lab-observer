// Package buildidentity defines the authoritative, non-secret release identity.
// Byte framing proves package consistency, not publisher authenticity.
package buildidentity

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
)

const MaxRecordBytes = 256
const MaxFileBytes = 200 << 20

var ErrInvalid = errors.New("BUILD_IDENTITY_INVALID")
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$`)

type Identity struct{ Role, Version, Commit, OS, Arch, HelperSHA256 string }

// Do not place a complete framing prefix in parser binaries: it would itself
// be an ambiguous dangling record. Real cross-build tests enforce this property.
func prefix() string { return strings.ReplaceAll("HLO-RELEASE-!IDENTITY-V1[", "!", "") }
func suffix() string { return strings.ReplaceAll("END-HLO-!IDENTITY", "!", "") }

func Encode(identity Identity) (string, error) {
	if identity.Validate() != nil {
		return "", ErrInvalid
	}
	digest := identity.HelperSHA256
	if digest == "" {
		digest = "-"
	}
	record := prefix() + strings.Join([]string{identity.Role, identity.Version, identity.Commit, identity.OS, identity.Arch, digest}, "|") + "]" + suffix()
	if len(record) > MaxRecordBytes {
		return "", ErrInvalid
	}
	return record, nil
}

func Parse(record string) (Identity, error) {
	if len(record) > MaxRecordBytes || !strings.HasPrefix(record, prefix()) || !strings.HasSuffix(record, "]"+suffix()) {
		return Identity{}, ErrInvalid
	}
	fields := strings.Split(record[len(prefix()):len(record)-len(suffix())-1], "|")
	if len(fields) != 6 {
		return Identity{}, ErrInvalid
	}
	i := Identity{Role: fields[0], Version: fields[1], Commit: fields[2], OS: fields[3], Arch: fields[4], HelperSHA256: fields[5]}
	if i.HelperSHA256 == "-" {
		i.HelperSHA256 = ""
	}
	canonical, err := Encode(i)
	if err != nil || canonical != record {
		return Identity{}, ErrInvalid
	}
	return i, nil
}

// Resolve preserves legacy development/v1 identity only when the record is
// empty. A nonempty malformed/mismatched record can never fall back.
func Resolve(record, legacyVersion, legacyCommit, role, goos, arch string) (Identity, error) {
	if record == "" {
		return Identity{Role: role, Version: legacyVersion, Commit: legacyCommit, OS: goos, Arch: arch}, nil
	}
	i, err := Parse(record)
	if err != nil || i.Role != role || i.OS != goos || i.Arch != arch {
		return Identity{}, ErrInvalid
	}
	return i, nil
}

func (i Identity) Validate() error {
	if i.Role != "observer" && i.Role != "journal-helper" {
		return ErrInvalid
	}
	if len(i.Version) > 64 || !versionPattern.MatchString(i.Version) || !lowerHex(i.Commit, 40) {
		return ErrInvalid
	}
	for _, part := range strings.Split(strings.SplitN(i.Version, "-", 2)[1], ".") {
		if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
			return ErrInvalid
		}
	}
	if i.OS != "linux" && i.OS != "windows" && i.OS != "darwin" {
		return ErrInvalid
	}
	if i.Arch != "amd64" && i.Arch != "arm64" {
		return ErrInvalid
	}
	if i.Role == "journal-helper" && i.OS != "linux" {
		return ErrInvalid
	}
	if i.Role == "observer" && i.OS == "linux" {
		if !lowerHex(i.HelperSHA256, 64) {
			return ErrInvalid
		}
	} else if i.HelperSHA256 != "" {
		return ErrInvalid
	}
	return nil
}

func lowerHex(value string, n int) bool {
	if len(value) != n {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Scan reads a bounded regular-file view supplied by the caller, retaining only
// a fixed chunk and record overlap. Every full prefix must form one valid record.
func Scan(reader io.Reader, size int64) (Identity, error) {
	if reader == nil || size <= 0 || size > MaxFileBytes {
		return Identity{}, ErrInvalid
	}
	limited := &io.LimitedReader{R: reader, N: size + 1}
	marker := []byte(prefix())
	ending := []byte("]" + suffix())
	buffer := make([]byte, 0, 32768+MaxRecordBytes)
	chunk := make([]byte, 32768)
	var found Identity
	count := 0
	var read int64
	for {
		n, err := limited.Read(chunk)
		read += int64(n)
		buffer = append(buffer, chunk[:n]...)
		if read > size {
			return Identity{}, ErrInvalid
		}
		final := err == io.EOF
		if err != nil && !final {
			return Identity{}, ErrInvalid
		}
		for {
			start := bytes.Index(buffer, marker)
			if start < 0 {
				keep := len(marker) - 1
				if len(buffer) > keep {
					buffer = append(buffer[:0], buffer[len(buffer)-keep:]...)
				}
				break
			}
			if start > 0 {
				buffer = append(buffer[:0], buffer[start:]...)
			}
			end := bytes.Index(buffer, ending)
			if end < 0 {
				if len(buffer) >= MaxRecordBytes || final {
					return Identity{}, ErrInvalid
				}
				break
			}
			length := end + len(ending)
			identity, parseErr := Parse(string(buffer[:length]))
			if parseErr != nil {
				return Identity{}, ErrInvalid
			}
			count++
			if count != 1 {
				return Identity{}, ErrInvalid
			}
			found = identity
			buffer = append(buffer[:0], buffer[length:]...)
		}
		if final {
			break
		}
		if n == 0 {
			return Identity{}, ErrInvalid
		}
	}
	if read != size || count != 1 {
		return Identity{}, ErrInvalid
	}
	return found, nil
}
