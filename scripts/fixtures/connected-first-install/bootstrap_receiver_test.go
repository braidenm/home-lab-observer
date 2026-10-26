package main

import (
	"crypto/tls"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func bootstrapFixture(mode string) *bootstrapReceiver {
	return &bootstrapReceiver{mode: mode, version: "0.1.0-canary.1", helper: []byte("public helper"), archive: []byte("public archive"), persist: func(bootstrapCounts) error { return nil }}
}

func bootstrapRequest(host, path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://"+host+path, nil)
	r.TLS = &tls.ConnectionState{ServerName: host}
	return r
}

func TestBootstrapDownloadModesAndClosedAuthority(t *testing.T) {
	const prefix = "/braidenm/home-lab-observer/releases/download/v0.1.0-canary.1/"
	const archive = "home-lab-observer-connected_0.1.0-canary.1_linux_amd64.tar.gz"
	for _, mode := range []string{"bootstrap-helper-corrupt", "bootstrap-corrupt", "bootstrap-downgrade", "bootstrap-loop", "bootstrap-valid"} {
		t.Run(mode, func(t *testing.T) {
			b := bootstrapFixture(mode)
			enrollments := 0
			handler := b.handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { enrollments++ }))
			helper := httptest.NewRecorder()
			handler.ServeHTTP(helper, bootstrapRequest("github.com", prefix+"install-connected-linux.sh"))
			if helper.Code != 200 || (helper.Body.String() == string(b.helper)) != (mode != "bootstrap-helper-corrupt") || b.counts.Helpers != 1 {
				t.Fatal("helper fixture bytes/count")
			}
			redirect := httptest.NewRecorder()
			handler.ServeHTTP(redirect, bootstrapRequest("github.com", prefix+archive))
			location := "https://release-assets.githubusercontent.com/hlo-fixture/" + archive
			if mode == "bootstrap-downgrade" {
				location = strings.Replace(location, "https:", "http:", 1)
			}
			if mode == "bootstrap-loop" {
				location = "https://github.com" + prefix + archive
			}
			if redirect.Code != 302 || redirect.Header().Get("Location") != location || b.counts.Redirects != 1 {
				t.Fatal("redirect fixture identity")
			}
			body := httptest.NewRecorder()
			handler.ServeHTTP(body, bootstrapRequest("release-assets.githubusercontent.com", "/hlo-fixture/"+archive))
			if mode == "bootstrap-valid" || mode == "bootstrap-corrupt" {
				if body.Code != 200 || len(body.Body.Bytes()) != len(b.archive) || (body.Body.String() == string(b.archive)) != (mode == "bootstrap-valid") || b.counts.Archives != 1 {
					t.Fatal("archive fixture identity")
				}
			} else if body.Code != 404 {
				t.Fatal("unexpected final asset authority")
			}
			if enrollments != 0 || b.counts.Enrollments != 0 {
				t.Fatal("download consumed enrollment")
			}
		})
	}
	for _, mutate := range []func(*http.Request){
		func(r *http.Request) { r.TLS = nil },
		func(r *http.Request) { r.TLS.ServerName = "other.example" },
		func(r *http.Request) { r.Host = "other.example" },
		func(r *http.Request) { r.Method = http.MethodPost },
		func(r *http.Request) { r.URL.RawQuery = "q=1" },
		func(r *http.Request) { r.URL.Path = "/other" },
		func(r *http.Request) { r.URL.RawPath = "/encoded%2falias" },
		func(r *http.Request) { r.Header.Set("Authorization", "synthetic") },
		func(r *http.Request) { r.Header.Set("Cookie", "synthetic") },
		func(r *http.Request) { r.Header.Set("Cookie", "") },
		func(r *http.Request) { r.Header.Set("Proxy-Authorization", "synthetic") },
	} {
		b := bootstrapFixture("bootstrap-valid")
		r := bootstrapRequest("github.com", prefix+"install-connected-linux.sh")
		mutate(r)
		w := httptest.NewRecorder()
		b.handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected enrollment") })).ServeHTTP(w, r)
		if w.Code != 404 || b.counts.Refused != 1 || b.counts.Helpers != 0 {
			t.Fatal("download authority widened")
		}
	}
}

func TestBootstrapDNSDoesNotChangeDirectScopes(t *testing.T) {
	for _, name := range []string{"github.com", "release-assets.githubusercontent.com", "other.example"} {
		for _, kind := range []uint16{1, 28, 16} {
			for _, enabled := range []bool{false, true} {
				r := dnsReplyFor(fixtureDNSQuery(name, kind, true), enabled)
				want := uint16(5)
				answers := uint16(0)
				if enabled && name != "other.example" && kind != 16 {
					want = 0
					if kind == 1 {
						answers = 1
					}
				}
				if binary.BigEndian.Uint16(r[2:4])&15 != want || binary.BigEndian.Uint16(r[6:8]) != answers {
					t.Fatal("bootstrap DNS authority")
				}
			}
		}
	}
}

func TestBootstrapHTTPTrapCountsActualAcceptsAndBoundsCounters(t *testing.T) {
	b := bootstrapFixture("bootstrap-downgrade")
	b.counts.Redirects = 128
	if b.count(func(c *bootstrapCounts) { c.Redirects++ }) == nil || b.counts.Redirects != 128 {
		t.Fatal("unbounded fixture counter")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	conn, err := (&bootstrapHTTPListener{Listener: listener, receiver: b}).Accept()
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if b.counts.HTTPAccepts != 1 {
		t.Fatal("HTTP trap did not count actual connection")
	}
}
