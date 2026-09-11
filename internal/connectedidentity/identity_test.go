package connectedidentity

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func fixture() Identity {
	return Identity{"collector", "0.1.0-canary.1", strings.Repeat("a", 40), "linux", "amd64"}
}

func TestClosedIdentity(t *testing.T) {
	for _, role := range []string{"collector", "uploader", "install"} {
		i := fixture()
		i.Role = role
		r, err := Encode(i)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Parse(r)
		if err != nil || got != i {
			t.Fatal("roundtrip")
		}
		got, err = Resolve(r, role)
		if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
			if err != nil || got != i {
				t.Fatal("runtime identity")
			}
		} else if err != ErrInvalid {
			t.Fatal("unsupported runtime")
		}
		if _, err := Resolve(r, "wrong"); err != ErrInvalid {
			t.Fatal("wrong role")
		}
	}
	for _, change := range []func(*Identity){func(i *Identity) { i.Role = "observer" }, func(i *Identity) { i.OS = "windows" }, func(i *Identity) { i.Arch = "arm64" }, func(i *Identity) { i.Version = "1.0.0" }, func(i *Identity) { i.Version = "1.0.0-01" }, func(i *Identity) { i.Commit = strings.Repeat("A", 40) }} {
		i := fixture()
		change(&i)
		if _, err := Encode(i); err != ErrInvalid {
			t.Fatal("invalid identity")
		}
	}
	if _, err := Resolve("", "collector"); err != ErrInvalid {
		t.Fatal("legacy fallback")
	}
}

func TestBoundedScan(t *testing.T) {
	r, _ := Encode(fixture())
	for offset := 32720; offset < 32790; offset++ {
		data := []byte(strings.Repeat("x", offset) + r + "tail")
		got, err := Scan(bytes.NewReader(data), int64(len(data)))
		if err != nil || got != fixture() {
			t.Fatal("chunk spanning record")
		}
	}
	for _, data := range []string{"", r + r, r + prefix(), prefix() + "broken", strings.Replace(r, "collector", "observer", 1), "no marker", strings.Repeat("x", 32760) + prefix() + strings.Repeat("x", MaxRecordBytes)} {
		if _, err := Scan(strings.NewReader(data), int64(len(data))); err != ErrInvalid {
			t.Fatal("hostile framing accepted")
		}
	}
	if _, err := Scan(strings.NewReader(r), int64(len(r)-1)); err != ErrInvalid {
		t.Fatal("size mismatch")
	}
	if _, err := Scan(strings.NewReader(r), int64(len(r)+1)); err != ErrInvalid {
		t.Fatal("truncated file")
	}
	if _, err := Scan(nil, 1); err != ErrInvalid {
		t.Fatal("nil")
	}
	if _, err := Scan(strings.NewReader(r), MaxFileBytes+1); err != ErrInvalid {
		t.Fatal("oversize")
	}
}
