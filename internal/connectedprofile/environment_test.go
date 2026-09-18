package connectedprofile

import "testing"

func TestMemoryPressureEnvironmentRemainsForbidden(t *testing.T) {
	if CheckEnvironment([]string{"GODEBUG=netdns=go"}) != nil {
		t.Fatal("minimal environment refused")
	}
	for _, entry := range []string{"MEMORY_PRESSURE_WATCH=/dev/null", "MEMORY_PRESSURE_WATCH=/sys/fs/cgroup/fixture/memory.pressure", "MEMORY_PRESSURE_WRITE=synthetic", "MEMORY_PRESSURE_WATCH=", "MEMORY_PRESSURE_WRITE="} {
		if CheckEnvironment([]string{"GODEBUG=netdns=go", entry}) != ErrUnsafe {
			t.Fatal("generated memory-pressure variable accepted")
		}
	}
}
