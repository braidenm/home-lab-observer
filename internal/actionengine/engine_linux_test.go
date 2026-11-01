//go:build linux

package actionengine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"golang.org/x/sys/unix"
)

const fullID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const versionJSON = `{"ApiVersion":"1.56","MinAPIVersion":"1.40","Os":"linux"}`

func unixFixture(t *testing.T, version string, handle http.HandlerFunc) socketEndpoint {
	t.Helper()
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("fixture directory")
	}
	path := filepath.Join(dir, "engine.sock")
	l, err := net.Listen("unix", path)
	if err != nil || os.Chmod(path, 0660) != nil {
		t.Fatal("fixture socket")
	}
	s := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, version)
			return
		}
		handle(w, r)
	})}
	go s.Serve(l)
	t.Cleanup(func() { s.Close() })
	var st unix.Stat_t
	if unix.Lstat(path, &st) != nil {
		t.Fatal("fixture witness")
	}
	return socketEndpoint{path: path, owner: uint32(os.Getuid()), approved: SocketWitness{Device: uint64(st.Dev), Inode: st.Ino, GroupID: st.Gid}}
}
func openFixture(t *testing.T, e socketEndpoint) *Engine {
	t.Helper()
	a, err := openEndpoint(context.Background(), e)
	if err != nil {
		t.Fatal("fixture open")
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func TestFixedRoutesAndNoEnvironmentDiscovery(t *testing.T) {
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_API_VERSION"} {
		t.Setenv(name, "https://invalid.example/secret-canary")
	}
	var calls atomic.Int32
	e := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Host != "docker" || r.ContentLength != 0 || r.Header.Get("Accept-Encoding") != "" || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" {
			t.Error("unexpected request authority/body/headers")
		}
		base := "/v1.45/containers/" + fullID
		if r.Method == "GET" && r.URL.Path == base+"/json" && r.URL.RawQuery == "" {
			fmt.Fprintf(w, `{"Id":%q,"State":{"Status":"running"},"Config":{"Env":["secret-canary"]}}`, fullID)
			return
		}
		if r.Method != "POST" || !((r.URL.Path == base+"/start" && r.URL.RawQuery == "") || (r.URL.Path == base+"/stop" && r.URL.RawQuery == "t=1") || (r.URL.Path == base+"/restart" && r.URL.RawQuery == "t=30")) {
			t.Error("nonfixed mutation")
		}
		w.WriteHeader(204)
	})
	a := openFixture(t, e)
	if state, err := a.Inspect(context.Background(), fullID); err != nil || state != actioncore.Running {
		t.Fatal("selected inspect")
	}
	if a.Start(context.Background(), fullID) != nil || a.Stop(context.Background(), fullID, 1) != nil || a.Restart(context.Background(), fullID, 30) != nil || calls.Load() != 4 {
		t.Fatal("fixed operations")
	}
	for _, id := range []string{"", "name", fullID[:12], strings.ToUpper(fullID), fullID + "/start", fullID + "?t=0", strings.Repeat("g", 64)} {
		if a.Start(context.Background(), id) == nil {
			t.Fatal("invalid ID accepted")
		}
	}
	for _, grace := range []uint8{0, 31, 255} {
		if a.Stop(context.Background(), fullID, grace) == nil || a.Restart(context.Background(), fullID, grace) == nil {
			t.Fatal("invalid grace accepted")
		}
	}
	if calls.Load() != 4 {
		t.Fatal("invalid parameters performed I/O")
	}
}

func TestVersionIntersectionAndExactFields(t *testing.T) {
	for _, body := range []string{
		`{"ApiVersion":"1.52","MinAPIVersion":"1.44","Os":"linux"}`,
		versionJSON,
		`{"ApiVersion":"1.45","MinAPIVersion":"1.45","Os":"linux","future":true}`,
	} {
		e := unixFixture(t, body, func(http.ResponseWriter, *http.Request) { t.Error("unexpected call") })
		a := openFixture(t, e)
		a.Close()
	}
	for _, body := range []string{
		`{"ApiVersion":"1.56","MinAPIVersion":"1.46","Os":"linux"}`,
		`{"ApiVersion":"1.44","MinAPIVersion":"1.40","Os":"linux"}`,
		`{"ApiVersion":"1.44","MinAPIVersion":"1.52","Os":"linux"}`,
		`{"ApiVersion":"01.56","MinAPIVersion":"1.40","Os":"linux"}`,
		`{"ApiVersion":"1.56","MinAPIVersion":"1.40","Os":"windows"}`,
		`{"ApiVersion":"1.56","ApiVersion":"1.45","MinAPIVersion":"1.40","Os":"linux"}`,
		`{"apiversion":"1.56","MinAPIVersion":"1.40","Os":"linux"}`,
		versionJSON + `{}`,
		strings.Repeat(" ", versionLimit+1),
	} {
		e := unixFixture(t, body, func(http.ResponseWriter, *http.Request) { t.Error("unexpected call") })
		if a, err := openEndpoint(context.Background(), e); err == nil || a != nil {
			t.Fatal("invalid version admitted")
		}
	}
}

func TestPublicOpenNeverAdoptsAndCloseRejectsCalls(t *testing.T) {
	if a, err := Open(context.Background(), SocketWitness{}); !errors.Is(err, ErrUnsafe) || a != nil {
		t.Fatal("unapproved public socket")
	}
	e := unixFixture(t, versionJSON, func(http.ResponseWriter, *http.Request) { t.Error("closed adapter performed I/O") })
	a := openFixture(t, e)
	if a.Close() != nil || a.Close() != nil || a.Start(context.Background(), fullID) == nil {
		t.Fatal("close")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if a.Stop(ctx, fullID, 1) == nil {
		t.Fatal("closed stop")
	}
}
