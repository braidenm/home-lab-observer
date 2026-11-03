//go:build linux

package actionipc

import (
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"golang.org/x/sys/unix"
)

func emptyClaim() actionprotocol.Envelope {
	return actionprotocol.Envelope{Version: actionprotocol.WorkerVersion, Kind: "NO_CLAIM", ConversationNonce: "11111111111111111111111111111111", Ordinal: "2", Payload: []byte("{}")}
}

func TestClaimDeliverySeparatesProcessingFromFrameTransfer(t *testing.T) {
	a, b := framePair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	idle := make(chan struct{})
	done := make(chan error, 1)
	go func() { <-idle; time.Sleep(600 * time.Millisecond); done <- Write(ctx, a, emptyClaim()) }()
	e, err := readClaimDelivery(ctx, b, 500*time.Millisecond, func(first bool) {
		if !first {
			close(idle)
		}
	})
	if err != nil || e.Kind != "NO_CLAIM" || <-done != nil {
		t.Fatal("processing wait consumed transfer cap")
	}
}

func TestClaimDeliveryBoundsPartialFramesAndOriginalDeadline(t *testing.T) {
	for _, mode := range []string{"partial-header", "partial-body", "original-deadline"} {
		t.Run(mode, func(t *testing.T) {
			a, b := framePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if mode == "original-deadline" {
				short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
				defer stop()
				if _, err := ReadClaimDelivery(short, b); err == nil {
					t.Fatal("original deadline ignored")
				}
				return
			}
			body, err := actionprotocol.EncodeEnvelope(emptyClaim())
			if err != nil {
				t.Fatal("frame")
			}
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], uint32(len(body)))
			partial := header[:1]
			if mode == "partial-body" {
				partial = append(header[:], body[:1]...)
			}
			if _, err := a.Write(partial); err != nil {
				t.Fatal("owned partial frame")
			}
			if _, err := readClaimDelivery(ctx, b, 30*time.Millisecond, nil); err == nil {
				t.Fatal("partial frame escaped cap")
			}
		})
	}
}

func TestClaimDeliveryCancelsBothPhasesAndRefusesOtherBranches(t *testing.T) {
	for _, first := range []bool{false, true} {
		a, b := framePair(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if first {
			if _, err := a.Write([]byte{0}); err != nil {
				t.Fatal("first header byte")
			}
		}
		_, err := readClaimDelivery(ctx, b, 5*time.Second, func(phase bool) {
			if phase == first {
				cancel()
			}
		})
		cancel()
		if err == nil {
			t.Fatal("cancellation ignored")
		}
	}
	a, b := framePair(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Write(ctx, a, greeting()) }()
	if _, err := ReadClaimDelivery(ctx, b); err == nil || <-done != nil {
		t.Fatal("claim receive accepted other branch")
	}
	if _, err := ReadClaimDelivery(context.Background(), b); err == nil {
		t.Fatal("unbounded parent admitted")
	}
}

func TestClaimDeliveryFirstByteRightsAreClosed(t *testing.T) {
	a, b := framePair(t)
	f, err := os.CreateTemp(t.TempDir(), "owned-claim-right")
	if err != nil {
		t.Fatal("owned file")
	}
	defer f.Close()
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal("fd accounting")
	}
	if _, _, err := a.WriteMsgUnix([]byte{0}, unix.UnixRights(int(f.Fd())), nil); err != nil {
		t.Fatal("owned right")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := ReadClaimDelivery(ctx, b); err == nil {
		t.Fatal("claim right accepted")
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(before) != len(after) {
		t.Fatal("claim right leaked")
	}
}
