package lang_test

// Regression tests for the stale-hash-cache defect found in PR #30 via
// clojure.set/join (see PROVENANCE.md "Stale hash cache on array-map
// assoc"): Map.clone() copied the cached hash/hasheq fields, so the result
// of Assoc (and therefore conj/merge on an array-map) carried the ORIGINAL
// map's cached hash. Any hash-addressed structure the result was stored in
// (a set, a hash-map key slot) filed it under the wrong hash, making it
// invisible to `=`/get/contains? lookups with a structurally equal probe.
// The trigger requires the source map's hash to already be cached — which
// is exactly what happens to any map fetched out of an existing set or
// used as a map key.

import (
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

func TestArrayMapAssocResetsHashCache(t *testing.T) {
	a := lang.NewKeyword("a")
	b := lang.NewKeyword("b")

	m := lang.NewMap(a, 1)
	// Force the caches to populate, as membership in a set/map would.
	_ = lang.HashEq(m)
	_ = lang.Hash(m)

	grown := m.(lang.Associative).Assoc(b, 2)     // append branch
	replaced := m.(lang.Associative).Assoc(a, 99) // replace-value branch
	fresh := lang.NewMap(a, 1, b, 2)
	freshReplaced := lang.NewMap(a, 99)

	if lang.HashEq(grown) != lang.HashEq(fresh) {
		t.Errorf("assoc (append) result inherited a stale hasheq: got %d, fresh equal map hashes to %d",
			lang.HashEq(grown), lang.HashEq(fresh))
	}
	if lang.Hash(grown) != lang.Hash(fresh) {
		t.Errorf("assoc (append) result inherited a stale hash: got %d, fresh equal map hashes to %d",
			lang.Hash(grown), lang.Hash(fresh))
	}
	if lang.HashEq(replaced) != lang.HashEq(freshReplaced) {
		t.Errorf("assoc (replace) result inherited a stale hasheq: got %d, fresh equal map hashes to %d",
			lang.HashEq(replaced), lang.HashEq(freshReplaced))
	}

	// The original must be untouched by either assoc.
	if !lang.Equiv(m, lang.NewMap(a, 1)) {
		t.Errorf("original map changed by assoc: %v", m)
	}
}

// The end-to-end shape of the clojure.set/join corruption: conj a pair onto
// a map fetched out of a set (hash already cached), put the result into a
// new set, and probe that set with a structurally equal fresh map.
func TestConjOnSetMemberKeepsSetEquality(t *testing.T) {
	a := lang.NewKeyword("a")
	b := lang.NewKeyword("b")
	c := lang.NewKeyword("c")

	member := lang.NewMap(a, 1, b, 2)
	src := lang.NewSet(member)

	fetched := src.Get(lang.NewMap(a, 1, b, 2)) // fetch from the set; hash is cached
	if fetched == nil {
		t.Fatal("set member not found")
	}
	merged := fetched.(lang.Associative).Assoc(c, 3)
	out := lang.NewSet(merged)

	probe := lang.NewMap(a, 1, b, 2, c, 3)
	if !out.Contains(probe) {
		t.Errorf("set built from a conj'd member does not contain a structurally equal map")
	}
	if !lang.Equiv(out, lang.NewSet(probe)) {
		t.Errorf("set built from a conj'd member is not Equiv to a fresh equal set: %v vs %v",
			out, lang.NewSet(probe))
	}
	// Source set unchanged.
	if !lang.Equiv(src, lang.NewSet(lang.NewMap(a, 1, b, 2))) {
		t.Errorf("source set corrupted: %v", src)
	}
}
