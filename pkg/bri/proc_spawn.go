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
// :wait / :kill closures. It is never inspected by Clojure directly.
type spawnHandle struct {
	cmd *exec.Cmd
}

// installProcSpawnShims interns cljg.process's private Go primitives: the
// spawn itself plus wait/kill over the returned handle.
func installProcSpawnShims(def func(name string, fn func(args ...any) any)) {
	// -proc-spawn (argv opts) -> {:in :out :err :-handle}. argv is a non-empty
	// vector [cmd arg…]; opts (may be nil) honors :env ({name value}, merged
	// onto the current environment) and :dir (working directory). A
	// missing/unrunnable binary panics → a cljgo error (it never started).
	def("-proc-spawn", procSpawnShim)
	// -proc-wait (handle) -> exit code (int). Blocks until the process exits;
	// returns its exit status (non-zero is a normal result, not an error).
	def("-proc-wait", func(args ...any) any {
		h := asSpawnHandle("-proc-wait", args)
		err := h.cmd.Wait()
		if err == nil {
			return int64(0)
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return int64(ee.ExitCode()) // ran, exited non-zero — a normal result
		}
		panic(fmt.Errorf("cljg.process: wait: %w", err))
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(fmt.Errorf("cljg.process: stdout pipe for %q: %w", argv[0], err))
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(fmt.Errorf("cljg.process: stderr pipe for %q: %w", argv[0], err))
	}

	if err := cmd.Start(); err != nil {
		panic(fmt.Errorf("cljg.process: spawn %q: %w", argv[0], err))
	}

	return lang.NewMap(
		lang.NewKeyword("in"), newWritableStream(stdin, stdin),
		lang.NewKeyword("out"), newReadableStream(stdout, stdout),
		lang.NewKeyword("err"), newReadableStream(stderr, stderr),
		lang.NewKeyword("-handle"), &spawnHandle{cmd: cmd},
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
