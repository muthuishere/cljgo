package reader

import (
	"fmt"
	"io"
)

// Position is a location in a source file. Line and Col are 1-based
// (rune columns, tabs count as one column, like Clojure's
// LineNumberingPushbackReader).
type Position struct {
	File string
	Line int
	Col  int
}

func (p Position) String() string {
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
}

// scanner is a position-tracking rune scanner with one rune of
// pushback (design/01-reader.md §3, modeled on Glojure's
// trackingRuneScanner).
type scanner struct {
	rs io.RuneScanner

	// next is the position of the next rune to be read.
	next Position
	// history holds prior values of next, for one-rune Unread.
	history [2]Position
}

func newScanner(rs io.RuneScanner, file string) *scanner {
	return &scanner{
		rs:   rs,
		next: Position{File: file, Line: 1, Col: 1},
	}
}

func (s *scanner) setFile(file string) {
	s.next.File = file
}

// Read reads the next rune, advancing the tracked position.
// Errors (including io.EOF) are returned unchanged and do not advance
// the position.
func (s *scanner) Read() (rune, error) {
	c, _, err := s.rs.ReadRune()
	if err != nil {
		return 0, err
	}
	s.history[1] = s.history[0]
	s.history[0] = s.next
	if c == '\n' {
		s.next.Line++
		s.next.Col = 1
	} else {
		s.next.Col++
	}
	return c, nil
}

// Unread pushes back the most recently read rune. Only one rune of
// pushback is supported; a second consecutive Unread panics (it is a
// reader bug, never an input error).
func (s *scanner) Unread() {
	if err := s.rs.UnreadRune(); err != nil {
		panic(fmt.Errorf("reader: invalid Unread: %w", err))
	}
	s.next = s.history[0]
	s.history[0] = s.history[1]
}

// Pos returns the position of the next rune to be read.
func (s *scanner) Pos() Position { return s.next }
