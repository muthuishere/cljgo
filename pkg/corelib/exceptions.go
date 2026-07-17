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

// CatchMatches reports whether a catch clause's class symbol matches the
// thrown value (design/03 §6). v0 mapping (faithful-ish): the catch-all
// classes match any thrown value; ExceptionInfo matches only an ex-info
// (anything that is/wraps a lang.IExceptionInfo). Unknown class names never
// match, so the throw propagates — as an unmatched Clojure catch would.
func CatchMatches(className string, thrown error) bool {
	switch className {
	case "Throwable", "Exception", "RuntimeException",
		"java.lang.Throwable", "java.lang.Exception", "java.lang.RuntimeException":
		return true
	case "ExceptionInfo", "clojure.lang.ExceptionInfo":
		var ei lang.IExceptionInfo
		return errors.As(thrown, &ei)
	default:
		return false
	}
}
