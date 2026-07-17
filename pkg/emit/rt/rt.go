// Package rt is the runtime bootstrap for cljgo-emitted binaries.
//
// Boot() interns the Go builtins (pkg/corelib) and runs the AOT-compiled
// core (pkg/coreaot, registered through RegisterCoreLoader), snapshotting
// the pristine builtin values that back the guarded arithmetic intrinsics
// below in between. Since ADR 0046 NOTHING here imports pkg/eval: a
// compiled binary has no reader, no analyzer and no tree-walk evaluator
// linked — that is the whole point of AOT core (ADR 0023 / 0037).
//
// Guarded intrinsics: a 2-argument call to a core arithmetic builtin
// (`+ - * / < > =`) emits as rt.Add2(v, x, y) etc. Each helper derefs
// the var PER CALL (one atomic load — ADR 0004 liveness) and compares
// the value against the boot-time builtin: pristine → open-coded int64
// fast path (or the lang numeric tower), redefined → the normal
// lang.Apply2 through the new value. Semantics are those of the
// evaluator's builtins for every input; the only deviation from strict
// evaluation order is that the operator deref happens after the
// argument expressions (observable only when an argument's side effect
// re-defs the operator var mid-call — JVM Clojure's :inline arithmetic
// doesn't even deref).
package rt

import (
	"fmt"
	"reflect"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

var (
	booted     bool
	coreLoader func()

	origAdd, origSub, origMul, origDiv any
	origLT, origGT, origEQ             any
)

// Boot initializes the runtime exactly once: the Go builtins into
// clojure.core (corelib.RegisterAll), then the AOT-compiled core — the
// same sources in the same order as the interpreter's boot, as compiled
// Go — which ends by interning `user` and rooting *ns* there
// (corelib.InitUserNS). Safe to call before any emitted Load().
//
// The builtin snapshot sits BETWEEN the two: the intrinsics compare
// against the PRISTINE builtin, and core's own compiled Load() already
// calls them. (core's sources never re-def +/-/*/<//>/=; if one ever
// did, the guard would simply take the redefined-value path.)
func Boot() {
	if booted {
		return
	}
	booted = true
	corelib.RegisterAll()
	get := func(name string) any {
		return lang.NSCore.FindInternedVar(lang.NewSymbol(name)).Get()
	}
	origAdd = get("+")
	origSub = get("-")
	origMul = get("*")
	origDiv = get("/")
	origLT = get("<")
	origGT = get(">")
	origEQ = get("=")
	if coreLoader == nil {
		panic("rt.Boot: no AOT core linked — a cljgo binary must blank-import github.com/muthuishere/cljgo/pkg/coreaot (the emitter does this; see ADR 0046)")
	}
	coreLoader()
}

// RegisterCoreLoader receives pkg/coreaot's Load from its init(). rt
// cannot import coreaot (coreaot's generated packages import rt for the
// arithmetic intrinsics), so the edge is inverted through this
// registration — the same shape ADR 0042 uses for namespace providers.
func RegisterCoreLoader(load func()) { coreLoader = load }

// RegisterLib registers a namespace's Load() in the lib-provider
// registry (ADR 0042 §2). Emitted dependency packages call it from
// init() — a plain map write, safe before Boot() — so the replayed
// (require …) form triggers the dependency load at exactly its source
// position, once (Load is guarded).
func RegisterLib(name string, load func()) { corelib.RegisterLibProvider(name, load) }

// The helpers keep their hot bodies small (slow tails split out) so the
// Go inliner can fuse the int64 fast path into emitted call sites.

// Add2 is (+ x y) with per-call deref and guarded open-coding.
func Add2(v *lang.Var, x, y any) any {
	if f := v.Get(); f != origAdd {
		return lang.Apply2(f, x, y)
	}
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		s := xi + yi
		if (xi^s)&(yi^s) >= 0 { // no signed overflow
			return s
		}
	}
	return lang.Add(x, y)
}

// Sub2 is (- x y).
func Sub2(v *lang.Var, x, y any) any {
	if f := v.Get(); f != origSub {
		return lang.Apply2(f, x, y)
	}
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		d := xi - yi
		if (xi^yi)&(xi^d) >= 0 { // no signed overflow
			return d
		}
	}
	return lang.Sub(x, y)
}

// Mul2 is (* x y).
func Mul2(v *lang.Var, x, y any) any {
	if f := v.Get(); f != origMul {
		return lang.Apply2(f, x, y)
	}
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return mulChecked(xi, yi)
	}
	return lang.Multiply(x, y)
}

func mulChecked(xi, yi int64) any {
	if xi == 0 || yi == 0 {
		return int64(0)
	}
	// Exclude the MinInt64/-1 pairs so neither the multiply nor the
	// verification divide can fault; they take the tower's checked path.
	if xi != -1 && yi != -1 {
		z := xi * yi
		if z/xi == yi { // no overflow
			return z
		}
	}
	return lang.Multiply(xi, yi)
}

// Div2 is (/ x y) — no open-coded path (ratio semantics live in the
// tower); the guard still skips the variadic builtin's []any.
func Div2(v *lang.Var, x, y any) any {
	if f := v.Get(); f != origDiv {
		return lang.Apply2(f, x, y)
	}
	return lang.Divide(x, y)
}

// LT2 is (< x y).
func LT2(v *lang.Var, x, y any) any {
	if f := v.Get(); f != origLT {
		return lang.Apply2(f, x, y)
	}
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi < yi
	}
	return lang.LT(x, y)
}

// GT2 is (> x y).
func GT2(v *lang.Var, x, y any) any {
	if f := v.Get(); f != origGT {
		return lang.Apply2(f, x, y)
	}
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi > yi
	}
	return lang.GT(x, y)
}

// EQ2 is (= x y).
func EQ2(v *lang.Var, x, y any) any {
	if f := v.Get(); f != origEQ {
		return lang.Apply2(f, x, y)
	}
	return lang.Equiv(x, y)
}

// LTBool/GTBool/EQBool are the unboxed variants the emitter uses
// directly in `if` tests (no interface boxing, no IsTruthy).

func LTBool(v *lang.Var, x, y any) bool {
	if f := v.Get(); f != origLT {
		return lang.IsTruthy(lang.Apply2(f, x, y))
	}
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi < yi
	}
	return lang.LT(x, y)
}

func GTBool(v *lang.Var, x, y any) bool {
	if f := v.Get(); f != origGT {
		return lang.IsTruthy(lang.Apply2(f, x, y))
	}
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi > yi
	}
	return lang.GT(x, y)
}

func EQBool(v *lang.Var, x, y any) bool {
	if f := v.Get(); f != origEQ {
		return lang.IsTruthy(lang.Apply2(f, x, y))
	}
	return lang.Equiv(x, y)
}

// --- Go interop shaping helpers (ADR 0010, design/05 §2) ----------------
//
// These back the AOT emitter's [v err] / `!` / normalization shaping so it
// is byte-identical to the interpreter's reflect path (pkg/eval/host.go).

// NormErr nil-normalizes a Go error into the [v err] slot: a nil error
// becomes Clojure nil (falsy in if/when), a non-nil error stays truthy and
// prints as the same #object[...] the interpreter renders (same type, same
// lang.PrintString).
func NormErr(err error) any {
	if err == nil {
		return nil
	}
	return err
}

// GoError is the value thrown by a `!` interop call whose trailing error is
// non-nil (or whose comma-ok is false). It satisfies `error` and is
// panicked exactly like any other cljgo exception, so the surrounding
// recover machinery handles it uniformly — matching the interpreter, which
// panics the raw error value.
func GoError(err error) error { return err }

// NilNorm maps a typed-nil Go result (pointer/interface/map/slice/chan/
// func) to Clojure nil; any other value passes through. Boxing a nil
// pointer into `any` yields a non-nil interface, so the reflect check is
// required (mirrors normalizeResult).
func NilNorm(v any) any {
	if v == nil {
		return nil
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		if rv.IsNil() {
			return nil
		}
	}
	return v
}

// CallMethod backs the AOT emission of a Clojure dot-form method call
// `(.Method recv arg...)` (ADR 0010, design/05 §1). The receiver's static
// type is unknown in M3.1, so the call is reflective in AOT too — and it
// delegates to the SAME corelib.CallGoMethod the interpreter uses, so the REPL
// and the compiled binary produce byte-identical results by construction.
func CallMethod(recv any, method string, throw bool, args ...any) any {
	return corelib.CallGoMethod(recv, method, throw, args)
}

// FieldGet backs the AOT emission of a Clojure dot-form field read
// `(.-Field recv)` (ADR 0010, design/05 §1). Reflective in AOT too (the
// receiver's static type is unknown in M3.2) and delegating to the SAME
// corelib.GoFieldGet the interpreter uses — byte-identical by construction.
func FieldGet(recv any, field string) any {
	return corelib.GoFieldGet(recv, field)
}

// FieldSet backs the AOT emission of a Go field assignment
// `(set! (.-Field recv) v)` (ADR 0010, design/05 §1), delegating to the SAME
// corelib.GoFieldSet the interpreter uses.
func FieldSet(recv any, field string, val any) any {
	return corelib.GoFieldSet(recv, field, val)
}

// MakeStruct backs the AOT emission of a struct-literal constructor
// `(pkg/Type. {...})` (ADR 0010, design/05 §1). v0 builds reflectively via
// the SAME corelib.MakeGoStruct the interpreter uses — byte-identical.
func MakeStruct(pkg, typeName string, fields any) any {
	return corelib.MakeGoStruct(pkg, typeName, fields)
}

// NewStruct backs the AOT emission of `(go/new pkg/Type)` (ADR 0010,
// design/05 §1), delegating to the SAME corelib.NewGoStruct the interpreter uses.
func NewStruct(pkg, typeName string) any {
	return corelib.NewGoStruct(pkg, typeName)
}

// --- Exception shaping helpers (design/03 §6) ---------------------------
//
// These back the AOT emitter's throw/try emission so it is byte-identical
// to the interpreter's OpThrow panic and OpTry recover: all three delegate
// to the SAME eval functions the tree-walk evaluator calls.

// Throw normalizes a thrown value into the error `panic` carries — a value
// already satisfying `error` throws as-is, anything else wraps so the
// catch-all classes still catch it (corelib.Throw).
func Throw(v any) error { return corelib.Throw(v) }

// Recover normalizes a recovered panic into the thrown error (corelib.Recover).
func Recover(r any) error { return corelib.Recover(r) }

// CatchMatches reports whether a catch clause's class symbol matches the
// thrown value (corelib.CatchMatches).
func CatchMatches(className string, thrown error) bool {
	return corelib.CatchMatches(className, thrown)
}

// ToFloat64 coerces a cljgo numeric arg (int64 or float64) to a Go float,
// matching the interpreter's coerceArg leniency for float parameters.
func ToFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	}
	panic(fmt.Errorf("cannot coerce %T to Go float64", v))
}
