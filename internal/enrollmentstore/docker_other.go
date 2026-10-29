//go:build !linux

package enrollmentstore

import (
	"context"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func OpenNewRichProcessesDocker(context.Context, string, RichProcessesDockerPrincipals) (*Setup, error) {
	return nil, ErrUnsupported
}
func OpenReadyRichProcessesDocker(context.Context, string, uploadstate.Binding, RichProcessesDockerPrincipals) (*Ready, error) {
	return nil, ErrUnsupported
}
