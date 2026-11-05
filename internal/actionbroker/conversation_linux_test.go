//go:build linux

package actionbroker

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"golang.org/x/sys/unix"
)

func conversationPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal("owned conversation pair")
	}
	var pair [2]*net.UnixConn
	for i, fd := range fds {
		file := os.NewFile(uintptr(fd), "synthetic-peer-not-authority")
		conn, err := net.FileConn(file)
		file.Close()
		if err != nil {
			t.Fatal("owned Unix connection")
		}
		pair[i] = conn.(*net.UnixConn)
		t.Cleanup(func() { pair[i].Close() })
	}
	return pair[0], pair[1]
}
func sendBeginning(t *testing.T, c *net.UnixConn) {
	t.Helper()
	if actionipc.Write(context.Background(), c, actionprotocol.Envelope{Version: actionprotocol.WorkerVersion, Kind: "BEGIN", ConversationNonce: "00000000000000000000000000000000", Ordinal: "1", Payload: []byte("{}")}) != nil {
		t.Fatal("begin greeting")
	}
}

func TestConsumedConversationNoClaimClosesAndReleasesOnlyAnonymousSlot(t *testing.T) {
	b, _ := reservationFixture(t)
	server, worker := conversationPair(t)
	done := make(chan error, 1)
	go func() {
		invocation, err := b.readWorkerInvocation(context.Background(), server)
		if invocation != nil {
			done <- ErrRecoveryRequired
			return
		}
		done <- err
	}()
	sendBeginning(t, worker)
	approval, err := actionipc.Read(context.Background(), worker)
	if err != nil || approval.Kind != "APPROVAL_SNAPSHOT" {
		t.Fatal("broker did not select execution")
	}
	if actionipc.Write(context.Background(), worker, actionprotocol.Envelope{Version: actionprotocol.WorkerVersion, Kind: "NO_CLAIM", ConversationNonce: approval.ConversationNonce, Ordinal: "2", Payload: []byte("{}")}) != nil || <-done != nil {
		t.Fatal("verified bodyless 204 continuation")
	}
	if len(b.live) != 0 || b.recoveryRequired {
		t.Fatal("known no-claim left anonymous reservation or recovery latch")
	}
	s, err := b.journal.AdmissionSnapshot(context.Background())
	if err != nil || len(s.Records) != 0 {
		t.Fatal("no-claim wrote journal")
	}
	if _, err := actionipc.Read(context.Background(), worker); err == nil {
		t.Fatal("no-claim connection remained upgradeable")
	}
}

func TestConsumedConversationControlClosesBeforeIndependentExecution(t *testing.T) {
	b, _ := reservationFixture(t)
	b.readiness = nil
	server, worker := conversationPair(t)
	done := make(chan error, 1)
	go func() {
		invocation, err := b.readWorkerInvocation(context.Background(), server)
		if invocation != nil {
			done <- ErrRecoveryRequired
			return
		}
		done <- err
	}()
	sendBeginning(t, worker)
	query, err := actionipc.Read(context.Background(), worker)
	if err != nil || query.Kind != "SNAPSHOT_QUERY" {
		t.Fatal("missing readiness did not choose closed control")
	}
	q, err := actionprotocol.DecodeSnapshotQuery(query.Payload)
	if err != nil {
		t.Fatal("typed query")
	}
	payload, _ := json.Marshal(controlReply(t, q))
	if actionipc.Write(context.Background(), worker, actionprotocol.Envelope{Version: actionprotocol.WorkerVersion, Kind: "SNAPSHOT_REPLY", ConversationNonce: query.ConversationNonce, Ordinal: "2", Payload: payload}) != nil || <-done != nil {
		t.Fatal("control exchange")
	}
	if len(b.live) != 0 || b.readiness == nil {
		t.Fatal("control reserved execution or failed readiness")
	}
	if _, err := actionipc.Read(context.Background(), worker); err == nil {
		t.Fatal("control connection remained upgradeable")
	}
}

func TestConsumedConversationEOFAndCrossNonceNeverMeanNoClaim(t *testing.T) {
	for _, mode := range []string{"eof", "wrong-nonce"} {
		t.Run(mode, func(t *testing.T) {
			b, _ := reservationFixture(t)
			server, worker := conversationPair(t)
			done := make(chan error, 1)
			go func() { _, err := b.readWorkerInvocation(context.Background(), server); done <- err }()
			sendBeginning(t, worker)
			approval, err := actionipc.Read(context.Background(), worker)
			if err != nil || approval.Kind != "APPROVAL_SNAPSHOT" {
				t.Fatal("execution nonce")
			}
			if mode == "eof" {
				worker.Close()
			} else {
				nonce := "00000000000000000000000000000001"
				if nonce == approval.ConversationNonce {
					nonce = "00000000000000000000000000000002"
				}
				if actionipc.Write(context.Background(), worker, actionprotocol.Envelope{Version: actionprotocol.WorkerVersion, Kind: "NO_CLAIM", ConversationNonce: nonce, Ordinal: "2", Payload: []byte("{}")}) != nil {
					t.Fatal("cross nonce fixture")
				}
			}
			if <-done == nil || !b.recoveryRequired {
				t.Fatal("ambiguous claim silently released")
			}
		})
	}
}
