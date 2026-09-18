//go:build linux

// Package connectedstartup gates this same uploader process before secret or
// ledger access. It has no caller-selected URL, listener or arbitrary file API.
package connectedstartup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedruntime"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

var ErrUnsafe = errors.New("connected_startup_refused")

func Run(parent context.Context, c connectedprofile.Config, status *connectedstatus.Writer) error {
	if parent == nil || status == nil || c.Validate() != nil {
		return ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if connectedruntime.CheckPrimitives(false) != nil || connectedruntime.CheckView(false) != nil {
		return ErrUnsafe
	}
	fd, err := unix.Open("/activation", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrUnsafe
	}
	directory := os.NewFile(uintptr(fd), "activation")
	defer directory.Close()
	if !rootDirectory(directory, c.UploaderGID) {
		return ErrUnsafe
	}
	return rendezvous(ctx, c, startupPorts{
		read: func(ctx context.Context, name string) ([]byte, error) {
			return waitRecord(ctx, directory, name, c.UploaderGID)
		},
		prove: func(ctx context.Context, request connectedactivation.Request) error {
			if !deniedLoopback(ctx, "tcp4", "127.0.0.1", request.IPv4Port) || !deniedLoopback(ctx, "tcp6", "::1", request.IPv6Port) || fixedTLS(ctx, c.Addresses) != nil {
				return ErrUnsafe
			}
			return nil
		}, publish: status.WriteActivationResponse, current: connectedprofile.Load, random: rand.Reader, invocation: os.Getenv("INVOCATION_ID"),
	})
}

// Private sequencing seam: production ports above remain fixed and are not a
// user-facing probe API. Tests substitute only synthetic records and outcomes.
type startupPorts struct {
	read       func(context.Context, string) ([]byte, error)
	prove      func(context.Context, connectedactivation.Request) error
	publish    func([]byte) error
	current    func() (connectedprofile.Config, error)
	random     io.Reader
	invocation string
}

func rendezvous(ctx context.Context, c connectedprofile.Config, p startupPorts) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	configuration, err := connectedprofile.Encode(c)
	if err != nil {
		return ErrUnsafe
	}
	digest := sha256.Sum256(configuration)
	// Missing request NEVER authorizes probes, credentials or ordinary work.
	requestBytes, err := p.read(ctx, "request.json")
	if err != nil {
		return ErrUnsafe
	}
	request, err := connectedactivation.DecodeRequest(requestBytes)
	if err != nil || request.ArtifactSHA256 != c.ArtifactSHA256 || request.ConfigSHA256 != hex.EncodeToString(digest[:]) || request.PolicyGeneration != c.PolicyGeneration {
		return ErrUnsafe
	}
	challenge := make([]byte, 32)
	if _, err := io.ReadFull(p.random, challenge); err != nil {
		return ErrUnsafe
	}
	requestDigest, err := connectedactivation.RequestDigest(request)
	if err != nil {
		return ErrUnsafe
	}
	response := connectedactivation.Response{Version: connectedactivation.ResponseVersion, RequestSHA256: requestDigest, InvocationID: p.invocation, Challenge: hex.EncodeToString(challenge), Result: connectedactivation.Pass}
	if _, err := connectedactivation.EncodeResponse(response); err != nil {
		return ErrUnsafe
	}
	if ctx.Err() != nil || p.prove(ctx, request) != nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	// All probe sockets have been closed before root sees PASS. Retain our own
	// response/challenge in memory; never reload response authority from disk.
	encoded, err := connectedactivation.EncodeResponse(response)
	if err != nil || p.publish(encoded) != nil {
		return ErrUnsafe
	}
	commitBytes, err := p.read(ctx, "commit.json")
	if err != nil {
		return ErrUnsafe
	}
	commit, err := connectedactivation.DecodeCommit(commitBytes)
	if err != nil || !connectedactivation.MatchCommit(request, response, commit) || ctx.Err() != nil {
		return ErrUnsafe
	}
	current, err := p.current()
	if err != nil {
		return ErrUnsafe
	}
	currentBytes, err := connectedprofile.Encode(current)
	if err != nil || !bytes.Equal(currentBytes, configuration) || ctx.Err() != nil {
		return ErrUnsafe
	}
	return nil
}

func deniedLoopback(ctx context.Context, network, host string, port uint16) bool {
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	dialer := net.Dialer{Timeout: 2 * time.Second}
	c, err := dialer.DialContext(probe, network, net.JoinHostPort(host, strconv.Itoa(int(port))))
	if c != nil {
		c.Close()
		return false
	}
	var networkError net.Error
	return err != nil && errors.As(err, &networkError) && networkError.Timeout() && ctx.Err() == nil
}

func fixedTLS(ctx context.Context, addresses []string) error {
	ca, err := connectedprofile.ReadRootFile("/etc/ssl/certs/ca-certificates.crt", 1<<20)
	if err != nil {
		return ErrUnsafe
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return ErrUnsafe
	}
	for _, address := range addresses {
		parsed, parseErr := netip.ParseAddr(address)
		if parseErr != nil || !connectedprofile.PublicAddress(parsed) {
			return ErrUnsafe
		}
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		dialer := net.Dialer{Timeout: 3 * time.Second}
		conn, err := dialer.DialContext(attempt, "tcp", net.JoinHostPort(address, "443"))
		if err != nil {
			cancel()
			continue
		}
		deadline, _ := attempt.Deadline()
		if conn.SetDeadline(deadline) != nil {
			conn.Close()
			cancel()
			continue
		}
		secure := tls.Client(conn, &tls.Config{ServerName: connectedprofile.Hostname, MinVersion: tls.VersionTLS12, RootCAs: roots})
		handshakeErr := secure.HandshakeContext(attempt)
		// This handshake-only probe sends no HTTP or application data. Close the
		// underlying socket directly: TLS close_notify may set its own deadline.
		closeErr := conn.Close()
		cancel()
		if handshakeErr == nil && closeErr == nil && ctx.Err() == nil {
			return nil
		}
	}
	return ErrUnsafe
}

func waitRecord(ctx context.Context, directory *os.File, name string, gid uint32) ([]byte, error) {
	for ctx.Err() == nil {
		data, err := readRecord(directory, name, gid)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, ErrUnsafe
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return nil, ErrUnsafe
}

func readRecord(directory *os.File, name string, gid uint32) ([]byte, error) {
	fd, err := unix.Openat(int(directory.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		return nil, ErrUnsafe
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	if !ok || !i.Mode().IsRegular() || st.Uid != 0 || st.Gid != gid || st.Nlink != 1 || i.Mode().Perm() != 0640 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || i.Size() < 1 || i.Size() > connectedactivation.MaxBytes || !noACL(fd) {
		return nil, ErrUnsafe
	}
	data, err := io.ReadAll(io.LimitReader(f, connectedactivation.MaxBytes+1))
	if err != nil || len(data) > connectedactivation.MaxBytes {
		return nil, ErrUnsafe
	}
	return data, nil
}

func rootDirectory(f *os.File, gid uint32) bool {
	i, err := f.Stat()
	if err != nil {
		return false
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	return ok && i.IsDir() && st.Uid == 0 && st.Gid == gid && i.Mode().Perm() == 0750 && i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0 && noACL(int(f.Fd()))
}

func noACL(fd int) bool {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		n, err := unix.Fgetxattr(fd, name, nil)
		if err == unix.ENODATA || err == unix.ENOTSUP {
			continue
		}
		if err != nil || n != 0 {
			return false
		}
	}
	return true
}
