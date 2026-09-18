package platformtransport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func TestNativeContractUsesSameAuthenticatedTransport(t *testing.T) {
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)}
	raw.CPU.State, raw.Memory.State, raw.Uptime.State, raw.Filesystems.State = observation.Unknown, observation.Unknown, observation.Unknown, observation.Unknown
	body, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: transportBinding.ServerID, Version: "0.1.0", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		got, readErr := io.ReadAll(r.Body)
		if readErr != nil || !bytes.Equal(got, body) || r.Method != http.MethodPut ||
			r.URL.Path != "/v1/connectors/home-lab/servers/"+transportBinding.ServerID+"/snapshot" ||
			r.Header.Get("Authorization") != "Connector "+transportCredential || r.Header.Get("X-Connector-Sequence") != "7" {
			t.Error("native upload changed authenticated request contract")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"server_id":%q,"sequence":7}`, transportBinding.ServerID)
	}))
	defer server.Close()
	provider := &fakeCredentials{credential: transportCredential}
	transport := newOwnedTransport(t, server, provider)
	request := uploadstate.Request{Binding: transportBinding, Sequence: 7, Body: body}
	result, err := transport.Send(context.Background(), request)
	if err != nil || result.Outcome != uploadstate.Acknowledged || requests.Load() != 1 {
		t.Fatal("native acknowledgement failed")
	}
	request.Body = bytes.Replace(body, []byte(transportBinding.ServerID), []byte("srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 1)
	if _, err := transport.Send(context.Background(), request); err != ErrRequest || provider.calls != 1 || requests.Load() != 1 {
		t.Fatal("cross-server native body reached credentials or network")
	}
}
