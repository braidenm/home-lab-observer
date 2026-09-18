package connectedpolicy

import (
	"sort"
	"strings"
	"testing"
)

const testedSyscallDenials = "~socket socketpair io_uring_setup io_uring_enter io_uring_register shmget shmat shmctl shmdt semget semctl semop semtimedop msgget msgctl msgsnd msgrcv mq_open mq_notify mq_unlink mq_timedsend mq_timedreceive mq_getsetattr ptrace process_vm_readv process_vm_writev bpf userfaultfd mknod mknodat pipe pipe2"

func offlineSample(t *testing.T, mode string) (OfflineExpectation, map[string]string) {
	t.Helper()
	e := OfflineExpectation{UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103, ArtifactSHA256: strings.Repeat("a", 64), Mode: mode}
	m, err := offlineExpected(e)
	if err != nil {
		t.Fatal(err)
	}
	m["SystemCallFilter"] = testedSyscallDenials
	return e, m
}
func offlineBytes(m map[string]string) []byte {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key + "=" + m[key] + "\n")
	}
	return []byte(b.String())
}
func TestOfflineModesAndActualManagerNormalization(t *testing.T) {
	for _, mode := range []string{"validate-enrollment", "validate-ledger", "validate-existing-ledger"} {
		e, m := offlineSample(t, mode)
		if validateOffline(offlineBytes(m), e) != nil {
			t.Fatal("normalized offline policy refused")
		}
		// RestrictNamespaces=yes, native architectures and :rbind are manager-
		// observed strings on systemd255, not invented template representations.
		if !strings.HasSuffix(m["BindPaths"], ":rbind") {
			t.Fatal("missing manager bind flags")
		}
	}
}
func TestOfflineRejectsDriftAndAdditionalAuthority(t *testing.T) {
	for key, value := range map[string]string{
		"User": "0", "Group": "0", "SupplementaryGroups": "60103 0",
		"PrivateNetwork": "no", "PrivateIPC": "no", "NoNewPrivileges": "no",
		"CapabilityBoundingSet": "cap_sys_admin", "AmbientCapabilities": "cap_net_admin",
		"RootDirectory": "/", "RootImage": "/other.img", "ReadWritePaths": "/state/enrollment /etc",
		"BindPaths":         "/var/lib/home-lab-observer-connected/enrollment:/state/enrollment:rbind /:/host:rbind",
		"BindReadOnlyPaths": "/etc:/etc:rbind", "NoExecPaths": "", "ExecPaths": "/bin/observer-connected-uploader /bin/sh",
		"DevicePolicy": "auto", "MountAPIVFS": "yes", "ProtectSystem": "no",
		"SystemCallArchitectures": "native x86", "SystemCallErrorNumber": "0", "SystemCallFilter": "socket socketpair",
		"LoadState": "not-found", "NeedDaemonReload": "yes", "DropInPaths": "/etc/override.conf", "Transient": "no",
		"LimitCORE": "infinity", "RestrictNamespaces": "no", "RestrictRealtime": "no", "RestrictSUIDSGID": "no", "LockPersonality": "no",
	} {
		t.Run(key, func(t *testing.T) {
			e, m := offlineSample(t, "validate-enrollment")
			m[key] = value
			if validateOffline(offlineBytes(m), e) != ErrUnsafe {
				t.Fatal("drift accepted")
			}
		})
	}
	e, m := offlineSample(t, "validate-ledger")
	for _, call := range strings.Fields(strings.TrimPrefix(testedSyscallDenials, "~")) {
		if call == "pipe" || call == "pipe2" {
			continue
		}
		x := strings.Fields(strings.TrimPrefix(testedSyscallDenials, "~"))
		kept := []string{}
		for _, v := range x {
			if v != call {
				kept = append(kept, v)
			}
		}
		m["SystemCallFilter"] = "~" + strings.Join(kept, " ")
		if validateOffline(offlineBytes(m), e) != ErrUnsafe {
			t.Fatalf("missing denial accepted: %s", call)
		}
	}
}
func TestOfflineRejectsIncompleteAmbiguousInputs(t *testing.T) {
	e, m := offlineSample(t, "validate-enrollment")
	data := offlineBytes(m)
	for _, bad := range [][]byte{nil, append(append([]byte(nil), data...), []byte("User=0\n")...), []byte(strings.Repeat("x", maxOutput+1)), append(append([]byte(nil), data...), []byte("Unknown=1\n")...)} {
		if validateOffline(bad, e) != ErrUnsafe {
			t.Fatal("invalid manager output accepted")
		}
	}
	for _, field := range offlineFields {
		e, m := offlineSample(t, "validate-enrollment")
		delete(m, field)
		if validateOffline(offlineBytes(m), e) != ErrUnsafe {
			t.Fatal("missing manager field accepted")
		}
	}
	for _, mutate := range []func(*OfflineExpectation){func(e *OfflineExpectation) { e.Mode = "enroll" }, func(e *OfflineExpectation) { e.UploaderUID = 0 }, func(e *OfflineExpectation) { e.SharedGID = e.UploaderGID }, func(e *OfflineExpectation) { e.ArtifactSHA256 = "../../other" }} {
		bad := e
		mutate(&bad)
		if validateOffline(data, bad) != ErrUnsafe {
			t.Fatal("invalid expected authority accepted")
		}
	}
}

func TestExistingLedgerRejectsEnrollmentAndAdditionalState(t *testing.T) {
	for _, bind := range []string{"/var/lib/home-lab-observer-connected/enrollment:/state/enrollment:rbind", "/var/lib/home-lab-observer-connected/ledger:/state/ledger:rbind /var/lib/home-lab-observer-connected/enrollment:/state/enrollment:rbind"} {
		e, m := offlineSample(t, "validate-existing-ledger")
		m["BindPaths"] = bind
		if validateOffline(offlineBytes(m), e) != ErrUnsafe {
			t.Fatal("existing validation acquired unrelated state")
		}
	}
}
