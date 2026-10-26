package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestActivationAdmissionSurvivesReceiverRestartWithoutReenrollment(t *testing.T) {
	connector := "agent_" + strings.Repeat("c", 32)
	var original atomic.Bool
	var persisted []byte
	enrollment := fixtureHandlerWithAdmission(&original, false, func(value string) error {
		var err error
		persisted, err = admissionBytes(value)
		return err
	})
	w := httptest.NewRecorder()
	enrollment.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"enrollment_secret":"`+grant+`","connector_instance_id":"`+connector+`"}`)))
	if w.Code != 200 || !original.Load() || bytes.Contains(persisted, []byte(credential)) {
		t.Fatal("synthetic admission was not persisted before exchange")
	}
	config := connectedprofile.Config{ServerID: serverID, ConnectorID: connector}
	secret, _ := json.Marshal(connectedcredential.CredentialRecord{Version: "observer-connected-credential/v1", ServerID: serverID, ConnectorID: connector, Secret: credential})
	var restarted atomic.Bool
	if validateAdmission(persisted, config, secret) != nil {
		t.Fatal("retained synthetic binding refused")
	}
	restarted.Store(true)
	body, err := os.ReadFile("../../../schemas/remote/fixtures/native-host-complete.json")
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.TrimSpace(bytes.ReplaceAll(body, []byte("srv_0123456789abcdef0123456789abcdef"), []byte(serverID)))
	upload := httptest.NewRequest(http.MethodPut, "/v1/connectors/home-lab/servers/"+serverID+"/snapshot", bytes.NewReader(body))
	upload.Header.Set("Authorization", "Connector "+credential)
	upload.Header.Set("X-Connector-Sequence", "3")
	uploadResponse := httptest.NewRecorder()
	(&activationReceiver{enrolled: &restarted}).handler(fixtureHandler(&restarted, false)).ServeHTTP(uploadResponse, upload)
	if uploadResponse.Code != 200 {
		t.Fatal("retained credential cannot upload after receiver restart")
	}
	response := httptest.NewRecorder()
	fixtureHandler(&restarted, false).ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"enrollment_secret":"`+grant+`","connector_instance_id":"`+connector+`"}`)))
	if response.Code != 400 || !restarted.Load() {
		t.Fatal("receiver restart reopened enrollment")
	}
	for _, changed := range [][]byte{append(append([]byte(nil), persisted...), '\n'), []byte(`{}`), bytes.ReplaceAll(persisted, []byte(connector), []byte("agent_"+strings.Repeat("d", 32)))} {
		if validateAdmission(changed, config, secret) == nil {
			t.Fatal("changed admission accepted")
		}
	}
	config.ConnectorID = "agent_" + strings.Repeat("e", 32)
	if validateAdmission(persisted, config, secret) == nil {
		t.Fatal("different installed connector admitted")
	}
}

func TestFailedAdmissionPersistenceNeverReturnsCredential(t *testing.T) {
	var consumed atomic.Bool
	handler := fixtureHandlerWithAdmission(&consumed, false, func(string) error { return errAdmission })
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"enrollment_secret":"`+grant+`","connector_instance_id":"agent_`+strings.Repeat("f", 32)+`"}`)))
	if w.Code != 500 || !consumed.Load() || strings.Contains(w.Body.String(), credential) {
		t.Fatal("failed persistence must consume and refuse without returning credential")
	}
}
