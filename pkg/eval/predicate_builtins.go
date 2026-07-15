package eval

import (
	"fmt"
	"reflect"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internPredicateBuiltins registers the "cheap breadth" clojure.core surface
// of design/08 §5 Batch 1 (ADR 0022): the type/number predicates, the scalar
// coercions, and the host-inspecting seq/coll primitives. These fill the fns
// that already-resolving suite files call in their bodies, so those files flip
// from error→pass transitively. The compositional fns that ride on top of
// these (pos-int?, the *-ident?/*-symbol? family, butlast/last/…, not=) live
// in the embedded core/predicates.cljg, loaded right after core.clj.
//
// Every name is a real clojure.core name — precedence-safe additions, never
// renames (CLAUDE.md precedence principle). Wired into internBuiltins by ONE
// line (e.internPredicateBuiltins(def)), per the merge-friendly discipline.
func (e *Evaluator) internPredicateBuiltins(def func(string, func(...any) any) *lang.Var) {
	// ---- type predicates ----------------------------------------------------

	// any?: always true — Clojure's (constantly true) spec predicate.
	def("any?", func(args ...any) any {
		oneArg("any?", args)
		return true
	})

	// coll?: an IPersistentCollection (lists, vectors, maps, sets, seqs).
	// (coll? [])/(coll? {}) => true; (coll? "x")/(coll? nil) => false.
	def("coll?", func(args ...any) any {
		_, ok := oneArg("coll?", args).(lang.IPersistentCollection)
		return ok
	})

	// ifn?: anything invokable — real fns AND the invokable data structures
	// (keywords, symbols, maps, vectors, sets all implement IFn in cljgo).
	def("ifn?", func(args ...any) any {
		_, ok := oneArg("ifn?", args).(lang.IFn)
		return ok
	})

	// fn?: a genuine function, NOT an invokable collection/keyword/symbol/var.
	// (fn? :a)/(fn? {}) => false; (fn? +)/(fn? (fn [] 1)) => true.
	def("fn?", func(args ...any) any {
		return isRealFn(oneArg("fn?", args))
	})

	// seqable?: (seq x) is legal — nil, strings, anything Seqable/ISeq, and
	// a raw Go slice/array (a cljgo array, ADR 0024 — (seq (object-array 3))
	// already works via lang.Seq's reflect fallback, so seqable? must agree).
	def("seqable?", func(args ...any) any {
		x := oneArg("seqable?", args)
		if x == nil {
			return true
		}
		switch x.(type) {
		case string, lang.Seqable, lang.ISeq:
			return true
		}
		switch reflect.ValueOf(x).Kind() {
		case reflect.Slice, reflect.Array:
			return true
		}
		return false
	})

	// counted?: constant-time count (Counted).
	def("counted?", func(args ...any) any {
		_, ok := oneArg("counted?", args).(lang.Counted)
		return ok
	})

	// associative?: an Associative (maps and vectors — not sets/lists).
	def("associative?", func(args ...any) any {
		_, ok := oneArg("associative?", args).(lang.Associative)
		return ok
	})

	// reversible?: supports rseq (Reversible — vectors, sorted maps/sets).
	def("reversible?", func(args ...any) any {
		_, ok := oneArg("reversible?", args).(lang.Reversible)
		return ok
	})

	// sorted?: a sorted collection (Sorted — sorted maps/sets).
	def("sorted?", func(args ...any) any {
		_, ok := oneArg("sorted?", args).(lang.Sorted)
		return ok
	})

	// set?: an IPersistentSet.
	def("set?", func(args ...any) any {
		_, ok := oneArg("set?", args).(lang.IPersistentSet)
		return ok
	})

	// list?: an IPersistentList (a list, not a vector or a bare seq).
	def("list?", func(args ...any) any {
		_, ok := oneArg("list?", args).(lang.IPersistentList)
		return ok
	})

	// indexed?: O(1) nth (Indexed — vectors).
	def("indexed?", func(args ...any) any {
		_, ok := oneArg("indexed?", args).(lang.Indexed)
		return ok
	})

	// uuid?: cljgo has no UUID value type, so this is always false. Batch 2
	// adds parse-uuid/random-uuid; until a UUID type exists nothing is one.
	def("uuid?", func(args ...any) any {
		oneArg("uuid?", args)
		return false
	})

	// ---- number predicates --------------------------------------------------

	// number?: a Number (every numeric type; Char is NOT a number).
	def("number?", func(args ...any) any {
		return lang.IsNumber(oneArg("number?", args))
	})

	// int?/integer?: a fixed-precision integer (int*, uint*, BigInt) — NOT a
	// float, ratio, or bigdecimal. (int? 1) => true; (int? 1.0) => false.
	def("int?", func(args ...any) any {
		return lang.IsInteger(oneArg("int?", args))
	})
	def("integer?", func(args ...any) any {
		return lang.IsInteger(oneArg("integer?", args))
	})

	// float?: a floating-point number (float32 or float64).
	// (float? 1.0) => true; (float? 1) => false.
	def("float?", func(args ...any) any {
		switch oneArg("float?", args).(type) {
		case float32, float64:
			return true
		}
		return false
	})

	// double?: a double specifically (float64).
	def("double?", func(args ...any) any {
		_, ok := oneArg("double?", args).(float64)
		return ok
	})

	// ratio?: a Ratio.
	def("ratio?", func(args ...any) any {
		_, ok := oneArg("ratio?", args).(*lang.Ratio)
		return ok
	})

	// decimal?: a BigDecimal.
	def("decimal?", func(args ...any) any {
		_, ok := oneArg("decimal?", args).(*lang.BigDecimal)
		return ok
	})

	// rational?: an integer, a ratio, or a bigdecimal.
	def("rational?", func(args ...any) any {
		x := oneArg("rational?", args)
		if lang.IsInteger(x) {
			return true
		}
		switch x.(type) {
		case *lang.Ratio, *lang.BigDecimal:
			return true
		}
		return false
	})

	// nan?: is x NaN. Non-floats are not NaN (Clojure coerces then tests).
	def("nan?", func(args ...any) any {
		return lang.IsNaN(oneArg("nan?", args))
	})

	// ---- value predicates ---------------------------------------------------

	// boolean?: a bool.
	def("boolean?", func(args ...any) any {
		_, ok := oneArg("boolean?", args).(bool)
		return ok
	})

	// char?: a Char.
	def("char?", func(args ...any) any {
		_, ok := oneArg("char?", args).(lang.Char)
		return ok
	})

	// ---- scalar coercions (kept simple; the numeric tower's promotion edges
	//      are Batch 2). Each coerces its single numeric/char argument. -------

	def("int", func(args ...any) any {
		return int64(lang.IntCast(oneArg("int", args)))
	})
	def("long", func(args ...any) any {
		return lang.LongCast(oneArg("long", args))
	})
	def("double", func(args ...any) any {
		return lang.AsFloat64(oneArg("double", args))
	})
	def("float", func(args ...any) any {
		return lang.FloatCast(oneArg("float", args))
	})
	def("short", func(args ...any) any {
		return lang.ShortCast(oneArg("short", args))
	})
	def("byte", func(args ...any) any {
		return lang.ByteCast(oneArg("byte", args))
	})
	def("char", func(args ...any) any {
		return lang.CharCast(oneArg("char", args))
	})
	def("boolean", func(args ...any) any {
		return lang.BooleanCast(oneArg("boolean", args))
	})
	// num: coerce to a Number (identity for numbers; error otherwise).
	def("num", func(args ...any) any {
		x := oneArg("num", args)
		if lang.IsNumber(x) {
			return x
		}
		panic(fmt.Errorf("num: not a number: %s", lang.PrintString(x)))
	})

	// ---- seq/coll host primitives -------------------------------------------

	// peek: top of a stack — vector's last, list/seq's first; nil => nil.
	def("peek", func(args ...any) any {
		x := oneArg("peek", args)
		if x == nil {
			return nil
		}
		s, ok := x.(lang.IPersistentStack)
		if !ok {
			panic(fmt.Errorf("peek: not a stack: %s", lang.PrintString(x)))
		}
		return s.Peek()
	})

	// pop: stack without its top — vector drops last, list/seq drops first.
	def("pop", func(args ...any) any {
		x := oneArg("pop", args)
		if x == nil {
			panic(lang.NewIllegalStateError("Can't pop empty list"))
		}
		s, ok := x.(lang.IPersistentStack)
		if !ok {
			panic(fmt.Errorf("pop: not a stack: %s", lang.PrintString(x)))
		}
		return s.Pop()
	})

	// subvec: (subvec v start) | (subvec v start end) — a view of v.
	def("subvec", func(args ...any) any {
		if len(args) != 2 && len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: subvec", len(args)))
		}
		v, ok := args[0].(lang.IPersistentVector)
		if !ok {
			panic(fmt.Errorf("subvec: not a vector: %s", lang.PrintString(args[0])))
		}
		start, ok := lang.AsInt(args[1])
		if !ok {
			panic(fmt.Errorf("subvec: start must be an integer, got: %s", lang.PrintString(args[1])))
		}
		end := v.Count()
		if len(args) == 3 {
			e, ok := lang.AsInt(args[2])
			if !ok {
				panic(fmt.Errorf("subvec: end must be an integer, got: %s", lang.PrintString(args[2])))
			}
			end = e
		}
		if start < 0 || end > v.Count() || start > end {
			panic(fmt.Errorf("subvec: index out of bounds [%d %d) of length %d", start, end, v.Count()))
		}
		return lang.NewSubVector(nil, v, start, end)
	})

	// rseq: reverse seq of a Reversible in O(1)-ish; nil when empty.
	def("rseq", func(args ...any) any {
		x := oneArg("rseq", args)
		r, ok := x.(lang.Reversible)
		if !ok {
			panic(fmt.Errorf("rseq: not reversible: %s", lang.PrintString(x)))
		}
		return r.RSeq()
	})

	// find: (find map k) — the MapEntry for k, or nil.
	def("find", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: find", len(args)))
		}
		if args[0] == nil {
			return nil
		}
		a, ok := args[0].(lang.Associative)
		if !ok {
			panic(fmt.Errorf("find: not associative: %s", lang.PrintString(args[0])))
		}
		if a.ContainsKey(args[1]) {
			return a.EntryAt(args[1])
		}
		return nil
	})

	// key/val: the key/value of a MapEntry (or any [k v] pair).
	def("key", func(args ...any) any {
		e, ok := oneArg("key", args).(lang.IMapEntry)
		if !ok {
			panic(fmt.Errorf("key: not a map entry: %s", lang.PrintString(args[0])))
		}
		return e.Key()
	})
	def("val", func(args ...any) any {
		e, ok := oneArg("val", args).(lang.IMapEntry)
		if !ok {
			panic(fmt.Errorf("val: not a map entry: %s", lang.PrintString(args[0])))
		}
		return e.Val()
	})

	// set: (set coll) — a set of coll's elements; nil => #{}.
	def("set", func(args ...any) any {
		coll := oneArg("set", args)
		if coll == nil {
			return lang.NewSet()
		}
		return lang.NewSet(lang.ToSlice(coll)...)
	})

	// disj: (disj set & ks) — set without the given keys; nil => nil.
	def("disj", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: disj"))
		}
		if args[0] == nil {
			return nil
		}
		s, ok := args[0].(lang.IPersistentSet)
		if !ok {
			panic(fmt.Errorf("disj: not a set: %s", lang.PrintString(args[0])))
		}
		for _, k := range args[1:] {
			s = s.Disjoin(k)
		}
		return s
	})

	// empty: an empty coll of x's type, or nil for non-collections/nil.
	def("empty", func(args ...any) any {
		c, ok := oneArg("empty", args).(lang.IPersistentCollection)
		if !ok {
			return nil
		}
		return c.Empty()
	})

	// identical?: reference identity (same object). Uncomparable dynamic
	// types can't be ==; treat those as not identical rather than panicking.
	def("identical?", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: identical?", len(args)))
		}
		return goIdentical(args[0], args[1])
	})

	// compare: total order — negative/zero/positive int64 (clojure.core).
	def("compare", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: compare", len(args)))
		}
		return int64(lang.Compare(args[0], args[1]))
	})

	// sorted-set: (sorted-set & xs) — a set sorted by compare.
	def("sorted-set", func(args ...any) any {
		return lang.CreatePersistentTreeSet(lang.NewList(args...))
	})

	// sorted-set-by: (sorted-set-by comparator & xs).
	def("sorted-set-by", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: sorted-set-by"))
		}
		comp, ok := args[0].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("sorted-set-by: comparator must be a function, got: %s", lang.PrintString(args[0])))
		}
		return lang.CreatePersistentTreeSetWithComparator(comp, lang.NewList(args[1:]...))
	})

	// realized?: has a pending value (lazy-seq/delay/future) been forced.
	def("realized?", func(args ...any) any {
		p, ok := oneArg("realized?", args).(lang.IPending)
		if !ok {
			panic(fmt.Errorf("realized?: not a pending value: %s", lang.PrintString(args[0])))
		}
		return p.IsRealized()
	})
}

// isRealFn reports whether x is a genuine function value rather than one of
// the invokable data structures (keyword/symbol/map/vector/set) or a var.
// Mirrors Clojure's Fn marker: (fn? :a) and (fn? {}) are false.
func isRealFn(x any) bool {
	if _, ok := x.(lang.IFn); !ok {
		return false
	}
	switch x.(type) {
	case lang.Keyword, *lang.Symbol, *lang.Var, lang.IPersistentCollection:
		return false
	}
	return true
}

// goIdentical is a panic-safe reference-identity test: Go's == panics when a
// dynamic type is uncomparable (e.g. a slice-backed value), which for
// identical? simply means "not the same object".
func goIdentical(a, b any) (res bool) {
	defer func() {
		if recover() != nil {
			res = false
		}
	}()
	return a == b
}
