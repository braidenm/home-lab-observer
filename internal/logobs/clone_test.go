package logobs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBoundaryValuesDeepClonePrivateAndPointerState(t *testing.T) {
	reason := ReasonCheckpointReset
	previous := testTime.Add(-2)
	coverage := testTime.Add(-3)
	request := ReadRequest{
		Source: SourceSystem,
		Checkpoint: Checkpoint{
			Revision: 3, Opaque: []byte("checkpoint-canary"), PreviousAttemptAt: &previous, CoverageThrough: &coverage,
		},
		QueryStartedAt: testTime,
	}
	requestClone := request.Clone()
	request.Checkpoint.Opaque[0] = 'X'
	*request.Checkpoint.PreviousAttemptAt = testTime
	if string(requestClone.Checkpoint.Opaque) != "checkpoint-canary" || requestClone.Checkpoint.PreviousAttemptAt.Equal(testTime) {
		t.Fatal("read request clone retained caller-owned checkpoint state")
	}

	batch := Batch{
		ReasonCode: &reason, Events: []Event{{EventCode: "WIN_1"}},
		Discards: []DiscardCount{{Count: 1}}, NextOpaque: []byte("next-canary"),
	}
	batchClone := batch.Clone()
	*batch.ReasonCode = ReasonReaderFailed
	batch.Events[0].EventCode = "WIN_2"
	batch.Discards[0].Count = 2
	batch.NextOpaque[0] = 'X'
	if *batchClone.ReasonCode != ReasonCheckpointReset || batchClone.Events[0].EventCode != "WIN_1" || batchClone.Discards[0].Count != 1 || string(batchClone.NextOpaque) != "next-canary" {
		t.Fatal("batch clone retained caller-owned state")
	}

	summary := validFullSummary()
	summaryClone := summary.Clone()
	*summary.Sources[0].Status.ObservedAt = summary.WindowStart
	summary.Sources[0].Buckets[0].Counts.Captured = 2
	summary.Sources[0].Counts.Captured = 2
	if summaryClone.Sources[0].Status.ObservedAt.Equal(summary.WindowStart) || summaryClone.Sources[0].Buckets[0].Counts.Captured != 0 || summaryClone.Sources[0].Counts.Captured != 0 {
		t.Fatal("summary clone retained caller-owned state")
	}

	snapshot := Snapshot{Status: successfulStatus(), TotalCount: 1, Events: []Event{{EventCode: "WIN_1"}}}
	snapshotClone := snapshot.Clone()
	snapshot.Events[0].EventCode = "WIN_2"
	*snapshot.Status.ObservedAt = snapshot.Status.ObservedAt.Add(-1)
	if snapshotClone.Events[0].EventCode != "WIN_1" || snapshotClone.Status.ObservedAt.Equal(*snapshot.Status.ObservedAt) {
		t.Fatal("snapshot clone retained caller-owned state")
	}
}

func TestOpaqueValuesAreExcludedFromJSON(t *testing.T) {
	checkpoint := Checkpoint{Revision: 1, Opaque: []byte("private-checkpoint-canary")}
	request := ReadRequest{Source: SourceSystem, Checkpoint: checkpoint, QueryStartedAt: testTime}
	batch := Batch{NextOpaque: []byte("private-next-canary")}
	for name, value := range map[string]any{"checkpoint": checkpoint, "request": request, "batch": batch} {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if strings.Contains(string(payload), "canary") || strings.Contains(string(payload), "Opaque") {
			t.Fatalf("%s serialized private checkpoint: %s", name, payload)
		}
	}
}
