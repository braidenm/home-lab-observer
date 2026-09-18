package connectedpolicy

import (
	"strings"
	"testing"
)

func onlineSample(t *testing.T) (OnlineExpectation, map[string]string) {
	t.Helper()
	base, _ := offlineSample(t, "validate-enrollment")
	expected := OnlineExpectation{base.UploaderUID, base.UploaderGID, base.SharedGID, base.ArtifactSHA256}
	want, err := onlineExpected(expected)
	if err != nil {
		t.Fatal(err)
	}
	want["SystemCallFilter"] = strings.Replace(testedSyscallDenials, "~socket ", "~", 1)
	return expected, want
}

func TestOnlineEnrollmentPregrantPolicy(t *testing.T) {
	e, sample := onlineSample(t)
	if validateOnline(offlineBytes(sample), e) != nil {
		t.Fatal("supported online confinement refused")
	}
	for key, value := range map[string]string{
		"User": "0", "Group": "0", "SupplementaryGroups": "60103 0",
		"RootDirectory": "/", "BindPaths": "/:/state/enrollment:rbind",
		"BindReadOnlyPaths": "/etc:/bin/observer-connected-uploader:rbind",
		"ReadWritePaths":    "/state/enrollment /etc", "PrivateNetwork": "yes",
		"PrivateIPC": "no", "NoNewPrivileges": "no", "CapabilityBoundingSet": "cap_sys_admin",
		"AmbientCapabilities": "cap_net_admin", "RestrictAddressFamilies": "AF_UNIX AF_INET AF_INET6",
		"MountAPIVFS": "yes", "ProtectSystem": "no", "DevicePolicy": "auto",
		"SystemCallFilter": "~socket", "DropInPaths": "/etc/override.conf",
	} {
		t.Run(key, func(t *testing.T) {
			changed := make(map[string]string, len(sample))
			for k, v := range sample {
				changed[k] = v
			}
			changed[key] = value
			if validateOnline(offlineBytes(changed), e) != ErrUnsafe {
				t.Fatal("weakened pregrant manager policy admitted")
			}
		})
	}
	for _, key := range onlineFields {
		changed := make(map[string]string, len(sample))
		for k, v := range sample {
			changed[k] = v
		}
		delete(changed, key)
		if validateOnline(offlineBytes(changed), e) != ErrUnsafe {
			t.Fatal("missing pregrant manager field admitted", key)
		}
	}
	if validateOnline(append(offlineBytes(sample), []byte("User=0\n")...), e) != ErrUnsafe {
		t.Fatal("duplicate manager field admitted")
	}
	for _, call := range []string{"socketpair", "shmget", "io_uring_setup", "ptrace", "bpf"} {
		changed := make(map[string]string, len(sample))
		for k, v := range sample {
			changed[k] = v
		}
		kept := make([]string, 0)
		for _, denied := range strings.Fields(strings.TrimPrefix(changed["SystemCallFilter"], "~")) {
			if denied != call {
				kept = append(kept, denied)
			}
		}
		changed["SystemCallFilter"] = "~" + strings.Join(kept, " ")
		if validateOnline(offlineBytes(changed), e) != ErrUnsafe {
			t.Fatal("missing pregrant syscall denial admitted", call)
		}
	}
}
