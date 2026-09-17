//go:build linux

package connectedcredential

import (
	"encoding/binary"
	"testing"
)

func TestExactManagerACL(t *testing.T) {
	for _, permission := range []uint16{4, 5} {
		b := make([]byte, 44)
		binary.LittleEndian.PutUint32(b, 2)
		for i, tag := range []uint16{1, 2, 4, 16, 32} {
			e := b[4+i*8 : 12+i*8]
			binary.LittleEndian.PutUint16(e, tag)
			p := uint16(0)
			if i == 0 || i == 1 || i == 3 {
				p = permission
			}
			binary.LittleEndian.PutUint16(e[2:], p)
			id := ^uint32(0)
			if i == 1 {
				id = 60102
			}
			binary.LittleEndian.PutUint32(e[4:], id)
		}
		if !exactACL(b, 60102, permission) {
			t.Fatal("manager ACL refused")
		}
		for i := range b {
			c := append([]byte(nil), b...)
			c[i] ^= 1
			if exactACL(c, 60102, permission) {
				t.Fatal("changed ACL accepted")
			}
		}
		if exactACL(append(b, 0), 60102, permission) || exactACL(b, 60103, permission) {
			t.Fatal("extra grant or foreign UID accepted")
		}
	}
}
