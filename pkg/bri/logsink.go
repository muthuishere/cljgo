package bri

// The buffered access-log sink behind bri.web.http's `-log-line`.
//
// s80 measured the zero-config stack at 58–64% of the lean stack's
// throughput across c=10/50/200, and the CPU profile pinned it on the
// sink itself: one unbuffered fmt.Fprintln(os.Stdout) per request — a
// serialized write syscall plus os.File mutex contention, ~13.5% of all
// CPU even with stdout on /dev/null. The logging LOGIC is cheap; the
// write discipline was the cost. So the sink buffers: whole lines are
// appended under one mutex, and the buffer reaches the fd on a periodic
// flush, on high water, and on server drain (ADR 0122).
//
// Lines are always appended whole, so a flush can interleave whole log
// lines with the host program's own stdout writes but can never tear
// one. The loss window on a hard kill (SIGKILL, power) is one flush
// interval; SIGTERM/SIGINT and :block false stops flush through drain.

import (
	"bufio"
	"io"
	"os"
	"sync"
	"time"
)

const (
	logSinkFlushEvery = 250 * time.Millisecond
	logSinkBufBytes   = 64 * 1024
)

var logSink = struct {
	mu    sync.Mutex
	w     *bufio.Writer
	dst   io.Writer // swap point for tests; defaults to os.Stdout
	once  sync.Once // starts the flush ticker on first write
	timer *time.Ticker
}{dst: os.Stdout}

// logLine buffers one complete access-log line. The flush goroutine is
// started lazily so programs that never log pay nothing.
func logLine(s string) {
	logSink.mu.Lock()
	if logSink.w == nil {
		logSink.w = bufio.NewWriterSize(logSink.dst, logSinkBufBytes)
	}
	_, _ = logSink.w.WriteString(s)
	_ = logSink.w.WriteByte('\n')
	logSink.mu.Unlock()

	logSink.once.Do(func() {
		logSink.timer = time.NewTicker(logSinkFlushEvery)
		go func() {
			for range logSink.timer.C {
				logFlush()
			}
		}()
	})
}

// logFlush drains the buffer to the underlying writer. Called by the
// ticker, by the server drain path, and by tests.
func logFlush() {
	logSink.mu.Lock()
	if logSink.w != nil {
		_ = logSink.w.Flush()
	}
	logSink.mu.Unlock()
}
