package connectedbundle

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
)

func TestV2ContractAndLegacyNonAuthority(t *testing.T) {
	legacy := fakeManifest()
	data, err := Encode(legacy)
	if err != nil || legacy.HasKnownContract() || bytes.Contains(data, []byte("contract_sha256")) {
		t.Fatal("legacy authority or format changed")
	}
	v2 := fakeManifest()
	v2.Schema = VersionV2
	v2.ContractSHA256 = connectedcompat.Digest()
	data, err = Encode(v2)
	if err != nil || !v2.HasKnownContract() {
		t.Fatal("known v2 refused")
	}
	decoded, err := Decode(data)
	if err != nil || !decoded.HasKnownContract() {
		t.Fatal("v2 roundtrip")
	}
	for _, change := range []func(*Manifest){
		func(m *Manifest) { m.ContractSHA256 = "" },
		func(m *Manifest) { m.ContractSHA256 = strings.Repeat("f", 64) },
		func(m *Manifest) { m.ContractSHA256 = strings.ToUpper(m.ContractSHA256) },
		func(m *Manifest) { m.Schema = Version },
	} {
		bad := v2
		change(&bad)
		b, _ := json.Marshal(bad)
		if _, e := Decode(b); e != ErrInvalid || bad.HasKnownContract() {
			t.Fatal("unknown or mixed contract accepted")
		}
	}
	for _, bad := range [][]byte{append(append([]byte{}, data...), '\n'), bytes.Replace(data, []byte(`"contract_sha256":`), []byte(`"contract_sha256":"wrong","contract_sha256":`), 1), bytes.Replace(data, []byte(`"contract_sha256":`), []byte(`"extra":0,"contract_sha256":`), 1)} {
		if _, e := Decode(bad); e != ErrInvalid {
			t.Fatal("noncanonical v2 accepted")
		}
	}
}
