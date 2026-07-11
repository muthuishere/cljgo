// S6 — hand-written Go approximating what the cljgo emitter WOULD produce
// for (defn fib [n] ...) / (defn fact [n] ...) under five emission strategies.
//
// Clojure source being "compiled":
//
//	(defn fib [n]  (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2)))))
//	(defn fact [n] (if (< n 2) 1 (* n (fact (- n 1)))))
//
// Variants:
//  1. Raw Go        — int64 params, direct calls. The ceiling.
//  2. Boxed         — any params/returns, direct Go calls. Prices boxing alone.
//  3. Fn + Apply1   — boxed + call through lang.Fn via Apply1, self-reference
//     through a plain (non-atomic) binding. Prices the calling convention.
//  4. Var per call  — variant 3 + atomic Var deref at EVERY call site.
//     The full REPL-live default.
//  5. Var hoisted   — Var deref once per top-level invocation; recursive
//     self-calls go direct to the deref'd closure value.
//
// Plus: variant 4 built on atomic.Pointer, sync.Mutex, sync.RWMutex vars to
// settle the Var-representation choice.
package s6

import (
	"github.com/muthuishere/cljgo/spikes/s6-var-indirection/lang"
)

// ---------------------------------------------------------------------------
// Variant 1 — raw Go (the ceiling)
// ---------------------------------------------------------------------------

func RawFib(n int64) int64 {
	if n < 2 {
		return n
	}
	return RawFib(n-1) + RawFib(n-2)
}

func RawFact(n int64) int64 {
	if n < 2 {
		return 1
	}
	return n * RawFact(n-1)
}

// ---------------------------------------------------------------------------
// Variant 2 — boxed values (any), direct Go call
// ---------------------------------------------------------------------------

func BoxedFib(n any) any {
	ni := n.(int64)
	if ni < 2 {
		return n
	}
	a := BoxedFib(ni - 1).(int64)
	b := BoxedFib(ni - 2).(int64)
	return a + b
}

func BoxedFact(n any) any {
	ni := n.(int64)
	if ni < 2 {
		return int64(1)
	}
	return ni * BoxedFact(ni-1).(int64)
}

// ---------------------------------------------------------------------------
// Variant 3 — boxed + lang.Fn calling convention via Apply1,
// self-reference through a plain package-level binding (no atomic).
// ---------------------------------------------------------------------------

var fnFib lang.Fn
var fnFact lang.Fn

func init() {
	fnFib = func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return args[0]
		}
		a := lang.Apply1(fnFib, n-1).(int64)
		b := lang.Apply1(fnFib, n-2).(int64)
		return a + b
	}
	fnFact = func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return int64(1)
		}
		return n * lang.Apply1(fnFact, n-1).(int64)
	}
}

// ---------------------------------------------------------------------------
// Variant 4 — full REPL-live default: atomic Var deref at every call site
// ---------------------------------------------------------------------------

var (
	VarFib  = lang.NewVar(nil)
	VarFact = lang.NewVar(nil)
)

func init() {
	VarFib.Set(func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return args[0]
		}
		a := lang.Apply1(VarFib.Deref(), n-1).(int64)
		b := lang.Apply1(VarFib.Deref(), n-2).(int64)
		return a + b
	})
	VarFact.Set(func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return int64(1)
		}
		return n * lang.Apply1(VarFact.Deref(), n-1).(int64)
	})
}

// ---------------------------------------------------------------------------
// Variant 4p — same, Var backed by atomic.Pointer instead of atomic.Value
// ---------------------------------------------------------------------------

var PtrVarFib = lang.NewPtrVar(nil)

func init() {
	PtrVarFib.Set(func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return args[0]
		}
		a := lang.Apply1(PtrVarFib.Deref(), n-1).(int64)
		b := lang.Apply1(PtrVarFib.Deref(), n-2).(int64)
		return a + b
	})
}

// ---------------------------------------------------------------------------
// Variant 4m — same, Var behind sync.Mutex (strawman) and sync.RWMutex
// ---------------------------------------------------------------------------

var MutexVarFib = lang.NewMutexVar(nil)

func init() {
	MutexVarFib.Set(func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return args[0]
		}
		a := lang.Apply1(MutexVarFib.Deref(), n-1).(int64)
		b := lang.Apply1(MutexVarFib.Deref(), n-2).(int64)
		return a + b
	})
}

var RWMutexVarFib = lang.NewRWMutexVar(nil)

func init() {
	RWMutexVarFib.Set(func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return args[0]
		}
		a := lang.Apply1(RWMutexVarFib.Deref(), n-1).(int64)
		b := lang.Apply1(RWMutexVarFib.Deref(), n-2).(int64)
		return a + b
	})
}

// ---------------------------------------------------------------------------
// Variant 5 — Var deref hoisted: one atomic load per top-level invocation,
// recursive self-calls hit the closure value directly. A re-def lands on the
// NEXT top-level call, not mid-recursion.
// ---------------------------------------------------------------------------

var (
	HoistVarFib  = lang.NewVar(nil)
	HoistVarFact = lang.NewVar(nil)
)

func init() {
	var fibSelf lang.Fn
	fibSelf = func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return args[0]
		}
		a := lang.Apply1(fibSelf, n-1).(int64)
		b := lang.Apply1(fibSelf, n-2).(int64)
		return a + b
	}
	HoistVarFib.Set(fibSelf)

	var factSelf lang.Fn
	factSelf = func(args ...any) any {
		n := args[0].(int64)
		if n < 2 {
			return int64(1)
		}
		return n * lang.Apply1(factSelf, n-1).(int64)
	}
	HoistVarFact.Set(factSelf)
}

// CallHoisted is what the emitter produces at an external call site under
// variant 5: deref once, then Apply1 the concrete closure.
func CallHoisted(v *lang.Var, arg any) any {
	return lang.Apply1(v.Deref(), arg)
}

// ---------------------------------------------------------------------------
// Variant 6 — the doc 04 §5 ladder rung: fixed-arity closure type
// (lang.Fn1 = func(any) any, no variadic slice) WITH per-call Var deref kept.
// REPL-live semantics identical to variant 4; only the call instruction and
// closure representation change.
// ---------------------------------------------------------------------------

var (
	FixedVarFib  = lang.NewVar1(nil)
	FixedVarFact = lang.NewVar1(nil)
)

func init() {
	FixedVarFib.Set(func(n any) any {
		ni := n.(int64)
		if ni < 2 {
			return n
		}
		a := FixedVarFib.Deref1()(ni - 1).(int64)
		b := FixedVarFib.Deref1()(ni - 2).(int64)
		return a + b
	})
	FixedVarFact.Set(func(n any) any {
		ni := n.(int64)
		if ni < 2 {
			return int64(1)
		}
		return ni * FixedVarFact.Deref1()(ni-1).(int64)
	})
}
