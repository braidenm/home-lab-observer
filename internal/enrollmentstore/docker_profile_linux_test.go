//go:build linux

package enrollmentstore

import (
	"context"
	"os"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func TestDockerEnrollmentLifecycleDerivesExactPolicy(t *testing.T) {
	ctx := context.Background()
	path := directory(t)
	p := RichProcessesDockerPrincipals{62001, 62002, 62003, 62004, 62005, 62006, 998}
	s, err := OpenNewRichProcessesDocker(ctx, path, p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.profile != (uploadstate.ProfileBinding{}) {
		t.Fatal("policy derived before generated connector")
	}
	through(t, s, 4)
	want, err := p.profile(testBinding())
	if err != nil || s.profile != want || want.Kind != uploadstate.RichProcessesDockerProfile {
		t.Fatal("Docker policy not bound")
	}
	if s.Close() != nil {
		t.Fatal("close failed")
	}
	for _, open := range []func() (*Ready, error){func() (*Ready, error) { return OpenReady(ctx, path, testBinding()) }, func() (*Ready, error) {
		return OpenReadyRichProcesses(ctx, path, testBinding(), RichProcessesPrincipals{p.CollectorUID, p.UploaderUID, p.UploaderGID, p.SharedGID})
	}, func() (*Ready, error) {
		wrong := p
		wrong.SocketGID++
		return OpenReadyRichProcessesDocker(ctx, path, testBinding(), wrong)
	}} {
		if r, err := open(); err == nil {
			r.Close()
			t.Fatal("cross-profile/policy adoption")
		}
	}
	r, err := OpenReadyRichProcessesDocker(ctx, path, testBinding(), p)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	record, err := r.Ledger().Load(ctx)
	if err != nil || record.Profile != want || record.Binding != testBinding() {
		t.Fatal("Docker ready binding")
	}
}
func TestInvalidDockerPrincipalsDoNotTouchStorage(t *testing.T) {
	for _, p := range []RichProcessesDockerPrincipals{{}, {1, 2, 3, 4, 1, 5, 6}, {1, 2, 3, 4, 5, 4, 6}, {1, 2, 3, 4, 5, 6, 0}} {
		path := directory(t)
		if s, err := OpenNewRichProcessesDocker(context.Background(), path, p); err != ErrRecovery {
			if s != nil {
				s.Close()
			}
			t.Fatal("invalid principal admitted")
		}
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 0 {
			t.Fatal("invalid principal mutated storage")
		}
	}
}
