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

func refusedAdmin() actionprotocol.Envelope {
	return actionprotocol.Envelope{Version: actionprotocol.AdminVersion, Kind: "REFUSED", ConversationNonce: "11111111111111111111111111111111", Ordinal: "3", Payload: []byte(`{"code":"RECOVERY_REQUIRED"}`)}
}

func TestAdminResponseWaitsForProcessingButBoundsTransferAndOriginalDeadline(t *testing.T) {
	a, b := framePair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	idle := make(chan struct{})
	done := make(chan error, 1)
	go func() { <-idle; time.Sleep(600 * time.Millisecond); done <- Write(ctx, a, refusedAdmin()) }()
	e, err := readAdminResponse(ctx, b, 500*time.Millisecond, func(first bool) {
		if !first {
			close(idle)
		}
	})
	if err != nil || e.Kind != "REFUSED" || <-done != nil {
		t.Fatal("admin processing consumed frame transfer cap")
	}
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if _, err := ReadAdminResponse(short, b); err == nil {
		t.Fatal("original admin deadline ignored")
	}
}

func TestAdminResponseRefusesPartialFramesOtherKindsAndFirstByteRights(t *testing.T) {
	for _, mode := range []string{"partial-header", "partial-body", "other-kind", "right"} {
		t.Run(mode, func(t *testing.T) {
			a, b := framePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if mode == "other-kind" {
				done := make(chan error, 1)
				go func() { done <- Write(ctx, a, greeting()) }()
				if _, err := ReadAdminResponse(ctx, b); err == nil || <-done != nil {
					t.Fatal("admin receive crossed branch")
				}
				return
			}
			if mode == "right" {
				f, err := os.CreateTemp(t.TempDir(), "owned-admin-right")
				if err != nil {
					t.Fatal("owned descriptor")
				}
				defer f.Close()
				before, _ := os.ReadDir("/proc/self/fd")
				if _, _, err := a.WriteMsgUnix([]byte{0}, unix.UnixRights(int(f.Fd())), nil); err != nil {
					t.Fatal("owned right")
				}
				if _, err := ReadAdminResponse(ctx, b); err == nil {
					t.Fatal("admin accepted descriptor")
				}
				after, err := os.ReadDir("/proc/self/fd")
				if err != nil || len(before) != len(after) {
					t.Fatal("admin descriptor leaked")
				}
				return
			}
			body, err := actionprotocol.EncodeEnvelope(refusedAdmin())
			if err != nil {
				t.Fatal("closed admin frame")
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
			if _, err := readAdminResponse(ctx, b, 30*time.Millisecond, nil); err == nil {
				t.Fatal("partial admin frame escaped transfer cap")
			}
		})
	}
}

func TestAdminResponseCancellationAtBothPhasesAndNoUnboundedParent(t *testing.T) {
	for _, first := range []bool{false, true} {
		a, b := framePair(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if first {
			if _, err := a.Write([]byte{0}); err != nil {
				t.Fatal("owned first byte")
			}
		}
		_, err := readAdminResponse(ctx, b, 5*time.Second, func(phase bool) {
			if phase == first {
				cancel()
			}
		})
		cancel()
		if err == nil {
			t.Fatal("admin cancellation ignored")
		}
	}
	a, b := framePair(t)
	defer a.Close()
	if _, err := ReadAdminResponse(context.Background(), b); err == nil {
		t.Fatal("unbounded admin context accepted")
	}
}
