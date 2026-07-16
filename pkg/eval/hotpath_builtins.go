package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internHotpathBuiltins interns the clojure.core fns whose per-element cost
// dominates real workloads — reduce, map, filter, mapv, comp — as native Go,
// per ADR 0039 (spikes S19/S21: interpreted `reduce` alone cost 8.2× on the
// let-go benchmark suite; every fast Clojure hosts these natively — let-go's
// reduce is Go, joker's core is Go, babashka's is GraalVM-compiled, JVM
// Clojure bottoms out in Java). Semantics replicate the former core.clj
// definitions exactly — the oracle comments below are verified against JVM
// Clojure 1.12.5 and the precedence principle applies: no drift, ever. One
// implementation serves both modes (design/00 §2): the REPL and emitted
// binaries call the same fns through the same vars. Kept in this file so
// builtins.go gains exactly one call line inside internBuiltins.
func (e *Evaluator) internHotpathBuiltins(def func(string, func(...any) any) *lang.Var) {
	// mapSeq : lazy 1-coll map, the hot arity.
	var mapSeq func(f, coll any) any
	mapSeq = func(f, coll any) any {
		return lang.NewLazySeq(func() any {
			s := lang.Seq(coll)
			if lang.IsNil(s) {
				return nil
			}
			return lang.NewCons(lang.Apply1(f, s.First()), mapSeq(f, s.Next()))
		})
	}

	// map2Seq : lazy 2-coll zip map; stops at the shorter input.
	var map2Seq func(f, c1, c2 any) any
	map2Seq = func(f, c1, c2 any) any {
		return lang.NewLazySeq(func() any {
			s1, s2 := lang.Seq(c1), lang.Seq(c2)
			if lang.IsNil(s1) || lang.IsNil(s2) {
				return nil
			}
			return lang.NewCons(lang.Apply2(f, s1.First(), s2.First()),
				map2Seq(f, s1.Next(), s2.Next()))
		})
	}

	// mapNSeq : lazy N-coll map (n ≥ 3); stops at the shortest input.
	var mapNSeq func(f any, colls []any) any
	mapNSeq = func(f any, colls []any) any {
		return lang.NewLazySeq(func() any {
			firsts := make([]any, len(colls))
			rests := make([]any, len(colls))
			for i, c := range colls {
				s := lang.Seq(c)
				if lang.IsNil(s) {
					return nil
				}
				firsts[i] = s.First()
				rests[i] = s.Next()
			}
			return lang.NewCons(lang.Apply(f, firsts), mapNSeq(f, rests))
		})
	}

	// oracle: (map inc [1 2 3]) => (2 3 4); (map + [1 2 3] [10 20 30]) => (11 22 33)
	// oracle: (map + [1 2] [10 20] [100 200]) => (111 222)
	// oracle: (into [] (map inc) [1 2 3]) => [2 3 4]  -- 1-arity is the transducer form
	def("map", func(args ...any) any {
		switch len(args) {
		case 1:
			f := args[0]
			return lang.NewFnFunc1(func(rf any) any {
				return lang.NewFnFunc(func(inner ...any) any {
					switch len(inner) {
					case 0:
						return lang.Apply(rf, nil)
					case 1:
						return lang.Apply1(rf, inner[0])
					case 2:
						return lang.Apply2(rf, inner[0], lang.Apply1(f, inner[1]))
					default:
						panic(fmt.Errorf("wrong number of args (%d) passed to: map transducer step", len(inner)))
					}
				})
			})
		case 2:
			return mapSeq(args[0], args[1])
		case 3:
			return map2Seq(args[0], args[1], args[2])
		default:
			if len(args) < 2 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: map", len(args)))
			}
			return mapNSeq(args[0], args[1:])
		}
	})

	// filterSeq : lazy filter; the thunk loops past rejects so a sparse
	// match does not build a chain of empty lazy nodes.
	var filterSeq func(pred, coll any) any
	filterSeq = func(pred, coll any) any {
		return lang.NewLazySeq(func() any {
			s := lang.Seq(coll)
			for !lang.IsNil(s) {
				x := s.First()
				if lang.IsTruthy(lang.Apply1(pred, x)) {
					return lang.NewCons(x, filterSeq(pred, s.Next()))
				}
				s = s.Next()
			}
			return nil
		})
	}

	// oracle: (filter even? (range 10)) => (0 2 4 6 8)
	// oracle: (into [] (filter even?) (range 10)) => [0 2 4 6 8]
	def("filter", func(args ...any) any {
		switch len(args) {
		case 1:
			pred := args[0]
			return lang.NewFnFunc1(func(rf any) any {
				return lang.NewFnFunc(func(inner ...any) any {
					switch len(inner) {
					case 0:
						return lang.Apply(rf, nil)
					case 1:
						return lang.Apply1(rf, inner[0])
					case 2:
						if lang.IsTruthy(lang.Apply1(pred, inner[1])) {
							return lang.Apply2(rf, inner[0], inner[1])
						}
						return inner[0]
					default:
						panic(fmt.Errorf("wrong number of args (%d) passed to: filter transducer step", len(inner)))
					}
				})
			})
		case 2:
			return filterSeq(args[0], args[1])
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: filter", len(args)))
		}
	})

	// reduce : the seq-walking fold. 2-arity seeds from the first element
	// and calls (f) on an empty coll; both arities honor the `reduced`
	// short-circuit box.
	// oracle: (reduce + 0 (range 1 11)) => 55; (reduce + (range 1 11)) => 55.
	def("reduce", func(args ...any) any {
		var f, acc any
		var s lang.ISeq
		switch len(args) {
		case 2:
			f = args[0]
			s = lang.Seq(args[1])
			if lang.IsNil(s) {
				return lang.Apply(f, nil)
			}
			acc = s.First()
			s = s.Next()
		case 3:
			f = args[0]
			acc = args[1]
			s = lang.Seq(args[2])
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: reduce", len(args)))
		}
		for !lang.IsNil(s) {
			acc = lang.Apply2(f, acc, s.First())
			if r, ok := acc.(*lang.Reduced); ok {
				return r.Deref()
			}
			s = s.Next()
		}
		return acc
	})

	// oracle: (mapv inc [1 2 3]) => [2 3 4]; (mapv + [1 2] [10 20]) => [11 22]
	def("mapv", func(args ...any) any {
		switch len(args) {
		case 2:
			out := make([]any, 0, 16)
			for s := lang.Seq(args[1]); !lang.IsNil(s); s = s.Next() {
				out = append(out, lang.Apply1(args[0], s.First()))
			}
			return lang.NewVector(out...)
		case 3:
			out := make([]any, 0, 16)
			s1, s2 := lang.Seq(args[1]), lang.Seq(args[2])
			for !lang.IsNil(s1) && !lang.IsNil(s2) {
				out = append(out, lang.Apply2(args[0], s1.First(), s2.First()))
				s1, s2 = s1.Next(), s2.Next()
			}
			return lang.NewVector(out...)
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: mapv", len(args)))
		}
	})

	// oracle: ((comp inc inc) 5) => 7; ((comp) :x) => :x; ((comp str +) 1 2) => "3"
	def("comp", func(args ...any) any {
		switch len(args) {
		case 0:
			return lang.NewFnFunc1(func(x any) any { return x })
		case 1:
			return args[0]
		default:
			fns := make([]any, len(args))
			copy(fns, args)
			return lang.NewFnFunc(func(callArgs ...any) any {
				ret := lang.Apply(fns[len(fns)-1], callArgs)
				for i := len(fns) - 2; i >= 0; i-- {
					ret = lang.Apply1(fns[i], ret)
				}
				return ret
			})
		}
	})
}
