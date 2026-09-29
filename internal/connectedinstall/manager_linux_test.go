//go:build linux

package connectedinstall

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

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

func TestInstalledUnitsRemainStoppedAndDisabled(t *testing.T) {
	for _, unit := range []string{collectorUnit, uploaderUnit} {
		data := []byte("Id=" + unit + "\nLoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\nFragmentPath=/etc/systemd/system/" + unit + "\nDropInPaths=\nNeedDaemonReload=no\n")
		state, err := parseUnitRuntimeState(data, unit)
		if err != nil || !stoppedDisabledUnit(state, unit) {
			t.Fatal("stopped installed unit refused", err)
		}
		for _, bad := range []string{"active", "enabled", "masked", "transient", "other-path", "drop-in", "reload-pending"} {
			changed := map[string]string{}
			for key, value := range state {
				changed[key] = value
			}
			switch bad {
			case "active":
				changed["ActiveState"] = "active"
			case "enabled":
				changed["UnitFileState"] = "enabled"
			case "masked":
				changed["UnitFileState"] = "masked"
			case "transient":
				changed["LoadState"] = "not-found"
			case "other-path":
				changed["FragmentPath"] = "/run/systemd/system/" + unit
			case "drop-in":
				changed["DropInPaths"] = "/etc/systemd/system/foreign.conf"
			case "reload-pending":
				changed["NeedDaemonReload"] = "yes"
			}
			if stoppedDisabledUnit(changed, unit) {
				t.Fatal("unapproved service state admitted", bad)
			}
		}
		for _, bad := range [][]byte{append(append([]byte(nil), data...), []byte("LoadState=loaded\n")...), []byte("Id=" + unit + "\nLoadState=loaded\nUnexpected=1\n"), []byte("Id=foreign\nLoadState=loaded\n")} {
			if _, err := parseUnitRuntimeState(bad, unit); err != ErrUnsafe {
				t.Fatal("ambiguous service state admitted")
			}
		}
	}
	for _, verb := range []string{"start", "enable", "restart", "disable"} {
		if service(context.Background(), verb, collectorUnit) != ErrUnsafe {
			t.Fatal("installed worker mutation exposed", verb)
		}
	}
}

func TestFirstInstallEnrollmentModesAreClosed(t *testing.T) {
	called := false
	_, err := runEnrollmentWithPreflight(context.Background(), nil, "/bin/observer-connected-uploader", "validate-existing-ledger", nil, connectedunits.EnrollmentInput{}, func(context.Context) (map[string]string, error) {
		called = true
		return nil, nil
	})
	if err != ErrUnsafe || called {
		t.Fatal("future refresh mode reached first-install manager")
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

func TestEnrollmentAwaitRequiresActualOwnedInvocation(t *testing.T) {
	for _, scenario := range []string{"ready", "foreign-marker", "foreign-id", "not-transient", "malformed-id", "failed", "deactivating", "active-empty", "replaced", "cleared", "canceled", "cancel-during-read", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			deadline := 2 * time.Second
			if scenario == "timeout" {
				deadline = 50 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), deadline)
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			reads := 0
			id, err := awaitEnrollmentWith(ctx, "fixture", func(context.Context) (map[string]string, error) {
				reads++
				state := map[string]string{"Id": enrollmentUnit, "Description": "fixture", "Transient": "yes", "LoadState": "loaded", "ActiveState": "inactive", "InvocationID": ""}
				switch scenario {
				case "cancel-during-read":
					cancel()
					state["ActiveState"] = "active"
					state["InvocationID"] = strings.Repeat("b", 32)
				case "ready":
					if reads > 1 {
						state["ActiveState"] = "active"
						state["InvocationID"] = strings.Repeat("b", 32)
					}
				case "foreign-marker":
					state["Description"] = "other"
				case "foreign-id":
					state["Id"] = "other.service"
				case "not-transient":
					state["Transient"] = "no"
				case "malformed-id":
					state["InvocationID"] = "bad"
				case "failed":
					state["ActiveState"] = "failed"
				case "deactivating":
					state["ActiveState"] = "deactivating"
				case "active-empty":
					state["ActiveState"] = "active"
				case "replaced", "cleared":
					state["InvocationID"] = strings.Repeat("b", 32)
					if reads > 1 {
						state["ActiveState"] = "active"
						state["InvocationID"] = ""
						if scenario == "replaced" {
							state["InvocationID"] = strings.Repeat("c", 32)
						}
					}
				}
				return state, nil
			})
			if scenario == "ready" {
				if err != nil || id != strings.Repeat("b", 32) || reads != 2 {
					t.Fatal("owned pending invocation not awaited")
				}
			} else if err != ErrUnsafe || id != "" {
				t.Fatal("unproven invocation admitted")
			}
			if scenario == "canceled" && reads != 0 {
				t.Fatal("canceled wait queried manager")
			}
		})
	}
}
