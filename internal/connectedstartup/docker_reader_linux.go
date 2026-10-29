//go:build linux

package connectedstartup

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedruntime"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

// RunRichProcessesDockerReader pauses this exact reader before Engine I/O or
// frame publication. Root must independently audit its response/PID/invocation,
// namespace, closed FDs and pinned socket before authoring the matching commit.
func RunRichProcessesDockerReader(parent context.Context, c connectedprofile.RichProcessesDockerConfig, status *connectedstatus.Writer) error {
	if parent == nil || parent.Err() != nil || status == nil || connectedprofile.CheckRichProcessesDockerReaderIdentity(c) != nil {
		return ErrUnsafe
	}
	configuration, err := dockerReaderConfiguration(c)
	if err != nil {
		return ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	fd, err := unix.Open("/activation", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrUnsafe
	}
	directory := os.NewFile(uintptr(fd), "reader-activation")
	defer directory.Close()
	if !rootDirectory(directory, c.FrameGID) {
		return ErrUnsafe
	}
	current := func() ([]byte, error) {
		now, err := connectedprofile.LoadRichProcessesDockerConfig()
		if err != nil || connectedprofile.CheckRichProcessesDockerReaderIdentity(now) != nil {
			return nil, ErrUnsafe
		}
		return dockerReaderConfiguration(now)
	}
	return rendezvousDockerReader(ctx, configuration, c.ArtifactSHA256, c.PolicyGeneration, boundStartupPorts{
		read: func(ctx context.Context, name string) ([]byte, error) {
			return waitRecord(ctx, directory, name, c.FrameGID)
		},
		prove: func(context.Context, connectedactivation.Request) error {
			now, err := current()
			if err != nil || !bytes.Equal(now, configuration) || connectedruntime.CheckRichProcessesDockerReaderView(c) != nil || connectedruntime.CheckRichProcessesDockerReaderPrimitives() != nil {
				return ErrUnsafe
			}
			return nil
		}, publish: status.WriteActivationResponse, current: current, random: rand.Reader, invocation: os.Getenv("INVOCATION_ID"),
	})
}

func dockerReaderConfiguration(c connectedprofile.RichProcessesDockerConfig) ([]byte, error) {
	if _, err := dockerConfiguration(c); err != nil {
		return nil, ErrUnsafe
	}
	return connectedprofile.EncodeRichProcessesDockerReaderStartup(c)
}

// The unchanged codec requires port fields. Reader startup fixes both to 1024;
// it never interprets them as destinations or performs any network handshake.
func rendezvousDockerReader(ctx context.Context, configuration []byte, artifact string, generation uint64, p boundStartupPorts) error {
	prove := p.prove
	if prove == nil {
		return ErrUnsafe
	}
	p.prove = func(ctx context.Context, r connectedactivation.Request) error {
		if r.IPv4Port != 1024 || r.IPv6Port != 1024 {
			return ErrUnsafe
		}
		return prove(ctx, r)
	}
	return rendezvousBound(ctx, configuration, artifact, generation, p)
}
