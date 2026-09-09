package collector

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGopsutilProcessesIncludesCurrentProcessOnWindows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := (GopsutilProvider{}).Processes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, process := range result.Processes {
		if process.PID != int32(os.Getpid()) {
			continue
		}
		if process.Name == "" || process.State != "unknown" || process.CreateTimeMS <= 0 {
			t.Fatalf("current process is incomplete: %+v", process)
		}
		return
	}
	t.Fatalf("current process %d was not returned: discovered=%d scanned=%d permission_denied=%d unsupported=%d failed=%d", os.Getpid(), result.Discovered, result.Scanned, result.PermissionDenied, result.Unsupported, result.Failed)
}
