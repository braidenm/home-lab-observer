//go:build linux

// Package actionipc implements bounded Unix-stream framing, not peer admission.
// Fixed-path consumers must authenticate the peer before reading these frames.
package actionipc

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"golang.org/x/sys/unix"
)

var ErrFrame = errors.New("action_ipc_refused")

// frameDeadline covers the WHOLE header/body. The owning operation supplies its
// shorter context and separately rechecks its original BOOTTIME phase budget.
// Joining the cancellation callback prevents it poisoning a subsequent frame.
func frameDeadline(ctx context.Context, c *net.UnixConn) (func(), error) {
	if ctx == nil || c == nil || ctx.Err() != nil {
		return nil, ErrFrame
	}
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if c.SetDeadline(deadline) != nil {
		return nil, ErrFrame
	}
	joined := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { c.SetDeadline(time.Now()); close(joined) })
	return func() {
		if !stop() {
			<-joined
		}
		c.SetDeadline(time.Time{})
	}, nil
}

func closeAncillary(raw []byte) error {
	closeError := false
	for len(raw) >= unix.CmsgLen(0) {
		header, data, remainder, err := unix.ParseOneSocketControlMessage(raw)
		if err != nil {
			return ErrFrame
		} // Earlier parsed rights are already closed.
		raw = remainder
		if header.Level == unix.SOL_SOCKET && header.Type == unix.SCM_RIGHTS {
			// Never guess descriptor numbers from malformed/unparsed bytes.
			// Linux-generated rights are an exact array of native int32s.
			if len(data)%4 != 0 {
				closeError = true
				continue
			}
			message := unix.SocketControlMessage{Header: header, Data: data}
			rights, err := unix.ParseUnixRights(&message)
			if err != nil {
				closeError = true
			}
			for _, fd := range rights {
				if unix.Close(fd) != nil {
					closeError = true
				}
			}
		}
	}
	if len(raw) != 0 || closeError {
		return ErrFrame
	}
	return nil
}

// Every data read is recvmsg, including partial headers/bodies. Go1.27.3's Linux
// UnixConn readMsg uses MSG_CMSG_CLOEXEC. Close every received right before ANY
// refusal; the kernel closes undisclosed rights when ancillary data truncates.
func receive(c *net.UnixConn, p []byte) error {
	for len(p) > 0 {
		var ancillary [2048]byte // Linux SCM_MAX_FD=253; deliberately bounded.
		n, oob, flags, _, err := c.ReadMsgUnix(p, ancillary[:])
		closeError := closeAncillary(ancillary[:oob])
		if err != nil || closeError != nil || oob != 0 || flags&(unix.MSG_CTRUNC|unix.MSG_TRUNC) != 0 || n <= 0 {
			return ErrFrame
		}
		p = p[n:]
	}
	return nil
}

func Read(ctx context.Context, c *net.UnixConn) (actionprotocol.Envelope, error) {
	finish, err := frameDeadline(ctx, c)
	if err != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	defer finish()
	var header [4]byte
	if receive(c, header[:]) != nil {
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
	envelope, err := actionprotocol.DecodeEnvelope(body)
	if err != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	return envelope, nil
}

// ReadExecutionMessage waits for bounded broker processing under the caller's
// ORIGINAL deadline. Only after the first header byte arrives does the unchanged
// five-second transfer cap begin. The worker must recheck its native phase both
// before and after this call. This is not a generic idle-timeout or peer grant.
func ReadExecutionMessage(ctx context.Context, c *net.UnixConn) (actionprotocol.Envelope, error) {
	return readExecutionMessage(ctx, c, 5*time.Second, nil)
}

// The private seam only shortens the transfer cap and observes phase entry for
// deterministic owned-socket tests. Production fixes5s and supplies no observer.
func readExecutionMessage(ctx context.Context, c *net.UnixConn, transferCap time.Duration, observe func(firstByte bool)) (actionprotocol.Envelope, error) {
	envelope, err := readPhaseMessage(ctx, c, transferCap, observe)
	if err != nil || envelope.Version != actionprotocol.WorkerVersion {
		return actionprotocol.Envelope{}, ErrFrame
	}
	if envelope.Kind == "AUTHORITY_CHALLENGE" && (envelope.Ordinal == "3" || envelope.Ordinal == "4") || envelope.Kind == "RESULT" && envelope.Ordinal == "5" {
		return envelope, nil
	}
	return actionprotocol.Envelope{}, ErrFrame
}

// ReadClaimDelivery is only the broker's pre-handoff ordinal2 receive. Its
// required deadline must come from the original preclaim DeliveryContext; the
// broker independently rechecks native timing and the execution nonce. Neither
// EOF nor this framing helper establishes an authenticated empty HTTP poll.
func ReadClaimDelivery(ctx context.Context, c *net.UnixConn) (actionprotocol.Envelope, error) {
	return readClaimDelivery(ctx, c, 5*time.Second, nil)
}

func readClaimDelivery(ctx context.Context, c *net.UnixConn, transferCap time.Duration, observe func(bool)) (actionprotocol.Envelope, error) {
	envelope, err := readPhaseMessage(ctx, c, transferCap, observe)
	if err != nil || envelope.Version != actionprotocol.WorkerVersion || envelope.Ordinal != "2" || (envelope.Kind != "CLAIMED_EXECUTE" && envelope.Kind != "NO_CLAIM") {
		return actionprotocol.Envelope{}, ErrFrame
	}
	return envelope, nil
}

// ReadAdminResponse admits only the final response to an already authenticated
// root operation. The caller supplies and rechecks its ORIGINAL native90s phase;
// this function neither restarts it nor matches the independent HELLO nonce.
func ReadAdminResponse(ctx context.Context, c *net.UnixConn) (actionprotocol.Envelope, error) {
	return readAdminResponse(ctx, c, 5*time.Second, nil)
}

func readAdminResponse(ctx context.Context, c *net.UnixConn, transferCap time.Duration, observe func(bool)) (actionprotocol.Envelope, error) {
	envelope, err := readPhaseMessage(ctx, c, transferCap, observe)
	if err != nil || envelope.Version != actionprotocol.AdminVersion || envelope.Ordinal != "3" || (envelope.Kind != "RECOVERY_STATE" && envelope.Kind != "REFUSED") {
		return actionprotocol.Envelope{}, ErrFrame
	}
	return envelope, nil
}

// Shared private framing only; each public phase has its own closed kind set.
func readPhaseMessage(ctx context.Context, c *net.UnixConn, transferCap time.Duration, observe func(firstByte bool)) (actionprotocol.Envelope, error) {
	if ctx == nil || c == nil || ctx.Err() != nil || transferCap <= 0 || transferCap > 5*time.Second {
		return actionprotocol.Envelope{}, ErrFrame
	}
	original, ok := ctx.Deadline()
	if !ok || c.SetReadDeadline(original) != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	joined := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { c.SetReadDeadline(time.Now()); close(joined) })
	defer func() {
		if !stop() {
			<-joined
		}
		c.SetReadDeadline(time.Time{})
	}()
	if observe != nil {
		observe(false)
	}
	if ctx.Err() != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	var header [4]byte
	if receive(c, header[:1]) != nil || ctx.Err() != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	firstByte := time.Now()
	if observe != nil {
		observe(true)
	}
	deadline := firstByte.Add(transferCap)
	if original.Before(deadline) {
		deadline = original
	}
	if c.SetReadDeadline(deadline) != nil || ctx.Err() != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	if receive(c, header[1:]) != nil {
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
	envelope, err := actionprotocol.DecodeEnvelope(body)
	if err != nil || ctx.Err() != nil {
		return actionprotocol.Envelope{}, ErrFrame
	}
	return envelope, nil
}

func Write(ctx context.Context, c *net.UnixConn, envelope actionprotocol.Envelope) error {
	body, err := actionprotocol.EncodeEnvelope(envelope)
	if err != nil {
		return ErrFrame
	}
	finish, err := frameDeadline(ctx, c)
	if err != nil {
		return ErrFrame
	}
	defer finish()
	frame := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(body)))
	copy(frame[4:], body)
	for len(frame) > 0 {
		n, err := c.Write(frame)
		if err != nil || n <= 0 || ctx.Err() != nil {
			return ErrFrame
		}
		frame = frame[n:]
	}
	return nil
}
