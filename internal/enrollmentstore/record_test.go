package enrollmentstore

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func testBinding() uploadstate.Binding {
	return uploadstate.Binding{ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32)}
}
func testSecret() []byte { return []byte("hlc_" + strings.Repeat("c", 43)) }

func TestClosedCanonicalRecords(t *testing.T) {
	b := testBinding()
	for _, state := range []string{"ATTEMPTED", "CREDENTIAL", "READY"} {
		var secret []byte
		if state == "CREDENTIAL" {
			secret = testSecret()
		}
		data, _ := json.Marshal(makeRecord(b, state, secret))
		if _, err := decode(data, b, state); err != nil {
			t.Fatal(err)
		}
		for _, bad := range [][]byte{
			append(append([]byte(nil), data...), '\n'), []byte(" " + string(data)),
			[]byte(strings.Replace(string(data), `"version":`, `"unknown":0,"version":`, 1)),
			[]byte(strings.Replace(string(data), `"version":`, `"version":"ignored","version":`, 1)),
			[]byte(strings.Replace(string(data), `"version":`, `"Version":`, 1)),
			[]byte(strings.Replace(string(data), `srv_`, `srv_0`, 1)),
			[]byte(strings.Replace(string(data), `agent_`, `agent_0`, 1)),
			[]byte(strings.Replace(string(data), "observer-enrollment/v1", "observer-enrollment/v2", 1)),
			[]byte(strings.Replace(string(data), state, "OTHER", 1)),
			[]byte(strings.Repeat(" ", maxRecord+1)), nil,
		} {
			if _, err := decode(bad, b, state); err != ErrRecovery {
				t.Fatalf("noncanonical accepted: %v", err)
			}
		}
	}
	for _, secret := range []string{"", string(testSecret()) + "x", "hlc_" + strings.Repeat("!", 43)} {
		data, _ := json.Marshal(makeRecord(b, "CREDENTIAL", []byte(secret)))
		if _, err := decode(data, b, "CREDENTIAL"); err != ErrRecovery {
			t.Fatal("invalid secret accepted")
		}
	}
	data, _ := json.Marshal(makeRecord(b, "READY", testSecret()))
	if _, err := decode(data, b, "READY"); err != ErrRecovery {
		t.Fatal("secret in marker accepted")
	}
}
