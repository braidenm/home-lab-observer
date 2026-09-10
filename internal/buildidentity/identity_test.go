package buildidentity

import (
	"bytes"
	"strings"
	"testing"
)

func fixture() Identity {
	return Identity{Role: "observer", Version: "0.1.0-preview.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", HelperSHA256: strings.Repeat("b", 64)}
}

func TestCanonicalIdentityAndAuthoritativeResolution(t *testing.T) {
	i := fixture()
	record, err := Encode(i)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Parse(record)
	if err != nil || decoded != i {
		t.Fatal("roundtrip failed")
	}
	resolved, err := Resolve(record, "ignored", "ignored", "observer", "linux", "amd64")
	if err != nil || resolved != i {
		t.Fatal("record was not authoritative")
	}
	for _, bad := range []string{record + "\n", strings.Replace(record, "|linux|", "|windows|", 1), strings.Replace(record, i.Commit, strings.Repeat("A", 40), 1), strings.Replace(record, i.Version, "0.1.0-preview.01", 1), strings.Replace(record, i.HelperSHA256, "-", 1), "private-canary"} {
		if _, err := Parse(bad); err != ErrInvalid {
			t.Fatal("invalid record accepted or leaked")
		}
	}
	if _, err := Resolve("bad", "0.1.0-preview.1", i.Commit, "observer", "linux", "amd64"); err != ErrInvalid {
		t.Fatal("malformed record fell back")
	}
	legacy, err := Resolve("", "dev", "unknown", "observer", "darwin", "arm64")
	if err != nil || legacy.Version != "dev" {
		t.Fatal("legacy identity changed")
	}
	if _, err := Resolve(record, "", "", "journal-helper", "linux", "amd64"); err != ErrInvalid {
		t.Fatal("role mismatch accepted")
	}
}

func TestScannerBoundsAndAmbiguity(t *testing.T) {
	record, _ := Encode(fixture())
	for _, offset := range []int{0, 1, 32768 - len(prefix()) + 1, 32767, 32768, 32769} {
		payload := append(bytes.Repeat([]byte{'x'}, offset), []byte(record)...)
		payload = append(payload, bytes.Repeat([]byte{'y'}, 40000)...)
		got, err := Scan(bytes.NewReader(payload), int64(len(payload)))
		if err != nil || got != fixture() {
			t.Fatal("boundary record not found")
		}
	}
	for name, payload := range map[string]string{
		"missing":            "synthetic binary",
		"duplicate":          record + record,
		"dangling-before":    prefix() + "canary" + record,
		"dangling-after":     record + prefix(),
		"truncated":          record[:len(record)-2],
		"oversized":          prefix() + strings.Repeat("x", MaxRecordBytes) + suffix(),
		"malformed-complete": prefix() + "private-canary]" + suffix(),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Scan(strings.NewReader(payload), int64(len(payload))); err != ErrInvalid {
				t.Fatal("ambiguous record accepted or leaked")
			}
		})
	}
	if _, err := Scan(strings.NewReader(record), int64(len(record)-1)); err != ErrInvalid {
		t.Fatal("size mismatch accepted")
	}
	if _, err := Scan(strings.NewReader(record), MaxFileBytes+1); err != ErrInvalid {
		t.Fatal("file bound bypassed")
	}
}
