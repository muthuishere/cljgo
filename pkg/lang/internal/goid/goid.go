// Package goid returns the current goroutine's ID.
//
// Get() is on the hottest path in the interpreter: every dynamic-var
// deref (Var.getDynamicBinding) and every thread-binding push/pop/clone
// keys off it. On supported configurations (see goid_fast.go) it is a
// two-instruction read of the runtime g struct's goid field; everywhere
// else it falls back to parsing runtime.Stack() output (getSlow below,
// the original vendored implementation). Selection is compile-time via
// build tags — ADR 0034.
package goid

import (
	"bytes"
	"runtime"
	"strconv"
)

var goroutinePrefix = []byte("goroutine ")

// getSlow extracts the goroutine ID by capturing and text-parsing a
// stack trace. Correct on every arch/toolchain, but expensive: it
// allocates and its cost scales with stack depth (spike S18 measured it
// at 72.85% of boot CPU when used on the hot path). It is the fallback
// implementation of Get and the oracle the fast path is verified
// against (init cross-check in goid_fast.go, concurrency pin in
// goid_test.go).
func getSlow() int64 {
	buf := make([]byte, 32)
	n := runtime.Stack(buf, false)
	buf = buf[:n]
	// goroutine 1 [running]: ...
	if !bytes.HasPrefix(buf, goroutinePrefix) {
		panic("unexpected goroutine stack format, missing prefix")
	}
	buf = buf[len(goroutinePrefix):]
	i := bytes.IndexByte(buf, ' ')
	if i < 0 {
		panic("unexpected goroutine stack format, missing space")
	}
	id, err := strconv.Atoi(string(buf[:i]))
	if err != nil {
		panic("unexpected goroutine stack format, invalid id")
	}
	return int64(id)
}
