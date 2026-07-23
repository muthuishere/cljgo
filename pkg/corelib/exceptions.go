// exceptions.go — the throw/catch normalization shared by BOTH modes
// (ADR 0007 dual-mode, ADR 0046): the tree-walk OpTry evaluator
// (pkg/eval) and the AOT emitter's deferred-recover closure (through
// pkg/emit/rt) call these SAME functions, so behavior is byte-identical
// by construction — the release-blocker discipline of design/03 §7d.
// They live in corelib because a compiled binary must shape exceptions
// without linking the interpreter (piece 3).
package corelib

import (
	"errors"
	"fmt"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Throw normalizes a thrown value into the Go error that `throw` panics.
// JVM Clojure requires a Throwable; cljgo v0 accepts any value — a value
// that already satisfies `error` (an ex-info, a Go error surfaced by `!`
// interop) is thrown as-is; anything else is wrapped in a *ThrownValue so
// the catch-all classes (Throwable/Exception/RuntimeException) still catch
// it. This is the faithful-ish v0 relaxation documented on ast.ThrowNode.
func Throw(v any) error {
	if err, ok := v.(error); ok {
		return err
	}
	return &ThrownValue{Val: v}
}

// ThrownValue wraps a thrown non-error Clojure value (e.g. (throw 42)).
type ThrownValue struct{ Val any }

func (t *ThrownValue) Error() string { return lang.ToString(t.Val) }

// Recover normalizes a recovered panic value into the thrown error. cljgo
// panics carry errors (the IFn boundary and `throw`), but a stray non-error
// panic is wrapped so catch matching stays total.
func Recover(r any) error {
	if err, ok := r.(error); ok {
		return err
	}
	return fmt.Errorf("%v", r)
}

// Interned targets for the errors.Is probes below. errors.Is(thrown, t)
// unwraps thrown (through lang.EvalError etc.) and consults each layer's
// Is method, so the JVM subclass edges encoded there (ArityError →
// IllegalArgumentError, NumberFormatError → IllegalArgumentError) hold for
// every catch and instance? check without re-stating them here.
var (
	arithmeticTarget  = &lang.ArithmeticError{}
	arityTarget       = &lang.ArityError{}
	illegalArgTarget  = &lang.IllegalArgumentError{}
	illegalStateT     = &lang.IllegalStateError{}
	unsupportedOpT    = &lang.UnsupportedOperationError{}
	indexOOBTarget    = &lang.IndexOutOfBoundsError{}
	classCastTarget   = &lang.ClassCastError{}
	nullPointerTarget = &lang.NullPointerError{}
	numberFormatT     = &lang.NumberFormatError{}
)

// throwableMatches maps a JVM exception-class name (simple or fully
// qualified) to the cljgo error values it catches, honoring the real JVM
// ancestry (oracle-verified against clojure 1.12.5, 2026-07-23; ADR 0039
// addendum): ArithmeticException / ClassCastException /
// NullPointerException / IndexOutOfBoundsException /
// IllegalArgumentException / IllegalStateException /
// UnsupportedOperationException / ExceptionInfo all extend
// RuntimeException < Exception < Throwable; NumberFormatException and
// clojure.lang.ArityException extend IllegalArgumentException. cljgo v0
// treats every thrown value as unchecked, so the three catch-all names
// match anything. Returns (matched, known): known=false means the name is
// outside the table entirely.
func throwableMatches(className string, thrown error) (bool, bool) {
	simple := className
	if i := strings.LastIndex(className, "."); i >= 0 {
		simple = className[i+1:]
	}
	switch simple {
	case "Throwable", "Exception", "RuntimeException":
		return true, true
	case "ExceptionInfo":
		var ei lang.IExceptionInfo
		return errors.As(thrown, &ei), true
	case "ArithmeticException":
		return errors.Is(thrown, arithmeticTarget), true
	case "ArityException":
		return errors.Is(thrown, arityTarget), true
	case "IllegalArgumentException":
		return errors.Is(thrown, illegalArgTarget), true
	case "NumberFormatException":
		return errors.Is(thrown, numberFormatT), true
	case "IllegalStateException":
		return errors.Is(thrown, illegalStateT), true
	case "UnsupportedOperationException":
		return errors.Is(thrown, unsupportedOpT), true
	case "IndexOutOfBoundsException", "StringIndexOutOfBoundsException":
		return errors.Is(thrown, indexOOBTarget), true
	case "ClassCastException":
		return errors.Is(thrown, classCastTarget), true
	case "NullPointerException":
		return errors.Is(thrown, nullPointerTarget), true
	}
	return false, false
}

// CatchMatches reports whether a catch clause's class symbol matches the
// thrown value (design/03 §6). The catch-all classes match any thrown
// value; the standard typed JVM exception names match the cljgo error
// values that correspond semantically, with the JVM ancestry honored
// (throwableMatches, ADR 0039 addendum). Unknown class names never match,
// so the throw propagates — as an unmatched Clojure catch would.
func CatchMatches(className string, thrown error) bool {
	matched, _ := throwableMatches(className, thrown)
	return matched
}
