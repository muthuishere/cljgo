package lang

import "testing"

// Batch 3 (ADR 0022, design/08 §5): Clojure-shaped transients over the
// vendored persistent vector / map / set. These exercise the two
// invariants that make transients faithful: (1) mutating a transient does
// NOT mutate the persistent source it was derived from, and (2) any op on
// a transient after persistent! throws (single-threaded ownership).

func TestTransientVectorSourceUnchanged(t *testing.T) {
	src := NewVector(int64(1), int64(2), int64(3))
	tv := src.AsTransient().(ITransientVector)
	tv.Conj(int64(4))
	tv.Conj(int64(5))
	if src.Count() != 3 {
		t.Fatalf("source vector mutated by transient: count=%d, want 3", src.Count())
	}
	res := tv.Persistent().(*Vector)
	if res.Count() != 5 {
		t.Fatalf("persistent! vector count=%d, want 5", res.Count())
	}
	if got := res.Nth(4); got != int64(5) {
		t.Fatalf("res[4]=%v, want 5", got)
	}
}

func TestTransientVectorInvalidationAfterPersistent(t *testing.T) {
	tv := NewVector().AsTransient().(ITransientVector)
	tv.Conj(int64(1))
	tv.Persistent()
	assertPanics(t, "conj! after persistent!", func() { tv.Conj(int64(2)) })
	assertPanics(t, "persistent! twice", func() { tv.Persistent() })
}

func TestTransientMapSourceUnchanged(t *testing.T) {
	src := NewMap(NewKeyword("a"), int64(1)).(*Map)
	tm := src.AsTransient().(ITransientAssociative)
	tm.Assoc(NewKeyword("b"), int64(2))
	tm.Assoc(NewKeyword("c"), int64(3))
	if src.Count() != 1 {
		t.Fatalf("source map mutated by transient: count=%d, want 1", src.Count())
	}
	res := tm.Persistent().(IPersistentMap)
	if res.Count() != 3 {
		t.Fatalf("persistent! map count=%d, want 3", res.Count())
	}
	if got := res.ValAt(NewKeyword("c")); got != int64(3) {
		t.Fatalf("res[:c]=%v, want 3", got)
	}
}

// TestTransientMapPromotes proves the transient survives the array-map ->
// hash-map promotion at hashmapThreshold: assoc! past the threshold and
// persistent! must still return every entry.
func TestTransientMapPromotes(t *testing.T) {
	tm := NewMap().(IEditableCollection).AsTransient().(ITransientAssociative)
	const n = 50
	for i := 0; i < n; i++ {
		tm = tm.Assoc(int64(i), int64(i*i))
	}
	res := tm.Persistent().(IPersistentMap)
	if res.Count() != n {
		t.Fatalf("promoted transient count=%d, want %d", res.Count(), n)
	}
	if got := res.ValAt(int64(49)); got != int64(49*49) {
		t.Fatalf("res[49]=%v, want %d", got, 49*49)
	}
}

func TestTransientMapInvalidationAfterPersistent(t *testing.T) {
	tm := NewMap().(IEditableCollection).AsTransient().(ITransientAssociative)
	tm.Assoc(NewKeyword("a"), int64(1))
	tm.Persistent()
	assertPanics(t, "assoc! after persistent!", func() { tm.Assoc(NewKeyword("b"), int64(2)) })
}

func TestTransientSetSourceUnchanged(t *testing.T) {
	src := NewSet(int64(1), int64(2), int64(3))
	ts := src.AsTransient().(ITransientSet)
	ts.Disjoin(int64(1))
	if src.Count() != 3 {
		t.Fatalf("source set mutated by transient: count=%d, want 3", src.Count())
	}
	res := ts.(ITransientCollection).Persistent().(IPersistentSet)
	if res.Count() != 2 {
		t.Fatalf("persistent! set count=%d, want 2", res.Count())
	}
	if res.Contains(int64(1)) {
		t.Fatalf("res still contains 1 after disj!")
	}
}

func TestTransientSetInvalidationAfterPersistent(t *testing.T) {
	ts := NewSet(int64(1)).AsTransient().(ITransientSet)
	ts.(ITransientCollection).Persistent()
	assertPanics(t, "conj! set after persistent!", func() {
		ts.(interface{ Conj(any) Conjer }).Conj(int64(2))
	})
}

func assertPanics(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s: expected panic, got none", what)
		}
	}()
	f()
}
