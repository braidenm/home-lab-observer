//go:build linux

package connectedpolicy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/netip"
	"os/exec"
	"slices"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// ValidateEffective rejects drift in an already-loaded owned service and every
// ancestor. Call only after installing verified units and daemon-reload, before
// starting a worker; recheck during explicit start/refresh. Never edits a slice.
func ValidateEffective(ctx context.Context, unit string, addresses []string) error {
	return validate(ctx, unit, addresses, true, managerQuery)
}

// ValidateParent preflights ancestors before an owned unit exists.
func ValidateParent(ctx context.Context, addresses []string) error {
	return validate(ctx, "system.slice", addresses, false, managerQuery)
}

type boundedOutput struct{ buffer bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > maxOutput-b.buffer.Len() {
		return 0, ErrUnsafe
	}
	return b.buffer.Write(p)
}

func managerQuery(ctx context.Context, unit string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--no-pager", "--property=Id", "--property=Slice", "--property=IPAddressAllow", "--property=IPAddressDeny", "--property=DropInPaths", "--property=LoadState", "--property=NeedDaemonReload", "--", unit)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat", "SYSTEMD_PAGERSECURE=1"}
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if cmd.Run() != nil || ctx.Err() != nil {
		return nil, ErrUnsafe
	}
	return output.buffer.Bytes(), nil
}

// Resolve authenticates reachable addresses of the single compiled hostname.
// No HTTP request or credential is sent. Only the fixed root-owned CA bundle is
// trusted: SSL_CERT_FILE, proxies and other inherited network settings are not
// authority. An unreachable IPv6 result does not exclude a proven IPv4 address.
func Resolve(ctx context.Context) ([]string, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ca, err := connectedprofile.ReadRootFile("/etc/ssl/certs/ca-certificates.crt", 1024*1024)
	if err != nil {
		return nil, ErrUnsafe
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, ErrUnsafe
	}
	lookupContext, stopLookup := context.WithTimeout(ctx, 5*time.Second)
	resolver := &net.Resolver{PreferGo: true}
	ips, err := resolver.LookupNetIP(lookupContext, "ip", connectedprofile.Hostname+".")
	stopLookup()
	if err != nil {
		return nil, ErrUnsafe
	}
	return resolveAddresses(ctx, ips, func(ctx context.Context, a netip.Addr) error {
		attempt, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 3 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: connectedprofile.Hostname, RootCAs: roots}}
		conn, err := dialer.DialContext(attempt, "tcp", net.JoinHostPort(a.String(), "443"))
		if err != nil {
			return ErrUnsafe
		}
		if conn.Close() != nil {
			return ErrUnsafe
		}
		return nil
	})
}

func resolveAddresses(ctx context.Context, ips []netip.Addr, verify func(context.Context, netip.Addr) error) ([]string, error) {
	if len(ips) == 0 || len(ips) > 8 || verify == nil {
		return nil, ErrUnsafe
	}
	unique := make(map[netip.Addr]bool, len(ips))
	for _, ip := range ips {
		if !connectedprofile.PublicAddress(ip) {
			return nil, ErrUnsafe
		}
		unique[ip] = true
	}
	ordered := make([]string, 0, len(unique))
	for ip := range unique {
		ordered = append(ordered, ip.String())
	}
	slices.Sort(ordered)
	accepted := make([]string, 0, len(ordered))
	for _, raw := range ordered {
		if ctx.Err() != nil {
			return nil, ErrUnsafe
		}
		if verify(ctx, netip.MustParseAddr(raw)) == nil {
			accepted = append(accepted, raw)
		}
	}
	if ctx.Err() != nil || len(accepted) == 0 {
		return nil, ErrUnsafe
	}
	return accepted, nil
}
