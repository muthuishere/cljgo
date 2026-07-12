package reader

import (
	"errors"
	"fmt"
)

// ErrEOF is returned by ReadOne when the input is cleanly exhausted:
// nothing but whitespace/comments remained. A truncated or malformed
// form is never reported as bare ErrEOF; it produces a positioned
// *Error (which may wrap an "EOF while reading ..." message).
// Contract per design/01-reader.md §3/§4.
var ErrEOF = errors.New("EOF")

// ErrIncomplete is wrapped by every positioned *Error caused by the input
// ending in the middle of a form (unterminated collection, string,
// character, dispatch, metadata, wrapped form). REPL frontends test
// errors.Is(err, ErrIncomplete) to read more input instead of reporting a
// syntax error. Distinct from ErrEOF, which means clean exhaustion
// between forms.
var ErrIncomplete = errors.New("EOF while reading")

// Error is a reader error carrying the source position where the
// problem was detected and, for unterminated forms (collections,
// strings), the position where the open form started.
type Error struct {
	Pos   Position
	Start *Position // where the unterminated form began, if applicable
	Err   error
}

func (e *Error) Error() string {
	if e.Start != nil {
		return fmt.Sprintf("%s: %v, starting at line %d column %d",
			e.Pos, e.Err, e.Start.Line, e.Start.Col)
	}
	return fmt.Sprintf("%s: %v", e.Pos, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }
