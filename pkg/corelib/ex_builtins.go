package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// exMessager / exCauser are the accessors ex-message / ex-cause reach for
// on a caught exception. *lang.ExceptionInfo implements both.
type exMessager interface{ Message() string }
type exCauser interface{ Cause() error }

// registerExceptionBuiltins interns ex-info / ex-data / ex-message /
// ex-cause into clojure.core (real clojure.core fns — precedence-safe
// additions per CLAUDE.md, never renames). Called by internBuiltins via a
// single added line so it lands in BOTH modes (rt.Boot interns the same set
// into an emitted binary).
func registerExceptionBuiltins(def func(string, func(...any) any) *lang.Var) {
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
