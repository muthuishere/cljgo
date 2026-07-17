package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internTransientBuiltins registers Clojure's transient surface
// (design/08 §5 Batch 3, ADR 0022): transient / persistent! / conj! /
// assoc! / dissoc! / disj! / pop!. The state lives entirely in pkg/lang
// transient types (vector/map/set), so BOTH the interpreter and emitted
// Go get identical behavior for free — rt.Boot() interns these into
// clojure.core before an emitted binary's Load() runs, and the emitter
// calls them as ordinary core vars (no intrinsic special-casing). Wired
// into internBuiltins by ONE line (e.internTransientBuiltins(def)), per
// the merge-friendly discipline.
//
// Semantics match Clojure exactly:
//   - a persistent op's `!` variant on a non-transient throws
//     (transient / persistent! type-check their argument);
//   - using a transient after persistent! throws ("transient used
//     after persistent! call", from the pkg/lang ensureEditable guards);
//   - conj!/assoc!/dissoc!/disj!/pop! return the NEW transient, which
//     must be re-bound (the pkg/lang types mutate in place and return
//     self, so re-binding is faithful and required).
func internTransientBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// transient: (transient coll) -> a transient over an editable
	// persistent collection. Throws on a non-editable collection.
	def("transient", func(args ...any) any {
		coll := oneArg("transient", args)
		ec, ok := coll.(lang.IEditableCollection)
		if !ok {
			panic(lang.NewIllegalArgumentError(
				fmt.Sprintf("Don't know how to create transient of %T", coll)))
		}
		return ec.AsTransient()
	})

	// persistent!: (persistent! tcoll) -> the immutable collection,
	// invalidating tcoll. Throws if tcoll is not a transient.
	def("persistent!", func(args ...any) any {
		coll := oneArg("persistent!", args)
		tc, ok := coll.(lang.ITransientCollection)
		if !ok {
			panic(lang.NewIllegalArgumentError(
				fmt.Sprintf("persistent! expects a transient, got %T", coll)))
		}
		return tc.Persistent()
	})

	// conj!: ([] (transient [])) ([coll] coll) ([coll x] ...). The
	// 2-arity mutates the transient and returns it.
	def("conj!", func(args ...any) any {
		switch len(args) {
		case 0:
			return lang.NewVector().AsTransient()
		case 1:
			return args[0]
		case 2:
			tc, ok := args[0].(lang.ITransientCollection)
			if !ok {
				panic(lang.NewIllegalArgumentError(
					fmt.Sprintf("conj! expects a transient, got %T", args[0])))
			}
			return tc.Conj(args[1])
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: conj!", len(args)))
		}
	})

	// assoc!: (assoc! coll k v) / (assoc! coll k v & kvs). Unlike assoc,
	// assoc! tolerates an odd number (> 1) of trailing args — a dangling
	// key with no value is assoc'd with nil. Oracle: (apply assoc! (transient
	// [1]) [0 1 1]) => [1 nil] (index 1 assoc'd with nil).
	def("assoc!", func(args ...any) any {
		if len(args) < 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: assoc!", len(args)))
		}
		ta, ok := args[0].(lang.ITransientAssociative)
		if !ok {
			panic(lang.NewIllegalArgumentError(
				fmt.Sprintf("assoc! expects a transient associative, got %T", args[0])))
		}
		var cur lang.ITransientAssociative = ta
		i := 1
		for ; i+1 < len(args); i += 2 {
			cur = cur.Assoc(args[i], args[i+1])
		}
		if i < len(args) {
			cur = cur.Assoc(args[i], nil)
		}
		return cur
	})

	// dissoc!: (dissoc! coll k & ks) -> mutating remove of keys from a
	// transient map.
	def("dissoc!", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: dissoc!", len(args)))
		}
		tm, ok := args[0].(transientWithout)
		if !ok {
			panic(lang.NewIllegalArgumentError(
				fmt.Sprintf("dissoc! expects a transient map, got %T", args[0])))
		}
		var cur transientWithout = tm
		for _, k := range args[1:] {
			cur = cur.Without(k).(transientWithout)
		}
		return cur
	})

	// disj!: (disj! set k & ks) -> mutating remove of keys from a
	// transient set.
	def("disj!", func(args ...any) any {
		if len(args) < 1 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: disj!", len(args)))
		}
		if len(args) == 1 {
			return args[0]
		}
		ts, ok := args[0].(lang.ITransientSet)
		if !ok {
			panic(lang.NewIllegalArgumentError(
				fmt.Sprintf("disj! expects a transient set, got %T", args[0])))
		}
		var cur lang.ITransientSet = ts
		for _, k := range args[1:] {
			cur = cur.Disjoin(k)
		}
		return cur
	})

	// pop!: (pop! coll) -> mutating remove of the last element of a
	// transient vector.
	def("pop!", func(args ...any) any {
		coll := oneArg("pop!", args)
		tv, ok := coll.(lang.ITransientVector)
		if !ok {
			panic(lang.NewIllegalArgumentError(
				fmt.Sprintf("pop! expects a transient vector, got %T", coll)))
		}
		return tv.Pop()
	})
}

// transientWithout is the mutating dissoc seam of a transient map. cljgo's
// TransientMap exposes Without returning itself (as ITransientMap); the
// ITransientMap interface does not declare it, so dissoc! reaches it
// through this local interface.
type transientWithout interface {
	Without(any) lang.ITransientMap
}
