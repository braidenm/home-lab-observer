//go:build linux

package dockerreader

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedruntime"
	"github.com/braidenm/home-lab-observer/internal/connectedstartup"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/containerobs"
	"github.com/braidenm/home-lab-observer/internal/sharedhandoff"
)

// RunInstalled has no caller-selected endpoint, path, listener or executable.
// Root audits the paused process before the existing startup handshake permits
// any Engine I/O. The uploader credential is not present in this namespace.
func RunInstalled(ctx context.Context) (result error) {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	c, err := connectedprofile.LoadRichProcessesDockerConfig()
	if err != nil {
		return ErrUnsafe
	}
	policy, err := connectedprofile.LoadRichProcessesDockerPolicy()
	if err != nil || connectedprofile.VerifyRichProcessesDockerBinding(c, policy) != nil {
		return ErrUnsafe
	}
	digest, err := connectedprofile.DigestRichProcessesDockerPolicy(policy)
	if err != nil {
		return ErrUnsafe
	}
	configuration, err := connectedprofile.EncodeRichProcessesDockerConfig(c)
	if err != nil {
		return ErrUnsafe
	}
	socket, err := captureSocket()
	if err != nil || socket.group != c.SocketGID {
		return ErrUnsafe
	}
	guard := func() error {
		current, err := connectedprofile.LoadRichProcessesDockerConfig()
		if err != nil {
			return ErrUnsafe
		}
		encoded, err := connectedprofile.EncodeRichProcessesDockerConfig(current)
		if err != nil || !bytes.Equal(encoded, configuration) {
			return ErrUnsafe
		}
		p, err := connectedprofile.LoadRichProcessesDockerPolicy()
		if err != nil || connectedprofile.VerifyRichProcessesDockerBinding(current, p) != nil {
			return ErrUnsafe
		}
		actual, err := connectedprofile.DigestRichProcessesDockerPolicy(p)
		if err != nil || actual != digest || connectedprofile.CheckRichProcessesDockerReaderIdentity(c) != nil || connectedruntime.CheckRichProcessesDockerReaderView(c) != nil || socket.check() != nil {
			return ErrUnsafe
		}
		return nil
	}
	if guard() != nil {
		return ErrUnsafe
	}
	status, err := connectedstatus.Open("/state/status")
	if err != nil {
		return ErrUnsafe
	}
	defer func() {
		if status.Close() != nil {
			result = ErrUnsafe
		}
	}()
	// No adapter or frame writer exists before root's same-process commit.
	if connectedstartup.RunRichProcessesDockerReader(ctx, c, status) != nil || guard() != nil {
		return ErrUnsafe
	}
	definitionBytes, err := hex.DecodeString(c.DefinitionSHA256)
	if err != nil || len(definitionBytes) != 32 {
		return ErrUnsafe
	}
	var definition [32]byte
	copy(definition[:], definitionBytes)
	writer, err := sharedhandoff.OpenDockerFrameWriter("/frame", sharedhandoff.DockerFramePolicy{
		Policy:           sharedhandoff.Policy{CollectorUID: c.ReaderUID, UploaderUID: c.CollectorUID, SharedGID: c.FrameGID, ServerID: c.ServerID},
		DefinitionDigest: definition, PolicyDigest: digest,
	})
	if err != nil {
		return ErrUnsafe
	}
	defer func() {
		if writer.Close() != nil {
			result = ErrUnsafe
		}
	}()
	adapter, err := containerobs.New(containerobs.Config{Endpoint: "unix:///engine/docker.sock"})
	if err != nil {
		return ErrUnsafe
	}
	defer func() {
		if adapter.Close() != nil {
			result = ErrUnsafe
		}
	}()
	return run(ctx, os.Getenv("INVOCATION_ID"), ports{guard: guard, collect: adapter.Collect, close: adapter.Close, publish: writer.Publish, status: status.Write, now: time.Now, wait: wait})
}
