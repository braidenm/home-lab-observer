package connectedpolicy

import (
	"strings"
	"testing"
)

func TestRichTransientPolicyIsExactAndNotNumeric(t *testing.T) {
	for _, mode := range []string{"validate-enrollment", "validate-ledger"} {
		e, numeric := offlineSample(t, mode)
		rich, err := offlineExpectedForProfile(e, true)
		if err != nil {
			t.Fatal("rich expectation refused")
		}
		rich["SystemCallFilter"] = testedSyscallDenials
		if validateOfflineForProfile(offlineBytes(rich), e, true) != nil {
			t.Fatal("rich manager policy refused")
		}
		if validateOfflineForProfile(offlineBytes(numeric), e, true) != ErrUnsafe || validateOffline(offlineBytes(rich), e) != ErrUnsafe {
			t.Fatal("cross-profile transient accepted")
		}
		for _, field := range []string{"Id", "RootDirectory", "BindPaths", "BindReadOnlyPaths", "ExecPaths"} {
			previous := rich[field]
			rich[field] = numeric[field]
			if validateOfflineForProfile(offlineBytes(rich), e, true) != ErrUnsafe {
				t.Fatal("numeric resource substitution accepted")
			}
			rich[field] = previous
		}
	}
	e := OnlineExpectation{UploaderUID: 61002, UploaderGID: 61012, SharedGID: 61011, ArtifactSHA256: strings.Repeat("a", 64)}
	rich, err := onlineExpectedForProfile(e, true)
	if err != nil {
		t.Fatal("online rich expectation refused")
	}
	rich["SystemCallFilter"] = strings.Replace(testedSyscallDenials, "~socket ", "~", 1)
	if validateOnlineForProfile(offlineBytes(rich), e, true) != nil || validateOnline(offlineBytes(rich), e) != ErrUnsafe {
		t.Fatal("online rich confinement mismatch")
	}
	if _, err := offlineExpectedForProfile(OfflineExpectation{UploaderUID: 61002, UploaderGID: 61012, SharedGID: 61011, ArtifactSHA256: e.ArtifactSHA256, Mode: "validate-existing-ledger"}, true); err != ErrUnsafe {
		t.Fatal("unsupported rich witness mode accepted")
	}
}
