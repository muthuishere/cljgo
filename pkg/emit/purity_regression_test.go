package emit

import (
	"path/filepath"
	"testing"
)

// TestBareRequireGoIsTainted — a namespace that DECLARES (require-go …) but
// never dereferences a member produces no OpHost* node, yet must still be
// classified Go-interop (ADR 0050 dec 2/3: require-go itself is the
// disqualifying surface). Regression for the clojars purity-gate bypass where a
// bare require-go slipped through as "pure" and its invalid form was copied into
// the JVM source tree.
func TestBareRequireGoIsTainted(t *testing.T) {
	m := classifyEntry(t, filepath.FromSlash("testdata/purity/barereq/barereq/core.clj"))

	if NamespacePure(m, "barereq.core") {
		t.Fatal("bare (require-go …) with no member access must be tainted, but ns was pure")
	}
	ok, off := WholeLibPure(m)
	if ok {
		t.Fatal("whole-lib must be impure when a namespace declares require-go")
	}
	if off == nil || off.Class != TaintGoInterop {
		t.Fatalf("offender should be a go-interop taint, got %+v", off)
	}
	if off.Line == 0 {
		t.Errorf("taint should carry the require-go form's line, got 0 (%+v)", off)
	}
}
