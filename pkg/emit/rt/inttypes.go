package rt

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Unboxed int64 arithmetic intrinsics (spike s42 / ADR 0067).
//
// These back the emitter's numeric type-inference pass: when the emitter
// PROVES both operands of a core arithmetic op are int64 (from literals,
// arithmetic on proven-int64 operands, numeric loop/recur carriers, or a
// specialized int64 parameter), it emits a raw call to one of these
// helpers on Go `int64` values instead of `rt.Add2(v, any, any)` on
// `any`. The value stays an unboxed Go int64 across the whole numeric
// chain — Go only boxes it (one runtime.convT64) at the boundary where it
// crosses back into `any` (a var store, a collection put, an fn arg/return
// not itself specialized). That boundary box is the accepted cost; the
// per-op boxing that dominated `(fact 15)` is gone.
//
// TOWER PRESERVATION. cljgo's pristine `+ - *` on two int64 THROW on
// overflow — they do NOT promote to BigInt (that is `+' -' *'`; see
// pkg/lang/numberops.go int64Ops.Add/Sub/Multiply). So a checked int64
// result is ALWAYS an int64 or an ArithmeticException — never a BigInt.
// That is precisely what lets us keep the value in an int64 Go local: the
// only non-int64 outcome is the same panic the tower raises, with the same
// message. These helpers reproduce int64Ops.Add/Sub/Multiply's overflow
// tests byte-for-byte, so the emitted error string stays conformance-frozen
// identical.
//
// REDEFINITION. These ops themselves do not consult the operator var —
// but every emitted region that reaches them sits behind an
// `if !rt.CoreDirty()` entry guard (the ADR 0066 sealed-core flag): a
// redefinition of clojure.core/+ et al trips lang.CoreArithDirty, the
// guard fails, and execution falls through to the boxed emission whose
// Add2/Sub2/Mul2 helpers re-check the flag per call and take the derefed
// redefined value. So with-redefs of core arithmetic is honored at typed
// call sites too, at region granularity (checked on entry to each fn
// call / loop; a redefinition landing MID-flight inside one region
// activation is seen from the next activation on — see ADR 0067).

// IAdd is (+ x y) on proven int64 operands: checked, panics "integer
// overflow" identically to lang int64Ops.Add.
func IAdd(x, y int64) int64 {
	c := x + y
	if (c > x) == (y > 0) {
		return c
	}
	panic(lang.NewArithmeticError("integer overflow"))
}

// ISub is (- x y) on proven int64 operands.
func ISub(x, y int64) int64 {
	c := x - y
	if (c < x) == (y > 0) {
		return c
	}
	panic(lang.NewArithmeticError("integer overflow"))
}

// IMul is (* x y) on proven int64 operands.
func IMul(x, y int64) int64 {
	if x == 0 || y == 0 {
		return 0
	}
	c := x * y
	if (c < 0) == ((x < 0) != (y < 0)) {
		if c/y == x {
			return c
		}
	}
	panic(lang.NewArithmeticError("integer overflow"))
}

// IInc is (inc x) / IDec is (dec x) on a proven int64 — checked, matching
// the tower (JVM inc/dec on longs throw on overflow).
func IInc(x int64) int64 { return IAdd(x, 1) }
func IDec(x int64) int64 { return ISub(x, 1) }

// MustInt64 re-types the `any` result of a call whose callee the emitter
// proved returns int64 (a self-recursive call into an int64-specialized
// fn). By construction the value is an int64 — the specialized fn's typed
// body returns int64 or panics with overflow — so this assertion is an
// inference invariant, not a user-facing check. It panics through the
// normal error channel (never a bare Go type-assertion panic) if the
// invariant is ever violated, so a mis-inference surfaces as a diagnosable
// error rather than a raw runtime crash.
func MustInt64(v any) int64 {
	if i, ok := v.(int64); ok {
		return i
	}
	panic(fmt.Errorf("internal: numeric-specialized call returned %T, expected int64 (spike s42 inference invariant violated)", v))
}
