package lang

// S4 spike: pins down the two defects design doc 02 §4 claims exist in
// the vendored Glojure pkg/lang. These tests PASS while the defects are
// present — they document today's (wrong) behavior. When we fix the
// defects, invert the assertions.

import "testing"

// Defect 1: Equiv is aliased to Equals (equal.go: `func Equiv(a, b any)
// bool { return Equals(a, b) }`). Clojure needs two relations:
//
//	Equiv  — Clojure `=`  : category-based numeric equality
//	Equals — Java .equals : type-strict
//
// Because of the alias there is only ONE relation. NumbersEqual happens
// to be category-aware, so Equiv's `=` semantics are mostly right; the
// observable damage is Equals being far too loose for interop.
func TestDefectEquivAliasedToEquals(t *testing.T) {
	// Same category, different types: Clojure `=` says true — fine for
	// Equiv, but Java .equals semantics say FALSE. The alias makes
	// Equals answer true.
	if !Equals(int32(1), int64(1)) {
		t.Error("defect assumption changed: Equals(int32(1), int64(1)) now false (type-strict?) — Equiv/Equals may have been split")
	}
	if !Equals(float32(1.5), float64(1.5)) {
		t.Error("defect assumption changed: Equals(float32, float64) now false — Equiv/Equals may have been split")
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
