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

	// --- data + state primitives that core/test.cljg consumes. All are
	// real clojure.core fns (precedence-safe additions, not renames).

	def("inc", func(args ...any) any {
		return lang.Add(oneArg("inc", args), int64(1))
	})
	def("dec", func(args ...any) any {
		return lang.Sub(oneArg("dec", args), int64(1))
	})
	def("get", func(args ...any) any {
		switch len(args) {
		case 2:
			return lang.Get(args[0], args[1])
		case 3:
			return lang.GetDefault(args[0], args[1], args[2])
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: get", len(args)))
		}
	})
	def("assoc", func(args ...any) any {
		if len(args) < 3 || len(args)%2 == 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: assoc", len(args)))
		}
		acc := args[0]
		for i := 1; i < len(args); i += 2 {
			acc = lang.Assoc(acc, args[i], args[i+1])
		}
		return acc
	})
	def("str", func(args ...any) any {
		var b strings.Builder
		for _, a := range args {
			if a == nil {
				continue // (str nil) => "", per clojure.core
			}
			b.WriteString(lang.ToString(a))
		}
		return b.String()
	})

	// Seq/coll + symbol/keyword primitives that core.clj's destructuring
	// machinery consumes (nth, nthnext, nnext, count, gensym, conj,
	// contains?, keys, name, namespace, symbol, keyword, and predicates).
	e.internSeqBuiltins(def)

	// Sequence & collection library runtime primitives (lazy-seq*, the
	// range/repeat/iterate/cycle producers, sort/sort-by/dissoc/vec/vals,
	// reduced, <=/>=/quot/rem/max/min and the numeric/value predicates)
	// that core.clj's map/filter/reduce/take/… are built on.
	e.internCollBuiltins(def)

	// Polymorphism substrate (defprotocol/deftype/defrecord/extend-*):
	// dispatch table + instance/registry builtins the core/protocols.cljg
	// macros expand onto (protocols.go).
	e.internProtocolBuiltins(def)

	// Multimethod substrate (defmulti/defmethod + methods/get-method/
	// remove-method): the flat =-based dispatch table the core.clj
	// defmulti/defmethod macros expand onto (multimethod_builtins.go).
	e.internMultimethodBuiltins(def)

	// atom / swap! / reset! / deref: the minimal mutable-cell set
	// (clojure.core). test.cljg holds its report counters in an atom.
	def("atom", func(args ...any) any {
		return lang.NewAtom(oneArg("atom", args))
	})
	def("swap!", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: swap!", len(args)))
		}
		a, ok := args[0].(*lang.Atom)
		if !ok {
			panic(fmt.Errorf("swap! expects an atom, got: %s", lang.PrintString(args[0])))
		}
		f, ok := args[1].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("swap! expects a function, got: %s", lang.PrintString(args[1])))
		}
		return a.Swap(f, lang.NewList(args[2:]...))
	})
	def("reset!", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: reset!", len(args)))
		}
		a, ok := args[0].(*lang.Atom)
		if !ok {
			panic(fmt.Errorf("reset! expects an atom, got: %s", lang.PrintString(args[0])))
		}
		return a.Reset(args[1])
	})
	def("deref", func(args ...any) any {
		d, ok := oneArg("deref", args).(lang.IDeref)
		if !ok {
			panic(fmt.Errorf("deref expects a dereferenceable, got: %s", lang.PrintString(args[0])))
		}
		return d.Deref()
	})

	// alter-meta!: (alter-meta! ref f & args) => (f (meta ref) & args)
	// becomes the new metadata (clojure.core). Backs deftest attaching a
	// :test thunk onto the test var.
	def("alter-meta!", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: alter-meta!", len(args)))
		}
		f, ok := args[1].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("alter-meta! expects a function, got: %s", lang.PrintString(args[1])))
		}
		rest := lang.NewList(args[2:]...)
		switch ref := args[0].(type) {
		case *lang.Var:
			return ref.AlterMeta(f, rest)
		case *lang.Namespace:
			return ref.AlterMeta(f, rest)
		default:
			panic(fmt.Errorf("alter-meta! expects a var or namespace, got: %s", lang.PrintString(args[0])))
		}
	})

	// require: M1 semantics with libspec support — the embedded namespaces
	// (e.g. clojure.string, clojure.test) are loaded at boot, so require
	// asserts the namespace exists (filesystem loading is not wired yet) and
	// then honors the libspec options. Accepted spec shapes match Clojure:
	//   (require 'clojure.string)                       ; bare symbol
	//   (require '[clojure.string :as str])             ; :as alias
	//   (require '[clojure.string :refer [join]])       ; :refer some
	//   (require '[clojure.string :refer :all])         ; :refer all publics
	//   (require '[clojure.string :as s :refer [join]]) ; both
	//   (require '(clojure string set))                 ; prefix list (bonus)
	// :as creates a namespace alias in the CURRENT ns (so the analyzer's
	// alias resolution — CurrentNS().LookupAlias — sees it); :refer interns
	// the named public vars into the current ns. Unknown options are no-ops.
	def("require", func(args ...any) any {
		for _, a := range args {
			requireSpec(a, nil)
		}
		return nil
	})

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

	// -guarded-call is the interim try/catch seam for core/test.cljg:
	// (-guarded-call thunk handler) runs (thunk); on a panic it runs
	// (handler recovered-value) and returns that. The evaluator has no
	// try/catch yet (analyzer blocks "try"); this host recover is how
	// clojure.test counts :error without it.
	defPrivate("-guarded-call", func(args ...any) (result any) {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -guarded-call", len(args)))
		}
		thunk, ok := args[0].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("-guarded-call expects a thunk, got: %s", lang.PrintString(args[0])))
		}
		handler, ok := args[1].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("-guarded-call expects a handler, got: %s", lang.PrintString(args[1])))
		}
		defer func() {
			if r := recover(); r != nil {
				var caught any = r
				if err, isErr := r.(error); isErr {
					caught = err
				}
				result = handler.Invoke(caught)
			}
		}()
		return thunk.Invoke()
	})

	// -collect-test-vars / -all-test-vars back run-tests / run-all-tests:
	// clojure.test discovers tests by :test metadata, not by filename.
	defPrivate("-collect-test-vars", func(args ...any) any {
		var nsList []*lang.Namespace
		if len(args) >= 1 && args[0] != nil {
			for s := lang.Seq(args[0]); s != nil; s = s.Next() {
				sym, ok := s.First().(*lang.Symbol)
				if !ok {
					panic(fmt.Errorf("-collect-test-vars expects namespace symbols, got: %s", lang.PrintString(s.First())))
				}
				ns := lang.FindNamespace(sym)
				if ns == nil {
					panic(fmt.Errorf("no namespace: %s", sym.FullName()))
				}
				nsList = append(nsList, ns)
			}
		}
		if len(nsList) == 0 {
			nsList = append(nsList, currentNS())
		}
		var vars []any
		for _, ns := range nsList {
			vars = collectTestVars(ns, vars)
		}
		return lang.NewList(vars...)
	})
	defPrivate("-all-test-vars", func(args ...any) any {
		var vars []any
		for s := lang.AllNamespaces(); s != nil; s = s.Next() {
			if ns, ok := s.First().(*lang.Namespace); ok {
				vars = collectTestVars(ns, vars)
			}
		}
		return lang.NewList(vars...)
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

	// --- Exceptions: ex-info / ex-data / ex-message / ex-cause -----------
	e.registerExceptionBuiltins(def)

	// --- clojure.test host seams (core/test.cljg, ADR 0012) --------------
	e.registerTestBuiltins(defPrivate)

	// --- regex core fns + clojure.string host prims (core/string.cljg) ---
	e.internStringBuiltins(def, defPrivate)

	// --- var reflection (resolve/find-var/ns-resolve/var?/eval): the
	// clojure-test-suite harness surface (ADR 0022, var_builtins.go).
	e.internVarBuiltins(def)

	// --- transients (transient/persistent!/conj!/assoc!/dissoc!/disj!/
	// pop!): Batch 3 (ADR 0022, transient_builtins.go). State lives in
	// pkg/lang transient types, so eval + emitted Go share it identically.
	e.internTransientBuiltins(def)
	// --- numeric tower (bigint/bigdec/ratios, promotion, bit-*, parse-*,
	// rand-*, ==): design/08 §5 Batch 2 (numeric_builtins.go).
	e.internNumericBuiltins(def)
	// --- Batch 1 cheap-breadth predicates + coercions + seq/coll host prims
	// (ADR 0022, design/08 §5, predicate_builtins.go). The compositional
	// fns that ride on these live in core/predicates.cljg (loadPredicates).
	e.internPredicateBuiltins(def)
	// --- version: (clojure-version)/*clojure-version* (the language level
	// we target) + (cljgo-version)/*cljgo-version* (ours, incl. the host Go
	// toolchain) — version_builtins.go, pkg/version is the source of truth.
	e.internVersionBuiltins(def)

	// --- Result/Option primitives (ADR 0014, spike S11) ------------------
	//
	// Constructors, predicates and combinators over the pkg/lang tagged
	// types (result.go). Registered as Go builtins so BOTH modes have them
	// identically — rt.Boot() interns these into clojure.core before an
	// emitted binary's Load() runs. `none` is a VALUE (a var bound to the
	// shared sentinel), not a call; `let?` is a core.clj macro over these.
	def("ok", func(args ...any) any { return lang.NewOk(oneArg("ok", args)) })
	def("err", func(args ...any) any { return lang.NewErr(oneArg("err", args)) })
	def("just", func(args ...any) any { return lang.NewJust(oneArg("just", args)) })

	// none: the single Option-absence value (not a function).
	noneVar := lang.NSCore.Intern(lang.NewSymbol("none"))
	noneVar.BindRoot(lang.None)

	def("result?", func(args ...any) any { return lang.IsResult(oneArg("result?", args)) })
	def("ok?", func(args ...any) any { return lang.IsOk(oneArg("ok?", args)) })
	def("err?", func(args ...any) any { return lang.IsErr(oneArg("err?", args)) })
	def("option?", func(args ...any) any { return lang.IsOption(oneArg("option?", args)) })
	def("just?", func(args ...any) any { return lang.IsJust(oneArg("just?", args)) })
	def("none?", func(args ...any) any { return lang.IsNone(oneArg("none?", args)) })

	// unwrap: the bridge to the exception world. ok/just -> payload;
	// err/none -> throw an ex-info carrying the failure payload (so a
	// railway value can escape into try/catch). Anything else is an error.
	def("unwrap", func(args ...any) any {
		x := oneArg("unwrap", args)
		switch {
		case lang.IsOk(x), lang.IsJust(x):
			return lang.ResultPayload(x)
		case lang.IsErr(x):
			data := lang.NewMap(lang.NewKeyword("cljgo/error"), lang.ResultPayload(x))
			panic(lang.NewExceptionInfo("unwrap called on "+lang.PrintString(x), data))
		case lang.IsNone(x):
			panic(lang.NewExceptionInfo("unwrap called on none", lang.NewMap()))
		}
		panic(fmt.Errorf("unwrap expects a Result or Option, got: %s", lang.PrintString(x)))
	})

	// unwrap-or: payload of ok/just, else the supplied default (err/none
	// and non-tagged values yield the default — never throws).
	def("unwrap-or", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: unwrap-or", len(args)))
		}
		x := args[0]
		if lang.IsOk(x) || lang.IsJust(x) {
			return lang.ResultPayload(x)
		}
		return args[1]
	})

	// map-ok: apply f to an ok/just payload, re-wrapping in the same tag;
	// err/none pass through unchanged (railway happy-path map).
	def("map-ok", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: map-ok", len(args)))
		}
		f, x := args[0], args[1]
		switch {
		case lang.IsOk(x):
			return lang.NewOk(lang.Apply1(f, lang.ResultPayload(x)))
		case lang.IsJust(x):
			return lang.NewJust(lang.Apply1(f, lang.ResultPayload(x)))
		}
		return x
	})

	// map-err: apply f to an err payload, re-wrapping as err; everything
	// else (ok/just/none) passes through unchanged.
	def("map-err", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: map-err", len(args)))
		}
		f, x := args[0], args[1]
		if lang.IsErr(x) {
			return lang.NewErr(lang.Apply1(f, lang.ResultPayload(x)))
		}
		return x
	})

	// and-then: railway bind. f receives the UNWRAPPED ok/just payload and
	// must itself return a Result/Option; err/none short-circuit unchanged.
	def("and-then", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: and-then", len(args)))
		}
		f, x := args[0], args[1]
		if lang.IsOk(x) || lang.IsJust(x) {
			return lang.Apply1(f, lang.ResultPayload(x))
		}
		return x
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

	// refer: (refer 'ns) maps ALL public interned vars of the named
	// namespace into the current one; (refer 'ns :only '[a b]) restricts to
	// the listed names and (refer 'ns :exclude '[c]) omits the listed names
	// (Clojure's refer filters — :rename is not supported in M1).
	def("refer", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: refer"))
		}
		sym, ok := args[0].(*lang.Symbol)
		if !ok {
			panic(fmt.Errorf("refer expects a symbol, got: %s", lang.PrintString(args[0])))
		}
		target := lang.FindNamespace(sym)
		if target == nil {
			panic(fmt.Errorf("no namespace: %s", sym.FullName()))
		}
		only := map[string]struct{}{}
		exclude := map[string]struct{}{}
		haveOnly := false
		for i := 1; i < len(args); i += 2 {
			kw, ok := args[i].(lang.Keyword)
			if !ok {
				panic(fmt.Errorf("refer option must be a keyword, got: %s", lang.PrintString(args[i])))
			}
			if i+1 >= len(args) {
				panic(fmt.Errorf("refer option %s is missing a value", kw.String()))
			}
			val := args[i+1]
			switch kw.Name() {
			case "only":
				haveOnly = true
				collectSymNames(val, only)
			case "exclude":
				collectSymNames(val, exclude)
			default:
				// :rename and other options are no-ops in M1.
			}
		}
		referSelected(currentNS(), target, only, haveOnly, exclude)
		return nil
	})

	// --- M4 channels & go (design/05 §4) ---------------------------------
	//
	// Goroutines ARE the cheap thing core.async's CPS `go` emulates on the
	// JVM, so there is NO IOC rewrite: `(go ...)` runs its body in a REAL
	// goroutine and <!/>! simply block (design/05 §4). All ops are Go
	// builtins wrapping the pkg/lang runtime helpers, so BOTH modes behave
	// identically — the interpreter calls them directly and rt.Boot() interns
	// the same builtins into an emitted binary. These are core.async names
	// (chan/>!/<!/close!/go/thread), none of which exist in clojure.core, so
	// this is a precedence-safe addition (CLAUDE.md), never a shadow/rename.

	// (chan) unbuffered; (chan n) buffered.
	def("chan", func(args ...any) any {
		switch len(args) {
		case 0:
			return lang.NewChan(0)
		case 1:
			n, ok := args[0].(int64)
			if !ok {
				panic(fmt.Errorf("chan expects an integer buffer size, got: %s", lang.PrintString(args[0])))
			}
			return lang.NewChan(int(n))
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: chan", len(args)))
		}
	})

	// >! / >!! : blocking put (aliases — no parking distinction without IOC).
	chanSend := func(op string) func(args ...any) any {
		return func(args ...any) any {
			if len(args) != 2 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
			}
			c, ok := args[0].(*lang.Channel)
			if !ok {
				panic(fmt.Errorf("%s expects a channel, got: %s", op, lang.PrintString(args[0])))
			}
			return lang.ChanSend(c, args[1])
		}
	}
	def(">!", chanSend(">!"))
	def(">!!", chanSend(">!!"))

	// <! / <!! : blocking take; closed+drained => nil (aliases).
	chanRecv := func(op string) func(args ...any) any {
		return func(args ...any) any {
			c, ok := oneArg(op, args).(*lang.Channel)
			if !ok {
				panic(fmt.Errorf("%s expects a channel, got: %s", op, lang.PrintString(args[0])))
			}
			return lang.ChanRecv(c)
		}
	}
	def("<!", chanRecv("<!"))
	def("<!!", chanRecv("<!!"))

	def("close!", func(args ...any) any {
		c, ok := oneArg("close!", args).(*lang.Channel)
		if !ok {
			panic(fmt.Errorf("close! expects a channel, got: %s", lang.PrintString(args[0])))
		}
		return lang.ChanClose(c)
	})

	// go* is the runtime seam: (go* thunk) spawns a goroutine running the
	// 0-arg thunk and returns its result channel (design/05 §4). `go` and
	// `thread` are macros (below) that wrap their body in (fn* [] body...)
	// and call go* — so the emitter needs NO new op: it compiles the fn
	// literal and the go* invoke like any other call, and lang.Go does the
	// real `go func(){}()` for both modes.
	def("go*", func(args ...any) any {
		return lang.Go(oneArg("go*", args))
	})

	// go / thread macros: (go body...) => (clojure.core/go* (fn* [] body...)).
	// thread is an alias of go (no parking distinction without IOC,
	// design/05 §4). Registered as native macros (like defmacro) so the
	// whole feature stays in pkg/lang + pkg/eval builtins — no core.clj edit.
	goMacro := func(args ...any) any {
		// args = [&form &env body...]; wrap the body in a 0-arg fn* thunk.
		body := args[2:]
		fnParts := append([]any{symFnStar, lang.NewVector()}, body...)
		return lang.NewList(lang.NewSymbol("clojure.core/go*"), lang.NewList(fnParts...))
	}
	def("go", goMacro).SetMacro()
	def("thread", goMacro).SetMacro()

	// M4+ concurrency: alts!/timeout/dropping+sliding buffers (chan_builtins.go).
	e.internChanExtras(def)

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
	referSelected(ns, from, nil, false, nil)
}

// referSelected refers the public vars interned in `from` into ns, honoring
// refer's :only / :exclude filters. When haveOnly is true, only names in
// `only` are referred; names in `exclude` are always skipped.
func referSelected(ns, from *lang.Namespace, only map[string]struct{}, haveOnly bool, exclude map[string]struct{}) {
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
		name := sym.Name()
		if haveOnly {
			if _, in := only[name]; !in {
				continue
			}
		}
		if _, ex := exclude[name]; ex {
			continue
		}
		ns.Refer(sym, v)
	}
}

// collectSymNames adds every symbol name in the seqable spec to set.
func collectSymNames(spec any, set map[string]struct{}) {
	for s := lang.Seq(spec); s != nil; s = s.Next() {
		if sym, ok := s.First().(*lang.Symbol); ok {
			set[sym.Name()] = struct{}{}
		}
	}
}

// kwTest is the :test metadata key clojure.test tags test vars with.
var kwTest = lang.NewKeyword("test")

// collectTestVars appends every var interned in ns (not merely referred
// into it) that carries truthy :test metadata — clojure.test's
// metadata-driven discovery. Order follows the namespace's mapping seq.
func collectTestVars(ns *lang.Namespace, acc []any) []any {
	for s := lang.Seq(ns.Mappings()); s != nil; s = s.Next() {
		entry, ok := s.First().(lang.IMapEntry)
		if !ok {
			continue
		}
		v, ok := entry.Val().(*lang.Var)
		if !ok || v.Namespace() != ns {
			continue
		}
		if lang.IsTruthy(lang.Get(v.Meta(), kwTest)) {
			acc = append(acc, v)
		}
	}
	return acc
}

// requireSpec processes one require spec: a bare namespace symbol, a libspec
// vector `[lib & opts]`, or a prefix list `(prefix sub ...)`. `prefix` (may
// be nil) is the dotted prefix accumulated from an enclosing prefix list.
func requireSpec(spec any, prefix *lang.Symbol) {
	if sym, ok := spec.(*lang.Symbol); ok {
		loadLib(combinePrefix(prefix, sym), nil)
		return
	}
	seq := lang.Seq(spec)
	if seq == nil {
		panic(fmt.Errorf("require expects a namespace symbol or libspec, got: %s", lang.PrintString(spec)))
	}
	head, ok := seq.First().(*lang.Symbol)
	if !ok {
		panic(fmt.Errorf("require expects a libspec whose head is a symbol, got: %s", lang.PrintString(spec)))
	}
	rest := seq.Next()
	// Distinguish a libspec-with-options from a prefix list: options are
	// keyword/value pairs (`[lib :as x]`); a prefix list holds further
	// libspecs (symbols/vectors) after the prefix (`(clojure string set)`).
	if rest != nil {
		if _, isKw := rest.First().(lang.Keyword); !isKw {
			full := combinePrefix(prefix, head)
			for x := rest; x != nil; x = x.Next() {
				requireSpec(x.First(), full)
			}
			return
		}
	}
	loadLib(combinePrefix(prefix, head), rest)
}

// combinePrefix joins a prefix-list prefix with a leaf symbol into a dotted
// namespace symbol (`clojure` + `string` => `clojure.string`).
func combinePrefix(prefix, sym *lang.Symbol) *lang.Symbol {
	if prefix == nil {
		return sym
	}
	return lang.NewSymbol(prefix.Name() + "." + sym.Name())
}

// loadLib asserts the (embedded) namespace exists, then applies the libspec
// options: :as adds an alias to the current ns; :refer interns the named
// public vars (or all publics for :refer :all) into the current ns.
func loadLib(libSym *lang.Symbol, opts lang.ISeq) {
	target := lang.FindNamespace(libSym)
	if target == nil {
		panic(fmt.Errorf("could not locate namespace %s (filesystem loading not yet supported; only embedded namespaces are available)", libSym.FullName()))
	}
	for s := opts; s != nil; s = s.Next() {
		kw, ok := s.First().(lang.Keyword)
		if !ok {
			panic(fmt.Errorf("require libspec option must be a keyword, got: %s", lang.PrintString(s.First())))
		}
		s = s.Next()
		if s == nil {
			panic(fmt.Errorf("require libspec option %s is missing a value", kw.String()))
		}
		val := s.First()
		switch kw.Name() {
		case "as":
			aliasSym, ok := val.(*lang.Symbol)
			if !ok {
				panic(fmt.Errorf(":as expects a symbol, got: %s", lang.PrintString(val)))
			}
			currentNS().AddAlias(aliasSym, target)
		case "refer":
			referSpec(currentNS(), target, val)
		default:
			// :reload, :verbose, :as-alias, etc. — no-ops in M1.
		}
	}
}

// referSpec handles a :refer value: `:all` refers every public var; a vector
// of symbols interns exactly those (throwing if a name is not interned in the
// target namespace, as Clojure does).
func referSpec(ns, from *lang.Namespace, spec any) {
	if kw, ok := spec.(lang.Keyword); ok {
		if kw.Name() == "all" {
			referAll(ns, from)
			return
		}
		panic(fmt.Errorf(":refer expects a vector of symbols or :all, got: %s", lang.PrintString(spec)))
	}
	seq := lang.Seq(spec)
	if seq == nil {
		panic(fmt.Errorf(":refer expects a vector of symbols or :all, got: %s", lang.PrintString(spec)))
	}
	for s := seq; s != nil; s = s.Next() {
		sym, ok := s.First().(*lang.Symbol)
		if !ok {
			panic(fmt.Errorf(":refer expects symbols, got: %s", lang.PrintString(s.First())))
		}
		v := from.FindInternedVar(sym)
		if v == nil {
			panic(fmt.Errorf("%s does not exist in namespace %s", sym.Name(), from.Name().Name()))
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
