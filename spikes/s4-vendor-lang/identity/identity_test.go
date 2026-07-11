// S4 spike: verifies the §4.4 identity contract (design doc 00) under
// the unique.Handle-based keyword interning that replaced go4.org/intern.
//
// Contract: keywords are globally interned and identity-comparable —
// `k1 == k2` must hold for same-name keywords created in different
// (separately compiled) packages, in different goroutines, and via
// different constructor entry points. Symbols are NOT interned (doc 02
// §3.1); they get structural equality, checked here for completeness.
package identity

import (
	"sync"
	"testing"

	"cljgo-spike-s4/identity/pkga"
	"cljgo-spike-s4/identity/pkgb"
	"cljgo-spike-s4/lang"
)

func TestKeywordIdentityAcrossPackages(t *testing.T) {
	if pkga.KwFoo != pkgb.KwFoo {
		t.Error("pkga.KwFoo != pkgb.KwFoo: cross-package keyword identity broken")
	}
	if pkga.KwNsBar != pkgb.KwNsBar {
		t.Error("pkga.KwNsBar != pkgb.KwNsBar: namespaced keyword identity broken")
	}
	// Different constructor entry points must converge on the same value.
	if pkga.KwStatus != pkgb.KwStatus {
		t.Error("InternKeywordString vs NewKeyword produced non-identical keywords")
	}
	local := lang.InternKeyword("my.ns", "bar")
	if local != pkga.KwNsBar || local != pkgb.KwNsBar {
		t.Error("locally interned keyword not identical to package-level vars")
	}
	// And distinct names must stay distinct.
	if pkga.KwFoo == pkga.KwStatus {
		t.Error("distinct keywords compared equal")
	}
	if lang.InternKeyword("", "bar") == pkga.KwNsBar {
		t.Error("bare :bar compared equal to :my.ns/bar")
	}
}

func TestKeywordIdentityAcrossGoroutines(t *testing.T) {
	const goroutines = 200
	ch := make(chan lang.Keyword, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch <- lang.InternKeyword("spike", "concurrent")
		}()
	}
	wg.Wait()
	close(ch)

	want := lang.InternKeyword("spike", "concurrent")
	n := 0
	for k := range ch {
		if k != want {
			t.Errorf("goroutine-interned keyword %v != main-interned %v", k, want)
		}
		n++
	}
	if n != goroutines {
		t.Errorf("got %d keywords, want %d", n, goroutines)
	}
}

// TestKeywordAsGoMapKey proves the practical consequence of identity:
// keywords work as native Go map keys, which the emitter relies on for
// zero-cost keyword dispatch tables.
func TestKeywordAsGoMapKey(t *testing.T) {
	m := map[lang.Keyword]int{
		pkga.KwFoo:   1,
		pkga.KwNsBar: 2,
	}
	if m[pkgb.KwFoo] != 1 || m[pkgb.KwNsBar] != 2 {
		t.Error("keyword from another package failed as Go map key")
	}
}

// TestKeywordEqualsAndHash: Equals must piggyback on identity, and
// HashEq/Hash must agree for identical keywords.
func TestKeywordEqualsAndHash(t *testing.T) {
	if !pkga.KwFoo.Equals(pkgb.KwFoo) {
		t.Error("Equals false for identical keywords")
	}
	if lang.HashEq(pkga.KwFoo) != lang.HashEq(pkgb.KwFoo) {
		t.Error("HashEq differs for identical keywords")
	}
}

// TestSymbolStructuralEquality: symbols are plain values, not interned.
func TestSymbolStructuralEquality(t *testing.T) {
	if pkga.SymBaz == pkgb.SymBaz {
		t.Log("note: *Symbol pointers happened to be equal (unexpected but harmless)")
	}
	if !lang.Equals(pkga.SymBaz, pkgb.SymBaz) {
		t.Error("structural Equals false for same-name symbols")
	}
}
