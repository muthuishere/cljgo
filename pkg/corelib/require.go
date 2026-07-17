// require.go — the `require` builtin and the lib-provider registry
// (ADR 0042 §2, relocated here by ADR 0046).
//
// `require` is NOT interpreter-coupled, and a compiled binary genuinely
// needs it: emitted code replays every (require …) form at its source
// position, and that replay is what triggers a dependency package's
// registered Load(). So the whole libspec surface (:as / :refer /
// prefix lists) plus the provider registry live here, where a binary can
// reach them without linking pkg/eval.
//
// The one half that IS interpreter-coupled — making a namespace exist by
// READING ITS SOURCE FILE — is a hook: pkg/eval installs it
// (SetLibFileLoader) at evaluator construction, so an interpreted
// session loads files exactly as before. With no interpreter linked the
// hook is nil and a require that resolves to neither an existing
// namespace nor a registered provider fails with a clear error naming
// the AOT limitation, instead of silently doing nothing.
package corelib

import (
	"fmt"
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// libProviders is the runtime registry of namespace loaders. Emitted
// packages register from init() (a plain map write — safe before
// rt.Boot()); require consults it before touching the filesystem.
var (
	libProvidersMu sync.Mutex
	libProviders   = map[string]func(){}
)

// RegisterLibProvider registers a loader for a namespace, keyed by its
// full name ("my-app.util"). Called by generated code via
// rt.RegisterLib. Last registration wins (re-registration is harmless:
// providers are guarded, load-once).
func RegisterLibProvider(name string, load func()) {
	libProvidersMu.Lock()
	defer libProvidersMu.Unlock()
	libProviders[name] = load
}

// LookupLibProvider returns the registered loader for a namespace, or nil.
func LookupLibProvider(name string) func() {
	libProvidersMu.Lock()
	defer libProvidersMu.Unlock()
	return libProviders[name]
}

// libFileLoader makes a namespace exist by loading its source file. nil
// in a compiled binary (no reader, no evaluator); pkg/eval installs the
// interpreter's loader through SetLibFileLoader.
var libFileLoader func(libSym *lang.Symbol)

// SetLibFileLoader installs the source-file half of require (pkg/eval's
// loadLibFile, bound to an evaluator). Namespaces and vars are
// process-global, so the most recently constructed evaluator wins — the
// same rule pkg/keel's provider wiring already follows.
func SetLibFileLoader(f func(libSym *lang.Symbol)) { libFileLoader = f }

// libsLoading is the in-progress load stack for cycle detection (load
// state is process-global, like the namespace registry itself).
var libsLoading []string

// CheckCyclicLoad panics when name's source file is already mid-load
// (JVM parity: "Cyclic load dependency"). Namespace existence is no
// proof of loadedness — a file's (in-ns …) runs before its requires.
func CheckCyclicLoad(name string) {
	for _, n := range libsLoading {
		if n == name {
			panic(fmt.Errorf("cyclic load dependency: %s -> %s",
				strings.Join(libsLoading, " -> "), name))
		}
	}
}

// PushLibLoading marks a lib's file as mid-load (cycle-checked).
func PushLibLoading(name string) {
	CheckCyclicLoad(name)
	libsLoading = append(libsLoading, name)
}

// PopLibLoading pops the in-progress load stack.
func PopLibLoading() { libsLoading = libsLoading[:len(libsLoading)-1] }

// registerRequire interns `require` into clojure.core. Accepted spec
// shapes match Clojure:
//
//	(require 'clojure.string)                       ; bare symbol
//	(require '[clojure.string :as str])             ; :as alias
//	(require '[clojure.string :refer [join]])       ; :refer some
//	(require '[clojure.string :refer :all])         ; :refer all publics
//	(require '[clojure.string :as s :refer [join]]) ; both
//	(require '(clojure string set))                 ; prefix list (bonus)
//
// :as creates a namespace alias in the CURRENT ns (so the analyzer's
// alias resolution — CurrentNS().LookupAlias — sees it); :refer interns
// the named public vars into the current ns. Unknown options are no-ops.
func registerRequire(def func(string, func(...any) any) *lang.Var) {
	def("require", func(args ...any) any {
		for _, a := range args {
			RequireSpec(a, nil)
		}
		return nil
	})
}

// RequireSpec processes one require spec: a bare namespace symbol, a libspec
// vector `[lib & opts]`, or a prefix list `(prefix sub ...)`. `prefix` (may
// be nil) is the dotted prefix accumulated from an enclosing prefix list.
func RequireSpec(spec any, prefix *lang.Symbol) {
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
				RequireSpec(x.First(), full)
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

// loadLib ensures the namespace exists — already present (embedded or
// previously loaded), else via a registered lib provider (an emitted
// package's Load(), ADR 0042 §2), else by loading its source file
// through the interpreter hook (ADR 0042 §4) — then applies the libspec
// options: :as adds an alias to the current ns; :refer interns the named
// public vars (or all publics for :refer :all) into the current ns.
func loadLib(libSym *lang.Symbol, opts lang.ISeq) {
	// A registered provider is authoritative and consulted FIRST: in an
	// emitted binary the namespace may already exist as a hollow shell
	// (another package's hoisted lang.InternVarName created it at Go
	// init), so mere existence does not mean loaded. Providers guard
	// with a loaded bool, so re-requires are no-ops.
	if provider := LookupLibProvider(libSym.FullName()); provider != nil {
		provider()
	}
	// A lib whose file is still mid-load is a cycle even though its
	// namespace already exists — the file's (in-ns …) ran before its
	// requires (JVM parity: Clojure tracks pending load paths, not
	// namespace existence, and throws "Cyclic load dependency").
	CheckCyclicLoad(libSym.FullName())
	target := lang.FindNamespace(libSym)
	if target == nil {
		if libFileLoader == nil {
			// A compiled binary: no reader, no analyzer, no evaluator.
			// Say so, rather than failing as an obscure nil map lookup.
			panic(fmt.Errorf("could not locate namespace %s: no registered provider, and this is an AOT-compiled binary — it has no interpreter to load %s from source (compile the namespace in, or run it with the cljgo interpreter)",
				libSym.FullName(), libSym.FullName()))
		}
		libFileLoader(libSym)
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
			referRequireSpec(currentNS(), target, val)
		default:
			// :reload, :verbose, :as-alias, etc. — no-ops in M1.
		}
	}
}

// referRequireSpec handles a :refer value: `:all` refers every public var; a
// vector of symbols interns exactly those (throwing if a name is not interned
// in the target namespace, as Clojure does).
func referRequireSpec(ns, from *lang.Namespace, spec any) {
	if kw, ok := spec.(lang.Keyword); ok {
		if kw.Name() == "all" {
			ReferAll(ns, from)
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
