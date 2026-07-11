package lang

// Inherited from spike S4: pinned the two defects design doc 02 §4
// identified in the vendored Glojure pkg/lang. Defect 1 (Equiv aliased
// to Equals) is FIXED in this tree — its assertions below are the S4
// pins INVERTED, per RESULTS.md §7. Defect 2 (no HAMT transients) is
// deferred (see TODO.md); its pin test still documents the gap.

import "testing"

// Defect 1 — FIXED (M0 stage A): Equiv and Equals are now two distinct
// relations per clojure.lang.Util (equiv vs equals):
//
//	Equiv  — Clojure `=`  : category-based numeric equality
//	Equals — Java .equals : type-strict
func TestDefectEquivAliasedToEquals(t *testing.T) {
	// Same category, different types: Clojure `=` says true (Equiv),
	// but Java .equals semantics say FALSE (Equals is type-strict).
	if Equals(int32(1), int64(1)) {
		t.Error("Equals(int32(1), int64(1)) = true; want false (type-strict Java .equals)")
	}
	if Equals(float32(1.5), float64(1.5)) {
		t.Error("Equals(float32(1.5), float64(1.5)) = true; want false (type-strict Java .equals)")
	}
	if !Equiv(int32(1), int64(1)) {
		t.Error("Equiv(int32(1), int64(1)) = false; want true (same numeric category)")
	}
	// Cross-category: both relations correctly say false (NumbersEqual
	// is category-aware), so (= 1 1.0) is NOT broken despite the alias.
	if Equiv(int64(1), float64(1.0)) {
		t.Error("Equiv(1, 1.0) = true; want false (category semantics)")
	}
	// Same-category big/fixed: (= 1 1N) must be true.
	if !Equiv(int64(1), NewBigIntFromInt64(1)) {
		t.Error("Equiv(1, 1N) = false; want true")
	}
}

// Defect 2: PersistentHashMap has no transient support. Only the
// array-backed *Map has AsTransient, and that returns a fake transient
// that delegates to persistent ops (persistentarraymap.go: "TODO:
// implement transients").
func TestDefectNoHAMTTransients(t *testing.T) {
	phm := NewPersistentHashMap(NewKeyword("a"), 1)
	if _, ok := phm.(interface{ AsTransient() ITransientCollection }); ok {
		t.Error("defect assumption changed: PersistentHashMap now has AsTransient — transients may have been implemented")
	}
	if _, ok := phm.(IEditableCollection); ok {
		t.Error("defect assumption changed: PersistentHashMap now implements IEditableCollection")
	}
}
