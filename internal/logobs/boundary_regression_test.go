package logobs

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrivateCheckpointJSONExcludesEncodedValues(t *testing.T) {
	private := []byte("synthetic-private-checkpoint-do-not-export")
	checkpoint := Checkpoint{Revision: 1, Opaque: private}
	for name, value := range map[string]any{
		"checkpoint": checkpoint,
		"request":    ReadRequest{Checkpoint: checkpoint},
		"batch":      Batch{NextOpaque: private},
	} {
		t.Run(name, func(t *testing.T) {
			payload, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{string(private), base64.StdEncoding.EncodeToString(private), "Opaque"} {
				if strings.Contains(string(payload), forbidden) {
					t.Fatal("private checkpoint crossed JSON boundary")
				}
			}
		})
	}
}

func TestAllDiscardedBatchRetainsDeferredSentinel(t *testing.T) {
	reason := ReasonBacklogDeferred
	batch := Batch{
		Kind: BatchNormal, Source: SourceSystem,
		QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
		SupportState: SupportSupported, CollectionState: CollectionPartial, ReasonCode: &reason,
		Discards:       []DiscardCount{{At: testTime, Count: MaxAcceptedEvents}},
		DiscardedCount: MaxAcceptedEvents, ExaminedCount: MaxExaminedEvents,
		Deferred: true, NextOpaque: []byte("synthetic-cursor-before-sentinel"),
	}
	if err := batch.Validate(); err != nil {
		t.Fatalf("bounded all-discarded batch rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Batch){
		"sentinel counted as discarded": func(b *Batch) {
			b.Discards[0].Count++
			b.DiscardedCount++
			b.ExaminedCount++
		},
		"sentinel omitted from examined": func(b *Batch) { b.ExaminedCount-- },
		"sentinel falsely caught up":     func(b *Batch) { b.CaughtUp = true },
		"cursor omitted":                 func(b *Batch) { b.NextOpaque = nil },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := batch.Clone()
			mutate(&candidate)
			if candidate.Validate() == nil {
				t.Fatal("invalid deferred batch accepted")
			}
		})
	}
}

func TestSummaryRejectsOverflowAcrossIndividuallySafeBuckets(t *testing.T) {
	summary := validFullSummary()
	first := summary.Sources[0].Buckets[0].Counts
	first.Captured, first.Severity.Info = MaxSafeInteger, MaxSafeInteger
	summary.Sources[0].Counts.Captured = MaxSafeInteger
	if err := summary.Validate(); err != nil {
		t.Fatalf("exact maximum safe aggregate rejected: %v", err)
	}
	second := summary.Sources[0].Buckets[1].Counts
	second.Captured, second.Severity.Warn = 1, 1
	summary.Sources[0].Counts.Captured++
	if summary.Validate() == nil {
		t.Fatal("aggregate overflow accepted despite individually safe buckets")
	}

	summary = validFullSummary()
	summary.Sources[0].Buckets[0].Counts.Discarded = MaxSafeInteger
	summary.Sources[0].Counts.Discarded = MaxSafeInteger
	if err := summary.Validate(); err != nil {
		t.Fatalf("exact maximum discarded aggregate rejected: %v", err)
	}
	summary.Sources[0].Buckets[1].Counts.Discarded = 1
	summary.Sources[0].Counts.Discarded++
	if summary.Validate() == nil {
		t.Fatal("discard aggregate overflow accepted")
	}
}
