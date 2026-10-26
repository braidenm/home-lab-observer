//go:build linux

package connectedinstall

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestActivationProbeBaseline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := newProbePair(ctx)
	if err != nil {
		t.Fatal("dual-stack baseline unavailable")
	}
	defer p.close()
	if !p.denied() || p.finish(ctx) != nil {
		t.Fatal("baseline accounting failed")
	}
	if p.denied() {
		t.Fatal("post-baseline accepted as denial phase")
	}
}

func TestActivationProbeUnexpectedConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := newProbePair(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	c, err := net.DialTimeout("tcp4", p.v4.listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	var ack [1]byte
	if _, err := io.ReadFull(c, ack[:]); err != nil {
		t.Fatal(err)
	}
	if p.denied() || p.finish(ctx) == nil {
		t.Fatal("unexpected connection accepted")
	}
}

func TestActivationProbeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p, err := newProbePair(ctx); err == nil {
		p.close()
		t.Fatal("canceled baseline accepted")
	}
}
