package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/version"
)

// internVersionBuiltins registers the version surface, in both modes.
//
// The precedence principle (CLAUDE.md) drives the shape here: *clojure-version*
// and (clojure-version) are CLOJURE's, and mean exactly what they mean on the
// JVM — the language level, not the implementation. cljgo targets Clojure
// 1.12.5, so (clojure-version) answers "1.12.5"; a portability check like
// (when (neg? (compare (clojure-version) "1.11")) …) must behave identically
// here. Reporting cljgo's own version there would be a lie that silently
// breaks that idiom.
//
// Our addition sits ALONGSIDE, never shadowing: *cljgo-version* /
// (cljgo-version) answer "which implementation, on which host". *cljgo-version*
// carries the standard four Clojure keys plus :go and :clojure, so one form
// gives a bug reporter everything.
//
// Wired into internBuiltins by ONE line (e.internVersionBuiltins(def)), per
// the merge-friendly discipline.
func (e *Evaluator) internVersionBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// --- Clojure's own surface: the language level we target -------------
	//
	// Ground truth (real Clojure 1.12.5 CLI):
	//   (clojure-version) => "1.12.5"
	//   *clojure-version* => {:major 1, :minor 12, :incremental 5, :qualifier nil}
	cljInfo := version.Parse(version.ClojureVersion)
	bindValueVar("*clojure-version*", versionMap(cljInfo))
	def("clojure-version", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: clojure-version", len(args)))
		}
		return version.ClojureVersion
	})

	// --- cljgo's surface: which implementation, on which host ------------
	//
	//   (cljgo-version) => "0.1.0"
	//   *cljgo-version* => {:major 0, :minor 1, :incremental 0, :qualifier nil,
	//                       :go "1.26.3", :clojure "1.12.5"}
	ourInfo := version.Parse(version.Version)
	ourMap := lang.NewMap(
		lang.NewKeyword("major"), int64(ourInfo.Major),
		lang.NewKeyword("minor"), int64(ourInfo.Minor),
		lang.NewKeyword("incremental"), int64(ourInfo.Incremental),
		lang.NewKeyword("qualifier"), qualifierOrNil(ourInfo.Qualifier),
		lang.NewKeyword("go"), version.GoVersion(),
		lang.NewKeyword("clojure"), version.ClojureVersion,
	)
	bindValueVar("*cljgo-version*", ourMap)
	def("cljgo-version", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: cljgo-version", len(args)))
		}
		return version.Version
	})
}

// versionMap builds Clojure's four-key version map. Numbers are int64 (cljgo
// integers are longs, as on the JVM) and an absent qualifier is nil, not "" —
// (:qualifier *clojure-version*) must be nil to match Clojure.
func versionMap(in version.Info) lang.IPersistentMap {
	return lang.NewMap(
		lang.NewKeyword("major"), int64(in.Major),
		lang.NewKeyword("minor"), int64(in.Minor),
		lang.NewKeyword("incremental"), int64(in.Incremental),
		lang.NewKeyword("qualifier"), qualifierOrNil(in.Qualifier),
	)
}

func qualifierOrNil(q string) any {
	if q == "" {
		return nil
	}
	return q
}

// bindValueVar interns a non-dynamic value var in clojure.core. The version
// vars are earmuffed by Clojure convention but are not rebindable on the JVM
// either, so they are plain rooted values.
func bindValueVar(name string, val any) {
	lang.InternVarReplaceRoot(lang.NSCore, lang.NewSymbol(name), val)
}
