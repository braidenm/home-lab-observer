package connectedcredential

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestBoundedExactCredential(t *testing.T) {
	r := CredentialRecord{Version: "observer-connected-credential/v1", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), Secret: "hlc_" + strings.Repeat("c", 43)}
	b, _ := json.Marshal(r)
	if _, err := DecodeCredential(b, r.ServerID, r.ConnectorID); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, append(append([]byte(nil), b...), '\n'), []byte(strings.Replace(string(b), `"secret":`, `"unknown":0,"secret":`, 1)), []byte(strings.Repeat("x", 513))} {
		if _, err := DecodeCredential(bad, r.ServerID, r.ConnectorID); err != ErrUnsafe {
			t.Fatal("malformed credential accepted")
		}
	}
	if _, err := DecodeCredential(b, r.ServerID, "agent_"+strings.Repeat("d", 32)); err != ErrUnsafe {
		t.Fatal("cross binding accepted")
	}
}

func TestCredentialFormattingIsRedacted(t *testing.T) {
	r := CredentialRecord{Version: "observer-connected-credential/v1", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), Secret: "hlc_" + strings.Repeat("c", 43)}
	for _, rendered := range []string{fmt.Sprint(r), fmt.Sprintf("%+v", r), fmt.Sprintf("%#v", r)} {
		if strings.Contains(rendered, r.Secret) || strings.Contains(rendered, r.ServerID) || strings.Contains(rendered, r.ConnectorID) {
			t.Fatal("credential formatting disclosed private fields")
		}
	}
	var out bytes.Buffer
	slog.New(slog.NewJSONHandler(&out, nil)).Info("credential-test", "credential", r)
	if strings.Contains(out.String(), r.Secret) || strings.Contains(out.String(), r.ServerID) || strings.Contains(out.String(), r.ConnectorID) {
		t.Fatal("structured log disclosed private fields")
	}
}
