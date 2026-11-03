//go:build linux

package actionipc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"golang.org/x/sys/unix"
)

func executionWire(t *testing.T) (actionprotocol.Envelope, []byte) {
	t.Helper()
	raw, err := os.ReadFile("../actionprotocol/testdata/paired-container-execution-v1.json")
	var fixture struct {
		Worker map[string]json.RawMessage `json:"positive_worker_ipc"`
	}
	if err != nil || json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("synthetic paired fixture")
	}
	var compact bytes.Buffer
	if json.Compact(&compact, fixture.Worker["RESULT"]) != nil {
		t.Fatal("fixture shape")
	}
	envelope, err := actionprotocol.DecodeEnvelope(compact.Bytes())
	if err != nil {
		t.Fatal("typed result fixture")
	}
	wire := make([]byte, 4+compact.Len())
	binary.BigEndian.PutUint32(wire[:4], uint32(compact.Len()))
	copy(wire[4:], compact.Bytes())
	return envelope, wire
}

func TestExecutionReceiveSeparatesProcessingWaitFromWholeFrame(t *testing.T) {
	a, b := framePair(t)
	expected, wire := executionWire(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		select {
		case <-time.After(600 * time.Millisecond):
			_, err := a.Write(wire)
			done <- err
		case <-ctx.Done():
			done <- ctx.Err()
		}
	}()
	// Private shortened transfer cap proves idle waiting is NOT charged to it.
	got, err := readExecutionMessage(ctx, b, 500*time.Millisecond, nil)
	if err != nil || got.Kind != expected.Kind || <-done != nil {
		t.Fatal("legitimate processing delay consumed frame transfer cap")
	}
	if _, err := ReadExecutionMessage(context.Background(), b); err == nil {
		t.Fatal("unbounded parent created an execution wait")
	}
}

func TestExecutionReceiveStillBoundsPartialHeaderAndBody(t *testing.T) {
	for _, body := range []bool{false, true} {
		t.Run(map[bool]string{false: "header", true: "body"}[body], func(t *testing.T) {
			a, b := framePair(t)
			_, wire := executionWire(t)
			prefix := 1
			if body {
				prefix = 5
			}
			if _, err := a.Write(wire[:prefix]); err != nil {
				t.Fatal("partial fixture")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			started := time.Now()
			if _, err := readExecutionMessage(ctx, b, 50*time.Millisecond, nil); err == nil {
				t.Fatal("partial frame accepted")
			}
			if time.Since(started) > time.Second {
				t.Fatal("partial frame used idle deadline instead of transfer cap")
			}
		})
	}
}

func TestExecutionReceiveCancellationInBothPhasesAndFirstByteRights(t *testing.T) {
	for _, afterFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-first", true: "after-first"}[afterFirst], func(t *testing.T) {
			a, b := framePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if afterFirst {
				if _, err := a.Write([]byte{0}); err != nil {
					t.Fatal("first byte")
				}
			}
			_, err := readExecutionMessage(ctx, b, 500*time.Millisecond, func(first bool) {
				if first == afterFirst {
					cancel()
				}
			})
			if err == nil || ctx.Err() == nil {
				t.Fatal("phase cancellation lost")
			}
		})
	}
	a, b := framePair(t)
	f, err := os.CreateTemp(t.TempDir(), "first-byte-right")
	if err != nil {
		t.Fatal("owned right")
	}
	defer f.Close()
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal("owned descriptor accounting")
	}
	if _, _, err := a.WriteMsgUnix([]byte{0}, unix.UnixRights(int(f.Fd())), nil); err != nil {
		t.Fatal("first byte ancillary fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := ReadExecutionMessage(ctx, b); err == nil {
		t.Fatal("first byte SCM_RIGHTS accepted")
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(after) != len(before) {
		t.Fatal("first byte received right leaked")
	}
}

func TestExecutionReceiveRejectsOtherBranches(t *testing.T) {
	a, b := framePair(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Write(ctx, a, greeting()) }()
	if _, err := ReadExecutionMessage(ctx, b); err == nil || <-done != nil {
		t.Fatal("execution receive accepted greeting/control branch")
	}
}

func framePair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	fd, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal("private socketpair")
	}
	connections := make([]*net.UnixConn, 0, 2)
	for _, number := range fd {
		f := os.NewFile(uintptr(number), "owned-ipc-test")
		c, err := net.FileConn(f)
		f.Close()
		if err != nil {
			t.Fatal("owned Unix connection")
		}
		u := c.(*net.UnixConn)
		t.Cleanup(func() { u.Close() })
		connections = append(connections, u)
	}
	return connections[0], connections[1]
}
func greeting() actionprotocol.Envelope {
	return actionprotocol.Envelope{Version: actionprotocol.WorkerVersion, Kind: "BEGIN", ConversationNonce: "00000000000000000000000000000000", Ordinal: "1", Payload: []byte("{}")}
}

func TestFramesUseBoundedCanonicalEnvelopeAndJoinedCancellation(t *testing.T) {
	a, b := framePair(t)
	done := make(chan error, 1)
	go func() { done <- Write(context.Background(), a, greeting()) }()
	got, err := Read(context.Background(), b)
	if err != nil || got.Kind != "BEGIN" || <-done != nil {
		t.Fatal("actual Unix framed roundtrip")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Read(ctx, b); err == nil {
		t.Fatal("canceled frame waited or succeeded")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := Read(ctx, b); err == nil {
		t.Fatal("partial/absent frame deadline ignored")
	}
}
func TestFramesRefuseOversizeBeforeBodyAndCloseReceivedRights(t *testing.T) {
	for _, length := range []uint32{0, 16385, ^uint32(0)} {
		t.Run("bounded-header", func(t *testing.T) {
			a, b := framePair(t)
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], length)
			if _, err := a.Write(header[:]); err != nil {
				t.Fatal("header")
			}
			if _, err := Read(context.Background(), b); err == nil {
				t.Fatal("unsafe length accepted")
			}
		})
	}
	a, b := framePair(t)
	f, err := os.CreateTemp(t.TempDir(), "owned-right")
	if err != nil {
		t.Fatal("private descriptor")
	}
	defer f.Close()
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal("descriptor accounting")
	}
	if _, _, err := a.WriteMsgUnix([]byte{0, 0, 0, 1}, unix.UnixRights(int(f.Fd())), nil); err != nil {
		t.Fatal("owned SCM_RIGHTS")
	}
	if _, err := Read(context.Background(), b); err == nil {
		t.Fatal("SCM_RIGHTS accepted")
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(after) != len(before) {
		t.Fatal("received descriptor leaked")
	}
}

func TestAncillaryCleanupKeepsParsedRightsWhenLaterHeaderIsMalformed(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "owned-prefix")
	if err != nil {
		t.Fatal("owned file")
	}
	defer f.Close()
	received, err := unix.Dup(int(f.Fd()))
	if err != nil {
		t.Fatal("owned received-right fixture")
	}
	defer func() {
		if received >= 0 {
			unix.Close(received)
		}
	}()
	unrelated, err := unix.Dup(int(f.Fd()))
	if err != nil {
		t.Fatal("unrelated owned sentinel")
	}
	defer unix.Close(unrelated)
	control := append(unix.UnixRights(received), make([]byte, unix.CmsgLen(0))...)
	if closeAncillary(control) == nil {
		t.Fatal("malformed suffix was accepted")
	}
	if _, err := unix.FcntlInt(uintptr(received), unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatal("parsed right leaked after malformed suffix")
	}
	received = -1 // The helper closed our duplicate; never close its reused number.
	if _, err := unix.FcntlInt(uintptr(unrelated), unix.F_GETFD, 0); err != nil {
		t.Fatal("cleanup guessed an unrelated descriptor")
	}
	if _, err := f.Stat(); err != nil {
		t.Fatal("sender original was closed")
	}
}
