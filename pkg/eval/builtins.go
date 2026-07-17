package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// internBuiltins pre-interns the native IFns into clojure.core. Since
// ADR 0043 (AOT-core piece 2) the interpreter-independent set lives in
// pkg/corelib (corelib.RegisterAll); this shell layers the FIVE
// evaluator-coupled builtins on top of the same corelib.Def seam:
// macroexpand-1 / macroexpand (the macro engine, design/03 §4),
// require (file loading through the Evaluator's LibLoader, ADR 0042),
// require-go (per-evaluator host aliases, ADR 0010) — and `eval`
// (var_builtins.go, Evaluator.EvalForm).
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

	// require: M1 semantics with libspec support — the embedded namespaces
	// (e.g. clojure.string, clojure.test) are loaded at boot; a missing
	// namespace loads via a registered lib provider or its source file
	// (libload.go, ADR 0042), then the libspec options apply. Accepted
	// spec shapes match Clojure:
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
			requireSpec(e, a, nil)
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

// currentNS mirrors Evaluator.CurrentNS for free functions (one *ns* world).
func currentNS() *lang.Namespace {
	if ns, ok := lang.VarCurrentNS.Deref().(*lang.Namespace); ok {
		return ns
	}
	return lang.NSCore
}

// requireSpec processes one require spec: a bare namespace symbol, a libspec
// vector `[lib & opts]`, or a prefix list `(prefix sub ...)`. `prefix` (may
// be nil) is the dotted prefix accumulated from an enclosing prefix list.
func requireSpec(e *Evaluator, spec any, prefix *lang.Symbol) {
	if sym, ok := spec.(*lang.Symbol); ok {
		loadLib(e, combinePrefix(prefix, sym), nil)
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
				requireSpec(e, x.First(), full)
			}
			return
		}
	}
	loadLib(e, combinePrefix(prefix, head), rest)
}

// combinePrefix joins a prefix-list prefix with a leaf symbol into a dotted
// namespace symbol (`clojure` + `string` => `clojure.string`).
func combinePrefix(prefix, sym *lang.Symbol) *lang.Symbol {
	if prefix == nil {
		return sym
	}
	return lang.NewSymbol(prefix.Name() + "." + sym.Name())
}

// loadLib ensures the namespace exists — already present (embedded or
// previously loaded), else via a registered lib provider (an emitted
// package's Load(), ADR 0042 §2), else by loading its source file
// resolved relative to the requiring file (ADR 0042 §4) — then applies
// the libspec options: :as adds an alias to the current ns; :refer
// interns the named public vars (or all publics for :refer :all) into
// the current ns.
func loadLib(e *Evaluator, libSym *lang.Symbol, opts lang.ISeq) {
	// A registered provider is authoritative and consulted FIRST: in an
	// emitted binary the namespace may already exist as a hollow shell
	// (another package's hoisted lang.InternVarName created it at Go
	// init), so mere existence does not mean loaded. Providers guard
	// with a loaded bool, so re-requires are no-ops.
	if provider := lookupLibProvider(libSym.FullName()); provider != nil {
		provider()
	}
	// A lib whose file is still mid-load is a cycle even though its
	// namespace already exists — the file's (in-ns …) ran before its
	// requires (JVM parity: Clojure tracks pending load paths, not
	// namespace existence, and throws "Cyclic load dependency").
	checkCyclicLoad(libSym.FullName())
	target := lang.FindNamespace(libSym)
	if target == nil {
		loadLibFile(e, libSym)
		target = lang.FindNamespace(libSym)
		if target == nil {
			panic(fmt.Errorf("namespace %s not found after loading its source", libSym.FullName()))
		}
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
			corelib.ReferAll(ns, from)
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
