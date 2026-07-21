package corelib

import (
	"fmt"
	"io"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// bootInstant anchors -nano-time: time.Since on a fixed instant rides
// Go's monotonic clock, so `time` measures real elapsed wall-clock even
// across system clock adjustments (the same guarantee System/nanoTime
// gives the JVM macro).
var bootInstant = time.Now()

// internMacroSupportBuiltins registers the private host substrate for the
// fundamentals batch-1 control-flow macros (core/core.clj: time, locking,
// defonce, io!, with-open). Wired into internBuiltins by ONE line, per the
// merge-friendly discipline (misc_builtins.go).
func internMacroSupportBuiltins(defPrivate func(name string, fn func(args ...any) any)) {
	// -nano-time: monotonic nanoseconds since process start — the `time`
	// macro's stopwatch (JVM: (. System (nanoTime))). Only differences of
	// two readings are meaningful.
	defPrivate("-nano-time", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -nano-time", len(args)))
		}
		return time.Since(bootInstant).Nanoseconds()
	})

	// -with-lock: run a thunk holding the per-object reentrant monitor
	// (lang.WithLock) — the `locking` macro's substrate (JVM:
	// monitor-enter/monitor-exit around the body).
	defPrivate("-with-lock", func(args ...any) any {
		obj, fnArg := twoArgs("-with-lock", args)
		f, ok := fnArg.(lang.IFn)
		if !ok {
			panic(fmt.Errorf("-with-lock: not a function: %s", lang.PrintString(fnArg)))
		}
		return lang.WithLock(obj, f)
	})

	// -has-root: does the var have a root binding — `defonce`'s guard
	// (JVM: (.hasRoot v)).
	defPrivate("-has-root", func(args ...any) any {
		v, ok := oneArg("-has-root", args).(*lang.Var)
		if !ok {
			panic(fmt.Errorf("-has-root expects a var, got: %s", lang.PrintString(args[0])))
		}
		return v.HasRoot()
	})

	// -io-guard: throw IllegalStateException when called inside a dosync
	// transaction — the `io!` macro's check (JVM:
	// clojure.lang.LockingTransaction/isRunning; oracle 1.12.5 message
	// "I/O in transaction").
	defPrivate("-io-guard", func(args ...any) any {
		msg, ok := oneArg("-io-guard", args).(string)
		if !ok {
			panic(fmt.Errorf("-io-guard expects a string message, got: %s", lang.PrintString(args[0])))
		}
		if lang.InTransaction() {
			panic(lang.NewIllegalStateError(msg))
		}
		return nil
	})

	// -close-resource: close a resource for `with-open`'s finally. On the
	// Go host a closeable is an io.Closer (any Go value with
	// Close() error, e.g. os.File via interop) or a cljgo channel
	// (idempotent close!, chan.go). A Close error propagates as a throw,
	// exactly as .close throwing does inside JVM with-open's finally.
	defPrivate("-close-resource", func(args ...any) any {
		x := oneArg("-close-resource", args)
		switch r := x.(type) {
		case io.Closer:
			if err := r.Close(); err != nil {
				panic(err)
			}
			return nil
		case *lang.Channel:
			return lang.ChanClose(r)
		default:
			panic(fmt.Errorf("with-open: value is not closeable: %s", lang.PrintString(x)))
		}
	})
}
