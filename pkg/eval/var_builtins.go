package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internVarBuiltins registers the var-reflection surface the jank
// clojure-test-suite harness needs (ADR 0022, design/08 Batch 0):
// resolve / ns-resolve / find-var / var? / eval. resolve powers the
// portability shim's when-var-exists (a macroexpand-time "does cljgo
// implement this clojure.core var?" check), so it must return nil on a
// miss rather than throw. Wired into internBuiltins by ONE line
// (e.internVarBuiltins(def)), per the merge-friendly discipline.
func (e *Evaluator) internVarBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// var?: is x a Var (design/08 Batch 1 lists it too; the harness uses it).
	def("var?", func(args ...any) any {
		_, ok := oneArg("var?", args).(*lang.Var)
		return ok
	})

	// resolve: (resolve sym) -> the Var sym names as seen from the current
	// namespace (honoring aliases + refers), or nil. (resolve env sym)
	// ignores env — cljgo has no &env-scoped class/local resolution. Never
	// throws on a miss; that is what makes when-var-exists a clean gate.
	def("resolve", func(args ...any) any {
		switch len(args) {
		case 1:
			return resolveInNS(currentNS(), symArg("resolve", args[0]))
		case 2:
			return resolveInNS(currentNS(), symArg("resolve", args[1]))
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: resolve", len(args)))
		}
	})

	// ns-resolve: (ns-resolve ns sym) / (ns-resolve ns env sym) -> Var or
	// nil, resolving sym as seen from ns (a namespace value or its symbol).
	def("ns-resolve", func(args ...any) any {
		var nsArg, symbol any
		switch len(args) {
		case 2:
			nsArg, symbol = args[0], args[1]
		case 3:
			nsArg, symbol = args[0], args[2]
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: ns-resolve", len(args)))
		}
		return resolveInNS(nsArg2("ns-resolve", nsArg), symArg("ns-resolve", symbol))
	})

	// find-var: (find-var 'fully.qualified/name) -> the Var, or nil when the
	// namespace exists but the var doesn't. Throws if the symbol is not
	// namespace-qualified (Clojure's contract).
	def("find-var", func(args ...any) any {
		sym := symArg("find-var", oneArg("find-var", args))
		if !sym.HasNamespace() {
			panic(fmt.Errorf("find-var: not fully qualified symbol: %s", sym.FullName()))
		}
		ns := lang.FindNamespace(lang.NewSymbol(sym.Namespace()))
		if ns == nil {
			panic(fmt.Errorf("no such namespace: %s", sym.Namespace()))
		}
		if v := ns.FindInternedVar(lang.NewSymbol(sym.Name())); v != nil {
			return v
		}
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

// resolveInNS resolves sym to its Var as seen from ns — a qualified name
// goes through ns's aliases then the absolute namespaces; an unqualified
// name goes through ns's mappings — returning nil on any miss. Mirrors
// Evaluator.resolveVar's lookup but never errors (resolve returns nil).
func resolveInNS(ns *lang.Namespace, sym *lang.Symbol) any {
	if sym.HasNamespace() {
		nsSym := lang.NewSymbol(sym.Namespace())
		target := ns.LookupAlias(nsSym)
		if target == nil {
			target = lang.FindNamespace(nsSym)
		}
		if target == nil {
			return nil
		}
		if v := target.FindInternedVar(lang.NewSymbol(sym.Name())); v != nil {
			return v
		}
		return nil
	}
	if m := ns.Mappings().ValAt(sym); m != nil {
		if v, ok := m.(*lang.Var); ok {
			return v
		}
	}
	return nil
}

// symArg coerces a single reflection argument to a Symbol.
func symArg(op string, v any) *lang.Symbol {
	sym, ok := v.(*lang.Symbol)
	if !ok {
		panic(fmt.Errorf("%s expects a symbol, got: %s", op, lang.PrintString(v)))
	}
	return sym
}

// nsArg2 coerces a namespace argument (a Namespace value or its symbol).
func nsArg2(op string, v any) *lang.Namespace {
	switch x := v.(type) {
	case *lang.Namespace:
		return x
	case *lang.Symbol:
		ns := lang.FindNamespace(x)
		if ns == nil {
			panic(fmt.Errorf("no such namespace: %s", x.FullName()))
		}
		return ns
	default:
		panic(fmt.Errorf("%s expects a namespace or symbol, got: %s", op, lang.PrintString(v)))
	}
}
