package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedAssetManifest(t *testing.T) {
	var names []string
	err := fs.WalkDir(Assets(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			names = append(names, name)
			content, err := fs.ReadFile(Assets(), name)
			if err != nil {
				return err
			}
			if strings.HasSuffix(name, ".map") || strings.Contains(string(content), "sourceMappingURL") {
				t.Fatalf("source map material embedded in %s", name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"index.html", "static/dashboard.css", "static/dashboard.js"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("embedded files = %v, want %v", names, want)
	}
}

func TestHandlerServesAssetsAndSPAFallback(t *testing.T) {
	handler := HTTPHandler()
	for _, test := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{"/", "text/html", `<div id="root"></div>`},
		{"/workloads", "text/html", `<div id="root"></div>`},
		{"/static/dashboard.js", "text/javascript", "Home Lab Observer"},
		{"/static/dashboard.css", "text/css", ".observer-shell"},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d", response.Code)
			}
			if got := response.Header().Get("Content-Type"); !strings.Contains(got, test.contentType) {
				t.Fatalf("content type = %q", got)
			}
			if !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("response does not contain %q", test.contains)
			}
			for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options", "X-Frame-Options"} {
				if response.Header().Get(header) == "" {
					t.Errorf("missing %s", header)
				}
			}
		})
	}
}

func TestHandlerRejectsUnsafeAndUnsupportedRequests(t *testing.T) {
	handler := HTTPHandler()
	for _, target := range []string{"/../go.mod", `/static\dashboard.js`, "/missing.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %q status = %d, want 404", target, response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST status/header = %d/%q", response.Code, response.Header().Get("Allow"))
	}
}

func TestHandlerHEADHasNoBody(t *testing.T) {
	response := httptest.NewRecorder()
	HTTPHandler().ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/static/dashboard.js", nil))
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("HEAD status/body = %d/%d", response.Code, response.Body.Len())
	}
}
