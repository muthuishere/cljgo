package eval

// Macroexpansion (eval v2, design/03 §4): macroexpand1 is the analyzer's
// injected hook; the bootstrap defmacro is a hand-built macro fn; the
// embedded core/core.clj defines the user-facing macros on top.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/muthuishere/cljgo/core"
	"github.com/muthuishere/cljgo/pkg/analyzer"
	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

var (
	symDo           = lang.NewSymbol("do")
	symDef          = lang.NewSymbol("def")
	symFnStar       = lang.NewSymbol("fn*")
	symVar          = lang.NewSymbol("var")
	symAmpForm      = lang.NewSymbol("&form")
	symAmpEnv       = lang.NewSymbol("&env")
	symSetMacroBang = lang.NewSymbol("clojure.core/-set-macro!")
)

// macroexpand1 expands form by one macro step, exactly Compiler.java's
// macroexpand1 (design/03 §4): non-seqs, specials, locals-shadowed
// operators, and operators that do not resolve to a :macro var pass
// through unchanged (the identical value — the analyzer loops on
// identity). A macro var's fn is invoked NOW, at analysis time, with
// &form (the whole call form) and &env (a map keyed by the local
// symbols in scope, nil when none) prepended to the call's arguments.
func (e *Evaluator) macroexpand1(form any, locals map[string]*ast.BindingNode) (any, error) {
	seq, ok := form.(lang.ISeq)
	if !ok || lang.Seq(seq) == nil {
		return form, nil
	}
	op, ok := seq.First().(*lang.Symbol)
	if !ok {
		return form, nil
	}
	if !op.HasNamespace() {
		if analyzer.IsSpecial(op.Name()) {
			return form, nil // specials are not macros
		}
		if _, shadowed := locals[op.Name()]; shadowed {
			return form, nil // a local shadows the macro name
		}
	}
	v, err := e.resolveVar(op)
	if err != nil || !v.IsMacro() {
		// Unresolvable operators are NOT an expansion error: the form is
		// simply not a macro call (parseInvoke reports resolution errors
		// with position).
		return form, nil
	}

	// (macrofn &form &env args...) — the two hidden leading args, per
	// Compiler.java l.7583.
	rest := lang.ToSlice(seq.Next())
	margs := make([]any, 0, 2+len(rest))
	margs = append(margs, form, localsEnvMap(locals))
	margs = append(margs, rest...)

	// The macro fn runs under the IFn panic convention; expansion happens
	// during analysis (outside evalTop's recover), so recover here.
	var expanded any
	err = func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				rerr, ok := r.(error)
				if !ok {
					rerr = fmt.Errorf("%v", r)
				}
				err = rerr
			}
		}()
		expanded = lang.Apply(v.Deref(), margs)
		return nil
	}()
	if err != nil {
		return nil, fmt.Errorf("macroexpanding %s: %w", op.Name(), err)
	}
	return expanded, nil
}

// localsEnvMap builds the &env value from the analysis-time locals: a
// map keyed by the local symbols (values nil — we carry no per-binding
// info yet), or nil when no locals are in scope, as on JVM Clojure.
func localsEnvMap(locals map[string]*ast.BindingNode) any {
	if len(locals) == 0 {
		return nil
	}
	kvs := make([]any, 0, 2*len(locals))
	for name := range locals {
		kvs = append(kvs, lang.NewSymbol(name), nil)
	}
	return lang.NewMap(kvs...)
}

// macroexpand is user-facing (macroexpand form): repeat macroexpand1 on
// the returned form until the operator is no longer a macro. Subforms
// are not expanded, as on JVM Clojure. Called with no locals in scope
// (&env = nil), like the clojure.core fns it backs.
func (e *Evaluator) macroexpand(form any) (any, error) {
	for i := 0; i < maxUserMacroExpansions; i++ {
		expanded, err := e.macroexpand1(form, nil)
		if err != nil {
			return nil, err
		}
		if expanded == form {
			return form, nil
		}
		form = expanded
	}
	return nil, fmt.Errorf("too many macroexpansions (limit %d)", maxUserMacroExpansions)
}

// maxUserMacroExpansions bounds the user-facing macroexpand loop (the
// analyzer's own loop has its own limit).
const maxUserMacroExpansions = 1000

// installDefmacro interns the bootstrap defmacro (design/03 §4): a
// hand-built macro fn — a Var flagged :macro whose value rewrites
//
//	(defmacro name doc? ([params] body...)+)   ; or single [params] body...
//
// into
//
//	(do (def name doc? (fn* name ([&form &env params...] body...)+))
//	    (clojure.core/-set-macro! (var name))
//	    (var name))
//
// JVM-style: the macro fn takes &form/&env as explicit leading params
// on every arity (clojure/core.clj's defmacro does the same rewrite),
// and the expansion sets the :macro flag on the var at eval time, so a
// defmacro typed at the REPL is a macro for the very next form
// (design/03 §7a). -set-macro! is the M1 stand-in for JVM Clojure's
// (. (var name) (setMacro)) — host interop lands in v3.
func (e *Evaluator) installDefmacro() {
	v := lang.NSCore.Intern(lang.NewSymbol("defmacro"))
	v.BindRoot(&nativeFn{nm: "defmacro", fn: defmacroExpand})
	v.SetMacro()
}

// defmacroExpand is defmacro's expander. args = [&form &env name doc?
// fdecl...]; the two hidden args are ignored.
func defmacroExpand(args ...any) any {
	if len(args) < 4 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: defmacro", len(args)-2))
	}
	name, ok := args[2].(*lang.Symbol)
	if !ok {
		panic(fmt.Errorf("first argument to defmacro must be a symbol, got: %s", lang.PrintString(args[2])))
	}
	fdecl := args[3:]

	var doc any
	if s, isStr := fdecl[0].(string); isStr && len(fdecl) > 1 {
		doc = s
		fdecl = fdecl[1:]
	}

	// Normalize the single-arity shorthand [params] body... to one
	// ([params] body...) method; otherwise every element is a method.
	var methods []any
	if _, isVec := fdecl[0].(lang.IPersistentVector); isVec {
		methods = []any{lang.NewList(fdecl...)}
	} else {
		methods = fdecl
	}

	fnParts := []any{symFnStar, name}
	for _, m := range methods {
		mseq, isSeq := m.(lang.ISeq)
		if !isSeq {
			panic(fmt.Errorf("invalid defmacro method form: %s", lang.PrintString(m)))
		}
		parts := lang.ToSlice(mseq)
		pvec, isVec := parts[0].(lang.IPersistentVector)
		if !isVec {
			panic(fmt.Errorf("defmacro method requires a parameter vector, got: %s", lang.PrintString(parts[0])))
		}
		// Prepend the hidden params. A trailing "& rest" pair keeps its
		// invariant (& stays second-to-last).
		params := append([]any{symAmpForm, symAmpEnv}, lang.ToSlice(pvec)...)
		method := append([]any{lang.NewVector(params...)}, parts[1:]...)
		fnParts = append(fnParts, lang.NewList(method...))
	}

	defParts := []any{symDef, name}
	if doc != nil {
		defParts = append(defParts, doc)
	}
	defParts = append(defParts, lang.NewList(fnParts...))

	theVar := lang.NewList(symVar, name)
	return lang.NewList(symDo,
		lang.NewList(defParts...),
		lang.NewList(symSetMacroBang, theVar),
		theVar)
}

// loadCore reads and evaluates the embedded core/core.clj into the
// clojure.core namespace, form by form, exactly as a file load
// (design/03 §7a): *ns* and *file* are bound for the duration. Boot is
// programmer-owned input — errors panic. Measured cost of the whole
// eval.New() boot (builtins + defmacro + core.clj): ~5ms on darwin/arm64
// (TestBootUnderBudget logs it; budget 100ms, design/00 §6 M1).
func (e *Evaluator) loadCore() {
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, lang.NSCore,
		lang.VarFile, "core.clj",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.Source),
		reader.WithFilename("core.clj"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading core.clj: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating core.clj: %w", err))
		}
	}
}

// loadNumeric reads and evaluates the embedded core/numeric.cljg into
// clojure.core — the Clojure-level numeric-tower fns (random-sample, …)
// that ride on the Batch 2 host primitives (numeric_builtins.go, design/08
// §5). It runs after loadCore so defn/filter/rand and the seq library are
// available; numeric.cljg's own (in-ns 'clojure.core) makes its body
// resolve unqualified core names, and its publics are referred into user
// like the rest of core.
func (e *Evaluator) loadNumeric() {
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, lang.NSCore,
		lang.VarFile, "numeric.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.NumericSource),
		reader.WithFilename("numeric.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading numeric.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating numeric.cljg: %w", err))
		}
	}
}

// loadHierarchies reads and evaluates the embedded core/hierarchies.cljg
// into clojure.core — make-hierarchy/derive/underive/ancestors/descendants/
// parents/isa? (ADR 0022 Track E, design/08 §5 batch E), riding on
// alter-var-root (var_builtins.go) to mutate the global-hierarchy Var. It
// runs after loadNumeric (needs nothing numeric-specific, but keeps the
// same "after core.clj" slot as the other batch-E .cljg loads); its own
// (in-ns 'clojure.core) makes its body resolve unqualified core names, and
// its publics are referred into user like the rest of core.
func (e *Evaluator) loadHierarchies() {
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, lang.NSCore,
		lang.VarFile, "hierarchies.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.HierarchiesSource),
		reader.WithFilename("hierarchies.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading hierarchies.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating hierarchies.cljg: %w", err))
		}
	}
}

// loadTransducers reads and evaluates the embedded core/transducers.cljg
// into clojure.core — transduce/eduction/sequence/completing/partition-by/
// dedupe/halt-when/replace + the `into` xform arity (design/08 §5 Batch 4,
// ADR 0022). It runs after loadCore/loadNumeric/loadPredicates (map/filter/
// take/…'s 1-arity xform forms, `cat`/`comp`/`unreduced`/`ensure-reduced`
// all live in core.clj itself hoisted where needed; `butlast` — used by
// eduction — lives in predicates.cljg) so transducers.cljg's own references
// resolve; its own (in-ns 'clojure.core) makes its body resolve unqualified
// core names, and its publics are referred into user like the rest of core.
func (e *Evaluator) loadTransducers() {
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, lang.NSCore,
		lang.VarFile, "transducers.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.TransducersSource),
		reader.WithFilename("transducers.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading transducers.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating transducers.cljg: %w", err))
		}
	}
}

// loadClojureString reads and evaluates the embedded core/string.cljg into
// the clojure.string namespace. It runs after loadCore so clojure.core (and
// the regex core fns + private -str-* host prims) are fully up. *ns* is
// bound to a freshly-created clojure.string for the load; string.cljg's own
// (in-ns ...) / (refer 'clojure.core) forms make its body resolve unqualified
// core names. This "embedded-ns registration" makes clojure.string loadable —
// (require 'clojure.string) then finds it (builtins.go require).
func (e *Evaluator) loadClojureString() {
	ns := lang.FindOrCreateNamespace(lang.NewSymbol("clojure.string"))
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, "string.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.StringSource),
		reader.WithFilename("string.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading string.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating string.cljg: %w", err))
		}
	}
}

// loadPredicates reads and evaluates the embedded core/predicates.cljg into
// clojure.core — the Batch 1 "cheap breadth" fns that compose over the Go
// predicate/coercion builtins (ADR 0022, design/08 §5). It runs immediately
// after loadCore (predicates are foundational): defn/destructuring and the
// seq library are up, and the Go builtins (int?/pos?/name/namespace/…) it
// leans on are interned before core.clj even loads. Its publics are referred
// into user like the rest of core.
func (e *Evaluator) loadPredicates() {
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, lang.NSCore,
		lang.VarFile, "predicates.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.PredicatesSource),
		reader.WithFilename("predicates.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading predicates.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating predicates.cljg: %w", err))
		}
	}
}

// loadProtocols reads and evaluates the embedded core/protocols.cljg into
// clojure.core — the defprotocol / deftype / defrecord / extend-type /
// extend-protocol macros (design/00 §6 M5). It runs after loadCore (so
// defn/defmacro/destructuring and the seq library are available) and its
// publics are referred into user like the rest of core. The runtime
// dispatch/instance builtins the macros expand onto are already interned
// by internProtocolBuiltins.
func (e *Evaluator) loadProtocols() {
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, lang.NSCore,
		lang.VarFile, "protocols.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.ProtocolsSource),
		reader.WithFilename("protocols.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading protocols.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating protocols.cljg: %w", err))
		}
	}
}

// loadBuild reads and evaluates the embedded core/build.cljg into the
// cljgo.build namespace — the interpreted Zig-style build runtime (ADR 0021,
// design/08 §1). It runs after loadCore so clojure.core is fully up. *ns* is
// bound to a freshly-created cljgo.build for the load; build.cljg's own
// (in-ns ...) / (refer 'clojure.core) forms make its body resolve unqualified
// core names. This "embedded-ns registration" makes cljgo.build requireable —
// `cljgo build` refers it into the build namespace before evaluating a
// project's build.cljgo. It does NOT refer cljgo.build into user.
func (e *Evaluator) loadBuild() {
	ns := lang.FindOrCreateNamespace(lang.NewSymbol("cljgo.build"))
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, "build.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.BuildSource),
		reader.WithFilename("build.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading build.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating build.cljg: %w", err))
		}
	}
}

// loadClojureSet reads and evaluates the embedded core/set.cljg into the
// clojure.set namespace (ADR 0022 batch/harness-misc) — the same
// embedded-ns registration pattern as loadClojureString, so
// (require '[clojure.set :refer [subset?]]) finds it already interned.
func (e *Evaluator) loadClojureSet() {
	ns := lang.FindOrCreateNamespace(lang.NewSymbol("clojure.set"))
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, "set.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.SetSource),
		reader.WithFilename("set.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading set.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating set.cljg: %w", err))
		}
	}
}

// loadClojureEdn reads and evaluates the embedded core/edn.cljg into the
// clojure.edn namespace (ADR 0022 batch/harness-misc) — same embedded-ns
// registration pattern as loadClojureSet.
func (e *Evaluator) loadClojureEdn() {
	ns := lang.FindOrCreateNamespace(lang.NewSymbol("clojure.edn"))
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, "edn.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.EdnSource),
		reader.WithFilename("edn.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading edn.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating edn.cljg: %w", err))
		}
	}
}

// loadClojureTest reads and evaluates the embedded core/test.cljg into the
// clojure.test namespace (the interpreted clojure.test slice, ADR 0012).
// It runs after loadCore so clojure.core is fully up. *ns* is bound to a
// freshly-created clojure.test for the load; test.cljg's own (in-ns ...) /
// (refer 'clojure.core) forms make its body resolve unqualified core
// names. This is the "embedded-ns registration" that makes clojure.test
// loadable — (require 'clojure.test) then finds it (builtins.go require).
// It does NOT refer clojure.test into user; users refer it explicitly.
func (e *Evaluator) loadClojureTest() {
	ns := lang.FindOrCreateNamespace(lang.NewSymbol("clojure.test"))
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, "test.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.TestSource),
		reader.WithFilename("test.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading test.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating test.cljg: %w", err))
		}
	}
}

// loadClojureTestPortability reads and evaluates the embedded
// core/clojure_test_portability.cljg into the clojure.core-test.portability
// namespace — the cljgo shim for the jank clojure-test-suite (ADR 0022,
// Batch 0). It runs after loadClojureTest so clojure.core and clojure.test are
// both up; the file's own (in-ns …portability) / (refer 'clojure.core) forms
// make its body resolve unqualified core names. Pre-loading it makes the suite's
// (require '[clojure.core-test.portability …]) succeed (require locates only
// embedded namespaces).
func (e *Evaluator) loadClojureTestPortability() {
	ns := lang.FindOrCreateNamespace(lang.NewSymbol("clojure.core-test.portability"))
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, "clojure_test_portability.cljg",
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(core.PortabilitySource),
		reader.WithFilename("clojure_test_portability.cljg"),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading clojure_test_portability.cljg: %w", err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating clojure_test_portability.cljg: %w", err))
		}
	}
}
