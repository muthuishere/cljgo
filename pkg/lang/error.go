package lang

import (
	"fmt"
	"strconv"
	"strings"
)

type (
	Error struct {
		msg string
	}

	TimeoutError struct {
		msg string
	}

	// IndexOutOfBoundsError is Java's IndexOutOfBoundsException (which on
	// the JVM extends RuntimeException). msg == "" keeps the historical
	// fixed wording; raise sites with a richer message set it.
	IndexOutOfBoundsError struct{ msg string }

	// ClassCastError is Java's ClassCastException (extends
	// RuntimeException): a value used where another type was required —
	// e.g. arithmetic on a non-number, compare on incomparables. Code
	// carries an optional registered diagnostic code from the raise site.
	ClassCastError struct {
		Msg  string
		Code string
	}

	// NullPointerError is Java's NullPointerException (extends
	// RuntimeException): nil where a value was required — e.g. (inc nil).
	NullPointerError struct {
		Msg  string
		Code string
	}

	// NumberFormatError is Java's NumberFormatException, which on the JVM
	// extends IllegalArgumentException — its Is matches both, so a catch
	// of IllegalArgumentException catches it (oracle 1.12.5, ADR 0039
	// addendum).
	NumberFormatError struct{ Msg string }

	// CodedError is a runtime error that carries a registered diagnostic
	// code chosen at the raise site (ADR 0048 batch 1). diag.FromError reads
	// DiagCode() to render the code + explain pointer without prose-matching.
	// The type lives in lang (diag imports lang, never the reverse) so raise
	// sites across lang/corelib attach codes without an import cycle. Msg is
	// returned verbatim by Error(), keeping the user-facing string byte-stable.
	CodedError struct {
		Msg  string
		Code string
	}

	IllegalArgumentError struct {
		msg  string
		code string // optional registered diagnostic code ("" = none)
	}

	// ArityError is a wrong-number-of-args mismatch — Clojure's
	// ArityException (which on the JVM extends IllegalArgumentException).
	// It is the compiled-binary counterpart of pkg/eval's *arityError:
	// carrying Name/Expected on the thrown value lets diag.FromError render
	// the same named, expected/found line the interpreter produces, so
	// compiled == interpreted (ADR 0048). Error() stays byte-stable: with a
	// Name it reads like the interpreter's arity error, and with only the
	// count it keeps the FnFuncN fast-path's historical wording.
	ArityError struct {
		Actual   int    // args actually passed
		Name     string // qualified fn name (user/f) when known, else ""
		Expected string // arity label ("1: [x] or 2: [x y]") when known, else ""
	}

	IllegalStateError struct {
		msg string
	}

	UnsupportedOperationError struct {
		msg string
	}

	ArithmeticError struct {
		msg string
	}

	CompilerError struct {
		file string
		line int
		col  int
		err  error
	}

	// Stacker is an interface for retrieving stack traces.
	Stacker interface {
		Stack() []StackFrame
	}

	// EvalError is a value that represents an evaluation error.
	EvalError struct {
		err   error
		stack []StackFrame
	}

	StackFrame struct {
		FunctionName string
		Filename     string
		Line         int
		Column       int
	}
)

////////////////////////////////////////////////////////////////////////////////

// NewError creates a new error value.
func NewError(msg string) error {
	return &Error{msg: msg}
}

// Error returns the error message.
func (e *Error) Error() string {
	return e.msg
}

////////////////////////////////////////////////////////////////////////////////

// NewTimeoutError creates a new timeout error.
func NewTimeoutError(msg string) error {
	return &TimeoutError{msg: msg}
}

// Error returns the error message.
func (e *TimeoutError) Error() string {
	return e.msg
}

func (e *TimeoutError) Is(other error) bool {
	_, ok := other.(*TimeoutError)
	return ok
}

////////////////////////////////////////////////////////////////////////////////

func NewIndexOutOfBoundsError() error {
	return &IndexOutOfBoundsError{}
}

// NewIndexOutOfBoundsErrorMsg builds an IndexOutOfBoundsError carrying a
// raise-site message (e.g. "index 5 out of bounds"), typed so a
// (catch IndexOutOfBoundsException e) clause matches it.
func NewIndexOutOfBoundsErrorMsg(msg string) error {
	return &IndexOutOfBoundsError{msg: msg}
}

func (e *IndexOutOfBoundsError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return "index out of bounds"
}

func (e *IndexOutOfBoundsError) Is(other error) bool {
	_, ok := other.(*IndexOutOfBoundsError)
	return ok
}

// DiagCode gives every IndexOutOfBoundsError the G5004 code — the type is
// this error by construction (ADR 0048 batch 1).
func (e *IndexOutOfBoundsError) DiagCode() string { return "G5004" }

////////////////////////////////////////////////////////////////////////////////

// NewCodedError builds a CodedError carrying a registered diagnostic code.
func NewCodedError(code, msg string) error {
	return &CodedError{Msg: msg, Code: code}
}

func (e *CodedError) Error() string { return e.Msg }

// DiagCode implements the raise-site code seam diag.FromError reads.
func (e *CodedError) DiagCode() string { return e.Code }

////////////////////////////////////////////////////////////////////////////////

// NewClassCastError builds a ClassCastError; code may be "" when the raise
// site has no registered diagnostic code.
func NewClassCastError(code, msg string) error {
	return &ClassCastError{Msg: msg, Code: code}
}

func (e *ClassCastError) Error() string { return e.Msg }

// DiagCode implements the raise-site code seam diag.FromError reads.
func (e *ClassCastError) DiagCode() string { return e.Code }

func (e *ClassCastError) Is(other error) bool {
	_, ok := other.(*ClassCastError)
	return ok
}

////////////////////////////////////////////////////////////////////////////////

// NewNullPointerError builds a NullPointerError; code may be "".
func NewNullPointerError(code, msg string) error {
	return &NullPointerError{Msg: msg, Code: code}
}

func (e *NullPointerError) Error() string { return e.Msg }

// DiagCode implements the raise-site code seam diag.FromError reads.
func (e *NullPointerError) DiagCode() string { return e.Code }

func (e *NullPointerError) Is(other error) bool {
	_, ok := other.(*NullPointerError)
	return ok
}

////////////////////////////////////////////////////////////////////////////////

func NewNumberFormatError(msg string) error {
	return &NumberFormatError{Msg: msg}
}

func (e *NumberFormatError) Error() string { return e.Msg }

// Is matches both *NumberFormatError and *IllegalArgumentError, mirroring
// the JVM hierarchy (NumberFormatException extends IllegalArgumentException,
// oracle 1.12.5): a catch of IllegalArgumentException catches this.
func (e *NumberFormatError) Is(other error) bool {
	switch other.(type) {
	case *NumberFormatError, *IllegalArgumentError:
		return true
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////

func NewIllegalArgumentError(msg string) error {
	return &IllegalArgumentError{msg: msg}
}

// NewCodedIllegalArgumentError builds an IllegalArgumentError carrying a
// registered diagnostic code (e.g. the ISeq-conversion G5003 site), so the
// error is BOTH typed for catch matching and coded for diag rendering.
func NewCodedIllegalArgumentError(code, msg string) error {
	return &IllegalArgumentError{msg: msg, code: code}
}

func (e *IllegalArgumentError) Error() string {
	return e.msg
}

// DiagCode implements the raise-site code seam diag.FromError reads.
func (e *IllegalArgumentError) DiagCode() string { return e.code }

func (e *IllegalArgumentError) Is(other error) bool {
	_, ok := other.(*IllegalArgumentError)
	return ok
}

////////////////////////////////////////////////////////////////////////////////

// NewArityError builds an ArityError. name and expected may be "" when the
// throw site cannot know them (the FnFuncN fast path), in which case Error()
// falls back to the historical count-only wording for byte-stability.
func NewArityError(actual int, name, expected string) error {
	return &ArityError{Actual: actual, Name: name, Expected: expected}
}

func (e *ArityError) Error() string {
	if e.Name != "" {
		return fmt.Sprintf("wrong number of args (%d) passed to: %s", e.Actual, e.Name)
	}
	return fmt.Sprintf("wrong number of arguments: expected %s, got %d", e.Expected, e.Actual)
}

// Is matches both *ArityError and *IllegalArgumentError, mirroring the JVM
// hierarchy (ArityException extends IllegalArgumentException) so replacing a
// former IllegalArgumentError throw with an ArityError never changes what an
// errors.Is check sees.
func (e *ArityError) Is(other error) bool {
	switch other.(type) {
	case *ArityError, *IllegalArgumentError:
		return true
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////

func NewUnsupportedOperationError(msg string) error {
	return &UnsupportedOperationError{msg: msg}
}

func (e *UnsupportedOperationError) Error() string {
	return e.msg
}

func (e *UnsupportedOperationError) Is(other error) bool {
	_, ok := other.(*UnsupportedOperationError)
	return ok
}

////////////////////////////////////////////////////////////////////////////////

func NewArithmeticError(msg string) error {
	return &ArithmeticError{msg: msg}
}

func (e *ArithmeticError) Error() string {
	return e.msg
}

func (e *ArithmeticError) Is(other error) bool {
	_, ok := other.(*ArithmeticError)
	return ok
}

// DiagCode gives divide-by-zero ArithmeticErrors the G5006 code via the
// raise-site seam diag.FromError reads (ADR 0048 batch 2). Only the two
// divide-by-zero messages ("Divide by zero" from `/`, mod and bigint quot/rem;
// "/ by zero" from fixnum quot/rem — both oracle-frozen in
// conformance/tests/divide-by-zero-messages.clj) map to G5006; other
// ArithmeticErrors ("integer overflow", "Non-terminating decimal expansion",
// …) return "" and fall through to the general G5000 code unchanged. The
// message text is never touched — Error() stays byte-stable.
func (e *ArithmeticError) DiagCode() string {
	if e.msg == "Divide by zero" || e.msg == "/ by zero" {
		return "G5006"
	}
	return ""
}

////////////////////////////////////////////////////////////////////////////////

func NewIllegalStateError(msg string) error {
	return &IllegalStateError{msg: msg}
}

func (e *IllegalStateError) Error() string {
	return e.msg
}

func (e *IllegalStateError) Is(other error) bool {
	_, ok := other.(*IllegalStateError)
	return ok
}

////////////////////////////////////////////////////////////////////////////////

func NewCompilerError(file string, line, col int, err error) error {
	return &CompilerError{
		file: file,
		line: line,
		col:  col,
		err:  err,
	}
}

func (e *CompilerError) Error() string {
	return fmt.Sprintf("compiler error at %s:%d:%d: %v", e.file, e.line, e.col, e.err)
}

////////////////////////////////////////////////////////////////////////////////
// TODO: Revisit

// NewEvalError creates a new error value.
func NewEvalError(frame StackFrame, err error) *EvalError {
	return &EvalError{
		err:   err,
		stack: []StackFrame{frame},
	}
}

// Error returns the error message.
func (e *EvalError) Error() string {
	var builder strings.Builder
	builder.WriteString(e.err.Error())
	builder.WriteString("\nStack trace (most recent call first):\n")
	for _, frame := range e.stack {
		builder.WriteString(frame.FunctionName)
		builder.WriteString("\n\t")
		filename := frame.Filename
		line := strconv.Itoa(frame.Line)
		column := strconv.Itoa(frame.Column)
		if filename == "" {
			filename = "<unknown>"
			line = "?"
			column = "?"
		}
		builder.WriteString(fmt.Sprintf("%s:%s:%s", filename, line, column))
		builder.WriteRune('\n')
	}
	return builder.String()
}

// Stack returns the stack trace.
func (e *EvalError) Stack() []StackFrame {
	return e.stack
}

// AddStack adds a new stack trace entry.
func (e *EvalError) AddStack(frame StackFrame) error {
	e.stack = append(e.stack, frame)
	return e
}

// Unwrap returns the underlying error.
func (e *EvalError) Unwrap() error {
	return e.err
}
