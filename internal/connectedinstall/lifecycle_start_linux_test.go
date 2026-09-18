//go:build linux

package connectedinstall

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

type startLeaseFixture struct{ close func() error }

func (l startLeaseFixture) Close() error { return l.close() }

func TestStartLeaseAndRefusalOrder(t *testing.T) {
	steps := []string{"lease", "markers", "inspect", "stop", "preflight", "audit", "activate", "close"}
	for _, failure := range append([]string{""}, steps...) {
		t.Run("failure-"+failure, func(t *testing.T) {
			calls := []string{}
			call := func(name string) error {
				calls = append(calls, name)
				if name == failure {
					return ErrUnsafe
				}
				return nil
			}
			binding := principalFixture()
			ops := startOperations{
				lease: func() (io.Closer, error) {
					if err := call("lease"); err != nil {
						return nil, err
					}
					return startLeaseFixture{func() error { return call("close") }}, nil
				},
				preflight: func(context.Context) error { return call("preflight") },
				markers:   func() error { return call("markers") },
				inspect:   func() (connectedprofile.Config, error) { return binding, call("inspect") },
				audit: func(_ context.Context, c connectedprofile.Config) error {
					if !reflect.DeepEqual(c, binding) {
						t.Fatal("binding changed")
					}
					return call("audit")
				},
				stop: func(context.Context) error { return call("stop") },
				activate: func(_ context.Context, c connectedprofile.Config) error {
					if !reflect.DeepEqual(c, binding) {
						t.Fatal("binding changed")
					}
					return call("activate")
				},
			}
			err := startInstalledWith(context.Background(), ops)
			if (failure == "") != (err == nil) {
				t.Fatal("wrong result", err)
			}
			want := steps
			if failure != "" && failure != "close" {
				for i, name := range steps {
					if name == failure {
						want = append([]string{}, steps[:i+1]...)
						if failure != "lease" {
							want = append(want, "close")
						}
						break
					}
				}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls %v want %v", calls, want)
			}
		})
	}
}

func TestStartCancellationNeverActivates(t *testing.T) {
	for _, at := range []string{"before", "audit", "stop"} {
		t.Run(at, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if at == "before" {
				cancel()
			}
			closed := false
			stopped := false
			ops := startOperations{
				lease: func() (io.Closer, error) {
					if at == "before" {
						t.Fatal("lease on canceled input")
					}
					return startLeaseFixture{func() error { closed = true; return nil }}, nil
				},
				preflight: func(context.Context) error { return nil }, markers: func() error { return nil },
				inspect: func() (connectedprofile.Config, error) { return principalFixture(), nil },
				audit: func(context.Context, connectedprofile.Config) error {
					if at == "audit" {
						cancel()
					}
					return nil
				},
				stop: func(context.Context) error {
					stopped = true
					if at == "stop" {
						cancel()
					}
					return nil
				},
				activate: func(context.Context, connectedprofile.Config) error {
					t.Fatal("activated canceled operation")
					return nil
				},
			}
			if startInstalledWith(ctx, ops) != ErrUnsafe {
				t.Fatal("cancellation not refused")
			}
			if closed != (at != "before") || stopped != (at != "before") {
				t.Fatal("wrong shutdown sequence")
			}
		})
	}
	if startInstalledWith(nil, startOperations{}) != ErrUnsafe {
		t.Fatal("nil context accepted")
	}
}
