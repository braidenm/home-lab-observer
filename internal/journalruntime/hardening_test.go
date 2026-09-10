package journalruntime

import (
	"errors"
	"reflect"
	"testing"
)

func TestEstablishPolicy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failAt     string
		soft, hard uint64
		dumpable   int
		wantCalls  []string
	}{
		{name: "verified", wantCalls: []string{"set-core", "set-dumpable", "get-core", "get-dumpable"}},
		{name: "set-core-fails", failAt: "set-core", wantCalls: []string{"set-core"}},
		{name: "set-dumpable-fails", failAt: "set-dumpable", wantCalls: []string{"set-core", "set-dumpable"}},
		{name: "get-core-fails", failAt: "get-core", wantCalls: []string{"set-core", "set-dumpable", "get-core"}},
		{name: "soft-not-zero", soft: 1, wantCalls: []string{"set-core", "set-dumpable", "get-core"}},
		{name: "hard-not-zero", hard: ^uint64(0), wantCalls: []string{"set-core", "set-dumpable", "get-core"}},
		{name: "get-dumpable-fails", failAt: "get-dumpable", wantCalls: []string{"set-core", "set-dumpable", "get-core", "get-dumpable"}},
		{name: "dumpable-user", dumpable: 1, wantCalls: []string{"set-core", "set-dumpable", "get-core", "get-dumpable"}},
		{name: "dumpable-root", dumpable: 2, wantCalls: []string{"set-core", "set-dumpable", "get-core", "get-dumpable"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			called := func(name string) error {
				calls = append(calls, name)
				if tc.failAt == name {
					return errors.New("PRIVATE_NATIVE_ERROR_CANARY")
				}
				return nil
			}
			err := establishPolicy(policyCalls{
				setCoreLimit: func(soft, hard uint64) error {
					if soft != 0 || hard != 0 {
						t.Fatal("core policy must lower both limits to zero")
					}
					return called("set-core")
				},
				setDumpable: func(value int) error {
					if value != 0 {
						t.Fatal("dumpability must be disabled")
					}
					return called("set-dumpable")
				},
				coreLimit: func() (uint64, uint64, error) { return tc.soft, tc.hard, called("get-core") },
				dumpable:  func() (int, error) { return tc.dumpable, called("get-dumpable") },
			})
			if tc.name == "verified" {
				if err != nil {
					t.Fatal("verified policy rejected")
				}
			} else if err != ErrUnavailable || err.Error() != "LOG_HELPER_UNAVAILABLE" {
				t.Fatal("policy failure must return only the fixed unavailable error")
			}
			if !reflect.DeepEqual(calls, tc.wantCalls) {
				t.Fatalf("unexpected policy call order: %v", calls)
			}
		})
	}
}
