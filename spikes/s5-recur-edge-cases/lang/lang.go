// Package lang is a micro version of the single shared runtime (design/00
// §4.2): Value = any, Fn func(...any) any, Apply fast paths, IsTruthy,
// Var with atomic root, int64 arithmetic builtins, println.
package lang

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Fn is the compiled-closure representation. Satisfies IFn via Invoke.
type Fn func(args ...any) any

func (f Fn) Invoke(args ...any) any { return f(args...) }

// IFn is the callable interface (deftypes, keywords, colls later).
type IFn interface{ Invoke(args ...any) any }

// ---- Vars ----------------------------------------------------------------

type varBox struct{ v any } // atomic.Value requires a consistent concrete type

// Var is the mutable indirection layer: every global reference derefs the
// root at call time so REPL re-def stays live (design/00 §4.2).
type Var struct {
	Name string
	root atomic.Value
}

func (v *Var) Get() any {
	b := v.root.Load()
	if b == nil {
		panic("unbound var: " + v.Name)
	}
	return b.(varBox).v
}

func (v *Var) BindRoot(x any) { v.root.Store(varBox{x}) }

var (
	registryMu sync.Mutex
	registry   = map[string]*Var{}
)

// InternVar is idempotent and side-effect free, so emitted code may hoist
// interns to package-level vars (design/00 §4.4 rationale, applied to vars).
func InternVar(name string) *Var {
	registryMu.Lock()
	defer registryMu.Unlock()
	if v, ok := registry[name]; ok {
		return v
	}
	v := &Var{Name: name}
	registry[name] = v
	return v
}

// ---- Calling convention ---------------------------------------------------

func Apply(f any, args []any) any {
	switch fn := f.(type) {
	case Fn:
		return fn(args...)
	case func(args ...any) any:
		return fn(args...)
	case IFn:
		return fn.Invoke(args...)
	}
	panic(fmt.Sprintf("not a function: %T", f))
}

// Apply0..2 fast paths: skip the []any literal at the call site and the
// Apply type-switch re-dispatch for the common Fn case. (True zero-alloc
// needs fixed-arity fn types — Fn1/Fn2 — which is performance-ladder work,
// not S1's; calling a variadic Go func always materializes a slice.)
func Apply0(f any) any {
	if fn, ok := f.(Fn); ok {
		return fn()
	}
	return Apply(f, nil)
}

func Apply1(f any, a1 any) any {
	if fn, ok := f.(Fn); ok {
		return fn(a1)
	}
	return Apply(f, []any{a1})
}

func Apply2(f any, a1, a2 any) any {
	if fn, ok := f.(Fn); ok {
		return fn(a1, a2)
	}
	return Apply(f, []any{a1, a2})
}

func CheckArity(args []any, n int) {
	if len(args) != n {
		panic(fmt.Sprintf("wrong number of args (%d), expected %d", len(args), n))
	}
}

// IsTruthy: only nil and false are falsy.
func IsTruthy(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

// ---- Builtins (micro-core, int64 only) -------------------------------------

func asInt64(v any) int64 {
	i, ok := v.(int64)
	if !ok {
		panic(fmt.Sprintf("expected int64, got %T (%v)", v, v))
	}
	return i
}

var PrintlnFn = Fn(func(args ...any) any {
	fmt.Println(args...)
	return nil
})

var addFn = Fn(func(args ...any) any {
	var acc int64
	for _, a := range args {
		acc += asInt64(a)
	}
	return acc
})

var subFn = Fn(func(args ...any) any {
	if len(args) == 0 {
		panic("wrong number of args (0) for -")
	}
	if len(args) == 1 {
		return -asInt64(args[0])
	}
	acc := asInt64(args[0])
	for _, a := range args[1:] {
		acc -= asInt64(a)
	}
	return acc
})

var mulFn = Fn(func(args ...any) any {
	acc := int64(1)
	for _, a := range args {
		acc *= asInt64(a) // wraps on overflow, same as the driver's oracle
	}
	return acc
})

var divFn = Fn(func(args ...any) any {
	if len(args) == 0 {
		panic("wrong number of args (0) for /")
	}
	acc := asInt64(args[0])
	for _, a := range args[1:] {
		acc /= asInt64(a)
	}
	return acc
})

func cmpChain(name string, pred func(a, b int64) bool) Fn {
	return Fn(func(args ...any) any {
		if len(args) < 1 {
			panic("wrong number of args (0) for " + name)
		}
		for i := 0; i+1 < len(args); i++ {
			if !pred(asInt64(args[i]), asInt64(args[i+1])) {
				return false
			}
		}
		return true
	})
}

var eqFn = Fn(func(args ...any) any {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != args[i+1] {
			return false
		}
	}
	return true
})

// ---- micro-vector (S5 case 1 needs conj/nth to collect closures) -----------
// Represented as []any with copy-on-conj (persistent enough for the spike).

func asVec(v any) []any {
	s, ok := v.([]any)
	if !ok {
		panic(fmt.Sprintf("expected vector, got %T (%v)", v, v))
	}
	return s
}

var vectorFn = Fn(func(args ...any) any {
	out := make([]any, len(args))
	copy(out, args)
	return out
})

var conjFn = Fn(func(args ...any) any {
	if len(args) != 2 {
		panic(fmt.Sprintf("wrong number of args (%d) for conj", len(args)))
	}
	old := asVec(args[0])
	out := make([]any, len(old), len(old)+1)
	copy(out, old)
	return append(out, args[1])
})

var nthFn = Fn(func(args ...any) any {
	if len(args) != 2 {
		panic(fmt.Sprintf("wrong number of args (%d) for nth", len(args)))
	}
	return asVec(args[0])[asInt64(args[1])]
})

var countFn = Fn(func(args ...any) any {
	if len(args) != 1 {
		panic(fmt.Sprintf("wrong number of args (%d) for count", len(args)))
	}
	return int64(len(asVec(args[0])))
})

var initOnce sync.Once

// Init binds the micro-core builtins. Idempotent; safe after package-level
// InternVar hoists have run (interning is order-independent).
func Init() {
	initOnce.Do(func() {
		bind := func(name string, f Fn) { InternVar(name).BindRoot(f) }
		bind("+", addFn)
		bind("-", subFn)
		bind("*", mulFn)
		bind("/", divFn)
		bind("<", cmpChain("<", func(a, b int64) bool { return a < b }))
		bind(">", cmpChain(">", func(a, b int64) bool { return a > b }))
		bind("<=", cmpChain("<=", func(a, b int64) bool { return a <= b }))
		bind(">=", cmpChain(">=", func(a, b int64) bool { return a >= b }))
		bind("=", eqFn)
		bind("println", PrintlnFn)
		bind("vector", vectorFn)
		bind("conj", conjFn)
		bind("nth", nthFn)
		bind("count", countFn)
	})
}
