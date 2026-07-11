package lang

// M0 stage A acceptance tests for the Equiv/Equals split (design doc 02
// §1.3; ground truth clojure.lang.Util.equiv vs Util.equals). These are
// the conformance cases the promotion task requires.

import "testing"

// (= 1 1.0) is false: numeric equality is category-based, and integer
// and floating are different categories.
func TestEquivNumericCategories(t *testing.T) {
	if Equiv(int64(1), float64(1.0)) {
		t.Error("Equiv(1, 1.0) = true; want false ((= 1 1.0) is false)")
	}
	if Equiv(float64(1.0), int64(1)) {
		t.Error("Equiv(1.0, 1) = true; want false (symmetric)")
	}
	// Same category, different representations: (= 1 1N) is true.
	if !Equiv(int64(1), NewBigIntFromInt64(1)) {
		t.Error("Equiv(1, 1N) = false; want true (same integer category)")
	}
	if !Equiv(int(1), int64(1)) {
		t.Error("Equiv(int(1), int64(1)) = false; want true (same integer category)")
	}
	// == (Numbers.equal without the category gate) is a different op;
	// Equiv must not answer it.
	if !NumbersEqual(int64(1), NewBigIntFromInt64(1)) {
		t.Error("NumbersEqual(1, 1N) = false; want true")
	}
	// Number vs non-number is never equiv.
	if Equiv(int64(1), "1") {
		t.Error(`Equiv(1, "1") = true; want false`)
	}
}

// (= [1 2] '(1 2)) is true: vectors, lists, and lazy seqs are all the
// sequential category; equality is element-wise equiv.
func TestEquivSequentialCategory(t *testing.T) {
	vec := NewVector(int64(1), int64(2))
	list := NewList(int64(1), int64(2))
	lazy := NewLazySeq(func() any { return NewCons(int64(1), NewCons(int64(2), nil)) })

	if !Equiv(vec, list) {
		t.Error("Equiv([1 2], '(1 2)) = false; want true (sequential category)")
	}
	if !Equiv(list, vec) {
		t.Error("Equiv('(1 2), [1 2]) = false; want true (symmetric)")
	}
	if !Equiv(vec, lazy) {
		t.Error("Equiv([1 2], (lazy-seq '(1 2))) = false; want true")
	}
	if Equiv(vec, NewList(int64(1), int64(2), int64(3))) {
		t.Error("Equiv([1 2], '(1 2 3)) = true; want false")
	}
	// Category semantics reach the elements too: [1] != [1.0].
	if Equiv(NewVector(int64(1)), NewVector(float64(1.0))) {
		t.Error("Equiv([1], [1.0]) = true; want false (element category)")
	}
}

// (= {:a 1} {:a 1}) is true, across map representations.
func TestEquivMaps(t *testing.T) {
	kwA := NewKeyword("a")
	am1 := NewMap(kwA, int64(1))              // array map
	am2 := NewMap(kwA, int64(1))              // array map
	hm := NewPersistentHashMap(kwA, int64(1)) // HAMT

	if !Equiv(am1, am2) {
		t.Error("Equiv({:a 1}, {:a 1}) = false; want true")
	}
	if !Equiv(am1, hm) {
		t.Error("Equiv(arraymap {:a 1}, hashmap {:a 1}) = false; want true")
	}
	if !Equiv(hm, am1) {
		t.Error("Equiv(hashmap {:a 1}, arraymap {:a 1}) = false; want true")
	}
	if Equiv(am1, NewMap(kwA, int64(2))) {
		t.Error("Equiv({:a 1}, {:a 2}) = true; want false")
	}
	// Map values compare by equiv: {:a 1} vs {:a 1.0} differ.
	if Equiv(am1, NewMap(kwA, float64(1.0))) {
		t.Error("Equiv({:a 1}, {:a 1.0}) = true; want false")
	}
}

// Hash consistency: Equiv(a,b) ⇒ HashEq(a) == HashEq(b).
func TestHashEqConsistentWithEquiv(t *testing.T) {
	// Same integer category, different representations.
	if HashEq(int64(1)) != HashEq(NewBigIntFromInt64(1)) {
		t.Error("HashEq(1) != HashEq(1N) though Equiv holds")
	}
	if HashEq(int(7)) != HashEq(int64(7)) {
		t.Error("HashEq(int(7)) != HashEq(int64(7)) though Equiv holds")
	}
	// Sequential category: vector and list of equiv elements.
	vec := NewVector(int64(1), int64(2))
	list := NewList(int64(1), int64(2))
	if !Equiv(vec, list) {
		t.Fatal("precondition: Equiv([1 2], '(1 2))")
	}
	if HashEq(vec) != HashEq(list) {
		t.Errorf("HashEq([1 2]) = %d, HashEq('(1 2)) = %d; want equal (equiv values must hash the same)",
			HashEq(vec), HashEq(list))
	}
	// Maps: array map and HAMT.
	kwA := NewKeyword("a")
	am := NewMap(kwA, int64(1))
	hm := NewPersistentHashMap(kwA, int64(1))
	if HashEq(am) != HashEq(hm) {
		t.Errorf("HashEq(arraymap) = %d, HashEq(hashmap) = %d; want equal", HashEq(am), HashEq(hm))
	}
}

// Map lookup uses Equiv for keys: an entry keyed by an int64 is found
// regardless of which same-category representation created or queries
// it, in both map implementations.
func TestMapLookupEquivKeys(t *testing.T) {
	type mkMap func(keyvals ...any) IPersistentMap
	impls := map[string]mkMap{
		"arraymap": NewMap,
		"hashmap":  NewPersistentHashMap,
	}
	for name, mk := range impls {
		// Created with int64 key, looked up with other integer reps.
		m := mk(int64(42), "v")
		for _, k := range []any{int64(42), int(42), int32(42), NewBigIntFromInt64(42)} {
			if got := m.ValAt(k); got != "v" {
				t.Errorf("%s created with int64(42): ValAt(%T %v) = %v; want \"v\"", name, k, k, got)
			}
			if !m.ContainsKey(k) {
				t.Errorf("%s created with int64(42): ContainsKey(%T %v) = false; want true", name, k, k)
			}
		}
		// Created with int key, looked up with int64.
		m2 := mk(int(42), "w")
		if got := m2.ValAt(int64(42)); got != "w" {
			t.Errorf("%s created with int(42): ValAt(int64(42)) = %v; want \"w\"", name, got)
		}
		// Cross-category must NOT find it: (get {42 :v} 42.0) is nil.
		if m.ContainsKey(float64(42.0)) {
			t.Errorf("%s: ContainsKey(42.0) = true; want false (cross-category)", name)
		}
		// Assoc with an equiv key replaces rather than duplicates.
		m3 := m.Assoc(int(42), "x").(IPersistentMap)
		if m3.Count() != 1 {
			t.Errorf("%s: assoc with equiv key gave Count() = %d; want 1", name, m3.Count())
		}
	}
}

// Equals keeps Java .equals semantics: type-strict for numbers.
func TestEqualsTypeStrict(t *testing.T) {
	if Equals(int32(1), int64(1)) {
		t.Error("Equals(int32(1), int64(1)) = true; want false")
	}
	if Equals(float32(1.5), float64(1.5)) {
		t.Error("Equals(float32(1.5), float64(1.5)) = true; want false")
	}
	if Equals(int64(1), float64(1.0)) {
		t.Error("Equals(1, 1.0) = true; want false")
	}
	if !Equals(int64(1), int64(1)) {
		t.Error("Equals(int64(1), int64(1)) = false; want true")
	}
	if !Equals("s", "s") {
		t.Error(`Equals("s", "s") = false; want true`)
	}
	if !Equals(NewBigIntFromInt64(3), NewBigIntFromInt64(3)) {
		t.Error("Equals(3N, 3N) = false; want true")
	}
	if Equals(NewBigIntFromInt64(3), int64(3)) {
		t.Error("Equals(3N, 3) = true; want false (type-strict)")
	}
	// Collections keep Java .equals shape: element-wise, type-strict.
	if !Equals(NewVector(int64(1)), NewList(int64(1))) {
		t.Error("Equals([1], '(1)) = false; want true (java.util.List contract)")
	}
	if Equals(NewVector(int64(1)), NewList(float64(1.0))) {
		t.Error("Equals([1], '(1.0)) = true; want false (element type-strict)")
	}
	if !Equals(NewMap(NewKeyword("a"), int64(1)), NewPersistentHashMap(NewKeyword("a"), int64(1))) {
		t.Error("Equals({:a 1}, {:a 1}) = false; want true across map impls")
	}
	if Equals(NewMap(NewKeyword("a"), int64(1)), NewMap(NewKeyword("a"), float64(1.0))) {
		t.Error("Equals({:a 1}, {:a 1.0}) = true; want false (value type-strict)")
	}
}
