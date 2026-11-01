//go:build linux

package actionengine

import (
	"context"
	"encoding/json"
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
)

func TestInspectSelectedStateAndMalformedResponses(t *testing.T) {
	for _, state := range []actioncore.State{actioncore.Created, actioncore.Exited, actioncore.Running, actioncore.Paused, actioncore.Restarting, actioncore.Removing, actioncore.Dead, actioncore.UnknownState} {
		body := fmt.Sprintf(`{"Id":%q,"id":"wrong-case-must-not-override","State":{"Status":%q,"status":"wrong-case-must-not-override"},"Config":{"Env":["secret-canary"]}}`, fullID, state)
		actual, err := inspectState([]byte(body), fullID)
		if err != nil || actual != state {
			t.Fatal("selected state/case handling")
		}
	}
	for _, body := range []string{
		fmt.Sprintf(`{"Id":%q,"State":{"Status":"running"}}`, strings.Repeat("b", 64)),
		fmt.Sprintf(`{"id":%q,"State":{"Status":"running"}}`, fullID),
		fmt.Sprintf(`{"Id":%q,"State":{"status":"running"}}`, fullID),
		fmt.Sprintf(`{"Id":%q,"Id":%q,"State":{"Status":"running"}}`, fullID, fullID),
		fmt.Sprintf(`{"Id":%q,"State":{"Status":"running","Status":"exited"}}`, fullID),
		fmt.Sprintf(`{"Id":%q,"State":{},"State":{"Status":"running"}}`, fullID),
		fmt.Sprintf(`{"Id":%q,"State":{"Status":true}}`, fullID),
		fmt.Sprintf(`{"Id":%q,"State":null}`, fullID),
		fmt.Sprintf(`{"Id":%q,"State":{"Status":"running"}} {}`, fullID),
	} {
		if _, err := inspectState([]byte(body), fullID); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "secret") {
			t.Fatal("malformed selected state accepted/raw error exposed")
		}
	}
	for _, status := range []int{200, 404, 500} {
		e := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			fmt.Fprint(w, strings.Repeat("x", inspectLimit+1))
		})
		a := openFixture(t, e)
		state, err := a.Inspect(context.Background(), fullID)
		if state != actioncore.UnknownState || (status == 404 && err != nil) || (status != 404 && err == nil) {
			t.Fatal("inspect body bound/status handling")
		}
	}
}

func TestSelectedJSONRejectsInvalidUTF8ButKeepsUnicodeExtras(t *testing.T) {
	valid := []byte(fmt.Sprintf(`{"Id":%q,"State":{"Status":"running"},"extra":"診断🟢"}`, fullID))
	if state, err := inspectState(valid, fullID); err != nil || state != actioncore.Running {
		t.Fatal("valid Unicode extra refused")
	}
	invalid := append([]byte(fmt.Sprintf(`{"Id":%q,"State":{"Status":"running"},"extra":"`, fullID)), 0xff)
	invalid = append(invalid, []byte(`"}`)...)
	var permissive map[string]json.RawMessage
	if json.Unmarshal(invalid, &permissive) != nil {
		t.Fatal("standard decoder replacement behavior not reproduced")
	}
	if _, err := inspectState(invalid, fullID); err != ErrUnavailable {
		t.Fatal("invalid UTF8 silently replaced")
	}
	invalidVersion := append([]byte(`{"ApiVersion":"1.56","MinAPIVersion":"1.40","Os":"linux","extra":"`), 0xff)
	invalidVersion = append(invalidVersion, []byte(`"}`)...)
	if supportedVersion(invalidVersion) != ErrUnavailable {
		t.Fatal("invalid version UTF8 silently replaced")
	}
}

func TestMutationFaultsAreFiniteAndNeverRetried(t *testing.T) {
	for _, kind := range []string{"lost-response", "redirect", "304", "404", "500", "oversized-header", "encoding"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			e := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch kind {
				case "lost-response":
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						conn.Close()
					}
					return
				case "redirect":
					w.Header().Set("Location", "/must-not-follow")
					w.WriteHeader(307)
				case "oversized-header":
					w.Header().Set("X-Oversized", strings.Repeat("s", 20*1024))
					w.WriteHeader(204)
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
					w.WriteHeader(204)
				case "304":
					w.WriteHeader(304)
				case "404":
					w.WriteHeader(404)
				default:
					w.WriteHeader(500)
				}
				fmt.Fprint(w, `{"message":"secret-canary"}`)
			})
			a := openFixture(t, e)
			if err := a.Restart(context.Background(), fullID, 30); err != ErrUnavailable || calls.Load() != 1 {
				t.Fatal("mutation error leaked/retried/acknowledged")
			}
		})
	}
}

func TestSocketDriftRefusesBeforeNextRequest(t *testing.T) {
	for _, kind := range []string{"mode", "replacement", "symlink", "witness", "parent"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			e := unixFixture(t, versionJSON, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
			a := openFixture(t, e)
			switch kind {
			case "mode":
				os.Chmod(e.path, 0666)
			case "replacement", "symlink":
				old := filepath.Join(filepath.Dir(e.path), "old.sock")
				if os.Rename(e.path, old) != nil {
					t.Fatal("fixture replacement")
				}
				if kind == "symlink" {
					if os.Symlink(old, e.path) != nil {
						t.Fatal("fixture symlink")
					}
				} else {
					l, err := net.Listen("unix", e.path)
					if err != nil {
						t.Fatal("replacement listener")
					}
					defer l.Close()
					os.Chmod(e.path, 0660)
				}
			case "parent":
				os.Chmod(filepath.Dir(e.path), 0770)
			case "witness":
				a.endpoint.approved.Inode++
			}
			if a.Start(context.Background(), fullID) != ErrUnsafe || calls.Load() != 0 {
				t.Fatal("socket drift adopted")
			}
		})
	}
}

func TestCancellationAndCloseJoinActualRequest(t *testing.T) {
	for _, closeAdapter := range []bool{false, true} {
		var calls atomic.Int32
		entered := make(chan struct{})
		e := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			close(entered)
			<-r.Context().Done()
		})
		a := openFixture(t, e)
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { result <- a.Stop(ctx, fullID, 1) }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("request did not enter fixture")
		}
		if closeAdapter {
			a.Close()
		} else {
			cancel()
		}
		select {
		case err := <-result:
			if err != ErrUnavailable || calls.Load() != 1 {
				t.Fatal("cancelled operation acknowledged/retried")
			}
		case <-time.After(time.Second):
			t.Fatal("cancelled request did not join")
		}
		cancel()
	}
	var zero Engine
	if zero.Start(context.Background(), fullID) != ErrUnavailable || zero.Close() != nil {
		t.Fatal("zero adapter gained authority")
	}
}

func TestCallerCancellationBoundsInspectBodyRead(t *testing.T) {
	var calls atomic.Int32
	flushed := make(chan struct{})
	e := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"Id":"`)
		w.(http.Flusher).Flush()
		close(flushed)
		<-r.Context().Done()
	})
	a := openFixture(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		state, err := a.Inspect(ctx, fullID)
		if state != actioncore.UnknownState {
			result <- ErrInvalid
			return
		}
		result <- err
	}()
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("body fixture did not flush")
	}
	cancel()
	select {
	case err := <-result:
		if err != ErrUnavailable || calls.Load() != 1 {
			t.Fatal("cancelled body read acknowledged/retried")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled body read did not join")
	}
	if _, err := a.Inspect(ctx, fullID); err != ErrUnavailable || calls.Load() != 1 {
		t.Fatal("pre-entry cancellation performed I/O")
	}
}
