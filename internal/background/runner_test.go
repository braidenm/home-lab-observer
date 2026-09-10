package background

import (
	"bytes"
	"context"
	"testing"
)

type recordingRunner struct {
	commands []command
	result   commandResult
	err      error
}

func (r *recordingRunner) run(_ context.Context, command command) (commandResult, error) {
	r.commands = append(r.commands, command)
	return r.result, r.err
}

func TestLimitedWriterReportsOverflowWithoutGrowing(t *testing.T) {
	var output bytes.Buffer
	writer := &limitedWriter{writer: &output, remaining: 4}
	if count, err := writer.Write([]byte("123456")); err != nil || count != 6 {
		t.Fatalf("write = %d, %v", count, err)
	}
	if output.String() != "1234" || !writer.overflow {
		t.Fatalf("bounded output = %q, overflow %v", output.String(), writer.overflow)
	}
}
