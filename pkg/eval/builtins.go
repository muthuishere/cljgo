package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// internBuiltins pre-interns the native IFns into clojure.core. Since
// ADR 0043 (AOT-core piece 2) the interpreter-independent set lives in
// pkg/corelib (corelib.RegisterAll); this shell layers the FOUR
// evaluator-coupled builtins on top of the same corelib.Def seam:
// macroexpand-1 / macroexpand (the macro engine, design/03 §4),
// require-go (per-evaluator host aliases, ADR 0010) and `eval`
// (Evaluator.EvalForm). In an AOT-compiled binary rt.Boot calls
// corelib.RegisterAll and these four stay UNBOUND — a compiled binary
// has no analyzer, so they cannot exist (ADR 0046 §5); touching one
// throws "Unable to resolve symbol", exactly as Clojure does for a name
// that is not there. `require` is no longer among them: ADR 0046 moved
// it to corelib, with the interpreter installing only its file-loading
// hook below.
func (e *Evaluator) internBuiltins() {
	corelib.RegisterAll()
	def := corelib.Def

	// macroexpand-1 / macroexpand expose the compiler's expander
	// (design/03 §4) — same code path the analyzer uses, &env = nil.
	def("macroexpand-1", func(args ...any) any {
		res, err := e.macroexpand1(oneArg("macroexpand-1", args), nil)
		if err != nil {
			panic(err)
		}
		return res
	})
	def("macroexpand", func(args ...any) any {
		res, err := e.macroexpand(oneArg("macroexpand", args))
		if err != nil {
			panic(err)
		}
		return res
	})

	// require itself is corelib's (ADR 0046: a compiled binary replays
	// every (require …) and must reach the provider registry without an
	// interpreter). What only the interpreter can do — make a namespace
	// exist by READING ITS SOURCE FILE — is installed here as corelib's
	// lib-file hook, bound to this evaluator (libload.go, ADR 0042 §4).
	corelib.SetLibFileLoader(func(libSym *lang.Symbol) { loadLibFile(e, libSym) })

	// require-go registers Go import aliases for the interpreted interop
	// path (ADR 0010, design/05 §1): (require-go '[strings]),
	// (require-go '[strconv :as sc]), (require-go '["net/http" :as http]).
	// Each libspec is a vector whose head is the path (a symbol — one
	// segment — or a string that may contain slashes) with an optional
	// `:as alias`; the default alias is the path's last `/`-segment. The
	// mapping is scoped to the current namespace. A precedence-safe
	// addition (CLAUDE.md): resolveHost yields to Clojure namespaces.
	def("require-go", func(args ...any) any {
		e.registerRequireGo(args)
		return nil
	})

	// eval: analyze + evaluate an already-read form through the SAME
	// Read→Analyze→Eval path the REPL uses (Evaluator.EvalForm). The argument
	// is data, not text, so there is no reader step here — this is the
	// value-level eval the suite exercises ((eval (list '+ 1 2)) => 3).
	// A read-string→eval combo composes the two, matching clojure.core.
	def("eval", func(args ...any) any {
		res, err := e.EvalForm(oneArg("eval", args))
		if err != nil {
			panic(err)
		}
		return res
	})
}

// oneArg asserts a 1-arg builtin's arity and returns the argument.
// (corelib keeps its own copy — the two packages' builtins are disjoint.)
func oneArg(op string, args []any) any {
	if len(args) != 1 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
	return args[0]
}
