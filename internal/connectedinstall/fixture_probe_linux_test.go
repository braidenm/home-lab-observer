//go:build linux

package connectedinstall

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// This test is compiled into a separately pinned fixture-only test executable.
// Ordinary package tests skip it; the no-NIC disposable guest alone opts in.
var vmReadonlyProbe = flag.Bool("hlo-vm-readonly-probe", false, "run fixed disposable-VM prerequisite diagnostics")

// The fixture build embeds the independently reviewed source commit with -X.
// A generic package-test binary without this binding cannot run the VM probe.
var vmSourceCommit string

const vmBundle = "/opt/observer-fixture/bundle/release"
const vmDigestFile = "/var/lib/hlo-first-install-fixture/manifest-sha"
const vmServerID = "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func vmMarker(t *testing.T, marker string) {
	t.Helper()
	serial, err := os.OpenFile("/dev/ttyS0", os.O_WRONLY, 0)
	if err != nil {
		t.FailNow()
	}
	_, err = fmt.Fprintln(serial, marker)
	closeErr := serial.Close()
	if err != nil || closeErr != nil {
		t.FailNow()
	}
}

func vmRefuse(t *testing.T, stage string) {
	t.Helper()
	vmMarker(t, vmFailureMarker(stage))
	t.FailNow()
}

func vmFailureMarker(stage string) string {
	switch stage {
	case "INPUT", "BUNDLE", "HOST", "TARGETS", "CA", "GO_DNS", "ADDRESS", "GO_TLS", "RESOLVE_PARITY", "PARENT", "CHECKREQUEST":
		return "HLO_VM_FAIL_EXACT_" + stage
	default:
		return "HLO_VM_FAIL_EXACT_UNKNOWN"
	}
}

func TestVMFailureMarkerAllowlist(t *testing.T) {
	if got := vmFailureMarker("hle_private"); got != "HLO_VM_FAIL_EXACT_UNKNOWN" {
		t.Fatal("probe marker accepted unrecognized text")
	}
	if got := vmFailureMarker("BUNDLE"); got != "HLO_VM_FAIL_EXACT_BUNDLE" {
		t.Fatal("probe marker rejected fixed stage")
	}
	for _, stage := range []string{"CA", "GO_DNS", "ADDRESS", "GO_TLS", "RESOLVE_PARITY"} {
		if got := vmFailureMarker(stage); got != "HLO_VM_FAIL_EXACT_"+stage {
			t.Fatal("probe marker rejected fixed network stage")
		}
	}
}

// These diagnostics mirror the production read-only dependencies immediately
// before calling Resolve unchanged. They are not an alternate admission path.
func vmDNSStages(t *testing.T, ctx context.Context) []string {
	t.Helper()
	ca, err := connectedprofile.ReadRootFile("/etc/ssl/certs/ca-certificates.crt", 1024*1024)
	if err != nil {
		vmRefuse(t, "CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		vmRefuse(t, "CA")
	}
	lookupContext, stopLookup := context.WithTimeout(ctx, 5*time.Second)
	resolver := &net.Resolver{PreferGo: true}
	ips, err := resolver.LookupNetIP(lookupContext, "ip", connectedprofile.Hostname+".")
	stopLookup()
	if err != nil {
		vmRefuse(t, "GO_DNS")
	}
	if len(ips) == 0 || len(ips) > 8 {
		vmRefuse(t, "ADDRESS")
	}
	unique := map[netip.Addr]bool{}
	for _, ip := range ips {
		if !connectedprofile.PublicAddress(ip) {
			vmRefuse(t, "ADDRESS")
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
		attempt, stop := context.WithTimeout(ctx, 3*time.Second)
		dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 3 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: connectedprofile.Hostname, RootCAs: roots}}
		conn, dialErr := dialer.DialContext(attempt, "tcp", net.JoinHostPort(raw, "443"))
		if dialErr == nil {
			closeErr := conn.Close()
			stop()
			if closeErr == nil {
				accepted = append(accepted, raw)
			}
		} else {
			stop()
		}
	}
	if ctx.Err() != nil || len(accepted) == 0 {
		vmRefuse(t, "GO_TLS")
	}
	return accepted
}

func TestVMReadonlyPreflightStages(t *testing.T) {
	if !*vmReadonlyProbe {
		t.Skip("manual no-NIC disposable VM only")
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		vmRefuse(t, "INPUT")
	}
	digestBytes, err := connectedprofile.ReadRootFile(vmDigestFile, 65)
	if err != nil || len(digestBytes) != 65 || digestBytes[64] != '\n' {
		vmRefuse(t, "INPUT")
	}
	digest := strings.TrimSuffix(string(digestBytes), "\n")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	manifest, err := connectedbundle.VerifyDirectory(vmBundle, digest)
	if err != nil || len(vmSourceCommit) != 40 || manifest.Commit != vmSourceCommit {
		vmRefuse(t, "BUNDLE")
	}
	if preflight(ctx) != nil {
		vmRefuse(t, "HOST")
	}
	if absentTargets(ctx) != nil {
		vmRefuse(t, "TARGETS")
	}
	verified := vmDNSStages(t, ctx)
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil || !slices.Equal(addresses, verified) {
		vmRefuse(t, "RESOLVE_PARITY")
	}
	if connectedpolicy.ValidateParent(ctx, addresses) != nil {
		vmRefuse(t, "PARENT")
	}
	if CheckRequest(ctx, vmBundle, digest, vmServerID) != nil {
		vmRefuse(t, "CHECKREQUEST")
	}
	vmMarker(t, "HLO_VM_EXACT_PREFLIGHT_PASS")
}
