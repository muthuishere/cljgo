package deps

// ADR 0112 — the lock refreshes itself, minimally, unless frozen.
//
// Each test here maps to one decision, and each asserts the behaviour a user
// sees rather than the mechanism underneath.

import (
	"strings"
	"testing"
)

// twoRoots publishes two independent roots, each with its own private
// transitive, so "did the OTHER root's pin move" is answerable.
func twoRoots(tb testing.TB, r *mvnRepoDouble, aVer string) []Dep {
	tb.Helper()
	for _, v := range []string{"1.0.0", "2.0.0"} {
		r.publish(Coord{Group: "st", Artifact: "atrans", Version: v}, "",
			map[string]string{"st/atrans.clj": "(ns st.atrans)\n(def v 1)\n"})
		r.publish(Coord{Group: "st", Artifact: "a", Version: v},
			depsXML(depXML("st", "atrans", v)),
			map[string]string{"st/a.clj": "(ns st.a)\n(def v 1)\n"})
	}
	r.publish(Coord{Group: "st", Artifact: "btrans", Version: "1.0.0"}, "",
		map[string]string{"st/btrans.clj": "(ns st.btrans)\n(def v 1)\n"})
	r.publish(Coord{Group: "st", Artifact: "b", Version: "1.0.0"},
		depsXML(depXML("st", "btrans", "1.0.0")),
		map[string]string{"st/b.clj": "(ns st.b)\n(def v 1)\n"})
	return []Dep{
		{Name: "st/a", MvnVersion: aVer, MvnDeclared: true},
		{Name: "st/b", MvnVersion: "1.0.0", MvnDeclared: true},
	}
}

// Decision 1: a moved declaration refreshes the lock instead of dead-ending.
func TestMovedDeclarationRefreshesTheLock(t *testing.T) {
	r := newMvnRepo(t)
	newCache(t)
	o := r.opts(t)
	o.Update = true
	first, err := Resolve(twoRoots(t, r, "1.0.0"), o)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	first.Lock.BuildHash = DeclaredSetHash(twoRoots(t, r, "1.0.0"))

	// Now bump st/a, exactly as editing build.cljgo would. Update is on
	// because build.go sees the set hash move.
	bumped := twoRoots(t, r, "2.0.0")
	o2 := r.opts(t)
	o2.Lock = first.Lock
	o2.Update = true
	res, err := Resolve(bumped, o2)
	if err != nil {
		t.Fatalf("a bumped version must re-pin, not error: %v", err)
	}
	if lk := res.Lock.find("st/a"); lk == nil || lk.MvnVersion != "2.0.0" {
		t.Fatalf("st/a not re-pinned: %+v", lk)
	}
}

// Decision 2: the hash is over the DECLARED SET, not the manifest's bytes.
func TestDeclaredSetHashIgnoresCosmeticChange(t *testing.T) {
	base := []Dep{
		{Name: "x/y", MvnVersion: "1.0.0", MvnDeclared: true},
		{Name: "p/q", MvnVersion: "2.0.0", MvnDeclared: true},
	}
	reordered := []Dep{base[1], base[0]}
	if DeclaredSetHash(base) != DeclaredSetHash(reordered) {
		t.Error("declaration ORDER changed the hash; a reorder declares nothing")
	}
	moved := []Dep{base[0], {Name: "p/q", MvnVersion: "2.0.1", MvnDeclared: true}}
	if DeclaredSetHash(base) == DeclaredSetHash(moved) {
		t.Error("a version bump did NOT change the hash")
	}
}

// Decision 3, the one that is a correctness requirement rather than an
// optimisation: bumping one root must not drift the other root's transitive.
func TestReResolveIsMinimal(t *testing.T) {
	r := newMvnRepo(t)
	newCache(t)
	o := r.opts(t)
	o.Update = true
	first, err := Resolve(twoRoots(t, r, "1.0.0"), o)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := first.Lock.find("st/btrans")
	if before == nil {
		t.Fatal("st/btrans missing from the seed lock")
	}
	beforeHash := before.DeclHash

	o2 := r.opts(t)
	o2.Lock = first.Lock
	o2.Update = true
	res, err := Resolve(twoRoots(t, r, "2.0.0"), o2)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// st/a moved, so it and its transitive re-resolve.
	if lk := res.Lock.find("st/atrans"); lk == nil || lk.MvnVersion != "2.0.0" {
		t.Errorf("st/atrans should have followed st/a to 2.0.0: %+v", lk)
	}
	// st/b did NOT move, so nothing reachable only from it may change.
	after := res.Lock.find("st/btrans")
	if after == nil {
		t.Fatal("st/btrans vanished from the lock")
	}
	if after.MvnVersion != "1.0.0" || after.DeclHash != beforeHash {
		t.Errorf("unrelated pin drifted: was %s/%s, now %s/%s",
			before.MvnVersion, beforeHash, after.MvnVersion, after.DeclHash)
	}
}

// Decision 4: frozen is an error, writes nothing, and names what diverged.
func TestFrozenRefusesAStaleLockAndWritesNothing(t *testing.T) {
	r := newMvnRepo(t)
	newCache(t)
	o := r.opts(t)
	o.Update = true
	seed := twoRoots(t, r, "1.0.0")
	first, err := Resolve(seed, o)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	first.Lock.BuildHash = DeclaredSetHash(seed)

	frozen := r.opts(t)
	frozen.Lock = first.Lock
	frozen.Update = true
	frozen.Frozen = true
	_, err = Resolve(twoRoots(t, r, "2.0.0"), frozen)
	if err == nil {
		t.Fatal("frozen must refuse a lock that does not match the manifest")
	}
	if !strings.Contains(err.Error(), "G5021") {
		t.Errorf("frozen refusal is not carrying its code: %v", err)
	}
	if !strings.Contains(err.Error(), "st/a") {
		t.Errorf("frozen refusal does not name what diverged: %v", err)
	}
}

// Frozen with a matching lock is a normal, successful, online build.
func TestFrozenWithMatchingLockSucceeds(t *testing.T) {
	r := newMvnRepo(t)
	newCache(t)
	o := r.opts(t)
	o.Update = true
	seed := twoRoots(t, r, "1.0.0")
	first, err := Resolve(seed, o)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	first.Lock.BuildHash = DeclaredSetHash(seed)

	frozen := r.opts(t)
	frozen.Lock = first.Lock
	frozen.Frozen = true
	if _, err := Resolve(seed, frozen); err != nil {
		t.Fatalf("frozen + matching lock must build: %v", err)
	}
}

// Decision 5: no diagnostic may send a user to a command that does not exist.
func TestNoDiagnosticNamesAResolveSubcommand(t *testing.T) {
	r := newMvnRepo(t)
	newCache(t)
	o := r.opts(t)
	o.Update = true
	seed := twoRoots(t, r, "1.0.0")
	first, _ := Resolve(seed, o)
	first.Lock.BuildHash = DeclaredSetHash(seed)

	// The pre-0112 divergence path: a lock present, Update off, manifest
	// disagreeing. It must still refuse — and must not name `resolve`.
	warm := r.opts(t)
	warm.Lock = first.Lock
	warm.Update = false
	_, err := Resolve(twoRoots(t, r, "2.0.0"), warm)
	if err == nil {
		t.Fatal("a warm resolve against a disagreeing manifest must refuse")
	}
	if strings.Contains(err.Error(), "resolve with -update") {
		t.Errorf("diagnostic still names a command that does not exist: %v", err)
	}
}
