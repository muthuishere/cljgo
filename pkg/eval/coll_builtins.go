package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internCollBuiltins interns the runtime primitives that the clojure.core
// sequence & collection library (map/filter/reduce/take/… — all defined in
// core.clj) is built on but which need host support: the lazy-seq thunk
// realizer, the native producers (range/repeat/iterate/cycle), the
// persistent-collection mutators sort/sort-by/dissoc/vec/vals, the reduced
// short-circuit box, and the numeric primitives (<=, >=, quot, rem, max,
// min, and the zero?/pos?/neg?/nil?/some?/true?/false? predicates) that the
// tower must own. Every name is a real clojure.core name — precedence-safe
// additions, never renames (CLAUDE.md precedence principle). Kept in this
// file so builtins.go gains exactly one call line inside internBuiltins.
func (e *Evaluator) internCollBuiltins(def func(string, func(...any) any) *lang.Var) {
	asFn := func(op string, x any) lang.IFn {
		f, ok := x.(lang.IFn)
		if !ok {
			panic(fmt.Errorf("%s: not a function: %s", op, lang.PrintString(x)))
		}
		return f
	}

	// lazy-seq* : (lazy-seq* thunk) — wrap a 0-arg fn as a LazySeq. The
	// `lazy-seq` macro (core.clj) expands its body into that thunk.
	def("lazy-seq*", func(args ...any) any {
		f := asFn("lazy-seq*", oneArg("lazy-seq*", args))
		return lang.NewLazySeq(func() any { return f.Invoke() })
	})

	// range : (range) infinite | (range end) | (range start end) |
	// (range start end step). Integer args ride the fast LongRange; a
	// non-integer bound falls back to the generic Range. 0-arg is the
	// canonical (iterate inc' 0).
	def("range", func(args ...any) any {
		switch len(args) {
		case 0:
			inc := lang.NewFnFunc1(func(a any) any { return lang.Inc(a) })
			return lang.CreateIterate(inc, int64(0))
		case 1:
			return makeRange(int64(0), args[0], int64(1))
		case 2:
			return makeRange(args[0], args[1], int64(1))
		case 3:
			return makeRange(args[0], args[1], args[2])
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: range", len(args)))
		}
	})

	// repeat : (repeat x) infinite | (repeat n x).
	def("repeat", func(args ...any) any {
		switch len(args) {
		case 1:
			return lang.NewRepeat(args[0])
		case 2:
			// (repeat n x): n coerces the way a Java numeric cast would —
			// including a bare bool (true=1, false=0). Oracle: (repeat
			// false :a) => (); (repeat true :a) => (:a).
			n := args[0]
			if b, ok := n.(bool); ok {
				if b {
					n = int64(1)
				} else {
					n = int64(0)
				}
			}
			return lang.NewRepeatN(lang.AsInt64(n), args[1])
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: repeat", len(args)))
		}
	})

	// iterate : (iterate f x) — x, (f x), (f (f x)), … lazily.
	def("iterate", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: iterate", len(args)))
		}
		return lang.CreateIterate(asFn("iterate", args[0]), args[1])
	})

	// cycle : (cycle coll) — infinite repetition of coll's elements.
	def("cycle", func(args ...any) any {
		s := lang.Seq(oneArg("cycle", args))
		if s == nil {
			return lang.NewList()
		}
		return lang.NewCycle(s)
	})

	// dissoc : (dissoc map & ks) — map without the given keys.
	def("dissoc", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: dissoc"))
		}
		acc := args[0]
		if acc == nil {
			return nil
		}
		m, ok := acc.(lang.IPersistentMap)
		if !ok {
			panic(fmt.Errorf("dissoc: not a map: %s", lang.PrintString(acc)))
		}
		for _, k := range args[1:] {
			m = m.Without(k)
		}
		return m
	})

	// vec : (vec coll) — a vector of coll's elements.
	def("vec", func(args ...any) any {
		coll := oneArg("vec", args)
		if coll == nil {
			return lang.NewVector()
		}
		return lang.NewVector(lang.ToSlice(coll)...)
	})

	// vals : (vals map) — a seq of the map's values, or nil when empty.
	def("vals", func(args ...any) any {
		coll := oneArg("vals", args)
		if coll == nil {
			return nil
		}
		return lang.Vals(coll)
	})

	// sort : (sort coll) | (sort comp coll) — a sorted seq (stable).
	def("sort", func(args ...any) any {
		var comp lang.IFn
		var coll any
		switch len(args) {
		case 1:
			comp = lang.NewFnFunc2(func(a, b any) any { return int64(lang.Compare(a, b)) })
			coll = args[0]
		case 2:
			comp = asFn("sort", args[0])
			coll = args[1]
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: sort", len(args)))
		}
		s := lang.ToSlice(coll)
		lang.SortSlice(s, comp)
		return lang.NewList(s...)
	})

	// sort-by : (sort-by keyfn coll) | (sort-by keyfn comp coll).
	def("sort-by", func(args ...any) any {
		var keyfn, comp lang.IFn
		var coll any
		switch len(args) {
		case 2:
			keyfn = asFn("sort-by", args[0])
			comp = lang.NewFnFunc2(func(a, b any) any { return int64(lang.Compare(a, b)) })
			coll = args[1]
		case 3:
			keyfn = asFn("sort-by", args[0])
			comp = asFn("sort-by", args[1])
			coll = args[2]
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: sort-by", len(args)))
		}
		s := lang.ToSlice(coll)
		keyed := lang.NewFnFunc2(func(a, b any) any {
			return comp.Invoke(keyfn.Invoke(a), keyfn.Invoke(b))
		})
		lang.SortSlice(s, keyed)
		return lang.NewList(s...)
	})

	// reduced / reduced? : the reduce short-circuit box (clojure.core).
	def("reduced", func(args ...any) any { return lang.NewReduced(oneArg("reduced", args)) })
	def("reduced?", func(args ...any) any { return lang.IsReduced(oneArg("reduced?", args)) })

	// <= / >= : chained comparisons over the numeric tower.
	def("<=", chainCompare("<=", func(x, y any) bool {
		return lang.Ops(x).Combine(lang.Ops(y)).LTE(x, y)
	}))
	def(">=", chainCompare(">=", func(x, y any) bool {
		return lang.Ops(x).Combine(lang.Ops(y)).GTE(x, y)
	}))

	// quot / rem : integer division primitives over the numeric tower.
	def("quot", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: quot", len(args)))
		}
		return lang.Ops(args[0]).Combine(lang.Ops(args[1])).Quotient(args[0], args[1])
	})
	def("rem", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: rem", len(args)))
		}
		return lang.Ops(args[0]).Combine(lang.Ops(args[1])).Remainder(args[0], args[1])
	})

	// max / min : variadic maxima/minima over the numeric tower.
	def("max", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: max"))
		}
		acc := args[0]
		for _, a := range args[1:] {
			acc = lang.Max(acc, a)
		}
		return acc
	})
	def("min", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: min"))
		}
		acc := args[0]
		for _, a := range args[1:] {
			acc = lang.Min(acc, a)
		}
		return acc
	})

	// Numeric / value predicates that the tower (or identity) must own.
	def("zero?", func(args ...any) any { return lang.IsZero(oneArg("zero?", args)) })
	def("pos?", func(args ...any) any {
		x := oneArg("pos?", args)
		return lang.Ops(x).IsPos(x)
	})
	def("neg?", func(args ...any) any {
		x := oneArg("neg?", args)
		return lang.Ops(x).IsNeg(x)
	})
	def("nil?", func(args ...any) any { return lang.IsNil(oneArg("nil?", args)) })
	def("some?", func(args ...any) any { return !lang.IsNil(oneArg("some?", args)) })
	def("true?", func(args ...any) any { return oneArg("true?", args) == true })
	def("false?", func(args ...any) any { return oneArg("false?", args) == false })
}

// makeRange builds a range over [start,end) with the given step, using the
// fast LongRange when all bounds are integers and the generic Range
// otherwise.
func makeRange(start, end, step any) any {
	if lang.IsInteger(start) && lang.IsInteger(end) && lang.IsInteger(step) {
		return lang.NewLongRange(lang.AsInt64(start), lang.AsInt64(end), lang.AsInt64(step))
	}
	return lang.NewRange(start, end, step)
}
