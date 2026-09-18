//go:build linux

package connectedpolicy

import (
	"context"
	"io"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func TestAuthenticatedReachableSubset(t *testing.T) {
	v4 := netip.MustParseAddr("93.184.216.34")
	v6 := netip.MustParseAddr("2606:4700::1111")
	result, err := resolveAddresses(context.Background(), []netip.Addr{v4, v6, v4}, func(_ context.Context, a netip.Addr) error {
		if a == v6 {
			return ErrUnsafe
		}
		return nil
	})
	if err != nil || !slices.Equal(result, []string{v4.String()}) {
		t.Fatal("reachable authenticated IPv4 subset lost")
	}
	calls := 0
	if _, err := resolveAddresses(context.Background(), []netip.Addr{v4, netip.MustParseAddr("127.0.0.1")}, func(context.Context, netip.Addr) error { calls++; return nil }); err != ErrUnsafe || calls != 0 {
		t.Fatal("private DNS result reached verifier")
	}
	if _, err := resolveAddresses(context.Background(), []netip.Addr{v4}, func(context.Context, netip.Addr) error { return ErrUnsafe }); err != ErrUnsafe {
		t.Fatal("unauthenticated endpoint accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolveAddresses(ctx, []netip.Addr{v4}, func(context.Context, netip.Addr) error { return nil }); err != ErrUnsafe {
		t.Fatal("canceled resolver accepted")
	}
}
func TestManagerOutputRemainsBoundedThroughIOCopy(t *testing.T) {
	var output boundedOutput
	_, err := io.Copy(&output, strings.NewReader(strings.Repeat("x", maxOutput+1)))
	if err != ErrUnsafe || output.buffer.Len() > maxOutput {
		t.Fatal("manager output escaped bound")
	}
}
