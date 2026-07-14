package eval

import (
	"errors"
	"fmt"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// This file holds the exception machinery shared by BOTH modes (ADR 0007
// dual-mode): the tree-walk OpTry evaluator here, and the AOT emitter's
// deferred-recover closure (pkg/emit) which calls the exact same Throw /
// Recover / CatchMatches functions through pkg/emit/rt. Behavior is
// byte-identical by construction — the release-blocker discipline of
// design/03 §7d.

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

// evalTry runs an OpTry (design/03 §6): the protected body, catch matching
// in order (first matching class binds the caught exception and runs its
// body), and a finally that always runs for side effect with its value
// discarded. A finally runs on the normal path, on a caught throw, and
// while an uncaught throw unwinds (Go's defer). recur never crosses a try
// (analysis-blocked), so a recurSignal returned by the body is propagated,
// never caught.
func (e *Evaluator) evalTry(n *ast.Node, s *Scope) (result any, rerr error) {
	sub := n.Sub.(*ast.TryNode)

	if sub.Finally != nil {
		defer func() {
			// finally's value is discarded; if finally itself errors it
			// replaces the try's outcome (as a finally that throws does).
			if _, ferr := e.Eval(sub.Finally, s); ferr != nil {
				result, rerr = nil, ferr
			}
		}()
	}

	val, thrown := e.evalProtected(sub.Body, s)
	if thrown == nil {
		return val, nil
	}
	if rs, ok := thrown.(*recurSignal); ok {
		return nil, rs // recur is analysis-blocked across try; be safe
	}
	for _, cn := range sub.Catches {
		c := cn.Sub.(*ast.CatchNode)
		if CatchMatches(c.ClassName, thrown) {
			cs := s.Push()
			b := c.Binding.Sub.(*ast.BindingNode)
			cs.Define(b.Name.Name(), thrown)
			// The catch body runs UNPROTECTED: a throw inside it is not
			// caught by this try, but the deferred finally still runs.
			return e.Eval(c.Body, cs)
		}
	}
	return nil, thrown // no catch matched: propagate (finally still runs)
}

// evalProtected evaluates the try body, turning a panic (a throw, or any
// builtin panicking) into a returned thrown error and passing a returned
// error (e.g. a failed dynamic binding) through unchanged.
func (e *Evaluator) evalProtected(body *ast.Node, s *Scope) (val any, thrown error) {
	defer func() {
		if r := recover(); r != nil {
			thrown = Recover(r)
		}
	}()
	v, err := e.Eval(body, s)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// exMessager / exCauser are the accessors ex-message / ex-cause reach for
// on a caught exception. *lang.ExceptionInfo implements both.
type exMessager interface{ Message() string }
type exCauser interface{ Cause() error }

// registerExceptionBuiltins interns ex-info / ex-data / ex-message /
// ex-cause into clojure.core (real clojure.core fns — precedence-safe
// additions per CLAUDE.md, never renames). Called by internBuiltins via a
// single added line so it lands in BOTH modes (rt.Boot interns the same set
// into an emitted binary).
func (e *Evaluator) registerExceptionBuiltins(def func(string, func(...any) any) *lang.Var) {
	// ex-info: (ex-info msg map) | (ex-info msg map cause). Builds a
	// lang.ExceptionInfo carrying the message string and data map.
	def("ex-info", func(args ...any) any {
		if len(args) < 2 || len(args) > 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: ex-info", len(args)))
		}
		msg, ok := args[0].(string)
		if !ok && args[0] != nil {
			panic(fmt.Errorf("ex-info expects a string message, got: %s", lang.PrintString(args[0])))
		}
		var data lang.IPersistentMap
		if args[1] != nil {
			data, ok = args[1].(lang.IPersistentMap)
			if !ok {
				panic(fmt.Errorf("ex-info expects a map, got: %s", lang.PrintString(args[1])))
			}
		}
		if len(args) == 3 && args[2] != nil {
			cause, ok := args[2].(error)
			if !ok {
				panic(fmt.Errorf("ex-info cause must be a Throwable, got: %s", lang.PrintString(args[2])))
			}
			return lang.NewExceptionInfoWithCause(msg, data, cause)
		}
		return lang.NewExceptionInfo(msg, data)
	})

	// ex-data: the data map of an ex-info (or ex-info in the cause chain),
	// else nil — exactly clojure.core/ex-data.
	def("ex-data", func(args ...any) any {
		x := oneArg("ex-data", args)
		if err, ok := x.(error); ok {
			if d := lang.GetExData(err); d != nil {
				return d
			}
		}
		return nil
	})

	// ex-message: the message of a Throwable (clojure.core/ex-message).
	def("ex-message", func(args ...any) any {
		x := oneArg("ex-message", args)
		if m, ok := x.(exMessager); ok {
			return m.Message()
		}
		if err, ok := x.(error); ok {
			return err.Error()
		}
		return nil
	})

	// ex-cause: the cause of an ExceptionInfo, else nil
	// (clojure.core/ex-cause).
	def("ex-cause", func(args ...any) any {
		x := oneArg("ex-cause", args)
		if c, ok := x.(exCauser); ok {
			if cause := c.Cause(); cause != nil {
				return cause
			}
		}
		return nil
	})
}
