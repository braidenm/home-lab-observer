package connectedpolicy

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const uploader = "home-lab-observer-connected-uploader.service"

var addresses = []string{"93.184.216.34"}

func sample(id, parent, allow, deny string) []byte {
	return []byte(fmt.Sprintf("Slice=%s\nIPAddressAllow=%s\nIPAddressDeny=%s\nId=%s\nLoadState=loaded\nDropInPaths=\nNeedDaemonReload=no\n", parent, allow, deny, id))
}
func topology() map[string][]byte {
	return map[string][]byte{
		uploader:       sample(uploader, "system.slice", "93.184.216.34/32", "::/0 0.0.0.0/0"),
		"system.slice": sample("system.slice", "-.slice", "", ""),
		"-.slice":      sample("-.slice", "", "", ""),
	}
}
func reader(m map[string][]byte) query {
	return func(_ context.Context, name string) ([]byte, error) {
		b, ok := m[name]
		if !ok {
			return nil, ErrUnsafe
		}
		return b, nil
	}
}
func TestActualManagerShapeAndParents(t *testing.T) {
	m := topology()
	if validate(context.Background(), uploader, addresses, true, reader(m)) != nil {
		t.Fatal("observed255 manager shape refused")
	}
	m["system.slice"] = sample("system.slice", "-.slice", "93.184.216.34/32", "")
	if validate(context.Background(), uploader, addresses, true, reader(m)) != nil {
		t.Fatal("exact inherited allow refused")
	}
	delete(m, uploader)
	if validate(context.Background(), "system.slice", addresses, false, reader(m)) != nil {
		t.Fatal("pre-unit ancestor check refused")
	}
}
func TestRejectPolicyWideningAndUncertainManagerState(t *testing.T) {
	for name, mutate := range map[string]func(map[string][]byte){
		"inherited any":     func(m map[string][]byte) { m["-.slice"] = sample("-.slice", "", "0.0.0.0/0", "") },
		"inherited private": func(m map[string][]byte) { m["system.slice"] = sample("system.slice", "-.slice", "127.0.0.1/32", "") },
		"broader subnet": func(m map[string][]byte) {
			m[uploader] = sample(uploader, "system.slice", "93.184.216.0/24", "::/0 0.0.0.0/0")
		},
		"missing deny v6": func(m map[string][]byte) {
			m[uploader] = sample(uploader, "system.slice", "93.184.216.34/32", "0.0.0.0/0")
		},
		"missing allow": func(m map[string][]byte) { m[uploader] = sample(uploader, "system.slice", "", "::/0 0.0.0.0/0") },
		"dropin": func(m map[string][]byte) {
			m[uploader] = []byte(strings.ReplaceAll(string(m[uploader]), "DropInPaths=\n", "DropInPaths=/etc/example.conf\n"))
		},
		"pending reload": func(m map[string][]byte) {
			m["system.slice"] = []byte(strings.ReplaceAll(string(m["system.slice"]), "NeedDaemonReload=no", "NeedDaemonReload=yes"))
		},
		"cycle":            func(m map[string][]byte) { m["system.slice"] = sample("system.slice", "system.slice", "", "") },
		"missing ancestor": func(m map[string][]byte) { delete(m, "-.slice") },
		"aliased identity": func(m map[string][]byte) {
			m[uploader] = sample("other.service", "system.slice", "93.184.216.34/32", "::/0 0.0.0.0/0")
		},
		"symbolic allow": func(m map[string][]byte) {
			m[uploader] = sample(uploader, "system.slice", "localhost", "::/0 0.0.0.0/0")
		},
		"duplicate property": func(m map[string][]byte) { m[uploader] = append(m[uploader], []byte("IPAddressDeny=\n")...) },
		"oversize":           func(m map[string][]byte) { m[uploader] = []byte(strings.Repeat(" ", maxOutput+1)) },
	} {
		t.Run(name, func(t *testing.T) {
			m := topology()
			mutate(m)
			if validate(context.Background(), uploader, addresses, true, reader(m)) != ErrUnsafe {
				t.Fatal("unsafe policy accepted")
			}
		})
	}
}
func TestClosedInputsAndCanceledValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, ctx} {
		if validate(ctx, uploader, addresses, true, reader(topology())) != ErrUnsafe {
			t.Fatal("invalid context accepted")
		}
	}
	for _, bad := range []string{"sshd.service", "--host=example.invalid", "system.slice", "home-lab-observer-connected-collector.service"} {
		if validate(context.Background(), bad, addresses, true, reader(topology())) != ErrUnsafe {
			t.Fatal("unowned service accepted")
		}
	}
	for _, bad := range [][]string{nil, {"127.0.0.1"}, {"93.184.216.34", "93.184.216.34"}, {"93.184.216.34", "2606:4700::1111"}} {
		if validate(context.Background(), uploader, bad, true, reader(topology())) != ErrUnsafe {
			t.Fatal("invalid expected addresses accepted")
		}
	}
}
