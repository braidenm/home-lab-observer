//go:build linux

package connectedinstall

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCommandJoinsBoundedlyWhenDescendantHoldsOutput(t *testing.T) {
	// Only synthetic child test processes are launched. The descendant owns no
	// listener or files and exits on its own after the inherited pipe is closed.
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "--hold-output" {
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "--spawn-holder" {
		c := exec.Command(os.Args[0], "-test.run=^TestCommandJoinsBoundedlyWhenDescendantHoldsOutput$", "--", "--hold-output")
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if c.Start() != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	_, err := command(ctx, os.Args[0], []string{"-test.run=^TestCommandJoinsBoundedlyWhenDescendantHoldsOutput$", "--", "--spawn-holder"}, nil, 128)
	if err != ErrRecovery || time.Since(start) > 3*time.Second {
		t.Fatal("held output escaped bounded join")
	}
}

func TestEnrollmentCleanupRequiresExactOwnedInvocation(t *testing.T) {
	marker := "observer-connected-enrollment-" + strings.Repeat("a", 32)
	data := []byte("Id=" + enrollmentUnit + "\nLoadState=loaded\nDescription=" + marker + "\nInvocationID=" + strings.Repeat("b", 32) + "\nTransient=yes\nActiveState=failed\n")
	state, err := parseEnrollmentState(data)
	if err != nil || !ownedEnrollment(state, marker) {
		t.Fatal("owned state refused")
	}
	for _, field := range []string{"Id", "Description", "InvocationID", "Transient"} {
		copy := map[string]string{}
		for k, v := range state {
			copy[k] = v
		}
		copy[field] = "foreign"
		if ownedEnrollment(copy, marker) {
			t.Fatal("foreign unit authorized for cleanup")
		}
	}
	for _, bad := range [][]byte{nil, append(append([]byte(nil), data...), []byte("Transient=yes\n")...), []byte(strings.Repeat("x", 2049))} {
		if _, err := parseEnrollmentState(bad); err != ErrUnsafe {
			t.Fatal("ambiguous manager state accepted")
		}
	}
}

func TestStrictBaselineAndPreAttemptFailure(t *testing.T) {
	if !ubuntu2404([]byte("ID=ubuntu\nVERSION_ID=\"24.04\"\n")) {
		t.Fatal("supported baseline refused")
	}
	for _, bad := range []string{"ID=debian\nVERSION_ID=\"24.04\"\n", "ID=ubuntu\nVERSION_ID=\"22.04\"\n", "ID=ubuntu\nID=ubuntu\nVERSION_ID=24.04\n"} {
		if ubuntu2404([]byte(bad)) {
			t.Fatal("wrong baseline accepted")
		}
	}
	err := Install(context.Background(), Request{})
	phase, ok := err.(*PhaseError)
	if !ok || phase.Phase != "PREFLIGHT_REFUSED" {
		t.Fatal("invalid request claimed consumed enrollment")
	}
}

func TestEnrollmentCollectionBetweenOwnershipReads(t *testing.T) {
	marker, invocation := "owned-fixture", strings.Repeat("b", 32)
	owned := map[string]string{"Id": enrollmentUnit, "LoadState": "loaded", "Description": marker, "InvocationID": invocation, "Transient": "yes", "ActiveState": "active"}
	for _, scenario := range []string{"collected", "foreign", "read-error", "owned-stop"} {
		t.Run(scenario, func(t *testing.T) {
			reads, stops := 0, 0
			read := func(context.Context) (map[string]string, error) {
				reads++
				if reads == 1 {
					return owned, nil
				}
				if scenario == "read-error" {
					return nil, ErrUnsafe
				}
				if scenario == "foreign" {
					return map[string]string{"LoadState": "loaded", "Id": enrollmentUnit, "Description": marker, "InvocationID": strings.Repeat("c", 32), "Transient": "yes"}, nil
				}
				if scenario == "owned-stop" && reads == 2 {
					return owned, nil
				}
				return map[string]string{"LoadState": "not-found"}, nil
			}
			err := stopEnrollmentWith(context.Background(), marker, invocation, read, func(context.Context) error { stops++; return nil })
			if scenario == "foreign" || scenario == "read-error" {
				if err != ErrRecovery || stops != 0 {
					t.Fatal("uncertain/replaced unit was stopped")
				}
			} else {
				wantStops := 0
				if scenario == "owned-stop" {
					wantStops = 1
				}
				if err != nil || stops != wantStops {
					t.Fatal("owned collection sequence refused")
				}
			}
		})
	}
}
