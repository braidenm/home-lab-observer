package localapi

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/localauth"
)

type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	RequestID string `json:"request_id"`
}

func ValidateBind(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return fmt.Errorf("local API bind must be an explicit 127.0.0.1 address")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("local API port must be between 1 and 65535")
	}
	return nil
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; form-action 'none'; frame-ancestors 'none'; object-src 'none'")
		headers.Set("Cross-Origin-Opener-Policy", "same-origin")
		headers.Set("Cross-Origin-Resource-Policy", "same-origin")
		headers.Set("Referrer-Policy", "no-referrer")
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func LocalBoundary(port int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.IsAbs() || !validHost(r.Host, port) || !validOrigin(r.Header.Get("Origin"), r.Host, port) || !validFetchSite(r.Header.Get("Sec-Fetch-Site")) {
			writeProblem(w, http.StatusForbidden, "LOCAL_BOUNDARY_REJECTED", "Request rejected by local boundary")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeProblem(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireBearer(expectedToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localauth.MatchesAuthorization(r.Header.Get("Authorization"), expectedToken) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="home-lab-observer"`)
			writeRequestProblem(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validHost(value string, port int) bool {
	host, portText, err := net.SplitHostPort(value)
	if err != nil || portText != strconv.Itoa(port) {
		return false
	}
	host = strings.ToLower(host)
	return host == "127.0.0.1" || host == "localhost"
}

func validOrigin(value, requestHost string, port int) bool {
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return validHost(parsed.Host, port) && strings.EqualFold(parsed.Host, requestHost)
}

func validFetchSite(value string) bool {
	switch strings.ToLower(value) {
	case "", "none", "same-origin":
		return true
	default:
		return false
	}
}

func writeProblem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{Type: "about:blank", Title: title, Status: status, Code: code, RequestID: requestID()})
}

func writeRequestProblem(w http.ResponseWriter, r *http.Request, status int, code, title string) {
	var encoded bytes.Buffer
	_ = json.NewEncoder(&encoded).Encode(Problem{Type: "about:blank", Title: title, Status: status, Code: code, RequestID: requestID()})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Content-Length", strconv.Itoa(encoded.Len()))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(encoded.Bytes())
	}
}

func requestID() string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "observer_internal"
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
