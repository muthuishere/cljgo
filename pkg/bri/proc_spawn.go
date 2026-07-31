// proc_spawn.go — the Go half of cljg.process's STREAMING spawn (ADR 0101):
// start a subprocess with live stdin/stdout/stderr pipes and hand back a
// handle {:in <writable> :out <readable> :err <readable> :wait :kill}. Where
// cljg.io's exec/sh (io_proc.go) run a command to completion and buffer its
// output, spawn keeps the process live so the caller streams into and out of it
// a chunk/line at a time (the cat/less/ffmpeg shape). Thin shim over stdlib
// os/exec's StdinPipe/StdoutPipe/StderrPipe; pure Go, so cljg.process stays a
// non-OptIn namespace. The pipes are wrapped as the same Readable/Writable
// stream handles cljg.stream and cljg.net.http use (stream.go) — ONE
// abstraction (spike s56). Interned as :private vars into cljg.process.
package bri

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// spawnHandle is the opaque process handle threaded back to Clojure inside the
// :wait / :kill / :alive? closures. It is never inspected by Clojure directly.
//
// cmd.Wait may only ever be called once (os/exec docs), so it has exactly one
// caller: the reaper goroutine launched right after Start(). Every other
// observer (-proc-wait, -proc-alive?) reads doneCh/exitCode/waitErr instead of
// touching cmd.Wait itself — this is what makes a non-blocking liveness check
// (issue #173) possible without a caller-side reaper thread, and what lets
// :wait be called more than once safely.
type spawnHandle struct {
	cmd      *exec.Cmd
	doneCh   chan struct{} // closed once the reaper's cmd.Wait() returns
	exitCode int64         // valid once doneCh is closed
	waitErr  error         // non-nil only for a genuine wait error (not a non-zero exit)
}

// startReaper launches the single goroutine that owns cmd.Wait() for the
// lifetime of the handle.
func (h *spawnHandle) startReaper() {
	h.doneCh = make(chan struct{})
	go func() {
		err := h.cmd.Wait()
		switch {
		case err == nil:
			h.exitCode = 0
		default:
			if ee, ok := err.(*exec.ExitError); ok {
				h.exitCode = int64(ee.ExitCode()) // ran, exited non-zero — a normal result
			} else {
				h.waitErr = err
			}
		}
		close(h.doneCh)
	}()
}

// alive reports whether the child has NOT yet been reaped — false once the
// reaper has observed its exit. Never blocks.
func (h *spawnHandle) alive() bool {
	select {
	case <-h.doneCh:
		return false
	default:
		return true
	}
}

// installProcSpawnShims interns cljg.process's private Go primitives: the
// spawn itself plus wait/kill/alive? over the returned handle.
func installProcSpawnShims(def func(name string, fn func(args ...any) any)) {
	// -proc-spawn (argv opts) -> {:in :out :err :-handle}. argv is a non-empty
	// vector [cmd arg…]; opts (may be nil) honors :env ({name value}, merged
	// onto the current environment) and :dir (working directory). A
	// missing/unrunnable binary panics → a cljgo error (it never started).
	def("-proc-spawn", procSpawnShim)
	// -proc-wait (handle) -> exit code (int). Blocks until the process exits;
	// returns its exit status (non-zero is a normal result, not an error).
	// Safe to call any number of times, and safe alongside -proc-alive? —
	// both just read the reaper's result.
	def("-proc-wait", func(args ...any) any {
		h := asSpawnHandle("-proc-wait", args)
		<-h.doneCh
		if h.waitErr != nil {
			panic(fmt.Errorf("cljg.process: wait: %w", h.waitErr))
		}
		return h.exitCode
	})
	// -proc-kill (handle) -> nil. Force-kills the process (SIGKILL); not an
	// error if it has already exited.
	def("-proc-kill", func(args ...any) any {
		h := asSpawnHandle("-proc-kill", args)
		if h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
		return nil
	})
	// -proc-alive? (handle) -> boolean. Non-blocking: false once the child has
	// exited (observed by the reaper), true while it is still running. Closes
	// the gap issue #173 found: koine had to run its own reaper thread to
	// answer this on cljgo when the JVM offers .isAlive() directly.
	def("-proc-alive?", func(args ...any) any {
		h := asSpawnHandle("-proc-alive?", args)
		return h.alive()
	})
}

func procSpawnShim(args ...any) any {
	if len(args) != 2 {
		panic(fmt.Errorf("-proc-spawn expects 2 args (argv opts), got %d", len(args)))
	}
	argv := toStringSlice(args[0])
	if len(argv) == 0 {
		panic(fmt.Errorf("cljg.process: spawn needs a non-empty command vector"))
	}
	opts, _ := args[1].(lang.IPersistentMap)

	cmd := exec.Command(argv[0], argv[1:]...)
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

	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(fmt.Errorf("cljg.process: stdin pipe for %q: %w", argv[0], err))
	}
	// os.Pipe, NOT cmd.StdoutPipe/StderrPipe — and this is load-bearing, not a
	// style choice. os/exec's docs say Wait "will close the pipe after seeing
	// the command exit, so most callers need not close it themselves; it is
	// thus incorrect to call Wait before all reads from the pipe have
	// completed." The reaper below calls Wait from the moment Start returns,
	// which is exactly that forbidden shape: the child exits, Wait closes the
	// read end underneath a caller still consuming (st/lines …), and the read
	// fails with "file already closed".
	//
	// Owning the pipes ourselves decouples the two. Wait reaps the process and
	// touches nothing the caller is holding; the read ends stay open until the
	// caller closes them or the stream is exhausted. Passing the write ends as
	// cmd.Stdout/Stderr and closing OUR copies after Start means the child owns
	// the only remaining writers, so readers still see a real EOF when it exits.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		panic(fmt.Errorf("cljg.process: stdout pipe for %q: %w", argv[0], err))
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		panic(fmt.Errorf("cljg.process: stderr pipe for %q: %w", argv[0], err))
	}
	cmd.Stdout, cmd.Stderr = stdoutW, stderrW

	if err := cmd.Start(); err != nil {
		stdoutW.Close()
		stderrW.Close()
		panic(fmt.Errorf("cljg.process: spawn %q: %w", argv[0], err))
	}
	// The child holds its own duplicates now; drop ours so the reader sees EOF
	// when the child exits rather than blocking on a writer we forgot about.
	stdoutW.Close()
	stderrW.Close()

	h := &spawnHandle{cmd: cmd}
	h.startReaper()

	return lang.NewMap(
		lang.NewKeyword("in"), newWritableStream(stdin, stdin),
		lang.NewKeyword("out"), newReadableStream(stdoutR, stdoutR),
		lang.NewKeyword("err"), newReadableStream(stderrR, stderrR),
		lang.NewKeyword("-handle"), h,
	)
}

// asSpawnHandle coerces args[0] to a *spawnHandle (arity 1).
func asSpawnHandle(name string, args []any) *spawnHandle {
	if len(args) != 1 {
		panic(fmt.Errorf("%s expects 1 arg (handle), got %d", name, len(args)))
	}
	h, ok := args[0].(*spawnHandle)
	if !ok {
		panic(fmt.Errorf("cljg.process: %s expects a process handle, got: %s", name, lang.PrintString(args[0])))
	}
	return h
}
