// Package corelib is the Go-native half of clojure.core: every builtin
// that does not require the tree-walk interpreter (ADR 0043, AOT-core
// piece 2). RegisterAll interns the whole set into clojure.core without
// constructing an Evaluator — pkg/eval layers the 5 interpreter-coupled
// builtins (macroexpand-1, macroexpand, eval, require, require-go) on
// top via the same Def seam, and piece 3's rt.Boot() will call
// RegisterAll directly so emitted binaries never link pkg/eval.
//
// Import discipline (test-enforced, imports_test.go): stdlib, pkg/lang,
// pkg/reader, pkg/version only — never pkg/eval, pkg/analyzer, pkg/ast,
// pkg/emit.
package corelib

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Out is where println/print/pr/prn write when *out* has no thread
// binding pointing elsewhere (the root value of *out*, lang.VarOut, is
// os.Stdout — this package var exists only so tests can swap the default
// without touching a Var). Package-level and swappable for tests; the
// REPL driver may point it elsewhere.
var Out io.Writer = os.Stdout

// outWriter resolves *out* to an io.Writer for the print family
// (design/08 batch E): a thread binding installed by `binding` (e.g.
// with-out-str's string-writer) wins; otherwise falls back to the
// package-level Out (so existing tests that swap Out keep working even
// though *out*'s root is os.Stdout, not Out).
func outWriter() io.Writer {
	if v := lang.VarOut.Deref(); v != os.Stdout {
		if w, ok := v.(io.Writer); ok {
			return w
		}
	}
	return Out
}

// stringWriter is the in-memory io.Writer with-out-str binds *out* to
// (design/08 batch E) — the substrate the private -string-writer /
// -string-writer-str builtins expose to core.clj's with-out-str macro.
type stringWriter struct {
	buf strings.Builder
}

func (w *stringWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *stringWriter) String() string              { return "#object[StringWriter]" }

// taps is the tap>/add-tap/remove-tap registry (design/08 batch E,
// ADR 0022): a process-wide slice of registered fns, guarded by
// tapsMtx. Package-level because tap fns are global (clojure.core's own
// tapset is a single top-level atom, not per-evaluator).
var (
	tapsMtx sync.RWMutex
	taps    []lang.IFn
)

// symFnStar is the fn* special-form symbol the go/thread macros expand
// with (pkg/eval/macro.go keeps its own copy for the macro engine).
var symFnStar = lang.NewSymbol("fn*")

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

// Def interns a Go builtin into clojure.core: the native IFn wrapper
// keeps the exact `#object[name]` printing and error→panic boundary the
// interpreter's def helper always had (design/03 §8).
func Def(name string, fn func(args ...any) any) *lang.Var {
	v := lang.NSCore.Intern(lang.NewSymbol(name))
	v.BindRoot(&nativeFn{nm: name, fn: fn})
	return v
}

// DefPrivate interns a core-internal helper (:private true — skipped
// by refer, invisible to user code by unqualified name).
func DefPrivate(name string, fn func(args ...any) any) {
	v := Def(name, fn)
	v.SetMeta(v.Meta().Assoc(lang.KWPrivate, true).(lang.IPersistentMap))
}

// NewNativeFn wraps a Go function as the same native IFn Def interns —
// the seam pkg/eval's defmacro bootstrap and host-interop path use.
func NewNativeFn(name string, fn func(args ...any) any) lang.IFn {
	return &nativeFn{nm: name, fn: fn}
}

// RegisterAll pre-interns the interpreter-independent native IFns into
// clojure.core: the v0 set (+ - * / = < > pr-str println; design/03
// §8), the M1 namespace ops (in-ns alias refer), the REPL affordance
// dynamic vars (*1 *2 *3 *e; design/03 §7b), and the v2 seq/coll
// primitives that syntax-quote expansions and core.clj's macros consume
// (list, cons, first, next, rest, second, seq, concat, apply, vector,
// hash-map, hash-set, with-meta, meta, seq?, string?, not), plus every
// satellite batch (seq/coll/protocol/multimethod/exception/test/string/
// var/transient/numeric/predicate/array/volatile/version/sorted/misc/
// format/chan). Namespaces made with `New` refer core's publics, as
// Clojure's `user` does; a bare in-ns namespace starts empty and reaches
// core via qualified names or (clojure.core/refer ...). Arithmetic goes
// through lang's numeric tower (int64 fast path, overflow checked);
// = is lang.Equiv. Interpreter-coupled builtins (macroexpand-1,
// macroexpand, eval, require, require-go) are NOT here — pkg/eval
// registers them per evaluator (ADR 0043).
//
// Idempotent the same way the interpreter's per-New() re-interning
// always was: BindRoot replaces the root value.
func RegisterAll() {
	def := Def
	defPrivate := DefPrivate

	// require + the lib-provider registry (require.go, ADR 0046): a
	// compiled binary replays every (require …) form, so this belongs in
	// the interpreter-free half.
	registerRequire(def)
	// What eval / macroexpand / macroexpand-1 / require-go do when there
	// is no interpreter (aot_stubs.go, ADR 0046 §5). pkg/eval overwrites
	// all four through this same seam when an Evaluator is constructed.
	registerAOTStubs(def)

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
		fmt.Fprintln(outWriter(), strings.Join(parts, " "))
		return nil
	})

	// print / prn / pr: the rest of the print family (design/08 batch E).
	// print/println are the "human readable" pair (lang.ToString — no
	// quotes on strings/chars); pr/prn are the "machine readable" pair
	// (lang.PrintString, same formatting pr-str already uses). All four
	// write through *out* (outWriter), not stdout directly, so
	// with-out-str's (binding [*out* a-string-writer] ...) actually
	// captures them.
	def("print", func(args ...any) any {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = lang.ToString(a)
		}
		fmt.Fprint(outWriter(), strings.Join(parts, " "))
		return nil
	})
	def("pr", func(args ...any) any {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = lang.PrintString(a)
		}
		fmt.Fprint(outWriter(), strings.Join(parts, " "))
		return nil
	})
	def("prn", func(args ...any) any {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = lang.PrintString(a)
		}
		fmt.Fprintln(outWriter(), strings.Join(parts, " "))
		return nil
	})

	// -string-writer / -string-writer-str: the private substrate
	// with-out-str's macro rides on (core.clj) — an in-memory io.Writer
	// *out* can be bound to, and a way to read back what was written.
	def("-string-writer", func(args ...any) any {
		return &stringWriter{}
	})
	def("-string-writer-str", func(args ...any) any {
		w, ok := oneArg("-string-writer-str", args).(*stringWriter)
		if !ok {
			panic(fmt.Errorf("-string-writer-str: not a string-writer: %s", lang.PrintString(args[0])))
		}
		return w.buf.String()
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
		return lang.NewPersistentArrayMapAsIfByAssoc(args)
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
	internSeqBuiltins(def)

	// Sequence & collection library runtime primitives (lazy-seq*, the
	// range/repeat/iterate/cycle producers, sort/sort-by/dissoc/vec/vals,
	// reduced, <=/>=/quot/rem/max/min and the numeric/value predicates)
	// that core.clj's map/filter/reduce/take/… are built on.
	internCollBuiltins(def)
	// Native hot-path core fns — reduce/map/filter/mapv/comp (ADR 0045,
	// hotpath_builtins.go). Must intern BEFORE core.clj loads, exactly as
	// when they lived in pkg/eval: the interpreter's loadCore runs after
	// internBuiltins, and a surviving defn would shadow the native.
	internHotpathBuiltins(def)

	// Polymorphism substrate (defprotocol/deftype/defrecord/extend-*):
	// dispatch table + instance/registry builtins the core/protocols.cljg
	// macros expand onto (protocols.go).
	internProtocolBuiltins(def)

	// Multimethod substrate (defmulti/defmethod + methods/get-method/
	// remove-method/prefer-method/prefers/remove-all-methods): the
	// dispatch table (exact-= fast path + isa?/preference resolution) the
	// core.clj defmulti/defmethod macros expand onto
	// (multimethod_builtins.go).
	internMultimethodBuiltins(def)

	// Namespace introspection (ns-name/the-ns/all-ns/ns-publics/
	// ns-interns/ns-map/ns-refers/ns-aliases/ns-imports) over the live
	// lang namespace registry (ns_builtins.go).
	internNamespaceBuiltins(def)

	// slurp/spit file convenience over the Go host (io_builtins.go).
	internIOBuiltins(def)

	// atom / swap! / reset! / deref: the minimal mutable-cell set
	// (clojure.core). test.cljg holds its report counters in an atom.
	// atom: (atom x) / (atom x & {:validator vf :meta m}), the trailing
	// options Clojure's ARef-backed constructors take (ADR 0022 batch E).
	// A :validator that rejects the initial value throws immediately,
	// same as (set-validator! the-atom vf) would.
	def("atom", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: atom"))
		}
		if len(args)%2 != 1 {
			panic(fmt.Errorf("atom: options must be key/value pairs"))
		}
		a := lang.NewAtom(args[0])
		for i := 1; i < len(args); i += 2 {
			switch args[i] {
			case lang.InternKeywordString("validator"):
				if args[i+1] != nil {
					vf, ok := args[i+1].(lang.IFn)
					if !ok {
						panic(fmt.Errorf("atom: :validator is not a function: %s", lang.PrintString(args[i+1])))
					}
					a.SetValidator(vf)
				}
			case lang.InternKeywordString("meta"):
				if args[i+1] != nil {
					m, ok := args[i+1].(lang.IPersistentMap)
					if !ok {
						panic(fmt.Errorf("atom: :meta is not a map: %s", lang.PrintString(args[i+1])))
					}
					a.SetMeta(m)
				}
			}
		}
		return a
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
	registerExceptionBuiltins(def)

	// --- clojure.test host seams (core/test.cljg, ADR 0012) --------------
	registerTestBuiltins(defPrivate)

	// --- regex core fns + clojure.string host prims (core/string.cljg) ---
	internStringBuiltins(def, defPrivate)

	// --- transients (transient/persistent!/conj!/assoc!/dissoc!/disj!/
	// pop!): Batch 3 (ADR 0022, transient_builtins.go). State lives in
	// pkg/lang transient types, so eval + emitted Go share it identically.
	internTransientBuiltins(def)
	// --- numeric tower (bigint/bigdec/ratios, promotion, bit-*, parse-*,
	// rand-*, ==): design/08 §5 Batch 2 (numeric_builtins.go).
	internNumericBuiltins(def)
	// --- Batch 1 cheap-breadth predicates + coercions + seq/coll host prims
	// (ADR 0022, design/08 §5, predicate_builtins.go). The compositional
	// fns that ride on these live in core/predicates.cljg (loadPredicates).
	internPredicateBuiltins(def)
	// --- arrays (to-array/int-array/object-array/aget/aset/alength/aclone/
	// into-array/…): Batch 4 (ADR 0022, ADR 0025, array_builtins.go). A
	// cljgo array is a native Go slice (ADR 0025).
	internArrayBuiltins(def)
	// --- volatile!/vswap!/vreset!/volatile?: Batch 4 (ADR 0022,
	// volatile_builtins.go). *lang.Volatile is vendored from Glojure.
	internVolatileBuiltins(def)

	// --- version: (clojure-version)/*clojure-version* (the language level
	// we target) + (cljgo-version)/*cljgo-version* (ours, incl. the host Go
	// toolchain) — version_builtins.go, pkg/version is the source of truth.
	internVersionBuiltins(def)
	// --- NaN?/array-map/sorted-map/sorted-map-by/subseq/rsubseq: the
	// biggest cheap-breadth blockers left in the clojure-test-suite
	// (ADR 0022, design/08 §5, sorted_builtins.go). sorted-set/-by already
	// existed in predicate_builtins.go.
	internSortedBuiltins(def)
	// --- misc harness vars: delay/force/delay?, instance? substrate,
	// add-watch/remove-watch, Thread/sleep seam (ADR 0022 batch/harness-misc,
	// misc_builtins.go).
	internMiscBuiltins(def, defPrivate)
	internFormatBuiltins(def)
	// --- var reflection (resolve/find-var/ns-resolve/var?): the
	// clojure-test-suite harness surface (ADR 0022, var_builtins.go);
	// `eval` itself stays interpreter-registered (pkg/eval).
	internVarBuiltins(def)

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

	// --- channels & go (design/05 §4, ADR 0040) --------------------------
	//
	// The whole core.async surface lives in chan_builtins.go since ADR
	// 0040: canonical vars interned in clojure.core.async, the shipped
	// M4-v0 names referred into clojure.core as aliases of the SAME vars.
	// (Wired at the tail of RegisterAll, see internChanExtras's successor
	// registerAsync.)

	// future-call: (future-call thunk) -> a real-goroutine future
	// (lang.AgentSubmit) that CONVEYS the calling goroutine's dynamic-var
	// bindings (design/08 batch E, ADR 0022 — the same substrate bound-fn*
	// uses, pkg/lang/agent.go). `future` (core.clj) is the
	// `(future-call (fn [] body...))` macro wrapper, matching real
	// clojure.core.
	def("future-call", func(args ...any) any {
		fn, ok := oneArg("future-call", args).(lang.IFn)
		if !ok {
			panic(fmt.Errorf("future-call: not a function: %s", lang.PrintString(args[0])))
		}
		return lang.AgentSubmit(fn)
	})

	// future-cancel / future-cancelled? / future-done? (ADR 0038): Cancel
	// settles a pending future (deref then throws; realized? turns true)
	// and returns whether THIS call cancelled it — false on an
	// already-completed future. Oracle (JVM 1.12.5): completed => false;
	// running => true, then realized?/future-cancelled? both true and
	// deref throws CancellationException. Cancellation is cooperative:
	// the body goroutine is not interrupted.
	def("future-cancel", func(args ...any) any {
		f, ok := oneArg("future-cancel", args).(interface{ Cancel() bool })
		if !ok {
			panic(fmt.Errorf("future-cancel: not a future: %s", lang.PrintString(args[0])))
		}
		return f.Cancel()
	})
	def("future-cancelled?", func(args ...any) any {
		f, ok := oneArg("future-cancelled?", args).(interface{ IsCancelled() bool })
		if !ok {
			panic(fmt.Errorf("future-cancelled?: not a future: %s", lang.PrintString(args[0])))
		}
		return f.IsCancelled()
	})
	def("future-done?", func(args ...any) any {
		f, ok := oneArg("future-done?", args).(lang.IPending)
		if !ok {
			panic(fmt.Errorf("future-done?: not a future: %s", lang.PrintString(args[0])))
		}
		return f.IsRealized()
	})
	// future?: is x a future (fundamentals audit 2026-07 — the one missing
	// member of the future family). The `IsCancelled() bool` assertion is
	// the same type test future-cancelled? above already uses, and
	// *lang.future is the only type in the runtime carrying it — promises
	// and delays are not futures (oracle 1.12.5: (future? (future 1)) =>
	// true; (future? 1) / (future? (promise)) / (future? (delay 1)) =>
	// false).
	def("future?", func(args ...any) any {
		_, ok := oneArg("future?", args).(interface{ IsCancelled() bool })
		return ok
	})

	// STM-lite refs (ADR 0038): ref is a mutex cell with watches; dosync
	// (core.clj macro over -tx-run) serializes on one global transaction
	// lock; alter/ref-set/commute demand a running transaction — outside
	// one they throw "No transaction running" (JVM oracle 1.12.5).
	def("ref", func(args ...any) any {
		return lang.NewRef(oneArg("ref", args))
	})
	def("ref-set", func(args ...any) any {
		refArg, val := twoArgs("ref-set", args)
		r, ok := refArg.(*lang.Ref)
		if !ok {
			panic(fmt.Errorf("ref-set: not a ref: %s", lang.PrintString(refArg)))
		}
		return r.TxSet(val)
	})
	refAlter := func(op string) func(args ...any) any {
		return func(args ...any) any {
			if len(args) < 2 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
			}
			r, ok := args[0].(*lang.Ref)
			if !ok {
				panic(fmt.Errorf("%s: not a ref: %s", op, lang.PrintString(args[0])))
			}
			f, ok := args[1].(lang.IFn)
			if !ok {
				panic(fmt.Errorf("%s: not a function: %s", op, lang.PrintString(args[1])))
			}
			return r.TxAlter(f, lang.NewList(args[2:]...).Seq())
		}
	}
	def("alter", refAlter("alter"))
	// commute is alter in STM-lite: with one global transaction lock there
	// is no concurrent commit to reorder against (deviation, ADR 0038).
	def("commute", refAlter("commute"))
	defPrivate("-tx-run", func(args ...any) any {
		f, ok := oneArg("-tx-run", args).(lang.IFn)
		if !ok {
			panic(fmt.Errorf("-tx-run: not a function: %s", lang.PrintString(args[0])))
		}
		return lang.RunInTransaction(f)
	})

	// Agents (ADR 0038): a value cell + a serialized action queue drained
	// by one goroutine. send/send-off are the same operation (the
	// go/thread collapse, design/05 §4); await drains via a latch action.
	def("agent", func(args ...any) any {
		return lang.NewAgent(oneArg("agent", args))
	})
	agentSend := func(op string) func(args ...any) any {
		return func(args ...any) any {
			if len(args) < 2 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
			}
			a, ok := args[0].(*lang.Agent)
			if !ok {
				panic(fmt.Errorf("%s: not an agent: %s", op, lang.PrintString(args[0])))
			}
			f, ok := args[1].(lang.IFn)
			if !ok {
				panic(fmt.Errorf("%s: not a function: %s", op, lang.PrintString(args[1])))
			}
			return a.Send(f, lang.NewList(args[2:]...).Seq())
		}
	}
	def("send", agentSend("send"))
	def("send-off", agentSend("send-off"))
	def("await", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: await"))
		}
		for _, arg := range args {
			a, ok := arg.(*lang.Agent)
			if !ok {
				panic(fmt.Errorf("await: not an agent: %s", lang.PrintString(arg)))
			}
			a.Await()
		}
		return nil
	})

	// agent-error / restart-agent (ADR 0038 follow-on, oracle-verified
	// against clojure 1.12.5, 2026-07-17): a failing send (action OR
	// watch) leaves the agent :failed — agent-error returns the stored
	// throwable (nil while :ready); restart-agent installs a new state
	// and clears it, throwing if the agent wasn't failed. cljgo models
	// only the JVM's default :fail error-mode (no error-handler/
	// error-mode support — unreached by the suite, a documented gap).
	def("agent-error", func(args ...any) any {
		a, ok := oneArg("agent-error", args).(*lang.Agent)
		if !ok {
			panic(fmt.Errorf("agent-error: not an agent: %s", lang.PrintString(args[0])))
		}
		if err := a.AgentError(); err != nil {
			return err
		}
		return nil
	})
	def("restart-agent", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: restart-agent", len(args)))
		}
		a, ok := args[0].(*lang.Agent)
		if !ok {
			panic(fmt.Errorf("restart-agent: not an agent: %s", lang.PrintString(args[0])))
		}
		return a.Restart(args[1])
	})

	// promise / deliver: a single-value cell (design/08 batch E, ADR 0022;
	// lang.Promise) — deref blocks until delivered; delivering twice is a
	// no-op (returns nil) rather than an error.
	def("promise", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: promise", len(args)))
		}
		return lang.NewPromise()
	})
	def("deliver", func(args ...any) any {
		p, val := twoArgs("deliver", args)
		pr, ok := p.(*lang.Promise)
		if !ok {
			panic(fmt.Errorf("deliver: not a promise: %s", lang.PrintString(p)))
		}
		if pr.Deliver(val) {
			return pr
		}
		return nil
	})

	// add-tap / remove-tap / tap>: the tap>-fan-out surface (design/08
	// batch E, ADR 0022). v0 is SYNCHRONOUS (real Clojure schedules tap
	// fns on an agent send-off pool; cljgo dispatches inline, before tap>
	// returns) — functionally equivalent for callers that just want every
	// tapped value to reach every registered fn once, in tap> order, and
	// simpler than standing up an agent-executor substrate for one
	// feature. tap> always returns true, matching the real fn's contract
	// (JVM: true unless the executor is shut down, which cljgo has none of).
	def("add-tap", func(args ...any) any {
		fn, ok := oneArg("add-tap", args).(lang.IFn)
		if !ok {
			panic(fmt.Errorf("add-tap: not a function: %s", lang.PrintString(args[0])))
		}
		tapsMtx.Lock()
		taps = append(taps, fn)
		tapsMtx.Unlock()
		return nil
	})
	def("remove-tap", func(args ...any) any {
		fn, ok := oneArg("remove-tap", args).(lang.IFn)
		if !ok {
			panic(fmt.Errorf("remove-tap: not a function: %s", lang.PrintString(args[0])))
		}
		tapsMtx.Lock()
		for i, t := range taps {
			// lang.Identical, not ==: an emitted (AOT-compiled) fn value
			// may be backed by a bare Go func type, which `==` panics on
			// comparing (design/08 batch E — hit by the compiled harness).
			if lang.Identical(t, fn) {
				taps = append(taps[:i], taps[i+1:]...)
				break
			}
		}
		tapsMtx.Unlock()
		return nil
	})
	def("tap>", func(args ...any) any {
		x := oneArg("tap>", args)
		tapsMtx.RLock()
		snapshot := make([]lang.IFn, len(taps))
		copy(snapshot, taps)
		tapsMtx.RUnlock()
		for _, fn := range snapshot {
			fn.Invoke(x)
		}
		return true
	})

	// core.async (ADR 0040): canonical vars in clojure.core.async, the
	// M4-v0 names referred into clojure.core (chan_builtins.go).
	registerAsync()

	// *1 *2 *3 *e are proper dynamic vars in core (design/03 §7b); the
	// REPL driver binds them per session and set!s them after each eval.
	for _, name := range []string{"*1", "*2", "*3", "*e"} {
		lang.InternVarReplaceRoot(lang.NSCore, lang.NewSymbol(name), nil).SetDynamic()
	}

	// `binding` resolve-vs-special-form fix (ADR 0022 batch E, ratified
	// 2026-07-16): cljgo's analyzer treats `binding` as a special form
	// (pkg/analyzer/analyzer.go specialParser, for implementation
	// convenience — it needs push/pop-thread-bindings machinery a plain
	// macro can't express without exposing that machinery as public
	// builtins). Real Clojure disagrees: `binding` is an ordinary
	// clojure.core MACRO, not a special form — (special-symbol? 'binding)
	// is false on the JVM, and (resolve 'binding) returns the Var. Without
	// a Var here, resolve returned nil for a name every user program can
	// call, breaking anything that reflects on `binding` (the suite's
	// when-var-exists gate included). The analyzer dispatches on the RAW
	// symbol text (specialParser, called before any var lookup), so this
	// placeholder is resolve/var?-visible only — it changes no evaluation
	// behavior; `(binding ...)` still takes the special-form path.
	// special-symbol? (var_builtins.go) deliberately excludes "binding"
	// from jvmSpecialSymbols so the two stay consistent with the oracle.
	bindingVar := def("binding", func(args ...any) any {
		panic(fmt.Errorf("binding: cannot call as a function (special form)"))
	})
	bindingVar.SetMacro()
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

// ReferAll refers every public var interned in `from` into ns — the
// minimal whole-namespace refer of design/00 §6 (M1). Exported for
// pkg/eval's boot (user ns) and require's :refer :all.
func ReferAll(ns, from *lang.Namespace) {
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

// InitUserNS creates the `user` namespace, refers clojure.core's publics
// into it, refers clojure.repl/doc (as JVM clojure.main's repl-requires
// does — ADR 0031: (doc x) works at any user prompt), and roots *ns* at
// user. It is the tail of the boot sequence (design/00 §6), shared by
// the interpreter (eval.New) and compiled binaries (rt.Boot, ADR 0046) —
// one implementation, so the two agree by construction. Callable only
// after the core sources are loaded.
func InitUserNS() {
	user := lang.FindOrCreateNamespace(lang.NewSymbol("user"))
	ReferAll(user, lang.NSCore)
	// The M4-v0 channel names are REFERS in clojure.core (aliases of the
	// clojure.core.async vars, ADR 0040 #6); ReferAll faithfully skips
	// non-interned mappings, so the aliases hop into user explicitly.
	ReferAsyncAliases(user)
	if nsRepl := lang.FindNamespace(lang.NewSymbol("clojure.repl")); nsRepl != nil {
		symDoc := lang.NewSymbol("doc")
		if v := nsRepl.FindInternedVar(symDoc); v != nil {
			user.Refer(symDoc, v)
		}
	}
	lang.VarCurrentNS.BindRoot(user)
}
