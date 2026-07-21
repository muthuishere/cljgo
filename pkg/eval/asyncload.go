// asyncload.go — the lazy lib provider for clojure.core.async's macro
// half (ADR 0040 #5, openspec core-async-first-class T1).
//
// The fn half of the namespace is Go-native (pkg/corelib registerAsync,
// interned at RegisterAll time); the macros (go-loop / alt! / alt!!)
// live in the embedded core/async.cljg, which deliberately is NOT a
// boot source — nothing evaluates until the first
// (require 'clojure.core.async), so the boot budget (ADR 0024) is
// untouched. The provider follows pkg/keel's shape: eval.New registers
// it with the freshest evaluator (namespaces and vars are
// process-global, so which live evaluator performs the load does not
// matter semantically), and the load is guarded by an interned-marker
// check rather than a bool so a test harness that removes/recreates
// namespaces stays coherent.
//
// Compiled binaries never consult this provider: macros are expanded at
// compile time, and rt.Boot's RegisterAll already interns the namespace
// the replayed (require …) form resolves against.
package eval

import (
	"github.com/muthuishere/cljgo/core"
	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// asyncMarker is a var async.cljg interns — its presence means the
// source already evaluated in this process.
var asyncMarker = lang.NewSymbol("go-loop")

// registerAsyncProvider wires the clojure.core.async provider to e.
// Last registration wins (eval.New per session/test file), matching the
// provider registry's contract.
func registerAsyncProvider(e *Evaluator) {
	corelib.RegisterLibProvider("clojure.core.async", func() {
		if lang.NSAsync.FindInternedVar(asyncMarker) != nil {
			return
		}
		e.loadBootSource(core.BootSource{
			NS:     "clojure.core.async",
			File:   "async.cljg",
			Source: &core.AsyncSource,
		})
	})
}
