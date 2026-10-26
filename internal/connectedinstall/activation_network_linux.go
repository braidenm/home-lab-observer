//go:build linux

package connectedinstall

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// probeListener is a short-lived root-owned loopback fixture, not a worker API.
// Its single acknowledgement byte proves the baseline was accepted and counted.
// Every accepted connection counts, including unsolicited or bodyless traffic.
type probeListener struct {
	listener *net.TCPListener
	count    atomic.Uint64
	failed   atomic.Bool
	closing  atomic.Bool
	done     chan struct{}
	once     sync.Once
}

func newProbeListener(network, address string) (*probeListener, error) {
	addr, err := net.ResolveTCPAddr(network, address)
	if err != nil {
		return nil, ErrUnsafe
	}
	l, err := net.ListenTCP(network, addr)
	if err != nil {
		return nil, ErrUnsafe
	}
	p := &probeListener{listener: l, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		for {
			c, err := l.AcceptTCP()
			if err != nil {
				if !p.closing.Load() {
					p.failed.Store(true)
				}
				return
			}
			p.count.Add(1)
			if c.SetDeadline(time.Now().Add(time.Second)) != nil {
				p.failed.Store(true)
				_ = c.Close()
				return
			}
			_, _ = c.Write([]byte{1})
			_ = c.Close()
		}
	}()
	return p, nil
}

func (p *probeListener) close() {
	p.once.Do(func() { p.closing.Store(true); _ = p.listener.Close(); <-p.done })
}

func (p *probeListener) port() uint16 { return uint16(p.listener.Addr().(*net.TCPAddr).Port) }

func (p *probeListener) baseline(ctx context.Context, network string, expected uint64) error {
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(probe, network, p.listener.Addr().String())
	if err != nil {
		return ErrUnsafe
	}
	defer c.Close()
	deadline, _ := probe.Deadline()
	if c.SetDeadline(deadline) != nil {
		return ErrUnsafe
	}
	var ack [1]byte
	if _, err := io.ReadFull(c, ack[:]); err != nil || ack[0] != 1 || p.failed.Load() || p.count.Load() != expected || probe.Err() != nil {
		return ErrUnsafe
	}
	return nil
}

type probePair struct{ v4, v6 *probeListener }

func newProbePair(ctx context.Context) (*probePair, error) {
	v4, err := newProbeListener("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	v6, err := newProbeListener("tcp6", "[::1]:0")
	if err != nil {
		v4.close()
		return nil, err
	}
	p := &probePair{v4, v6}
	if v4.baseline(ctx, "tcp4", 1) != nil || v6.baseline(ctx, "tcp6", 1) != nil {
		p.close()
		return nil, ErrUnsafe
	}
	return p, nil
}

func (p *probePair) close() { p.v4.close(); p.v6.close() }
func (p *probePair) denied() bool {
	return !p.v4.failed.Load() && !p.v6.failed.Load() && p.v4.count.Load() == 1 && p.v6.count.Load() == 1
}
func (p *probePair) finish(ctx context.Context) error {
	if !p.denied() || p.v4.baseline(ctx, "tcp4", 2) != nil || p.v6.baseline(ctx, "tcp6", 2) != nil {
		return ErrUnsafe
	}
	p.close()
	if p.v4.failed.Load() || p.v6.failed.Load() || p.v4.count.Load() != 2 || p.v6.count.Load() != 2 {
		return ErrUnsafe
	}
	return nil
}
