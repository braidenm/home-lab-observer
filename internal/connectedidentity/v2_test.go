package connectedidentity

import (
	"bytes"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
)

func TestV2BindingAndV1NonAuthority(t *testing.T) {
	legacy := fixture()
	old, err := Encode(legacy)
	if err != nil || legacy.HasKnownContract() || !strings.HasPrefix(old, prefix()) {
		t.Fatal("legacy authority or encoding changed")
	}
	for _, role := range []string{"collector", "uploader", "install"} {
		i := fixture()
		i.Role = role
		i.ContractSHA256 = connectedcompat.Digest()
		r, err := Encode(i)
		if err != nil || !strings.HasPrefix(r, prefixV2()) || len(r) > MaxRecordBytes {
			t.Fatal("v2 encoding")
		}
		got, err := Parse(r)
		if err != nil || got != i || !got.HasKnownContract() {
			t.Fatal("v2 binding")
		}
		for offset := 32720; offset < 32790; offset++ {
			data := []byte(strings.Repeat("x", offset) + r + "tail")
			scanned, e := Scan(bytes.NewReader(data), int64(len(data)))
			if e != nil || scanned != i {
				t.Fatal("v2 chunk boundary")
			}
		}
		for _, bad := range []string{old + r, r + old, r + r, strings.Replace(r, connectedcompat.Digest(), strings.Repeat("f", 64), 1), strings.Replace(r, connectedcompat.Digest(), "", 1), strings.Replace(r, prefixV2(), prefix(), 1), r + scanPrefix() + "3[bad", strings.Replace(r, prefixV2(), strings.Replace(prefixV2(), "V2[", "V3[", 1), 1)} {
			if _, e := Scan(strings.NewReader(bad), int64(len(bad))); e != ErrInvalid {
				t.Fatal("mixed or unrecognized declaration accepted")
			}
		}
		i.ContractSHA256 = strings.Repeat("f", 64)
		if _, e := Encode(i); e != ErrInvalid || i.HasKnownContract() {
			t.Fatal("self asserted digest accepted")
		}
	}
}
