package connectedpolicy

import (
	"strings"
	"testing"
)

func TestDockerEnrollmentManagerBoundaryRejectsOldAndExtraAuthority(t *testing.T) {
	if !ownedUnit(dockerEnrollmentUnit) || !ownedUnit("home-lab-observer-connected-rich-docker-uploader.service") || ownedUnit("home-lab-observer-connected-rich-docker-reader.service") || ownedUnit(dockerEnrollmentUnit+".extra") {
		t.Fatal("Docker egress unit allowlist widened")
	}
	for _, mode := range []string{"validate-enrollment", "validate-ledger"} {
		e, _ := offlineSample(t, mode)
		want, err := dockerOfflineExpected(e)
		if err != nil {
			t.Fatal(err)
		}
		want["SystemCallFilter"] = testedSyscallDenials
		if validateOfflineAgainst(offlineBytes(want), want) != nil || validateOfflineForProfile(offlineBytes(want), e, true) != ErrUnsafe {
			t.Fatal("Docker offline identity boundary")
		}
		old, _ := offlineExpectedForProfile(e, true)
		old["SystemCallFilter"] = testedSyscallDenials
		if validateOfflineAgainst(offlineBytes(old), want) != ErrUnsafe {
			t.Fatal("process-only executable admitted")
		}
		for _, field := range []string{"SupplementaryGroups", "BindPaths", "BindReadOnlyPaths", "ExecPaths"} {
			copy := map[string]string{}
			for k, v := range want {
				copy[k] = v
			}
			copy[field] += " /engine/docker.sock"
			if validateOfflineAgainst(offlineBytes(copy), want) != ErrUnsafe {
				t.Fatal("extra enrollment authority admitted")
			}
		}
	}
	e := OnlineExpectation{UploaderUID: 62002, UploaderGID: 62003, SharedGID: 62004, ArtifactSHA256: strings.Repeat("a", 64)}
	want, err := dockerOnlineExpected(e)
	if err != nil {
		t.Fatal(err)
	}
	want["SystemCallFilter"] = strings.Replace(testedSyscallDenials, "~socket ", "~", 1)
	if validateOnlineAgainst(offlineBytes(want), want) != nil || validateOnlineForProfile(offlineBytes(want), e, true) != ErrUnsafe {
		t.Fatal("Docker online identity boundary")
	}
}
