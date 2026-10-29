//go:build !linux

package sharedhandoff

import (
	"context"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
	"time"
)

type RichProcessesDockerWriter struct{}
type RichProcessesDockerReader struct{}
type DockerFrameWriter struct{}
type DockerFrameReader struct{}

func OpenRichProcessesDockerWriter(string, RichProcessesDockerPolicy) (*RichProcessesDockerWriter, error) {
	return nil, ErrUnsupported
}
func OpenRichProcessesDockerReader(string, RichProcessesDockerPolicy) (*RichProcessesDockerReader, error) {
	return nil, ErrUnsupported
}
func OpenDockerFrameWriter(string, DockerFramePolicy) (*DockerFrameWriter, error) {
	return nil, ErrUnsupported
}
func OpenDockerFrameReader(string, DockerFramePolicy) (*DockerFrameReader, error) {
	return nil, ErrUnsupported
}
func (*RichProcessesDockerWriter) Publish([]byte) error { return ErrUnsupported }
func (*RichProcessesDockerReader) Read(context.Context, string) ([]byte, error) {
	return nil, ErrUnsupported
}
func (*DockerFrameWriter) Publish(workloadprojection.ContainerSection, time.Time) error {
	return ErrUnsupported
}
func (*DockerFrameReader) Read(context.Context, string, time.Time) (workloadprojection.ContainerSection, error) {
	return workloadprojection.ContainerSection{}, ErrUnsupported
}
func (*RichProcessesDockerWriter) Close() error { return nil }
func (*RichProcessesDockerReader) Close() error { return nil }
func (*DockerFrameWriter) Close() error         { return nil }
func (*DockerFrameReader) Close() error         { return nil }
