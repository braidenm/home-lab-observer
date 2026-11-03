//go:build linux

package actionipc

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"golang.org/x/sys/unix"
)

// ErrNoSelection is only a clean pre-first-byte EOF on an already authenticated
// initial BEGIN conversation. It is never an empty claim or execution result.
var ErrNoSelection = errors.New("action_worker_selection_absent")

// ReadWorkerSelection retains the ordinary five-second whole-frame limit and
// closed ordinal1 worker kinds. Only this initial receive distinguishes EOF;
// every partial frame, ancillary message and cancellation remains a refusal.
func ReadWorkerSelection(ctx context.Context, c *net.UnixConn) (actionprotocol.Envelope, error) {
	finish, err := frameDeadline(ctx, c)
	if err != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	defer finish()
	var header [4]byte
	var ancillary [2048]byte
	n, oob, flags, _, readErr := c.ReadMsgUnix(header[:1], ancillary[:])
	rightsErr := closeAncillary(ancillary[:oob])
	if ctx.Err() != nil || rightsErr != nil || oob != 0 || flags&^(unix.MSG_CMSG_CLOEXEC) != 0 {
		return actionprotocol.Envelope{}, ErrFrame
	}
	if n == 0 && errors.Is(readErr, io.EOF) {
		return actionprotocol.Envelope{}, ErrNoSelection
	}
	if readErr != nil || n != 1 || receive(c, header[1:]) != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	length := binary.BigEndian.Uint32(header[:])
	if length == 0 || length > actionprotocol.MaxEnvelopeBytes {
		return actionprotocol.Envelope{}, ErrFrame
	}
	body := make([]byte, int(length))
	if receive(c, body) != nil || ctx.Err() != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	e, err := actionprotocol.DecodeEnvelope(body)
	if err != nil || e.Version != actionprotocol.WorkerVersion || e.Ordinal != "1" || (e.Kind != "APPROVAL_SNAPSHOT" && e.Kind != "SNAPSHOT_QUERY" && e.Kind != "RECOVERY_QUERY") {
		return actionprotocol.Envelope{}, ErrFrame
	}
	return e, nil
}
