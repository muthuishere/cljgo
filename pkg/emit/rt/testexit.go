package rt

import (
	"github.com/muthuishere/cljgo/pkg/lang"
)

// TestFailures returns clojure.test's PROCESS-level tally of failing and
// erroring assertions — the value of the `clojure.test/-process-failures`
// atom, or 0 when clojure.test was never loaded (the common case: a program
// that has nothing to do with tests).
//
// It exists because a compiled test binary used to exit 0 on a red suite
// (ADR 0105 task 2.1): the user's `-main` calls run-tests, throws the summary
// away, and CI goes green on red. `cljgo test` already mapped the summary to
// exit 1; the emitted func main() now consults this, so the interpreted and
// compiled legs agree on the exit code as well as on the output — the same
// REPL-vs-binary parity bar the rest of the toolchain is held to.
//
// The counter is deliberately process-level rather than per-run: a binary
// that runs several suites fails if ANY of them failed, and a reporter that
// replaces the built-in :fail/:error methods cannot hide the failure (the
// tally lives in do-report, the choke point).
func TestFailures() int {
	ns := lang.FindNamespace(lang.NewSymbol("clojure.test"))
	if ns == nil {
		return 0
	}
	v := ns.FindInternedVar(lang.NewSymbol("-process-failures"))
	if v == nil {
		return 0
	}
	a, ok := v.Get().(lang.IDeref)
	if !ok {
		return 0
	}
	n, _ := a.Deref().(int64)
	return int(n)
}

// TestsFailed reports whether any clojure.test assertion failed or errored
// in this process. See TestFailures.
func TestsFailed() bool { return TestFailures() > 0 }
