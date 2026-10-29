//go:build !linux

package sharedhandoff

import "context"

type RichProcessesWriter struct{}
type RichProcessesReader struct{}

func OpenRichProcessesWriter(string, RichProcessesPolicy) (*RichProcessesWriter, error) {
	return nil, ErrUnsupported
}
func OpenRichProcessesReader(string, RichProcessesPolicy) (*RichProcessesReader, error) {
	return nil, ErrUnsupported
}
func (*RichProcessesWriter) Publish([]byte) error                         { return ErrUnsupported }
func (*RichProcessesReader) Read(context.Context, string) ([]byte, error) { return nil, ErrUnsupported }
func (*RichProcessesWriter) Close() error                                 { return nil }
func (*RichProcessesReader) Close() error                                 { return nil }
