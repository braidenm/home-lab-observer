package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestActivationReceiverRequiresEnrollmentAndExactNativeSequence(t *testing.T) {
	body, err := os.ReadFile("../../../schemas/remote/fixtures/native-host-complete.json")
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.TrimSpace(bytes.ReplaceAll(body, []byte("srv_0123456789abcdef0123456789abcdef"), []byte(serverID)))
	var enrolled atomic.Bool
	var counts activationCounts
	a := &activationReceiver{enrolled: &enrolled, write: func(value activationCounts) error { counts = value; return nil }}
	handler := a.handler(fixtureHandler(&enrolled, false))
	for _, test := range []struct {
		enrolled       bool
		auth, sequence string
		body           []byte
		want           int
	}{
		{false, "Connector " + credential, "1", body, 401}, {true, "wrong", "1", body, 401},
		{true, "Connector " + credential, "0", body, 400}, {true, "Connector " + credential, "1", []byte("{}"), 400},
		{true, "Connector " + credential, "1", body, 200}, {true, "Connector " + credential, "1", body, 200},
		{true, "Connector " + credential, "1", bytes.ReplaceAll(body, []byte("preview.3"), []byte("preview.4")), 409},
		{true, "Connector " + credential, "2", body, 200}, {true, "Connector " + credential, "1", body, 409},
	} {
		enrolled.Store(test.enrolled)
		r := httptest.NewRequest(http.MethodPut, "/v1/connectors/home-lab/servers/"+serverID+"/snapshot", bytes.NewReader(test.body))
		r.Header.Set("Authorization", test.auth)
		r.Header.Set("X-Connector-Sequence", test.sequence)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("sequence %s: got %d want %d", test.sequence, w.Code, test.want)
		}
		if strings.Contains(w.Body.String(), credential) {
			t.Fatal("credential leaked")
		}
	}
	if counts.Sequence != 2 || counts.Accepted != 2 || counts.Duplicates != 1 || !counts.UsefulHost {
		t.Fatal("bounded counter contract failed")
	}
}
