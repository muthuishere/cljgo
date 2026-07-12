package eval

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Out is where println writes. Package-level and swappable for tests; the
// REPL driver may point it elsewhere.
var Out io.Writer = os.Stdout

// nativeFn wraps a Go function as a lang.IFn (the pre-interned builtins of
// design/03 §8 v0). Errors panic, per the IFn-boundary convention.
type nativeFn struct {
	nm string
	fn func(args ...any) any
}

var _ lang.IFn = (*nativeFn)(nil)

func (n *nativeFn) Invoke(args ...any) any     { return n.fn(args...) }
func (n *nativeFn) ApplyTo(args lang.ISeq) any { return n.Invoke(lang.ToSlice(args)...) }
func (n *nativeFn) String() string             { return "#object[" + n.nm + "]" }

// internBuiltins pre-interns the native IFns into clojure.core: the v0
// set (+ - * / = < > pr-str println; design/03 §8), the M1 namespace
// ops (in-ns alias refer), the REPL affordance dynamic vars
// (*1 *2 *3 *e; design/03 §7b), and the v2 seq/coll primitives that
// syntax-quote expansions and core.clj's macros consume (list, cons,
// first, next, rest, second, seq, concat, apply, vector, hash-map,
// hash-set, with-meta, meta, seq?, string?, not) plus macroexpand-1 /
// macroexpand (design/03 §4). Namespaces made with `New` refer core's
// publics, as Clojure's `user` does; a bare in-ns namespace starts empty
// and reaches core via qualified names or (clojure.core/refer ...).
// Arithmetic goes through lang's numeric tower (int64 fast path, overflow
// checked); = is lang.Equiv.
func (e *Evaluator) internBuiltins() {
	def := func(name string, fn func(args ...any) any) *lang.Var {
		v := lang.NSCore.Intern(lang.NewSymbol(name))
		v.BindRoot(&nativeFn{nm: name, fn: fn})
		return v
	}
	// defPrivate interns a core-internal helper (:private true — skipped
	// by refer, invisible to user code by unqualified name).
	defPrivate := func(name string, fn func(args ...any) any) {
		v := def(name, fn)
		v.SetMeta(v.Meta().Assoc(lang.KWPrivate, true).(lang.IPersistentMap))
	}

	def("+", func(args ...any) any {
		var acc any = int64(0)
		for i, a := range args {
			if i == 0 {
				acc = a
				continue
			}
			acc = lang.Add(acc, a)
		}
		return acc
	})
	def("-", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: -"))
		}
		if len(args) == 1 {
			return lang.Sub(int64(0), args[0])
		}
		acc := args[0]
		for _, a := range args[1:] {
			acc = lang.Sub(acc, a)
		}
		return acc
	})
	def("*", func(args ...any) any {
		var acc any = int64(1)
		for i, a := range args {
			if i == 0 {
				acc = a
				continue
			}
			acc = lang.Multiply(acc, a)
		}
		if len(args) == 0 {
			return int64(1)
		}
		return acc
	})
	def("/", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: /"))
		}
		if len(args) == 1 {
			return lang.Divide(int64(1), args[0])
		}
		acc := args[0]
		for _, a := range args[1:] {
			acc = lang.Divide(acc, a)
		}
		return acc
	})
	def("=", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: ="))
		}
		for i := 1; i < len(args); i++ {
			if !lang.Equiv(args[i-1], args[i]) {
				return false
			}
		}
		return true
	})
	def("<", chainCompare("<", lang.LT))
	def(">", chainCompare(">", lang.GT))

	def("pr-str", func(args ...any) any {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = lang.PrintString(a)
		}
		return strings.Join(parts, " ")
	})
	def("println", func(args ...any) any {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = lang.ToString(a)
		}
		fmt.Fprintln(Out, strings.Join(parts, " "))
		return nil
	})

	// --- v2 seq/coll primitives (macro fuel: syntax-quote expands to
	// clojure.core/{list,concat,seq,apply,vector,hash-map,hash-set,
	// with-meta}, and core.clj's macro bodies use the rest). Eager and
	// minimal for M1; the lazy seq library is M5.

	def("list", func(args ...any) any {
		return lang.NewList(args...)
	})
	def("cons", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: cons", len(args)))
		}
		return lang.NewCons(args[0], args[1])
	})
	def("first", func(args ...any) any {
		return lang.First(oneArg("first", args))
	})
	def("next", func(args ...any) any {
		return lang.Next(oneArg("next", args))
	})
	def("rest", func(args ...any) any {
		return lang.Rest(oneArg("rest", args))
	})
	def("second", func(args ...any) any {
		return lang.First(lang.Next(oneArg("second", args)))
	})
	def("seq", func(args ...any) any {
		return lang.Seq(oneArg("seq", args))
	})
	// concat is EAGER in M1 (real Clojure's is lazy); fine for macro
	// expansion fuel, revisit with the seq library (M5).
	def("concat", func(args ...any) any {
		var items []any
		for _, a := range args {
			for s := lang.Seq(a); s != nil; s = s.Next() {
				items = append(items, s.First())
			}
		}
		return lang.NewList(items...)
	})
	def("apply", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: apply", len(args)))
		}
		spread := make([]any, 0, len(args))
		spread = append(spread, args[1:len(args)-1]...)
		for s := lang.Seq(args[len(args)-1]); s != nil; s = s.Next() {
			spread = append(spread, s.First())
		}
		return lang.Apply(args[0], spread)
	})
	def("vector", func(args ...any) any {
		return lang.NewVector(args...)
	})
	def("hash-map", func(args ...any) any {
		return lang.NewMap(args...)
	})
	def("hash-set", func(args ...any) any {
		return lang.NewSet(args...)
	})
	def("with-meta", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: with-meta", len(args)))
		}
		var m lang.IPersistentMap
		if args[1] != nil {
			mm, ok := args[1].(lang.IPersistentMap)
			if !ok {
				panic(fmt.Errorf("with-meta expects a map, got: %s", lang.PrintString(args[1])))
			}
			m = mm
		}
		v, err := lang.WithMeta(args[0], m)
		if err != nil {
			panic(err)
		}
		return v
	})
	def("meta", func(args ...any) any {
		if im, ok := oneArg("meta", args).(lang.IMeta); ok {
			if m := im.Meta(); m != nil {
				return m
			}
		}
		return nil
	})
	def("seq?", func(args ...any) any {
		_, ok := oneArg("seq?", args).(lang.ISeq)
		return ok
	})
	def("string?", func(args ...any) any {
		_, ok := oneArg("string?", args).(string)
		return ok
	})
	def("not", func(args ...any) any {
		return !lang.IsTruthy(oneArg("not", args))
	})

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

	// -set-macro! backs defmacro's expansion: flip the var's :macro flag
	// (design/03 §4 setMacro; JVM spells it (. (var name) (setMacro)) —
	// host interop is v3, so M1 keeps a private core hook).
	defPrivate("-set-macro!", func(args ...any) any {
		v, ok := oneArg("-set-macro!", args).(*lang.Var)
		if !ok {
			panic(fmt.Errorf("-set-macro! expects a var, got: %s", lang.PrintString(args[0])))
		}
		v.SetMacro()
		return v
	})
	// -illegal-argument backs core.clj's expansion-time errors (cond's
	// odd-clause check) until `throw` lands in v3.
	defPrivate("-illegal-argument", func(args ...any) any {
		msg, _ := oneArg("-illegal-argument", args).(string)
		panic(lang.NewIllegalArgumentError(msg))
	})

	// in-ns: create-if-absent and switch *ns* (design/03 §7a). Under a
	// bound *ns* (REPL session, file load) this sets the thread binding,
	// exactly Clojure's in-ns; without one it rebinds the root (Clojure
	// would throw — kept lenient for bare EvalForm use, see setVarValue).
	def("in-ns", func(args ...any) any {
		sym := symbolArg("in-ns", args)
		ns := lang.FindOrCreateNamespace(sym)
		setVarValue(lang.VarCurrentNS, ns)
		return ns
	})

	// alias: (alias 'shorthand 'full.ns-name) in the current namespace.
	def("alias", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: alias", len(args)))
		}
		aliasSym, ok := args[0].(*lang.Symbol)
		if !ok {
			panic(fmt.Errorf("alias expects a symbol, got: %s", lang.PrintString(args[0])))
		}
		nsSym, ok := args[1].(*lang.Symbol)
		if !ok {
			panic(fmt.Errorf("alias expects a symbol, got: %s", lang.PrintString(args[1])))
		}
		target := lang.FindNamespace(nsSym)
		if target == nil {
			panic(fmt.Errorf("no namespace: %s found", nsSym.FullName()))
		}
		currentNS().AddAlias(aliasSym, target)
		return nil
	})

	// refer: minimal M1 semantics — map ALL public interned vars of the
	// named namespace into the current one (no :only/:exclude filters).
	def("refer", func(args ...any) any {
		sym := symbolArg("refer", args)
		target := lang.FindNamespace(sym)
		if target == nil {
			panic(fmt.Errorf("no namespace: %s", sym.FullName()))
		}
		referAll(currentNS(), target)
		return nil
	})

	// *1 *2 *3 *e are proper dynamic vars in core (design/03 §7b); the
	// REPL driver binds them per session and set!s them after each eval.
	for _, name := range []string{"*1", "*2", "*3", "*e"} {
		lang.InternVarReplaceRoot(lang.NSCore, lang.NewSymbol(name), nil).SetDynamic()
	}
}

// oneArg asserts a 1-arg builtin's arity and returns the argument.
func oneArg(op string, args []any) any {
	if len(args) != 1 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
	return args[0]
}

// symbolArg extracts the single symbol argument of a namespace op.
func symbolArg(op string, args []any) *lang.Symbol {
	if len(args) != 1 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
	sym, ok := args[0].(*lang.Symbol)
	if !ok {
		panic(fmt.Errorf("%s expects a symbol, got: %s", op, lang.PrintString(args[0])))
	}
	return sym
}

// currentNS mirrors Evaluator.CurrentNS for builtins (one *ns* world).
func currentNS() *lang.Namespace {
	if ns, ok := lang.VarCurrentNS.Deref().(*lang.Namespace); ok {
		return ns
	}
	return lang.NSCore
}

// setVarValue sets v's thread binding when the current goroutine has one
// (Clojure's set! path, used by in-ns under a bound *ns*), else rebinds
// the root. The fallback is a deliberate M1 leniency: pkg/lang exports no
// "has thread binding?" predicate other than GetThreadBindings, and bare
// EvalForm callers (tests) run without session bindings.
func setVarValue(v *lang.Var, val any) {
	if lang.GetThreadBindings().EntryAt(v) != nil {
		v.Set(val)
	} else {
		v.BindRoot(val)
	}
}

// referAll refers every public var interned in `from` into ns — the
// minimal whole-namespace refer of design/00 §6 (M1).
func referAll(ns, from *lang.Namespace) {
	for s := lang.Seq(from.Mappings()); s != nil; s = s.Next() {
		entry := s.First().(lang.IMapEntry)
		sym, ok := entry.Key().(*lang.Symbol)
		if !ok {
			continue
		}
		v, ok := entry.Val().(*lang.Var)
		if !ok || v.Namespace() != from || !v.IsPublic() {
			continue
		}
		ns.Refer(sym, v)
	}
}

func chainCompare(name string, cmp func(x, y any) bool) func(args ...any) any {
	return func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: %s", name))
		}
		for i := 1; i < len(args); i++ {
			if !cmp(args[i-1], args[i]) {
				return false
			}
		}
		return true
	}
}
