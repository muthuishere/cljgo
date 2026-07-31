package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// internBuiltins pre-interns the native IFns into clojure.core. The four
// evaluator-coupled builtins layered on corelib.Def are macroexpand-1,
// macroexpand, eval and load-file; require-go registers per-evaluator host
// aliases. In an AOT-compiled binary these stay bound to the corelib stubs
// (aot_stubs.go), since a compiled binary has no reader or analyzer linked
// (ADR 0046).
func (e *Evaluator) internBuiltins() {
	corelib.RegisterAll()
	def := corelib.Def

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

	corelib.SetLibFileLoader(func(libSym *lang.Symbol) { loadLibFile(e, libSym) })

	def("require-go", func(args ...any) any {
		e.registerRequireGo(args)
		return nil
	})

	def("eval", func(args ...any) any {
		res, err := e.EvalForm(oneArg("eval", args))
		if err != nil {
			panic(err)
		}
		return res
	})

	// load-file (issue 167): read and evaluate every top-level form in an
	// arbitrary file path, exactly the load frame libload.go pushes for a
	// required namespace, keyed off the given path directly instead of a
	// resolved lib symbol. See loadFile in libload.go.
	def("load-file", func(args ...any) any {
		path, ok := oneArg("load-file", args).(string)
		if !ok {
			panic(fmt.Errorf("load-file: cannot coerce %s to a file path", lang.PrintString(args[0])))
		}
		return loadFile(e, path)
	})
}

// oneArg asserts a 1-arg builtin arity and returns the argument.
func oneArg(op string, args []any) any {
	if len(args) != 1 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
	return args[0]
}
