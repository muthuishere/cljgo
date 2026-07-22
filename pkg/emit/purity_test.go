package emit

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// classifyEntry compiles a fixture entry and runs the Go-interop classifier.
func classifyEntry(t *testing.T, entry string) map[string]*Taint {
	t.Helper()
	prog, err := CompileProgram(entry)
	if err != nil {
		t.Fatalf("CompileProgram(%s): %v", entry, err)
	}
	return ClassifyGoInterop(prog)
}

// TestPureFixtureZeroTaints — a wholly pure library produces zero taints (no
// false positives) and every namespace passes the per-ns gate.
func TestPureFixtureZeroTaints(t *testing.T) {
	m := classifyEntry(t, filepath.FromSlash("testdata/purity/pure/pure/core.clj"))

	if len(m) != 0 {
		t.Fatalf("pure fixture: want 0 taints, got %d: %v", len(m), m)
	}
	for _, ns := range []string{"pure.core", "pure.util"} {
		if !NamespacePure(m, ns) {
			t.Errorf("pure fixture: %s should be pure", ns)
		}
	}
	if ok, off := WholeLibPure(m); !ok {
		t.Errorf("pure fixture: whole-lib should be pure, offender=%+v", off)
	}
}

// TestGoBuriedCaughtAtLeaf — a require-go buried two levels deep
// (core→mid→leaf) is caught and cited at the leaf's file:line, while the pure
// ancestors pass the per-namespace gate.
func TestGoBuriedCaughtAtLeaf(t *testing.T) {
	m := classifyEntry(t, filepath.FromSlash("testdata/purity/gob/gob/core.clj"))

	// Ancestors pure per-ns, independently usable.
	for _, ns := range []string{"gob.core", "gob.mid"} {
		if !NamespacePure(m, ns) {
			t.Errorf("%s should be pure (per-ns gate), got %+v", ns, m[ns])
		}
	}
	// The leaf is tainted.
	leaf := m["gob.leaf"]
	if leaf == nil {
		t.Fatalf("gob.leaf should be tainted, got clean")
	}
	if leaf.Class != TaintGoInterop {
		t.Errorf("gob.leaf class = %q, want %q", leaf.Class, TaintGoInterop)
	}
	if !strings.HasSuffix(filepath.ToSlash(leaf.Path), "gob/leaf.clj") {
		t.Errorf("gob.leaf path = %q, want …gob/leaf.clj", leaf.Path)
	}
	// (require-go '[strconv :as sc]) is line 2; (sc/Itoa …) is line 3. The
	// first host op is the sc/Itoa call on line 3.
	if leaf.Line != 3 {
		t.Errorf("gob.leaf line = %d, want 3 (the sc/Itoa call)", leaf.Line)
	}
	if !strings.Contains(leaf.Detail, "strconv") {
		t.Errorf("gob.leaf detail = %q, want mention of strconv", leaf.Detail)
	}

	// Whole-lib fails, citing the single offender.
	if ok, off := WholeLibPure(m); ok {
		t.Errorf("whole-lib should FAIL for go-buried")
	} else if off == nil || off.NS != "gob.leaf" {
		t.Errorf("whole-lib offender = %+v, want gob.leaf", off)
	}
}

// TestMixedFixture — one pure namespace and one Go-interop namespace under a
// pure entry; only the go side is tainted.
func TestMixedFixture(t *testing.T) {
	m := classifyEntry(t, filepath.FromSlash("testdata/purity/mix/mix/core.clj"))

	if !NamespacePure(m, "mix.pureside") {
		t.Errorf("mix.pureside should be pure, got %+v", m["mix.pureside"])
	}
	if !NamespacePure(m, "mix.core") {
		t.Errorf("mix.core (entry) should be pure, got %+v", m["mix.core"])
	}
	goside := m["mix.goside"]
	if goside == nil {
		t.Fatalf("mix.goside should be tainted")
	}
	if goside.Class != TaintGoInterop {
		t.Errorf("mix.goside class = %q, want go-interop", goside.Class)
	}
	if goside.Line != 3 {
		t.Errorf("mix.goside line = %d, want 3 (the s/ToUpper call)", goside.Line)
	}
	if ok, _ := WholeLibPure(m); ok {
		t.Errorf("mixed whole-lib should FAIL")
	}
}

// TestWholeLibEqualsAndOfPerNS — the invariant ADR 0050 §3 asserts:
// WholeLibPure == AND(NamespacePure over every reachable namespace). Checked
// across all fixtures via the captured Program's full namespace set.
func TestWholeLibEqualsAndOfPerNS(t *testing.T) {
	entries := []string{
		"testdata/purity/pure/pure/core.clj",
		"testdata/purity/gob/gob/core.clj",
		"testdata/purity/mix/mix/core.clj",
	}
	for _, e := range entries {
		prog, err := CompileProgram(filepath.FromSlash(e))
		if err != nil {
			t.Fatalf("CompileProgram(%s): %v", e, err)
		}
		m := ClassifyGoInterop(prog)

		// Every reachable namespace's real name.
		names := []string{nsRealName(prog.Entry)}
		for _, d := range prog.Deps {
			names = append(names, nsRealName(d))
		}
		sort.Strings(names)

		and := true
		for _, n := range names {
			and = and && NamespacePure(m, n)
		}
		whole, _ := WholeLibPure(m)
		if whole != and {
			t.Errorf("%s: WholeLibPure(%v) != AND(per-ns)(%v) over %v", e, whole, and, names)
		}
	}
}

// TestEntryNameRecovered — the entry's "" Name is recovered to its declared
// namespace so it is addressable in the map/gates.
func TestEntryNameRecovered(t *testing.T) {
	prog, err := CompileProgram(filepath.FromSlash("testdata/purity/pure/pure/core.clj"))
	if err != nil {
		t.Fatalf("CompileProgram: %v", err)
	}
	if prog.Entry.Name != "" {
		t.Skipf("entry Name unexpectedly set to %q; recovery path not exercised", prog.Entry.Name)
	}
	if got := nsRealName(prog.Entry); got != "pure.core" {
		t.Errorf("recovered entry name = %q, want pure.core", got)
	}
}
