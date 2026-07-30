package repl

import (
	"strings"
	"testing"
)

// TestArityErrorNamesRaiserNotOuterCall pins the fix for the arity
// mis-attribution in docs/known-issues-2026-07-28.md §8: an arity error that
// merely UNWINDS through an outer call must keep the name (and the accepted
// arities) of the fn that actually mismatched.
//
// (println (map h [1] [2] [3])) realizes the lazy seq inside println, so the
// mismatch on h is raised under println's frame. The JVM oracle (clojure
// 1.12.5) reports `Wrong number of args (3) passed to: user/h` — never
// println. Before the fix the rendered REPL/`cljgo run` line said
// `passed to: clojure.core/println`, while `cljgo build` and the compiled
// binary said `user/h`: a REPL-vs-binary divergence in error text.
func TestArityErrorNamesRaiserNotOuterCall(t *testing.T) {
	d, _, errOut := newSession("(defn h [x] x)\n(println (map h [1] [2] [3]))\n")
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := errOut.String()
	if !strings.Contains(got, "wrong number of args (3) passed to: user/h (expects 1: [x])") {
		t.Fatalf("arity error must name the fn that mismatched, got: %q", got)
	}
	if strings.Contains(got, "clojure.core/println") {
		t.Fatalf("arity error blamed the outer call: %q", got)
	}
}

// TestArityErrorStillNamesOwnCallee guards the other side of the same rule:
// when the call site's OWN callee is the one that mismatched, the enrichment
// still upgrades the bare name to the resolved Var's qualified name and adds
// the accepted arities (the ADR 0048 / spike s28 behaviour).
func TestArityErrorStillNamesOwnCallee(t *testing.T) {
	d, _, errOut := newSession("(defn g ([x] x) ([x y] [x y]))\n(g 1 2 3)\n")
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := errOut.String(); !strings.Contains(got,
		"wrong number of args (3) passed to: user/g (expects 1: [x] or 2: [x y])") {
		t.Fatalf("direct arity error lost its enrichment: %q", got)
	}
}
