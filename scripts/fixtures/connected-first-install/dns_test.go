package main

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var errFixtureStop = errors.New("stop fixture packet source")

type scriptedPacketConn struct {
	packet []byte
	remote net.Addr
	reads  int
	writes int
}

func (c *scriptedPacketConn) ReadFrom(target []byte) (int, net.Addr, error) {
	if c.reads > 0 {
		return 0, nil, errFixtureStop
	}
	c.reads++
	return copy(target, c.packet), c.remote, nil
}
func (c *scriptedPacketConn) WriteTo(packet []byte, _ net.Addr) (int, error) {
	c.writes++
	return len(packet), nil
}
func (*scriptedPacketConn) Close() error { return nil }
func (*scriptedPacketConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53}
}
func (*scriptedPacketConn) SetDeadline(time.Time) error      { return nil }
func (*scriptedPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (*scriptedPacketConn) SetWriteDeadline(time.Time) error { return nil }

type countedPacketConn struct {
	net.PacketConn
	queries    atomic.Int64
	replies    atomic.Int64
	flags      atomic.Int64
	additional atomic.Int64
	length     atomic.Int64
}

func (c *countedPacketConn) ReadFrom(packet []byte) (int, net.Addr, error) {
	n, address, err := c.PacketConn.ReadFrom(packet)
	if err == nil {
		c.queries.Add(1)
		c.flags.Store(int64(binary.BigEndian.Uint16(packet[2:4])))
		c.additional.Store(int64(binary.BigEndian.Uint16(packet[10:12])))
		c.length.Store(int64(n))
	}
	return n, address, err
}

func (c *countedPacketConn) WriteTo(packet []byte, address net.Addr) (int, error) {
	n, err := c.PacketConn.WriteTo(packet, address)
	if err == nil {
		c.replies.Add(1)
	}
	return n, err
}

func fixtureDNSQuery(name string, kind uint16, edns bool) []byte {
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[0:2], 0x1234)
	binary.BigEndian.PutUint16(packet[2:4], 0x0120) // Go requests RD and AD; neither is granted.
	binary.BigEndian.PutUint16(packet[4:6], 1)
	for _, label := range strings.Split(name, ".") {
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	packet = append(packet, 0, byte(kind>>8), byte(kind), 0, 1)
	if edns {
		binary.BigEndian.PutUint16(packet[10:12], 1)
		packet = append(packet, 0, 0, 41, 0x04, 0xd0, 0, 0, 0, 0, 0, 0) // Empty OPT, UDP 1232.
	}
	return packet
}

func TestFixtureDNSClosedProtocol(t *testing.T) {
	for _, test := range []struct {
		name, query string
		kind        uint16
		edns        bool
		rcode       uint16
		answers     uint16
	}{
		{"fixed A", dnsHostname, 1, false, 0, 1},
		{"fixed A with EDNS", strings.ToUpper(dnsHostname), 1, true, 0, 1},
		{"AAAA NODATA", dnsHostname, 28, true, 0, 0},
		{"other name refused", "other.example", 1, false, 5, 0},
		{"other type refused", dnsHostname, 16, false, 5, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := dnsReply(fixtureDNSQuery(test.query, test.kind, test.edns))
			if response == nil || binary.BigEndian.Uint16(response[0:2]) != 0x1234 ||
				binary.BigEndian.Uint16(response[2:4])&0x8000 == 0 ||
				binary.BigEndian.Uint16(response[2:4])&0x0080 != 0 ||
				binary.BigEndian.Uint16(response[2:4])&0xf != test.rcode ||
				binary.BigEndian.Uint16(response[6:8]) != test.answers ||
				binary.BigEndian.Uint16(response[8:10]) != 0 ||
				binary.BigEndian.Uint16(response[10:12]) != 0 {
				t.Fatal("fixture DNS response violated closed protocol")
			}
			if test.answers == 1 {
				if len(response) < 16 || string(response[len(response)-4:]) != string(dnsAlias[:]) {
					t.Fatal("fixture DNS A answer changed")
				}
			}
		})
	}
}

func TestFixtureDNSDropsMalformedAndOversizedPackets(t *testing.T) {
	valid := fixtureDNSQuery(dnsHostname, 1, true)
	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"empty", func(_ []byte) []byte { return nil }},
		{"oversized", func(p []byte) []byte { return append(p, make([]byte, dnsMaxPacket+1-len(p))...) }},
		{"compression pointer", func(p []byte) []byte { p[12] = 0xc0; return p }},
		{"multiple questions", func(p []byte) []byte { p[5] = 2; return p }},
		{"response bit", func(p []byte) []byte { p[2] = 0x81; return p }},
		{"OPT payload", func(p []byte) []byte { p[len(p)-1] = 1; return p }},
		{"trailing bytes", func(p []byte) []byte { return append(p, 0) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet := test.mutate(append([]byte(nil), valid...))
			if dnsReply(packet) != nil {
				t.Fatal("malformed fixture DNS query received a response")
			}
		})
	}
}

func TestFixtureDNSServesOnlyLoopbackSources(t *testing.T) {
	for _, test := range []struct {
		name   string
		ip     net.IP
		writes int
	}{
		{"loopback", net.IPv4(127, 0, 0, 1), 1},
		{"nonloopback alias", net.IPv4(93, 184, 216, 34), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn := &scriptedPacketConn{
				packet: fixtureDNSQuery(dnsHostname, 1, true),
				remote: &net.UDPAddr{IP: test.ip, Port: 12345},
			}
			if err := serveDNS(conn); !errors.Is(err, errFixtureStop) || conn.writes != test.writes {
				t.Fatal("fixture DNS source admission changed")
			}
		})
	}
}

func TestFixtureDNSGoResolverReturnsNativeIPv4(t *testing.T) {
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	tracked := &countedPacketConn{PacketConn: listener}
	done := make(chan struct{})
	go func() {
		_ = serveDNS(tracked)
		close(done)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp4", listener.LocalAddr().String())
	}}
	addresses, err := resolver.LookupNetIP(ctx, "ip", dnsHostname+".")
	if err != nil || len(addresses) != 1 || addresses[0] != netip.AddrFrom4(dnsAlias) {
		t.Fatalf("Go resolver did not receive only the native IPv4 alias: error=%t count=%d queries=%d replies=%d flags=%x additional=%d length=%d", err != nil, len(addresses), tracked.queries.Load(), tracked.replies.Load(), tracked.flags.Load(), tracked.additional.Load(), tracked.length.Load())
	}
	if tracked.queries.Load() == 0 || tracked.replies.Load() == 0 {
		t.Fatal("Go resolver bypassed the fixture DNS responder")
	}
	ipv6, err := resolver.LookupNetIP(ctx, "ip6", dnsHostname+".")
	if err == nil || len(ipv6) != 0 {
		t.Fatal("fixture DNS AAAA query did not return NODATA")
	}
	listener.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fixture DNS responder did not stop")
	}
}
