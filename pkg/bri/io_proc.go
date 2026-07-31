// io_proc.go — the Go half of cljg.io's process exec (ADR 0089 §3): run a
// subprocess and capture {:out :err :exit}, with optional stdin, env, working
// dir, and timeout. Thin shim over stdlib os/exec + context — pure Go, so
// CGO_ENABLED=0 + cljgo dist hold, and cljg.io stays non-OptIn. The ergonomic
// API (sh/exec/sh!) is portable Clojure (core/cljg/io.cljg). Interned as a
// :private var into cljg.io alongside the filesystem shims.
package bri

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// installProcShims interns cljg.io's process-exec primitive. Called from
// installIOShims so cljg.io owns both the filesystem and process surface.
func installProcShims(def func(name string, fn func(args ...any) any)) {
	def("-proc-exec", procExecShim)
}

// procDrainGrace bounds how long a timed-out exec waits for its own copy
// goroutines to notice their pipe was closed (issue #175) before giving up on
// them entirely. It exists only to make procExecShim's own return provably
// bounded even if closing a pipe were ever slow to unblock a Read — the
// ordinary path (killed process, closed pipe) returns almost immediately.
const procDrainGrace = 200 * time.Millisecond

// procExecShim runs one subprocess: (argv opts) -> {:out :err :exit :timed-out?}.
//
//	argv  a non-empty vector [cmd arg…]
//	opts  a map (may be nil) with :in (stdin string), :env ({name value}, merged
//	      onto the current environment), :dir (working directory), :timeout-ms.
//
// A missing/unrunnable binary panics → a cljgo error; a command that runs and
// exits non-zero is a NORMAL result ({:exit n}), not a panic — the Clojure `sh`
// wrapper surfaces it, and `sh!` is the throwing variant. A timeout kills the
// process, sets :timed-out? true and :exit -1.
//
// :timeout-ms bounds the CALL, not just the process (issue #175 fix). The
// naive approach — os/exec.CommandContext + cmd.Stdout/Stderr set to a
// Writer — makes cmd.Wait() block until stdout/stderr reach EOF, and EOF never
// comes while ANY process holds the pipe's write end open: a forking command
// (`sh -c "sleep 5; echo x"`) leaves the write end inherited by the
// grandchild, which outlives the killed immediate child. So Wait — and the
// whole call — blocked for the child's full runtime regardless of the
// timeout, returning the right answer 16x too late.
//
// The fix reads stdout/stderr through our OWN goroutines over manually opened
// pipes (StdoutPipe/StderrPipe, exactly as cljg.process/spawn already does),
// so cmd.Wait() only waits for the process itself to exit — not for the
// pipes to reach EOF. On a timeout: kill the process, then close OUR read
// ends of the pipes. Closing a pipe out from under a blocked Read unblocks it
// immediately with an error, so the copy goroutines stop within microseconds
// even though a grandchild is still holding the far end open — output on the
// timeout path is necessarily best-effort (whatever was captured before the
// kill), matching what the JVM effectively does. The normal (non-timeout)
// path is unchanged: wait for the copies to finish normally, so a full run
// still returns full output.
func procExecShim(args ...any) any {
	if len(args) != 2 {
		panic(fmt.Errorf("-proc-exec expects 2 args (argv opts), got %d", len(args)))
	}
	argv := toStringSlice(args[0])
	if len(argv) == 0 {
		panic(fmt.Errorf("cljg.io: exec needs a non-empty command vector"))
	}

	opts, _ := args[1].(lang.IPersistentMap)
	cmd := exec.Command(argv[0], argv[1:]...)
	if s := optStr(opts, "in"); s != "" {
		cmd.Stdin = bytes.NewReader([]byte(s))
	}
	if dir := optStr(opts, "dir"); dir != "" {
		cmd.Dir = dir
	}
	if env := optMap(opts, "env"); env != nil {
		merged := os.Environ()
		for s := lang.Seq(env); s != nil; s = lang.Next(s) {
			e := lang.First(s)
			merged = append(merged, asString(lang.First(e))+"="+asString(lang.Get(e, int64(1))))
		}
		cmd.Env = merged
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		panic(fmt.Errorf("cljg.io: exec %q: %w", argv[0], err))
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		panic(fmt.Errorf("cljg.io: exec %q: %w", argv[0], err))
	}

	if err := cmd.Start(); err != nil {
		// could not start (binary not found, permission) — a real error
		panic(fmt.Errorf("cljg.io: exec %q: %w", argv[0], err))
	}

	var mu sync.Mutex
	var outBuf, errBuf bytes.Buffer
	var copyWG sync.WaitGroup
	copyWG.Add(2)
	go func() { defer copyWG.Done(); drainPipe(&mu, &outBuf, stdoutPipe) }()
	go func() { defer copyWG.Done(); drainPipe(&mu, &errBuf, stderrPipe) }()

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	var timedOut bool
	var runErr error
	if ms := optInt(opts, "timeout-ms"); ms > 0 {
		select {
		case runErr = <-waitDone:
			copyWG.Wait() // finished within the deadline: capture full output
		case <-time.After(time.Duration(ms) * time.Millisecond):
			timedOut = true
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			runErr = <-waitDone // returns once the process itself exits (not EOF)
			// Do not wait for EOF from a surviving grandchild — close our own
			// read ends, which unblocks any goroutine still blocked in Read.
			_ = stdoutPipe.Close()
			_ = stderrPipe.Close()
			drained := make(chan struct{})
			go func() { copyWG.Wait(); close(drained) }()
			select {
			case <-drained:
			case <-time.After(procDrainGrace):
			}
		}
	} else {
		runErr = <-waitDone
		copyWG.Wait()
	}

	exit := 0
	switch {
	case timedOut:
		exit = -1
	case runErr != nil:
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exit = ee.ExitCode() // ran, exited non-zero — a normal result
		} else {
			// could not start (binary not found, permission) — a real error
			panic(fmt.Errorf("cljg.io: exec %q: %w", argv[0], runErr))
		}
	}

	mu.Lock()
	outStr, errStr := outBuf.String(), errBuf.String()
	mu.Unlock()

	return lang.NewMap(
		lang.NewKeyword("out"), outStr,
		lang.NewKeyword("err"), errStr,
		lang.NewKeyword("exit"), int64(exit),
		lang.NewKeyword("timed-out?"), timedOut,
	)
}

// drainPipe copies r into buf, one read at a time, holding mu only around
// each buffer write (never around the blocking Read) so the timeout path can
// safely read buf's partial contents concurrently under the same mutex.
func drainPipe(mu *sync.Mutex, buf *bytes.Buffer, r io.Reader) {
	tmp := make([]byte, 32*1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			mu.Lock()
			buf.Write(tmp[:n])
			mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// --- small opt accessors over a (possibly nil) cljgo opts map ---------------

func optStr(m lang.IPersistentMap, k string) string {
	if m == nil {
		return ""
	}
	if v, ok := lang.Get(m, lang.NewKeyword(k)).(string); ok {
		return v
	}
	return ""
}

func optInt(m lang.IPersistentMap, k string) int {
	if m == nil {
		return 0
	}
	v := lang.Get(m, lang.NewKeyword(k))
	if v == nil {
		return 0
	}
	return asInt(v)
}

func optMap(m lang.IPersistentMap, k string) lang.IPersistentMap {
	if m == nil {
		return nil
	}
	sub, _ := lang.Get(m, lang.NewKeyword(k)).(lang.IPersistentMap)
	return sub
}
