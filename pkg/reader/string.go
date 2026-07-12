package reader

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// String and character literal readers. These port
// clojure.lang.LispReader.StringReader / CharacterReader verbatim —
// deliberately NOT strconv.Unquote (Go accepts \a \v \x and different
// octal ranges; Clojure does not — design/01-reader.md §4).

func digitVal(c rune, base int) int {
	var d int
	switch {
	case c >= '0' && c <= '9':
		d = int(c - '0')
	case c >= 'a' && c <= 'z':
		d = int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		d = int(c-'A') + 10
	default:
		return -1
	}
	if d >= base {
		return -1
	}
	return d
}

// readUnicodeChar ports LispReader.readUnicodeChar(PushbackReader,...):
// initch is the first digit (already read); length is the total digit
// count including initch. In exact mode, fewer than length digits is
// an error; otherwise the first non-digit is unread and the value so
// far is returned.
func (r *Reader) readUnicodeChar(pos Position, initch rune, base, length int, exact bool) (rune, error) {
	uc := digitVal(initch, base)
	if uc == -1 {
		return 0, r.errAt(pos, "Invalid digit: %c", initch)
	}
	i := 1
	for ; i < length; i++ {
		c, err := r.s.Read()
		if err != nil {
			break
		}
		if isWhitespace(c) || isMacro(c) {
			r.s.Unread()
			break
		}
		d := digitVal(c, base)
		if d == -1 {
			return 0, r.errAt(pos, "Invalid digit: %c", c)
		}
		uc = uc*base + d
	}
	if i != length && exact {
		return 0, r.errAt(pos, "Invalid character length: %d, should be: %d", i, length)
	}
	return rune(uc), nil
}

// readString reads a string literal; the opening '"' has been
// consumed and start is its position.
func (r *Reader) readString(start Position) (any, error) {
	var b strings.Builder
	for {
		c, err := r.s.Read()
		if err != nil {
			return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w string", ErrIncomplete)}
		}
		if c == '"' {
			return b.String(), nil
		}
		if c != '\\' {
			b.WriteRune(c)
			continue
		}
		escPos := r.s.Pos() // position of the escape selector rune
		c, err = r.s.Read()
		if err != nil {
			return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w string", ErrIncomplete)}
		}
		switch c {
		case 't':
			b.WriteRune('\t')
		case 'r':
			b.WriteRune('\r')
		case 'n':
			b.WriteRune('\n')
		case '\\':
			b.WriteRune('\\')
		case '"':
			b.WriteRune('"')
		case 'b':
			b.WriteRune('\b')
		case 'f':
			b.WriteRune('\f')
		case 'u':
			c, err = r.s.Read()
			if err != nil {
				return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w string", ErrIncomplete)}
			}
			if digitVal(c, 16) == -1 {
				return nil, r.errAt(escPos, "Invalid unicode escape: \\u%c", c)
			}
			ch, err := r.readUnicodeChar(escPos, c, 16, 4, true)
			if err != nil {
				return nil, err
			}
			// NB: like Java, lone surrogates are not rejected in
			// strings; Go's WriteRune encodes them as U+FFFD.
			b.WriteRune(ch)
		default:
			if c >= '0' && c <= '9' {
				// Octal escape: bare digits, NO 'o' prefix (that is
				// char-literal syntax only). CLI checks:
				// (read-string "\"\\101\"") => "A";
				// (read-string "\"\\o101\"") => "Unsupported escape
				// character: \o"; (read-string "\"\\8\"") =>
				// "Invalid digit: 8".
				ch, err := r.readUnicodeChar(escPos, c, 8, 3, false)
				if err != nil {
					return nil, err
				}
				if ch > 0377 {
					// CLI check: (read-string "\"\\400\"") errors.
					return nil, r.errAt(escPos, "Octal escape sequence must be in range [0, 377]")
				}
				b.WriteRune(ch)
			} else {
				return nil, r.errAt(escPos, "Unsupported escape character: \\%c", c)
			}
		}
	}
}

// readChar reads a character literal; the leading '\' has been
// consumed and start is its position. Ports LispReader.CharacterReader.
func (r *Reader) readChar(start Position) (any, error) {
	c, err := r.s.Read()
	if err != nil {
		return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w character", ErrIncomplete)}
	}
	token := r.readToken(c)
	if utf8.RuneCountInString(token) == 1 {
		// Single rune: the character itself. Covers \a, \(, \\, and
		// bare \u / \o (CLI check: (read-string "\\u") => \u).
		rn, _ := utf8.DecodeRuneInString(token)
		return lang.NewChar(rn), nil
	}
	switch token {
	case "newline":
		return lang.NewChar('\n'), nil
	case "space":
		return lang.NewChar(' '), nil
	case "tab":
		return lang.NewChar('\t'), nil
	case "backspace":
		return lang.NewChar('\b'), nil
	case "formfeed":
		return lang.NewChar('\f'), nil
	case "return":
		return lang.NewChar('\r'), nil
	}
	if strings.HasPrefix(token, "u") {
		// Exactly 4 hex digits: \uXXXX.
		rest := token[1:]
		if utf8.RuneCountInString(rest) != 4 {
			return nil, r.errAt(start, "Invalid unicode character: \\%s", token)
		}
		uc := 0
		for _, dc := range rest {
			d := digitVal(dc, 16)
			if d == -1 {
				return nil, r.errAt(start, "Invalid digit: %c", dc)
			}
			uc = uc*16 + d
		}
		if uc >= 0xD800 && uc <= 0xDFFF {
			// Surrogates are invalid as char literals (unlike inside
			// strings). CLI check: (read-string "\\uD800") =>
			// "Invalid character constant: \ud800".
			return nil, r.errAt(start, "Invalid character constant: \\u%x", uc)
		}
		return lang.NewChar(rune(uc)), nil
	}
	if strings.HasPrefix(token, "o") {
		rest := token[1:]
		if len(rest) > 3 {
			return nil, r.errAt(start, "Invalid octal escape sequence length: %d", len(rest))
		}
		uc := 0
		for _, dc := range rest {
			d := digitVal(dc, 8)
			if d == -1 {
				return nil, r.errAt(start, "Invalid digit: %c", dc)
			}
			uc = uc*8 + d
		}
		if uc > 0377 {
			// CLI check: (read-string "\\o400") => "Octal escape
			// sequence must be in range [0, 377]."
			return nil, r.errAt(start, "Octal escape sequence must be in range [0, 377]")
		}
		return lang.NewChar(rune(uc)), nil
	}
	return nil, r.errAt(start, "Unsupported character: \\%s", token)
}
