//go:build linux

package actionipc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"golang.org/x/sys/unix"
)

func recoveryContinuation(t *testing.T) actionprotocol.Envelope {
	t.Helper()
	raw, err := os.ReadFile("../actionprotocol/testdata/paired-container-execution-v1.json")
	var fixture struct {
		Worker map[string]actionprotocol.Envelope `json:"positive_worker_ipc"`
	}
	if err != nil || json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("synthetic recovery fixture")
	}
	e := fixture.Worker["RECOVERY_QUERY"]
	q := actionprotocol.RecoveryQuery{Operation: "STATUS", ActionID: "hla_11111111111111111111111111111111", RemoteReceiptDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Body: json.RawMessage("null")}
	e.Payload, _ = json.Marshal(q)
	e.Ordinal = "2"
	if _, err := actionprotocol.EncodeEnvelope(e); err != nil {
		t.Fatal("reachable query fixture")
	}
	return e
}

func TestRecoveryContinuationWaitsForProcessingInsideOriginalDeadline(t *testing.T) {
	a, b := framePair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	idle := make(chan struct{})
	done := make(chan error, 1)
	expected := recoveryContinuation(t)
	go func() {
		<-idle
		time.Sleep(600 * time.Millisecond)
		done <- Write(ctx, a, expected)
	}()
	e, err := readRecoveryContinuation(ctx, b, 500*time.Millisecond, func(first bool) {
		if !first {
			close(idle)
		}
	})
	if err != nil || e.Kind != "RECOVERY_QUERY" || e.Ordinal != "2" || <-done != nil {
		t.Fatal("broker processing consumed transfer cap")
	}
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if _, err := ReadRecoveryContinuation(short, b); err == nil {
		t.Fatal("original deadline ignored")
	}
	if _, err := ReadRecoveryContinuation(context.Background(), b); err == nil {
		t.Fatal("unbounded wait admitted")
	}
}

func TestRecoveryContinuationRefusesCrossBranchInitialQueryPartialAndAncillary(t *testing.T) {
	for _, mode := range []string{"branch", "initial", "partial-header", "partial-body", "right"} {
		t.Run(mode, func(t *testing.T) {
			a, b := framePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			e := recoveryContinuation(t)
			if mode == "branch" || mode == "initial" {
				if mode == "branch" {
					e = greeting()
				} else {
					e.Ordinal = "1"
				}
				done := make(chan error, 1)
				go func() { done <- Write(ctx, a, e) }()
				if _, err := ReadRecoveryContinuation(ctx, b); err == nil || <-done != nil {
					t.Fatal("wrong continuation branch accepted")
				}
				return
			}
			if mode == "right" {
				f, err := os.CreateTemp(t.TempDir(), "owned-recovery-right")
				if err != nil {
					t.Fatal("owned descriptor")
				}
				defer f.Close()
				before, _ := os.ReadDir("/proc/self/fd")
				if _, _, err := a.WriteMsgUnix([]byte{0}, unix.UnixRights(int(f.Fd())), nil); err != nil {
					t.Fatal("owned right write")
				}
				if _, err := ReadRecoveryContinuation(ctx, b); err == nil {
					t.Fatal("ancillary accepted")
				}
				after, err := os.ReadDir("/proc/self/fd")
				if err != nil || len(before) != len(after) {
					t.Fatal("received right leaked")
				}
				return
			}
			body, err := actionprotocol.EncodeEnvelope(e)
			if err != nil {
				t.Fatal("closed query fixture")
			}
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], uint32(len(body)))
			partial := header[:1]
			if mode == "partial-body" {
				partial = append(header[:], body[:1]...)
			}
			if _, err := a.Write(partial); err != nil {
				t.Fatal("owned partial write")
			}
			start := time.Now()
			if _, err := readRecoveryContinuation(ctx, b, 30*time.Millisecond, nil); err == nil || time.Since(start) > 500*time.Millisecond {
				t.Fatal("partial escaped transfer cap")
			}
		})
	}
}

func TestRecoveryContinuationCancellationAtBothReceivePhases(t *testing.T) {
	for _, first := range []bool{false, true} {
		a, b := framePair(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if first {
			if _, err := a.Write([]byte{0}); err != nil {
				t.Fatal("owned first byte")
			}
		}
		_, err := readRecoveryContinuation(ctx, b, 5*time.Second, func(phase bool) {
			if phase == first {
				cancel()
			}
		})
		cancel()
		if err == nil {
			t.Fatal("cancellation ignored")
		}
	}
}
