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

// libInProgram reports whether a namespace is already part of the program
// currently being COMPILED. It is nil for `cljgo run`, the REPL, and compiled
// binaries — the three cases where "the namespace exists" really does mean
// "it is loaded".
//
// It is not nil during `cljgo build`, and that is the whole point. lang's
// namespace registry is PROCESS-GLOBAL, so when one `build.cljgo` declares two
// `exe` artifacts, the second build's fresh evaluator still sees every
// namespace the first build interned. loadLib would then take the
// already-present branch, never call the file loader, and emit a program with
// the namespace MISSING — its vars interned as hollow shells and unbound at
// runtime. The first binary worked, the second died on
// "cannot call unbound var", and a project shipping an app plus a test runner
// shipped a working app and a test suite that could not run.
//
// So during a build, global existence is not evidence. The module compiler's
// own done-set is.
var libInProgram func(name string) bool

// SetLibInProgram installs the build-scoped membership predicate described on
// libInProgram. Pass nil to clear it; emit's module compiler sets and clears it
// around each program it compiles.
func SetLibInProgram(f func(name string) bool) { libInProgram = f }

// SetLibFileLoader installs the source-file half of require (pkg/eval's
// loadLibFile, bound to an evaluator). Namespaces and vars are
// process-global, so the most recently constructed evaluator wins — the
// same rule pkg/bri's provider wiring already follows.
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
	provider := LookupLibProvider(libSym.FullName())
	if provider != nil {
		provider()
	}
	// A lib whose file is still mid-load is a cycle even though its
	// namespace already exists — the file's (in-ns …) ran before its
	// requires (JVM parity: Clojure tracks pending load paths, not
	// namespace existence, and throws "Cyclic load dependency").
	CheckCyclicLoad(libSym.FullName())
	target := lang.FindNamespace(libSym)
	// During a build, a namespace that exists globally but is not in THIS
	// program still has to be loaded — see libInProgram. The module
	// compiler's loader is idempotent per program, so a namespace required
	// twice is compiled once.
	//
	// A PROVIDER-SERVED namespace is exempt, and that exemption is
	// load-bearing: clojure.string, cljg.*, bri.* and every emitted package
	// exist because a provider ran, not because a file was read, and there is
	// no file to re-read. Providers already guard with their own loaded flag,
	// so they are correct across repeated builds; only file-backed namespaces
	// need the per-program check.
	if target != nil && provider == nil && libInProgram != nil && !libInProgram(libSym.FullName()) {
		target = nil
	}
	if target == nil {
		if libFileLoader == nil {
			// A compiled binary: no reader, no analyzer, no evaluator. The
			// namespace was not compiled in and no provider serves it, so
			// hard-error naming it (ADR 0053 dec 3) rather than silently
			// no-op'ing or failing as an obscure nil map lookup. A static
			// binary legitimately cannot load un-compiled source at runtime.
			panic(fmt.Errorf("namespace %s was not compiled into this binary: no registered provider, and an AOT-compiled binary has no interpreter to load it from source (compile the namespace in, or run it with the cljgo interpreter)",
				libSym.FullName()))
		}
		libFileLoader(libSym)
		target = lang.FindNamespace(libSym)
		if target == nil {
			// The file WAS found and evaluated; it just never created this
			// namespace. Say that, because "not found after loading its
			// source" — the old wording — names the wrong condition and
			// sends the reader hunting for a missing file that is right
			// there. In practice it means the file has no `(ns …)` form at
			// all, or declares a name that disagrees with its path.
			//
			// Found by the toolnexus Clojure port while confirming the #182
			// fix: their repro turned out to be two problems wearing one
			// error message, and only the first was #182.
			panic(lang.NewCodedError("G5026", fmt.Sprintf(
				"%s: the file was loaded but did not define this namespace "+
					"(expected a top-level (ns %s …) matching its path; found none) — "+
					"add the ns form, or rename the file to match the ns it declares",
				libSym.FullName(), libSym.FullName())))
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

// registerUse interns `use` into clojure.core (fundamentals batch A4):
// require + refer in one step, exactly Clojure's compat idiom. Accepted
// spec shapes mirror require's, with refer's filters as libspec options:
//
//	(use 'clojure.set)                                ; require + refer all
//	(use '[clojure.string :only [upper-case]])        ; refer only these
//	(use '[clojure.string :exclude [join]])           ; refer all but these
//	(use '[clojure.string :as s :only [upper-case]])  ; alias too
//	(use '(clojure set string))                       ; prefix list
//
// :rename is accepted-and-ignored (same M1 stance as refer's). Oracle
// (JVM 1.12.5): (use 'clojure.set) => nil, then (union #{1} #{2}) =>
// #{1 2}; (use '[clojure.string :only [upper-case] :as ss]) refers only
// upper-case and adds the ss alias.
func registerUse(def func(string, func(...any) any) *lang.Var) {
	def("use", func(args ...any) any {
		for _, a := range args {
			useSpec(a, nil)
		}
		return nil
	})
}

// useSpec processes one use spec: a bare namespace symbol (refer all
// publics), a libspec vector `[lib & opts]` (:as/:refer handled by
// loadLib; :only/:exclude drive the refer), or a prefix list.
func useSpec(spec any, prefix *lang.Symbol) {
	if sym, ok := spec.(*lang.Symbol); ok {
		full := combinePrefix(prefix, sym)
		loadLib(full, nil)
		ReferAll(currentNS(), lang.FindNamespace(full))
		return
	}
	seq := lang.Seq(spec)
	if seq == nil {
		panic(fmt.Errorf("use expects a namespace symbol or libspec, got: %s", lang.PrintString(spec)))
	}
	head, ok := seq.First().(*lang.Symbol)
	if !ok {
		panic(fmt.Errorf("use expects a libspec whose head is a symbol, got: %s", lang.PrintString(spec)))
	}
	rest := seq.Next()
	// Prefix list vs libspec-with-options: same discrimination as require.
	if rest != nil {
		if _, isKw := rest.First().(lang.Keyword); !isKw {
			full := combinePrefix(prefix, head)
			for x := rest; x != nil; x = x.Next() {
				useSpec(x.First(), full)
			}
			return
		}
	}
	full := combinePrefix(prefix, head)
	// loadLib handles :as (and a require-style :refer, harmless here);
	// the refer filters are ours.
	loadLib(full, rest)
	target := lang.FindNamespace(full)
	only := map[string]struct{}{}
	exclude := map[string]struct{}{}
	haveOnly := false
	for s := rest; s != nil; s = s.Next() {
		kw, ok := s.First().(lang.Keyword)
		if !ok {
			panic(fmt.Errorf("use libspec option must be a keyword, got: %s", lang.PrintString(s.First())))
		}
		s = s.Next()
		if s == nil {
			panic(fmt.Errorf("use libspec option %s is missing a value", kw.String()))
		}
		switch kw.Name() {
		case "only":
			haveOnly = true
			collectSymNames(s.First(), only)
		case "exclude":
			collectSymNames(s.First(), exclude)
		default:
			// :as already handled by loadLib; :rename etc. are no-ops.
		}
	}
	referSelected(currentNS(), target, only, haveOnly, exclude)
}
