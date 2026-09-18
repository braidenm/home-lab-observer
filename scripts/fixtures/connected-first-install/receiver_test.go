package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSyntheticReceiverIsOneUseAndClosed(t *testing.T) {
	var consumed atomic.Bool
	handler := fixtureHandler(&consumed)
	requestBody := `{"enrollment_secret":"` + grant + `","connector_instance_id":"agent_` + strings.Repeat("a", 32) + `"}`
	for i, tc := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodGet, path, requestBody, http.StatusNotFound},
		{http.MethodPost, path + "?q=1", requestBody, http.StatusNotFound},
		{http.MethodPost, path, strings.Replace(requestBody, grant, "wrong", 1), http.StatusBadRequest},
		{http.MethodPost, path, requestBody, http.StatusOK},
		{http.MethodPost, path, requestBody, http.StatusBadRequest},
	} {
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("case %d returned %d", i, response.Code)
		}
		if tc.want == http.StatusOK {
			body := response.Body.String()
			if !strings.Contains(body, serverID) || !strings.Contains(body, credential) || strings.Contains(body, grant) {
				t.Fatal("synthetic response contract violated")
			}
		}
	}
}
