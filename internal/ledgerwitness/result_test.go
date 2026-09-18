package ledgerwitness

import (
	"bytes"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func TestPrivateResultClosedProtocol(t *testing.T) {
	binding := uploadstate.Binding{ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32)}
	fingerprint := [32]byte{0xab, 0xcd}
	data, err := EncodeResult(binding, fingerprint)
	if err != nil {
		t.Fatal("result encoding refused")
	}
	got, err := DecodeResult(data, binding)
	if err != nil || got != fingerprint {
		t.Fatal("result roundtrip refused")
	}
	for _, bad := range [][]byte{
		nil, bytes.Repeat([]byte("x"), MaxResultBytes+1), append(append([]byte(nil), data...), '\n'),
		bytes.Replace(data, []byte(ResultVersion), []byte("observer-existing-ledger-result/v2"), 1),
		bytes.Replace(data, []byte("abcd"), []byte("ABCD"), 1),
		bytes.Replace(data, []byte("abcd"), []byte("zzzz"), 1),
		bytes.Replace(data, []byte("abcd"), []byte("abc"), 1),
		bytes.Replace(data, []byte(`{"version":`), []byte(`{"unknown":0,"version":`), 1),
		bytes.Replace(data, []byte(`{"version":`), []byte(`{"version":"observer-existing-ledger-result/v1","version":`), 1),
	} {
		if _, err := DecodeResult(bad, binding); err != ErrUnsafe {
			t.Fatal("ambiguous private result accepted")
		}
	}
	foreign := binding
	foreign.ConnectorID = "agent_" + strings.Repeat("c", 32)
	if _, err := DecodeResult(data, foreign); err != ErrUnsafe {
		t.Fatal("foreign result accepted")
	}
	foreign = binding
	foreign.ServerID = "srv_" + strings.Repeat("c", 32)
	if _, err := DecodeResult(data, foreign); err != ErrUnsafe {
		t.Fatal("foreign server result accepted")
	}
	if _, err := EncodeResult(uploadstate.Binding{}, fingerprint); err != ErrUnsafe {
		t.Fatal("empty binding encoded")
	}
}
