//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedenroll"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"github.com/braidenm/home-lab-observer/internal/ledgerwitness"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const collectorUnit = "home-lab-observer-connected-collector.service"
const uploaderUnit = "home-lab-observer-connected-uploader.service"
const enrollmentUnit = "home-lab-observer-connected-enrollment.service"

type boundedOutput struct {
	data  []byte
	limit int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-len(b.data) {
		return 0, ErrUnsafe
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

// command has no shell, inherited input/environment or unbounded child output.
// Callers select only fixed executable paths and typed literal argument vectors.
func command(ctx context.Context, path string, args []string, input []byte, limit int) ([]byte, error) {
	child, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := exec.CommandContext(child, path, args...)
	c.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	c.Stdin = bytes.NewReader(input)
	b := &boundedOutput{limit: limit}
	c.Stdout = b
	c.Stderr = io.Discard
	c.WaitDelay = time.Second
	if err := c.Run(); err != nil {
		clear(b.data)
		return nil, ErrRecovery
	}
	return b.data, nil
}

func service(ctx context.Context, verb, unit string) error {
	if unit != collectorUnit && unit != uploaderUnit && unit != enrollmentUnit {
		return ErrUnsafe
	}
	switch verb {
	case "start", "stop", "enable", "disable":
	default:
		return ErrUnsafe
	}
	_, err := command(ctx, "/usr/bin/systemctl", []string{verb, unit}, nil, 1024)
	return err
}

func runEnrollment(ctx context.Context, properties []string, executable, mode string, input []byte, policy connectedunits.EnrollmentInput) ([]byte, error) {
	return runEnrollmentWithPreflight(ctx, properties, executable, mode, input, policy, enrollmentState)
}

// The private reader seam proves invalid requests are rejected before querying
// the manager. Production always supplies the fixed enrollmentState reader.
func runEnrollmentWithPreflight(ctx context.Context, properties []string, executable, mode string, input []byte, policy connectedunits.EnrollmentInput, beforeState func(context.Context) (map[string]string, error)) ([]byte, error) {
	if executable != "/bin/observer-connected-uploader" || (mode != "enroll" && mode != "validate-enrollment" && mode != "validate-ledger" && mode != "validate-existing-ledger") || len(properties) > 64 || len(input) > 512 {
		return nil, ErrUnsafe
	}
	var existingBinding uploadstate.Binding
	if mode == "validate-existing-ledger" {
		var expected connectedenroll.ValidationInput
		if json.Unmarshal(input, &expected) != nil {
			return nil, ErrUnsafe
		}
		canonical, _ := json.Marshal(expected)
		if !bytes.Equal(input, canonical) || expected.UploaderUID != policy.UploaderUID || expected.UploaderGID != policy.UploaderGID || expected.SharedGID != policy.SharedGID {
			return nil, ErrUnsafe
		}
		existingBinding = uploadstate.Binding{ServerID: expected.ServerID, ConnectorID: expected.ConnectorID}
		if uploadstate.ValidateRecord(uploadstate.Record{Binding: existingBinding}, existingBinding) != nil {
			return nil, ErrUnsafe
		}
	}
	before, err := beforeState(ctx)
	if err != nil || before["LoadState"] != "not-found" {
		return nil, ErrUnsafe
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, ErrUnsafe
	}
	marker := "observer-connected-enrollment-" + hex.EncodeToString(nonce)
	args := []string{"--quiet", "--collect", "--pipe", "--wait", "--service-type=exec", "--unit=" + enrollmentUnit, "--description=" + marker}
	for _, p := range properties {
		if strings.ContainsAny(p, "\r\n\x00") {
			return nil, ErrUnsafe
		}
		args = append(args, "--property="+p)
	}
	args = append(args, "--", executable, mode)
	child, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := exec.CommandContext(child, "/usr/bin/systemd-run", args...)
	c.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	c.WaitDelay = time.Second
	output := &boundedOutput{limit: 512}
	c.Stdout = output
	c.Stderr = io.Discard
	pipe, err := c.StdinPipe()
	if err != nil {
		return nil, ErrUnsafe
	}
	if c.Start() != nil {
		pipe.Close()
		return nil, ErrUnsafe
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	invocation, err := awaitEnrollment(child, marker)
	if err == nil {
		if mode == "validate-ledger" || mode == "validate-enrollment" || mode == "validate-existing-ledger" {
			err = connectedpolicy.ValidateOffline(child, enrollmentUnit, connectedpolicy.OfflineExpectation{UploaderUID: policy.UploaderUID, UploaderGID: policy.UploaderGID, SharedGID: policy.SharedGID, ArtifactSHA256: policy.ArtifactSHA256, Mode: mode})
		} else {
			err = connectedpolicy.ValidateEffective(child, enrollmentUnit, policy.Addresses)
		}
	}
	if err == nil {
		state, e := enrollmentState(child)
		if e != nil || !ownedEnrollment(state, marker) || state["InvocationID"] != invocation {
			err = ErrUnsafe
		}
	}
	if err != nil {
		pipe.Close()
		cancel()
		<-done
		clear(output.data)
		if stopEnrollment(marker, invocation) != nil {
			return nil, ErrRecovery
		}
		return nil, ErrUnsafe // The grant was never written.
	}
	n, writeErr := pipe.Write(input)
	closeErr := pipe.Close()
	if writeErr != nil || n != len(input) || closeErr != nil {
		cancel()
	}
	waitErr := <-done
	if writeErr != nil || n != len(input) || closeErr != nil || waitErr != nil {
		clear(output.data)
		if stopEnrollment(marker, invocation) != nil {
			return nil, ErrRecovery
		}
		return nil, ErrRecovery
	}
	if mode == "validate-existing-ledger" {
		if _, err := ledgerwitness.DecodeResult(output.data, existingBinding); err != nil {
			clear(output.data)
			return nil, ErrRecovery
		}
	}
	return output.data, nil
}

func awaitEnrollment(ctx context.Context, marker string) (string, error) {
	return awaitEnrollmentWith(ctx, marker, enrollmentState)
}

func awaitEnrollmentWith(ctx context.Context, marker string, read func(context.Context) (map[string]string, error)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	observedInvocation := ""
	for ctx.Err() == nil {
		state, err := read(ctx)
		if ctx.Err() != nil {
			return "", ErrUnsafe
		}
		if err == nil && state["LoadState"] == "loaded" {
			// A newly loaded transient unit can have our nonce but no invocation
			// yet. Never deliver input until an actual active invocation exists.
			if state["Id"] != enrollmentUnit || state["Description"] != marker || state["Transient"] != "yes" {
				return "", ErrUnsafe
			}
			if state["InvocationID"] != "" && !invocationPattern.MatchString(state["InvocationID"]) {
				return "", ErrUnsafe
			}
			if observedInvocation != "" && state["InvocationID"] != observedInvocation {
				return "", ErrUnsafe
			}
			if state["InvocationID"] != "" {
				observedInvocation = state["InvocationID"]
			}
			if state["InvocationID"] != "" && (state["ActiveState"] == "active" || state["ActiveState"] == "activating") {
				return state["InvocationID"], nil
			}
			if state["ActiveState"] != "inactive" && state["ActiveState"] != "activating" {
				return "", ErrUnsafe
			}
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return "", ErrUnsafe
}

func stopEnrollment(marker, invocation string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return stopEnrollmentWith(ctx, marker, invocation, enrollmentState, func(ctx context.Context) error { return service(ctx, "stop", enrollmentUnit) })
}

// The private seam exercises manager collection races without native services.
func stopEnrollmentWith(ctx context.Context, marker, invocation string, read func(context.Context) (map[string]string, error), stop func(context.Context) error) error {
	state, err := read(ctx)
	if err != nil {
		return ErrRecovery
	}
	if state["LoadState"] == "not-found" {
		return nil
	}
	if !ownedEnrollment(state, marker) || (invocation != "" && state["InvocationID"] != invocation) {
		return ErrRecovery
	}
	invocation = state["InvocationID"]
	check, err := read(ctx)
	if err != nil {
		return ErrRecovery
	}
	// --collect may remove the joined unit between the two ownership reads.
	// Absence requires no stop; a replacement still must match the captured ID.
	if check["LoadState"] == "not-found" {
		return nil
	}
	if !ownedEnrollment(check, marker) || check["InvocationID"] != invocation {
		return ErrRecovery
	}
	if stop(ctx) != nil {
		return ErrRecovery
	}
	final, err := read(ctx)
	if err != nil || (final["LoadState"] != "not-found" && (!ownedEnrollment(final, marker) || final["InvocationID"] != invocation || (final["ActiveState"] != "inactive" && final["ActiveState"] != "failed"))) {
		return ErrRecovery
	}
	return nil
}

var invocationPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func ownedEnrollment(state map[string]string, marker string) bool {
	return state["Id"] == enrollmentUnit && state["Description"] == marker && state["Transient"] == "yes" && invocationPattern.MatchString(state["InvocationID"])
}
func enrollmentState(ctx context.Context) (map[string]string, error) {
	b, err := command(ctx, "/usr/bin/systemctl", []string{"show", "--property=Id", "--property=LoadState", "--property=Description", "--property=InvocationID", "--property=Transient", "--property=ActiveState", "--", enrollmentUnit}, nil, 2048)
	if err != nil {
		return nil, ErrUnsafe
	}
	return parseEnrollmentState(b)
}
func parseEnrollmentState(b []byte) (map[string]string, error) {
	if len(b) > 2048 || strings.ContainsAny(string(b), "\r\x00") {
		return nil, ErrUnsafe
	}
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, ErrUnsafe
		}
		switch key {
		case "Id", "LoadState", "Description", "InvocationID", "Transient", "ActiveState":
		default:
			return nil, ErrUnsafe
		}
		if _, found := m[key]; found {
			return nil, ErrUnsafe
		}
		m[key] = value
	}
	if len(m) != 6 || m["Id"] != enrollmentUnit {
		return nil, ErrUnsafe
	}
	return m, nil
}
