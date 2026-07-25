// io_proc.go — the Go half of cljg.io's process exec (ADR 0089 §3): run a
// subprocess and capture {:out :err :exit}, with optional stdin, env, working
// dir, and timeout. Thin shim over stdlib os/exec + context — pure Go, so
// CGO_ENABLED=0 + cljgo dist hold, and cljg.io stays non-OptIn. The ergonomic
// API (sh/exec/sh!) is portable Clojure (core/cljg/io.cljg). Interned as a
// :private var into cljg.io alongside the filesystem shims.
package bri

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// installProcShims interns cljg.io's process-exec primitive. Called from
// installIOShims so cljg.io owns both the filesystem and process surface.
func installProcShims(def func(name string, fn func(args ...any) any)) {
	def("-proc-exec", procExecShim)
}

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
func procExecShim(args ...any) any {
	if len(args) != 2 {
		panic(fmt.Errorf("-proc-exec expects 2 args (argv opts), got %d", len(args)))
	}
	argv := toStringSlice(args[0])
	if len(argv) == 0 {
		panic(fmt.Errorf("cljg.io: exec needs a non-empty command vector"))
	}

	opts, _ := args[1].(lang.IPersistentMap)
	ctx := context.Background()
	var cancel context.CancelFunc
	if ms := optInt(opts, "timeout-ms"); ms > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
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

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()

	timedOut := ctx.Err() == context.DeadlineExceeded
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

	return lang.NewMap(
		lang.NewKeyword("out"), outBuf.String(),
		lang.NewKeyword("err"), errBuf.String(),
		lang.NewKeyword("exit"), int64(exit),
		lang.NewKeyword("timed-out?"), timedOut,
	)
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
