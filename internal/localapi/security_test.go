package localapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateBindRejectsNonLoopbackAndHostnames(t *testing.T) {
	for _, address := range []string{"", "localhost:9847", "0.0.0.0:9847", "192.168.1.10:9847", "[::1]:9847", "127.0.0.1:0", "127.0.0.1:70000"} {
		if ValidateBind(address) == nil {
			t.Fatalf("accepted unsafe bind %q", address)
		}
	}
	if err := ValidateBind("127.0.0.1:9847"); err != nil {
		t.Fatalf("rejected default bind: %v", err)
	}
}

func TestLocalBoundaryAndSecurityHeaders(t *testing.T) {
	called := false
	handler := SecurityHeaders(LocalBoundary(9847, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	request.Host = "127.0.0.1:9847"
	request.Header.Set("Origin", "http://127.0.0.1:9847")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("valid local request code=%d called=%v", response.Code, called)
	}
	for _, name := range []string{"Content-Security-Policy", "Cross-Origin-Resource-Policy", "Referrer-Policy", "X-Content-Type-Options", "X-Frame-Options"} {
		if response.Header().Get(name) == "" {
			t.Fatalf("missing security header %s", name)
		}
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS header must not be emitted")
	}
}

func TestLocalBoundaryRejectsHostOriginFetchSiteAndMethod(t *testing.T) {
	tests := []struct {
		name, host, origin, fetchSite, method string
		status                                int
	}{
		{"host", "attacker.example:9847", "", "", http.MethodGet, http.StatusForbidden},
		{"origin", "127.0.0.1:9847", "https://attacker.example", "", http.MethodGet, http.StatusForbidden},
		{"origin alias mismatch", "127.0.0.1:9847", "http://localhost:9847", "", http.MethodGet, http.StatusForbidden},
		{"fetch site", "127.0.0.1:9847", "", "cross-site", http.MethodGet, http.StatusForbidden},
		{"method", "127.0.0.1:9847", "", "", http.MethodPost, http.StatusMethodNotAllowed},
	}
	handler := SecurityHeaders(LocalBoundary(9847, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("downstream called") })))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/api/v1/capabilities", nil)
			request.Host = test.host
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Sec-Fetch-Site", test.fetchSite)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("code=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			var problem Problem
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil || problem.RequestID == "" || strings.Contains(response.Body.String(), "attacker.example") {
				t.Fatalf("unsafe problem response: err=%v body=%s", err, response.Body.String())
			}
		})
	}
}

func TestRequireBearerDoesNotEchoCredentials(t *testing.T) {
	const token = "0123456789012345678901234567890123456789012"
	handler := RequireBearer(token, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, supplied := range []string{"", "Bearer wrong", "Basic " + token} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", supplied)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), token) || response.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("unsafe auth response code=%d body=%q", response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid bearer code=%d", response.Code)
	}
}
