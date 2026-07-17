// aot_stubs.go — what the interpreter-coupled builtins do in a binary
// that has no interpreter (ADR 0046 §5).
//
// Four clojure.core names need the analyzer: macroexpand-1, macroexpand,
// eval, require-go. RegisterAll interns these definitions; pkg/eval
// OVERWRITES all four with the real ones through the same Def seam when
// an Evaluator is constructed (internBuiltins), so an interpreted
// session — REPL, `cljgo run`, the conformance eval half — is completely
// unchanged. Only an AOT binary, which never constructs an Evaluator,
// keeps what is here.
//
// The oracle (Clojure 1.12.5, 2026-07-17) cannot decide this one: on the
// JVM, `(eval '(+ 1 2))` => 3 in AOT-compiled code too, because a JVM
// program always links clojure.jar — Compiler and all. A cljgo binary
// follows the CLJS model (design/00, ADR 0001): the compiler is a
// build-time tool, the binary is plain Go, and there is no analyzer in
// it to run. ClojureScript answers the same question the same way —
// `eval` is simply not in cljs.core, and macroexpansion is compile-time
// only, unless you opt into the self-hosted compiler artifact.
//
// So this is a documented DEVIATION, and it is spelled out rather than
// papered over:
//
//   - eval / macroexpand / macroexpand-1 are BOUND to a stub that throws
//     "… is not available in an AOT-compiled binary". Bound-and-throwing
//     beats unbound: an unbound var reports "cannot call unbound var:
//     Unbound: #'clojure.core/eval", which reads like a broken boot,
//     while the stub names the real constraint and where it comes from.
//     Referencing the var (resolve, bound?, passing #'eval around) also
//     keeps behaving as in the REPL; only CALLING it fails, and it fails
//     honestly.
//   - require-go is a NO-OP, not an error. It is a compile-time
//     directive (it registers Go import aliases for the ANALYZER); by
//     the time a binary replays it, the emitter has already resolved and
//     linked those calls directly. Failing here would break every AOT
//     interop program for no reason (it did — pkg/build's websocket
//     example is the regression test).
//
// `require` is deliberately NOT in this file: ADR 0046 made it work in
// binaries for real, off the provider registry (require.go).
package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// registerAOTStubs interns the AOT behavior of the interpreter-coupled
// builtins. Called from RegisterAll (i.e. in BOTH modes); pkg/eval
// overwrites all four when an evaluator exists.
func registerAOTStubs(def func(string, func(...any) any) *lang.Var) {
	for _, name := range []string{"eval", "macroexpand", "macroexpand-1"} {
		name := name
		def(name, func(args ...any) any {
			panic(fmt.Errorf("%s is not available in an AOT-compiled binary: it needs the analyzer, and a compiled cljgo binary is plain Go with no compiler linked (ADR 0046; the CLJS model — run this through the cljgo REPL or `cljgo run` instead)", name))
		})
	}

	// require-go: already applied at compile time (the emitter linked the
	// Go calls directly), so replaying it in a binary is a no-op.
	def("require-go", func(args ...any) any { return nil })
}
