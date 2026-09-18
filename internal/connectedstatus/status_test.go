package connectedstatus

import (
	"bytes"
	"math"
	"testing"
	"time"
)

func TestClosedBoundedStatus(t *testing.T) {
	r := Record{Version: "observer-connected-status/v1", State: "WAITING_FIRST_UPLOAD", UpdatedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC), Attempts: math.MaxUint64, Acknowledgements: math.MaxUint64}
	b, err := Encode(r)
	if err != nil || len(b) > 1024 || bytes.Contains(b, []byte("secret")) {
		t.Fatal("status bound failed")
	}
	if got, err := Decode(b); err != nil || got != r {
		t.Fatal("canonical status did not round trip")
	}
	for _, bad := range [][]byte{append(append([]byte(nil), b...), '\n'), []byte(`{"version":"observer-connected-status/v1","state":"COLLECTING","unexpected":"private"}`), bytes.Repeat([]byte("x"), 1025)} {
		if _, err := Decode(bad); err != ErrUnsafe {
			t.Fatal("ambiguous status accepted")
		}
	}
	r.State = "private-error-with-token"
	if _, err := Encode(r); err != ErrUnsafe {
		t.Fatal("unbounded state accepted")
	}
}
