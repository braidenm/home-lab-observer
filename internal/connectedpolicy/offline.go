package connectedpolicy

import (
	"fmt"
	"regexp"
	"strings"
)

// OfflineExpectation is installer-owned identity, not data from the worker pipe.
// The two modes intentionally mount different single private state directories.
type OfflineExpectation struct {
	UploaderUID, UploaderGID, SharedGID uint32
	ArtifactSHA256                      string
	Mode                                string
}

var artifactDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

const enrollmentUnit = "home-lab-observer-connected-enrollment.service"

var offlineFields = []string{
	"Id", "LoadState", "NeedDaemonReload", "DropInPaths", "Transient",
	"User", "Group", "SupplementaryGroups", "RootDirectory", "RootImage",
	"BindPaths", "BindReadOnlyPaths", "ReadWritePaths", "PrivateNetwork", "PrivateIPC",
	"NoNewPrivileges", "CapabilityBoundingSet", "AmbientCapabilities", "MountAPIVFS",
	"ProtectSystem", "DevicePolicy", "SystemCallArchitectures", "SystemCallFilter",
	"SystemCallErrorNumber", "NoExecPaths", "ExecPaths", "LimitCORE",
	"RestrictNamespaces", "RestrictRealtime", "RestrictSUIDSGID", "LockPersonality",
}

func offlineExpected(e OfflineExpectation) (map[string]string, error) {
	for _, id := range []uint32{e.UploaderUID, e.UploaderGID, e.SharedGID} {
		if id == 0 || id == ^uint32(0) {
			return nil, ErrUnsafe
		}
	}
	if e.UploaderGID == e.SharedGID || !artifactDigest.MatchString(e.ArtifactSHA256) {
		return nil, ErrUnsafe
	}
	state := "enrollment"
	if e.Mode == "validate-ledger" {
		state = "ledger"
	} else if e.Mode != "validate-enrollment" {
		return nil, ErrUnsafe
	}
	return map[string]string{
		"Id": enrollmentUnit, "LoadState": "loaded", "NeedDaemonReload": "no", "DropInPaths": "", "Transient": "yes",
		"User": fmt.Sprint(e.UploaderUID), "Group": fmt.Sprint(e.UploaderGID), "SupplementaryGroups": fmt.Sprint(e.SharedGID),
		"RootDirectory": "/var/lib/home-lab-observer-connected/uploader-root", "RootImage": "",
		"BindPaths":         "/var/lib/home-lab-observer-connected/" + state + ":/state/" + state + ":rbind",
		"BindReadOnlyPaths": "/opt/home-lab-observer-connected/releases/" + e.ArtifactSHA256 + "/observer-connected-uploader:/bin/observer-connected-uploader:rbind",
		"ReadWritePaths":    "/state/" + state, "PrivateNetwork": "yes", "PrivateIPC": "yes", "NoNewPrivileges": "yes",
		"CapabilityBoundingSet": "", "AmbientCapabilities": "", "MountAPIVFS": "no", "ProtectSystem": "strict", "DevicePolicy": "closed",
		"SystemCallArchitectures": "native", "SystemCallErrorNumber": "1", "NoExecPaths": "/", "ExecPaths": "/bin/observer-connected-uploader",
		"LimitCORE": "0", "RestrictNamespaces": "yes", "RestrictRealtime": "yes", "RestrictSUIDSGID": "yes", "LockPersonality": "yes",
	}, nil
}

// validateOffline checks normalized manager properties, not template text. The
// manager expands syscall groups, so require the individual native denials that
// establish network/IPC isolation; additional denied calls are safe to accept.
func validateOffline(data []byte, e OfflineExpectation) error {
	expected, err := offlineExpected(e)
	if err != nil || len(data) == 0 || len(data) > maxOutput || strings.ContainsAny(string(data), "\x00\r") {
		return ErrUnsafe
	}
	actual := make(map[string]string, len(offlineFields))
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return ErrUnsafe
		}
		if _, ok := actual[key]; ok {
			return ErrUnsafe
		}
		if key != "SystemCallFilter" {
			if want, ok := expected[key]; !ok || want != value {
				return ErrUnsafe
			}
		}
		actual[key] = value
	}
	if len(actual) != len(offlineFields) {
		return ErrUnsafe
	}
	filter := actual["SystemCallFilter"]
	if !strings.HasPrefix(filter, "~") {
		return ErrUnsafe
	}
	denied := make(map[string]bool)
	for _, call := range strings.Fields(strings.TrimPrefix(filter, "~")) {
		denied[call] = true
	}
	for _, call := range []string{"socket", "socketpair", "io_uring_setup", "io_uring_enter", "io_uring_register", "shmget", "shmat", "shmctl", "shmdt", "semget", "semctl", "semop", "semtimedop", "msgget", "msgctl", "msgsnd", "msgrcv", "mq_open", "mq_notify", "mq_unlink", "mq_timedsend", "mq_timedreceive", "mq_getsetattr", "ptrace", "process_vm_readv", "process_vm_writev", "bpf", "userfaultfd", "mknod", "mknodat"} {
		if !denied[call] {
			return ErrUnsafe
		}
	}
	return nil
}
