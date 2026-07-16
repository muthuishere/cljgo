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

	// get-thread-bindings / push-thread-bindings / pop-thread-bindings: the
	// raw thread-binding-frame primitives bound-fn*/bound-fn ride on
	// (design/08 batch E, ADR 0022) — the same lang.PushThreadBindings/
	// PopThreadBindings the `binding` special form itself uses
	// (pkg/analyzer parseBinding), now exposed as ordinary fns so
	// core.clj's bound-fn* can capture-and-replay a binding frame across
	// goroutines without new evaluator machinery.
	def("get-thread-bindings", func(args ...any) any {
		return lang.GetThreadBindings()
	})
	def("push-thread-bindings", func(args ...any) any {
		m, ok := oneArg("push-thread-bindings", args).(lang.IPersistentMap)
		if !ok {
			panic(fmt.Errorf("push-thread-bindings: not a map: %s", lang.PrintString(args[0])))
		}
		lang.PushThreadBindings(m)
		return nil
	})
	def("pop-thread-bindings", func(args ...any) any {
		lang.PopThreadBindings()
		return nil
	})

	// var-get / var-set: (var-get #'v) derefs a Var directly (distinct from
	// the reader's #'v => (var v) special form, which already yields the
	// Var itself — var-get is the fn wrapper the suite's intern.cljc
	// exercises: (var-get (intern ns name val))). var-set requires an
	// existing thread binding (Clojure's contract; set! on an unbound var
	// throws "Can't change/establish root binding").
	def("var-get", func(args ...any) any {
		v, ok := oneArg("var-get", args).(*lang.Var)
		if !ok {
			panic(fmt.Errorf("var-get: not a var: %s", lang.PrintString(args[0])))
		}
		return v.Get()
	})
	def("var-set", func(args ...any) any {
		v, val := twoArgs("var-set", args)
		vr, ok := v.(*lang.Var)
		if !ok {
			panic(fmt.Errorf("var-set: not a var: %s", lang.PrintString(v)))
		}
		return vr.Set(val)
	})

	// alter-var-root: (alter-var-root #'v f & args) -> (v.AlterRoot f args),
	// the mutable-root primitive derive/underive ride on (ADR 0022 batch E,
	// design/08). Oracle: (def ^:private g 0) (alter-var-root #'g inc) => 1.
	def("alter-var-root", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: alter-var-root", len(args)))
		}
		v, ok := args[0].(*lang.Var)
		if !ok {
			panic(fmt.Errorf("alter-var-root: not a var: %s", lang.PrintString(args[0])))
		}
		fn, ok := args[1].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("alter-var-root: not a function: %s", lang.PrintString(args[1])))
		}
		return v.AlterRoot(fn, lang.NewSliceSeq(args[2:]))
	})

	// special-symbol?: a static membership check against the JVM's
	// Compiler/specials set (design/08 batch E) — NOT "does cljgo's
	// analyzer implement this as a special form". Faithful to the oracle
	// even where cljgo's analyzer subset differs (e.g. cljgo has no
	// case*/new/./deftype*/letfn*/reify*/monitor-enter/monitor-exit/
	// import*, yet special-symbol? must still say true for them; and
	// cljgo's analyzer treats `binding` as special for implementation
	// convenience even though the JVM does not — see the `binding` Var
	// placeholder in builtins.go for the resolve-side half of that fix).
	// Oracle (clojure 1.12): (special-symbol? 'quote) => true;
	// (special-symbol? 'binding) => false; (special-symbol? 'a) => false.
	def("special-symbol?", func(args ...any) any {
		x := oneArg("special-symbol?", args)
		sym, ok := x.(*lang.Symbol)
		if !ok {
			return false
		}
		_, isSpecial := jvmSpecialSymbols[sym.FullName()]
		return isSpecial
	})

	// intern: (intern ns name) / (intern ns name val) -> the Var, creating
	// it in ns if absent (2-arity leaves an existing var's root untouched;
	// 3-arity always sets it). ns may be a Namespace or its symbol; a
	// symbol naming no existing namespace throws (the-ns semantics) rather
	// than auto-creating one. Oracle-verified (clojure 1.12,
	// clojure-test-suite intern.cljc): (intern 'unknown-ns 'x) throws.
	def("intern", func(args ...any) any {
		if len(args) != 2 && len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: intern", len(args)))
		}
		ns := theNS("intern", args[0])
		sym := symArg("intern", args[1])
		if len(args) == 3 {
			return ns.InternWithValue(sym, args[2], true)
		}
		return ns.Intern(sym)
	})

	// create-ns: (create-ns sym) -> the namespace named sym, creating it
	// if absent (does not switch *ns*, unlike in-ns).
	def("create-ns", func(args ...any) any {
		return lang.FindOrCreateNamespace(symArg("create-ns", oneArg("create-ns", args)))
	})

	// find-ns: (find-ns sym) -> the namespace named sym, or nil.
	def("find-ns", func(args ...any) any {
		ns := lang.FindNamespace(symArg("find-ns", oneArg("find-ns", args)))
		if ns == nil {
			return nil
		}
		return ns
	})

	// get-validator / set-validator!: the atom (and, in principle, var)
	// validator surface (design/08 batch E). Only *lang.Atom implements a
	// working Validator/SetValidator today; other IRefs still panic
	// "not implemented" (pkg/lang/var.go), an acceptable gap since the
	// suite only exercises atoms (core_test/atom.cljc).
	def("get-validator", func(args ...any) any {
		r, ok := oneArg("get-validator", args).(lang.IRef)
		if !ok {
			panic(fmt.Errorf("get-validator: not a ref: %s", lang.PrintString(args[0])))
		}
		return r.Validator()
	})
	def("set-validator!", func(args ...any) any {
		r, vf := twoArgs("set-validator!", args)
		ref, ok := r.(lang.IRef)
		if !ok {
			panic(fmt.Errorf("set-validator!: not a ref: %s", lang.PrintString(r)))
		}
		var fn lang.IFn
		if vf != nil {
			fn, ok = vf.(lang.IFn)
			if !ok {
				panic(fmt.Errorf("set-validator!: not a function: %s", lang.PrintString(vf)))
			}
		}
		ref.SetValidator(fn)
		return nil
	})
}

// jvmSpecialSymbols is clojure.lang.Compiler/specials — the fixed set
// special-symbol? checks membership against, independent of which of
// these cljgo's own analyzer implements (design/08 batch E).
var jvmSpecialSymbols = map[string]struct{}{
	"&": {}, ".": {}, "case*": {}, "catch": {}, "def": {}, "deftype*": {},
	"do": {}, "finally": {}, "fn*": {}, "if": {}, "clojure.core/import*": {},
	"import*": {}, "let*": {}, "letfn*": {}, "loop*": {}, "new": {},
	"quote": {}, "recur": {}, "reify*": {}, "set!": {}, "throw": {},
	"try": {}, "var": {}, "monitor-enter": {}, "monitor-exit": {},
}

// theNS coerces an intern-family argument to a Namespace: a Namespace
// value passes through; a Symbol must name an EXISTING namespace (never
// auto-created), matching Clojure's (the-ns sym) contract.
func theNS(op string, v any) *lang.Namespace {
	switch x := v.(type) {
	case *lang.Namespace:
		return x
	case *lang.Symbol:
		ns := lang.FindNamespace(x)
		if ns == nil {
			panic(fmt.Errorf("%s: no such namespace: %s", op, x.FullName()))
		}
		return ns
	default:
		panic(fmt.Errorf("%s: not a namespace or symbol: %s", op, lang.PrintString(v)))
	}
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
