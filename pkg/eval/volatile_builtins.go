package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internVolatileBuiltins interns volatile!/vswap!/vreset!/volatile? (ADR
// 0022 Batch 4). *lang.Volatile is vendored, unmodified, from Glojure
// (pkg/lang/volatile.go) — a bare mutable box with Deref/Reset, no CAS
// loop and no watches, matching the actual JVM contract (ADR 0024):
// clojure.lang.Volatile is a plain, non-thread-safe mutable field; the
// name refers to Java memory-visibility semantics, not compare-and-swap.
// `deref`/`@` already work on it generically via lang.IDeref. Wired into
// internBuiltins by ONE line (e.internVolatileBuiltins(def)).
func (e *Evaluator) internVolatileBuiltins(def func(string, func(...any) any) *lang.Var) {
	def("volatile!", func(args ...any) any {
		return lang.NewVolatile(oneArg("volatile!", args))
	})

	def("volatile?", func(args ...any) any {
		_, ok := oneArg("volatile?", args).(*lang.Volatile)
		return ok
	})

	def("vreset!", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: vreset!", len(args)))
		}
		v, ok := args[0].(*lang.Volatile)
		if !ok {
			panic(fmt.Errorf("vreset! expects a volatile, got: %s", lang.PrintString(args[0])))
		}
		return v.Reset(args[1])
	})

	def("vswap!", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: vswap!", len(args)))
		}
		v, ok := args[0].(*lang.Volatile)
		if !ok {
			panic(fmt.Errorf("vswap! expects a volatile, got: %s", lang.PrintString(args[0])))
		}
		f, ok := args[1].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("vswap! expects a function, got: %s", lang.PrintString(args[1])))
		}
		callArgs := append([]any{v.Deref()}, args[2:]...)
		return v.Reset(f.Invoke(callArgs...))
	})
}
