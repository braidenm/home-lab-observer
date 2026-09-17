package connectedpolicy

import "testing"

func TestOfflineMemoryPressurePolicy(t *testing.T) {
	for _, mode := range []string{"validate-enrollment", "validate-ledger"} {
		for _, value := range []string{"skip", "off", "on", "auto", "unknown", "", "SKIP"} {
			e, m := offlineSample(t, mode)
			m["MemoryPressureWatch"] = value
			if (validateOffline(offlineBytes(m), e) == nil) != (value == "skip") {
				t.Fatalf("incorrect offline memory-pressure admission for %s", value)
			}
		}
		e, m := offlineSample(t, mode)
		m["MemoryPressureWatch"] = "skip"
		if validateOffline(append(offlineBytes(m), []byte("MemoryPressureWatch=skip\n")...), e) != ErrUnsafe {
			t.Fatal("duplicate offline memory-pressure policy accepted")
		}
		delete(m, "MemoryPressureWatch")
		if validateOffline(offlineBytes(m), e) != ErrUnsafe {
			t.Fatal("missing offline memory-pressure policy accepted")
		}
	}
}
