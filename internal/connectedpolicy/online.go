package connectedpolicy

import (
	"strings"
)

// OnlineExpectation is the installer-owned identity for the one transient
// enrollment exchange. Network destinations are independently constrained by
// ValidateEffective; this checks the rest of the manager-enforced sandbox.
type OnlineExpectation struct {
	UploaderUID, UploaderGID, SharedGID uint32
	ArtifactSHA256                      string
}

var onlineFields = append(append([]string(nil), offlineFields...), "RestrictAddressFamilies")

func onlineExpected(e OnlineExpectation) (map[string]string, error) {
	want, err := offlineExpected(OfflineExpectation{
		UploaderUID: e.UploaderUID, UploaderGID: e.UploaderGID,
		SharedGID: e.SharedGID, ArtifactSHA256: e.ArtifactSHA256,
		Mode: "validate-enrollment",
	})
	if err != nil {
		return nil, ErrUnsafe
	}
	want["PrivateNetwork"] = "no"
	want["RestrictAddressFamilies"] = "AF_INET AF_INET6"
	return want, nil
}

func validateOnline(data []byte, e OnlineExpectation) error {
	want, err := onlineExpected(e)
	if err != nil || len(data) == 0 || len(data) > maxOutput || strings.ContainsAny(string(data), "\x00\r") {
		return ErrUnsafe
	}
	actual := make(map[string]string, len(onlineFields))
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return ErrUnsafe
		}
		if _, duplicate := actual[key]; duplicate {
			return ErrUnsafe
		}
		if key != "SystemCallFilter" {
			if expected, known := want[key]; !known || value != expected {
				return ErrUnsafe
			}
		}
		actual[key] = value
	}
	if len(actual) != len(onlineFields) {
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
	for _, call := range []string{
		"socketpair", "io_uring_setup", "io_uring_enter", "io_uring_register",
		"shmget", "shmat", "shmctl", "shmdt", "semget", "semctl", "semop",
		"semtimedop", "msgget", "msgctl", "msgsnd", "msgrcv", "mq_open",
		"mq_notify", "mq_unlink", "mq_timedsend", "mq_timedreceive",
		"mq_getsetattr", "ptrace", "process_vm_readv", "process_vm_writev",
		"bpf", "userfaultfd", "mknod", "mknodat",
	} {
		if !denied[call] {
			return ErrUnsafe
		}
	}
	return nil
}
