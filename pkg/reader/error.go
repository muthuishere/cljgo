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
