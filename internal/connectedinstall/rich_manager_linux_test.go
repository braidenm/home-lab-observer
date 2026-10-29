//go:build linux

package connectedinstall

import (
	"context"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

func TestRichTransientRefusesWrongPairBeforeManagerRead(t *testing.T) {
	read := func(context.Context) (map[string]string, error) {
		t.Fatal("invalid invocation read manager")
		return nil, ErrUnsafe
	}
	for _, tc := range []struct {
		pair       workerPair
		executable string
	}{{richProcessesPair, "/bin/observer-connected-uploader"}, {numericPair, "/bin/observer-connected-rich-uploader"}, {workerPair(255), "/bin/observer-connected-rich-uploader"}} {
		if _, err := runEnrollmentForPair(context.Background(), nil, tc.executable, "enroll", nil, connectedunits.EnrollmentInput{}, read, tc.pair); err != ErrUnsafe {
			t.Fatal("cross-profile invocation accepted")
		}
	}
}

func TestRichTransientCleanupNeverStopsNumericOrReplacedInvocation(t *testing.T) {
	_, _, unit, _ := richProcessesPair.units()
	marker, invocation := "synthetic-marker", strings.Repeat("a", 32)
	for _, changed := range []string{"numeric-unit", "other-invocation", "other-marker"} {
		calls := 0
		read := func(context.Context) (map[string]string, error) {
			calls++
			state := map[string]string{"Id": unit, "LoadState": "loaded", "Description": marker, "Transient": "yes", "InvocationID": invocation, "ActiveState": "active"}
			if calls == 2 {
				switch changed {
				case "numeric-unit":
					state["Id"] = enrollmentUnit
				case "other-invocation":
					state["InvocationID"] = strings.Repeat("b", 32)
				case "other-marker":
					state["Description"] = "other"
				}
			}
			return state, nil
		}
		stop := func(context.Context) error { t.Fatal("replacement was stopped"); return nil }
		if stopEnrollmentWithForPair(context.Background(), marker, invocation, read, stop, richProcessesPair) != ErrRecovery {
			t.Fatal("uncertain rich cleanup reported success")
		}
	}
	for _, unit := range []string{"unrelated.service", enrollmentUnit + ".extra", "home-lab-observer-connected-rich-enrollment.service.extra"} {
		if knownInstalledUnit(unit, true) {
			t.Fatal("unknown manager target admitted")
		}
	}
}
