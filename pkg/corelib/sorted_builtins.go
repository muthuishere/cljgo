package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internSortedBuiltins registers the sorted-collection surface that the
// jank clojure-test-suite's biggest blockers need (ADR 0022, design/08 §5):
// NaN?, array-map, sorted-map/sorted-map-by, and subseq/rsubseq. sorted-set/
// sorted-set-by already existed (predicate_builtins.go); pkg/lang already
// carries the SortedMap/SortedSet wrapper types (persisttenttreemap.go,
// set.go) from the original vendor promotion — this file only wires
// clojure.core names onto them. Wired into internBuiltins by ONE line
// (e.internSortedBuiltins(def)), per the merge-friendly discipline.
func internSortedBuiltins(def func(string, func(...any) any) *lang.Var) {
	// NaN?: is x NaN. Oracle (clojure 1.12): (NaN? ##NaN) => true;
	// (NaN? 1) => false; (NaN? 1.0) => false; non-numbers (incl. nil,
	// keywords, chars, strings) throw a ClassCastException on the JVM
	// (NaN? is ^double-typed) — we mirror that with a panic rather than
	// silently returning false.
	def("NaN?", func(args ...any) any {
		x := oneArg("NaN?", args)
		if !lang.IsNumber(x) {
			panic(fmt.Errorf("NaN?: not a number: %s", lang.PrintString(x)))
		}
		return lang.IsNaN(lang.AsFloat64(x))
	})

	// array-map: (array-map & keyvals) — always a PersistentArrayMap,
	// regardless of pair count (unlike hash-map/{}, which promote past
	// pkg/lang's hashmapThreshold at construction time). Duplicate keys
	// keep the first key's position with the last value, matching real
	// Clojure: (array-map :a 1 :b 2 :a 3) => {:a 3, :b 2}. A later assoc
	// on the result still promotes normally (lang.Map.Assoc), matching
	// the JVM's (class (assoc (array-map ...) k v)) => PersistentHashMap
	// once past the threshold.
	def("array-map", func(args ...any) any {
		if len(args)%2 != 0 {
			panic(lang.NewCodedError("G5007", fmt.Sprintf("array-map: no value supplied for key: %s", lang.PrintString(args[len(args)-1]))))
		}
		return lang.NewArrayMapForce(args...)
	})

	// sorted-map: (sorted-map & keyvals) — a map sorted by (compare).
	def("sorted-map", func(args ...any) any {
		if len(args)%2 != 0 {
			panic(lang.NewCodedError("G5007", fmt.Sprintf("sorted-map: no value supplied for key: %s", lang.PrintString(args[len(args)-1]))))
		}
		return lang.CreatePersistentTreeMap(lang.NewList(args...))
	})

	// sorted-map-by: (sorted-map-by comparator & keyvals).
	def("sorted-map-by", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: sorted-map-by"))
		}
		comp, ok := args[0].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("sorted-map-by: comparator must be a function, got: %s", lang.PrintString(args[0])))
		}
		rest := args[1:]
		if len(rest)%2 != 0 {
			panic(lang.NewCodedError("G5007", fmt.Sprintf("sorted-map-by: no value supplied for key: %s", lang.PrintString(rest[len(rest)-1]))))
		}
		return lang.CreatePersistentTreeMapWithComparator(comp, lang.NewList(rest...))
	})

	// subseq / rsubseq: (subseq sc test key) or (subseq sc stest skey etest ekey).
	// `test` must be one of clojure.core's </<=/>/>= — Clojure dispatches on
	// which one it is rather than invoking it on the keys (so it works on
	// non-numeric sorted keys, e.g. keywords), and so do we.
	def("subseq", func(args ...any) any {
		return doSubseq("subseq", args, false)
	})
	def("rsubseq", func(args ...any) any {
		return doSubseq("rsubseq", args, true)
	})
}

// sortedColl is the minimal surface subseq/rsubseq need: sorted ordering
// (Comparator/EntryKey per lang.Sorted) plus ascending/descending iteration.
type sortedColl interface {
	lang.Sorted
	lang.Seqable
	lang.Reversible
}

// testName maps a passed-in test fn to one of the four canonical boundary
// tests by identity of the underlying native builtin (same trick real
// Clojure plays by checking against #'clojure.core/< & co.). Since our `<`
// `<=` `>` `>=` are nativeFn values interned once at boot, comparing the
// wrapped name is equivalent to identity here and doesn't require the caller
// to have re-resolved the exact var.
func testName(fn any) (string, bool) {
	nf, ok := fn.(*nativeFn)
	if !ok {
		return "", false
	}
	switch nf.nm {
	case "<", "<=", ">", ">=":
		return nf.nm, true
	default:
		return "", false
	}
}

// boundaryMatches reports whether cmp (compare(entryKey, boundaryKey))
// satisfies the named test.
func boundaryMatches(test string, cmp int) bool {
	switch test {
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	}
	panic(fmt.Errorf("subseq: unsupported test: %s", test))
}

// sortedCompare orders two keys the same way sc itself does: sc's own
// comparator if it has one, else lang.LenientCompare (matching
// SortedMap.sortedKeys/SortedSet.sortedElements' default).
func sortedCompare(sc lang.Sorted, a, b any) int {
	comp := sc.Comparator()
	if comp == nil {
		return lang.LenientCompare(a, b)
	}
	r := comp.Invoke(a, b)
	if bres, ok := r.(bool); ok {
		if bres {
			return -1
		}
		if bv := comp.Invoke(b, a); bv == true {
			return 1
		}
		return 0
	}
	return int(lang.AsInt64(r))
}

func doSubseq(op string, args []any, reverse bool) any {
	if len(args) != 3 && len(args) != 5 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
	sc, ok := args[0].(sortedColl)
	if !ok {
		panic(fmt.Errorf("%s: not a sorted collection: %s", op, lang.PrintString(args[0])))
	}
	testA, okA := testName(args[1])
	if !okA {
		panic(fmt.Errorf("%s: test must be one of < <= > >=, got: %s", op, lang.PrintString(args[1])))
	}
	keyA := args[2]
	haveB := len(args) == 5
	var testB string
	var keyB any
	if haveB {
		tb, okB := testName(args[3])
		if !okB {
			panic(fmt.Errorf("%s: test must be one of < <= > >=, got: %s", op, lang.PrintString(args[3])))
		}
		testB = tb
		keyB = args[4]
	}

	var entries []any
	for seq := sc.Seq(); seq != nil; seq = seq.Next() {
		e := seq.First()
		ek := sc.EntryKey(e)
		if !boundaryMatches(testA, sortedCompare(sc, ek, keyA)) {
			continue
		}
		if haveB && !boundaryMatches(testB, sortedCompare(sc, ek, keyB)) {
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return nil
	}
	if reverse {
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
	}
	return lang.NewSliceSeq(entries)
}
