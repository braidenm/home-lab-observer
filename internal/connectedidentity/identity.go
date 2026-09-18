// Package connectedidentity defines the separate connected-canary build identity.
// Its framing establishes consistency, not publisher authenticity.
package connectedidentity

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"runtime"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
)

const MaxRecordBytes = 256
const MaxFileBytes = 200 << 20

var ErrInvalid = errors.New("connected_identity_invalid")
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

type Identity struct{ Role, Version, Commit, OS, Arch, ContractSHA256 string }

// Split framing avoids embedding a dangling prefix in binaries that parse it.
func prefix() string     { return strings.ReplaceAll("HLO-CONNECTED-!IDENTITY-V1[", "!", "") }
func prefixV2() string   { return strings.ReplaceAll("HLO-CONNECTED-!IDENTITY-V2[", "!", "") }
func scanPrefix() string { return strings.ReplaceAll("HLO-CONNECTED-!IDENTITY-V", "!", "") }
func suffix() string     { return strings.ReplaceAll("END-HLO-CONNECTED-!IDENTITY", "!", "") }

func (i Identity) Validate() error {
	if i.ContractSHA256 != "" && !connectedcompat.Known(i.ContractSHA256) {
		return ErrInvalid
	}
	if (i.Role != "collector" && i.Role != "uploader" && i.Role != "install") || i.OS != "linux" || i.Arch != "amd64" || len(i.Version) > 40 || !versionPattern.MatchString(i.Version) || !commitPattern.MatchString(i.Commit) {
		return ErrInvalid
	}
	for _, part := range strings.Split(strings.SplitN(i.Version, "-", 2)[1], ".") {
		if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
			return ErrInvalid
		}
	}
	return nil
}

// HasKnownContract recognizes a declaration, not package authenticity or
// permission to select code, recover storage or activate a worker.
func (i Identity) HasKnownContract() bool {
	return i.Validate() == nil && connectedcompat.Known(i.ContractSHA256)
}

func Encode(i Identity) (string, error) {
	if i.Validate() != nil {
		return "", ErrInvalid
	}
	fields := []string{i.Role, i.Version, i.Commit, i.OS, i.Arch}
	marker := prefix()
	if i.ContractSHA256 != "" {
		marker = prefixV2()
		fields = append(fields, i.ContractSHA256)
	}
	r := marker + strings.Join(fields, "|") + "]" + suffix()
	if len(r) > MaxRecordBytes {
		return "", ErrInvalid
	}
	return r, nil
}

func Parse(record string) (Identity, error) {
	marker, count := prefix(), 5
	if strings.HasPrefix(record, prefixV2()) {
		marker, count = prefixV2(), 6
	}
	if len(record) > MaxRecordBytes || !strings.HasPrefix(record, marker) || !strings.HasSuffix(record, "]"+suffix()) {
		return Identity{}, ErrInvalid
	}
	f := strings.Split(record[len(marker):len(record)-len(suffix())-1], "|")
	if len(f) != count {
		return Identity{}, ErrInvalid
	}
	i := Identity{Role: f[0], Version: f[1], Commit: f[2], OS: f[3], Arch: f[4]}
	if count == 6 {
		i.ContractSHA256 = f[5]
		if !connectedcompat.Known(i.ContractSHA256) {
			return Identity{}, ErrInvalid
		}
	}
	canonical, err := Encode(i)
	if err != nil || canonical != record {
		return Identity{}, ErrInvalid
	}
	return i, nil
}

// Resolve has no development/legacy fallback. This initial installed profile is
// Linux/amd64 only; cross-platform local Observer identity remains independent.
func Resolve(record, expectedRole string) (Identity, error) {
	i, err := Parse(record)
	if err != nil || i.Role != expectedRole || i.OS != runtime.GOOS || i.Arch != runtime.GOARCH {
		return Identity{}, ErrInvalid
	}
	return i, nil
}

// Scan retains one fixed chunk plus record overlap. Every complete prefix must
// form the sole valid record. The caller verifies a bounded regular file first.
// Kept separate from native-v2 so its closed roles/validators do not change.
func Scan(reader io.Reader, size int64) (Identity, error) {
	if reader == nil || size <= 0 || size > MaxFileBytes {
		return Identity{}, ErrInvalid
	}
	limited := &io.LimitedReader{R: reader, N: size + 1}
	marker, ending := []byte(scanPrefix()), []byte("]"+suffix())
	buffer := make([]byte, 0, 32768+MaxRecordBytes)
	chunk := make([]byte, 32768)
	var found Identity
	var read int64
	count := 0
	for {
		n, err := limited.Read(chunk)
		read += int64(n)
		buffer = append(buffer, chunk[:n]...)
		if read > size || (err != nil && err != io.EOF) {
			return Identity{}, ErrInvalid
		}
		final := err == io.EOF
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
			i, parseErr := Parse(string(buffer[:length]))
			count++
			if parseErr != nil || count != 1 {
				return Identity{}, ErrInvalid
			}
			found = i
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
