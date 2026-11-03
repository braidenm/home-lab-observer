//go:build linux

package actionipc

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"golang.org/x/sys/unix"
)

func TestSelectionEOFIsOnlyCleanPreFirstByte(t *testing.T) {
	for _, mode := range []string{"clean", "partial-header", "partial-body", "invalid-json", "zero-length", "oversized", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			a, b := framePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var wire []byte
			switch mode {
			case "partial-header":
				wire = []byte{0}
			case "partial-body":
				wire = []byte{0, 0, 0, 8, '{'}
			case "invalid-json":
				wire = []byte{0, 0, 0, 1, '{'}
			case "zero-length":
				wire = make([]byte, 4)
			case "oversized":
				wire = make([]byte, 4)
				binary.BigEndian.PutUint32(wire, actionprotocol.MaxEnvelopeBytes+1)
			case "canceled":
				cancel()
			}
			if len(wire) > 0 {
				if _, err := a.Write(wire); err != nil {
					t.Fatal("owned partial bytes")
				}
			}
			a.Close()
			_, err := ReadWorkerSelection(ctx, b)
			if mode == "clean" {
				if !errors.Is(err, ErrNoSelection) {
					t.Fatal("clean greeting refusal not distinguished")
				}
			} else if !errors.Is(err, ErrFrame) || errors.Is(err, ErrNoSelection) {
				t.Fatal("unsafe frame became transient selection")
			}
		})
	}
}

func TestSelectionRefusesOtherKindsAndAncillaryWithoutLeaking(t *testing.T) {
	for _, mode := range []string{"worker-begin", "admin-hello", "claim-reply", "right"} {
		t.Run(mode, func(t *testing.T) {
			a, b := framePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if mode == "right" {
				f, err := os.CreateTemp(t.TempDir(), "owned-selection-right")
				if err != nil {
					t.Fatal("owned descriptor")
				}
				defer f.Close()
				before, err := os.ReadDir("/proc/self/fd")
				if err != nil {
					t.Fatal("fd count")
				}
				if _, _, err := a.WriteMsgUnix([]byte{0}, unix.UnixRights(int(f.Fd())), nil); err != nil {
					t.Fatal("owned right")
				}
				if _, err := ReadWorkerSelection(ctx, b); !errors.Is(err, ErrFrame) {
					t.Fatal("ancillary became unselected EOF")
				}
				after, err := os.ReadDir("/proc/self/fd")
				if err != nil || len(before) != len(after) {
					t.Fatal("selection descriptor leaked")
				}
				return
			}
			e := greeting()
			if mode == "admin-hello" {
				e = actionprotocol.Envelope{Version: actionprotocol.AdminVersion, Kind: "HELLO", ConversationNonce: "11111111111111111111111111111111", Ordinal: "1", Payload: []byte("{}")}
			}
			if mode == "claim-reply" {
				e = emptyClaim()
			}
			if Write(ctx, a, e) != nil {
				t.Fatal("closed wrong-branch fixture")
			}
			if _, err := ReadWorkerSelection(ctx, b); !errors.Is(err, ErrFrame) || errors.Is(err, ErrNoSelection) {
				t.Fatal("cross-branch message retried")
			}
		})
	}
}
