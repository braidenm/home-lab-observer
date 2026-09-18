package main

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// This is a deliberately tiny, non-recursive DNS responder for the disposable
// no-NIC guest. It never reads a host resolver or forwards a query.
const dnsHostname = connectedprofile.Hostname
const dnsMaxPacket = 512

var dnsAlias = [4]byte{93, 184, 216, 34}

func dnsReply(packet []byte) []byte {
	if len(packet) < 17 || len(packet) > dnsMaxPacket {
		return nil
	}
	flags := binary.BigEndian.Uint16(packet[2:4])
	questions := binary.BigEndian.Uint16(packet[4:6])
	additional := binary.BigEndian.Uint16(packet[10:12])
	// Go's resolver sets RD and may set AD in a query. Neither grants
	// recursion or authenticated-data status in this synthetic response.
	if flags&^0x0120 != 0 || questions != 1 || binary.BigEndian.Uint16(packet[6:8]) != 0 ||
		binary.BigEndian.Uint16(packet[8:10]) != 0 || additional > 1 {
		return nil
	}
	name, end, ok := dnsQuestionName(packet)
	if !ok || end+4 > len(packet) {
		return nil
	}
	questionEnd := end + 4
	qtype := binary.BigEndian.Uint16(packet[end : end+2])
	qclass := binary.BigEndian.Uint16(packet[end+2 : questionEnd])
	if additional == 0 {
		if questionEnd != len(packet) {
			return nil
		}
	} else if !dnsEmptyOPT(packet[questionEnd:]) {
		return nil
	}

	response := make([]byte, questionEnd)
	copy(response, packet[:questionEnd])
	responseFlags := uint16(0x8400) | (flags & 0x0100) // QR, authoritative, no recursion available.
	answerA := name == dnsHostname && qclass == 1 && qtype == 1
	if name != dnsHostname || qclass != 1 || (qtype != 1 && qtype != 28) {
		responseFlags |= 5 // REFUSED: no other name, class or type is served.
	}
	binary.BigEndian.PutUint16(response[2:4], responseFlags)
	binary.BigEndian.PutUint16(response[6:8], 0)
	binary.BigEndian.PutUint16(response[8:10], 0)
	binary.BigEndian.PutUint16(response[10:12], 0)
	if answerA {
		binary.BigEndian.PutUint16(response[6:8], 1)
		response = append(response,
			0xc0, 0x0c, // Answer name points to the already checked question.
			0, 1, 0, 1, // A, IN.
			0, 0, 0, 0, // Zero TTL avoids carrying fixture data into a later case.
			0, 4, dnsAlias[0], dnsAlias[1], dnsAlias[2], dnsAlias[3],
		)
	}
	return response
}

func dnsQuestionName(packet []byte) (string, int, bool) {
	position := 12
	var labels []string
	nameLength := 0
	for {
		if position >= len(packet) {
			return "", 0, false
		}
		length := int(packet[position])
		position++
		if length == 0 {
			break
		}
		// Compression pointers and extended label forms are unnecessary for a
		// fixed client question and are refused rather than followed.
		if length > 63 || position+length > len(packet) || nameLength+length+1 > 253 {
			return "", 0, false
		}
		label := packet[position : position+length]
		for _, char := range label {
			if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') &&
				!(char >= '0' && char <= '9') && char != '-' {
				return "", 0, false
			}
		}
		labels = append(labels, strings.ToLower(string(label)))
		nameLength += length + 1
		position += length
	}
	if len(labels) == 0 {
		return "", 0, false
	}
	return strings.Join(labels, "."), position, true
}

// Go may attach a zero-data EDNS0 OPT. Accept only that bounded extension and
// omit it from the answer. A query carrying other records is dropped.
func dnsEmptyOPT(packet []byte) bool {
	if len(packet) != 11 || packet[0] != 0 || binary.BigEndian.Uint16(packet[1:3]) != 41 {
		return false
	}
	udpSize := binary.BigEndian.Uint16(packet[3:5])
	return udpSize >= dnsMaxPacket && udpSize <= 4096 &&
		binary.BigEndian.Uint32(packet[5:9]) == 0 && binary.BigEndian.Uint16(packet[9:11]) == 0
}

func serveDNS(conn net.PacketConn) error {
	var packet [dnsMaxPacket + 1]byte
	for {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}
		length, remote, err := conn.ReadFrom(packet[:])
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			continue
		}
		if err != nil {
			return err
		}
		address, ok := remote.(*net.UDPAddr)
		if !ok || !address.IP.IsLoopback() {
			continue
		}
		reply := dnsReply(packet[:length])
		if reply == nil {
			continue
		}
		if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}
		written, err := conn.WriteTo(reply, remote)
		if err != nil {
			return err
		}
		if written != len(reply) {
			return errors.New("short fixture DNS write")
		}
	}
}
