package format14

import (
	"regexp"
	"strconv"
	"strings"
)

// Directive is one parsed %-conversion from a Java/Clojure format string.
// The parser is shared between the translate-then-delegate and direct
// interpreter candidates — argument-index resolution and flag/width/
// precision extraction is identical either way; only VALUE RENDERING
// differs, which is where the two approaches actually diverge.
type Directive struct {
	Literal string // non-empty for a plain-text run; Conv==0 in that case

	ExplicitIndex int  // 1-based; 0 = not given (use implicit/relative)
	Relative      bool // "%<" — reuse the previous directive's argument

	Flags     string // raw flag chars, e.g. "-+0,("
	HasWidth  bool
	Width     int
	HasPrec   bool
	Precision int
	Conv      byte // 's','d','f','e','g','x','o','c','b','n','%', or upper-case variant
}

func (d Directive) HasFlag(f byte) bool { return strings.IndexByte(d.Flags, f) >= 0 }

// Java's Formatter.fsPattern (java.util.Formatter), minus date/time (%t/%T
// conversions — out of scope, see README "Scope note").
var directiveRe = regexp.MustCompile(`%(\d+\$)?([-#+ 0,(<]*)(\d+)?(\.\d+)?([a-zA-Z%])`)

// ParseFormat splits a Java-grammar format string into literal runs and
// directives, in source order.
func ParseFormat(f string) ([]Directive, error) {
	var out []Directive
	idx := 0
	for idx < len(f) {
		loc := directiveRe.FindStringSubmatchIndex(f[idx:])
		if loc == nil {
			out = append(out, Directive{Literal: f[idx:]})
			break
		}
		if loc[0] > 0 {
			out = append(out, Directive{Literal: f[idx : idx+loc[0]]})
		}
		m := make([]string, 6)
		for i := 0; i < 6; i++ {
			if loc[2*i] < 0 {
				m[i] = ""
			} else {
				m[i] = f[idx+loc[2*i] : idx+loc[2*i+1]]
			}
		}
		d := Directive{}
		if m[1] != "" {
			n, _ := strconv.Atoi(strings.TrimSuffix(m[1], "$"))
			d.ExplicitIndex = n
		}
		d.Flags = m[2]
		if strings.IndexByte(d.Flags, '<') >= 0 {
			d.Relative = true
		}
		if m[3] != "" {
			d.HasWidth = true
			d.Width, _ = strconv.Atoi(m[3])
		}
		if m[4] != "" {
			d.HasPrec = true
			d.Precision, _ = strconv.Atoi(strings.TrimPrefix(m[4], "."))
		}
		d.Conv = m[5][0]
		out = append(out, d)
		idx += loc[1]
	}
	return out, nil
}

// hasUpperConv reports whether Conv is an uppercase letter (Java: uppercase
// the rendered result, e.g. %X, %S, %B — but only for conversions that HAVE
// an uppercase form; %D/%O do not exist and must error, matching real Java).
func hasUpperConv(c byte) bool { return c >= 'A' && c <= 'Z' }

// noUpperForm rejects the conversion letters that have NO uppercase form in
// java.util.Formatter — only b,h,s,c,x,e,g,a,t do; d,o,f do not, so %D/%O/%F
// must throw UnknownFormatConversionException, same as real Java/Clojure.
var noUpperForm = map[byte]bool{'D': true, 'O': true, 'F': true}
