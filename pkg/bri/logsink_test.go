package bri

import (
	"bytes"
	"strings"
	"testing"
)

// The sink's contract: lines are buffered (not on the fd immediately),
// arrive whole and in order on flush, and flushing twice is safe.
func TestLogSinkBuffersAndFlushesWholeLines(t *testing.T) {
	var buf bytes.Buffer
	logSink.mu.Lock()
	prevDst, prevW := logSink.dst, logSink.w
	logSink.dst, logSink.w = &buf, nil
	logSink.mu.Unlock()
	defer func() {
		logFlush()
		logSink.mu.Lock()
		logSink.dst, logSink.w = prevDst, prevW
		logSink.mu.Unlock()
	}()

	logLine(`{"kind":"access","path":"/a"}`)
	logLine(`{"kind":"access","path":"/b"}`)
	if got := buf.String(); got != "" {
		t.Fatalf("lines reached the writer before a flush: %q", got)
	}

	logFlush()
	want := "{\"kind\":\"access\",\"path\":\"/a\"}\n{\"kind\":\"access\",\"path\":\"/b\"}\n"
	if got := buf.String(); got != want {
		t.Fatalf("flush wrote %q, want %q", got, want)
	}

	logFlush() // idempotent: nothing new appears
	if got := buf.String(); got != want {
		t.Fatalf("second flush changed the output: %q", got)
	}
	if n := strings.Count(buf.String(), "\n"); n != 2 {
		t.Fatalf("expected 2 complete lines, got %d", n)
	}
}
