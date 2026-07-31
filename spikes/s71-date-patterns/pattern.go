// Package pattern is the s71 prototype: java.time pattern -> Go layout.
//
// WHY THIS EXISTS. cljg.date/format takes a GO reference-time layout
// ("2006-01-02"). Its own docstring admits there is no JVM equivalent and
// tells you to use format-iso for the portable case. That is a Go
// implementation detail leaking through a cljg.* surface — precisely what
// cljg.* exists to prevent, and it cost a real consumer a real capability:
// koine dropped pattern formatting entirely rather than either leak Go
// layouts into portable .cljc or hand-translate and silently mis-format.
//
// The agreed vocabulary is java.time's pattern language, for one reason that
// is not taste: it puts the translation risk on ONE side. A .cljc library's
// :clj branch is then DateTimeFormatter/ofPattern with no translation at all
// — the JVM IS the oracle for the pattern language — and every Go-hosted
// Clojure translates against that same oracle.
//
// The design rule this prototype exists to price:
//
//	A pattern language that errors on 20% of inputs is fine.
//	One that silently mis-formats on 2% is not.
//
// So Compile REFUSES every token it cannot represent exactly, naming the
// token, at pattern-compile time — never at format time, and never as a
// silent drop.
package pattern

import (
	"fmt"
	"strings"
)

// Compiled is a translated pattern. Layout is the Go reference-time layout;
// it is produced once and reused, which is the whole performance point.
type Compiled struct {
	Pattern string
	Layout  string
}

// unsupported is a token cljgo refuses rather than approximates, mapped to
// the reason a user needs to hear. Go's layout language simply has no
// spelling for these, so any support would be an approximation that reads as
// success.
var unsupported = map[byte]string{
	'G': "era (G) has no Go layout equivalent",
	'Q': "quarter-of-year (Q) has no Go layout equivalent",
	'D': "day-of-year (D) has no Go layout equivalent",
	'w': "week-of-year (w) has no Go layout equivalent",
	'W': "week-of-month (W) has no Go layout equivalent",
	'F': "day-of-week-in-month (F) has no Go layout equivalent",
	'k': "clock-hour-of-day 1-24 (k) has no Go layout equivalent",
	'K': "hour-of-am-pm 0-11 (K) has no Go layout equivalent",
	'Y': "week-based-year (Y) is not the calendar year; use yyyy",
	'u': "uuuu (proleptic year) differs from yyyy only before year 1; use yyyy",
	'z': "zone NAME (z) does not round-trip; use XXX or Z for the offset",
	'V': "zone ID (V) has no Go layout equivalent",
}

// runs splits a pattern into (char, count) runs, honouring java.time's
// single-quote literal escaping: ” is a literal quote, '…' is a literal run.
type run struct {
	ch      byte
	n       int
	literal string
	isLit   bool
}

func lex(p string) ([]run, error) {
	var out []run
	for i := 0; i < len(p); {
		c := p[i]
		if c == '\'' {
			if i+1 < len(p) && p[i+1] == '\'' {
				out = append(out, run{literal: "'", isLit: true})
				i += 2
				continue
			}
			j := strings.IndexByte(p[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("unterminated quoted literal in pattern %q", p)
			}
			out = append(out, run{literal: p[i+1 : i+1+j], isLit: true})
			i += j + 2
			continue
		}
		if !isPatternChar(c) {
			out = append(out, run{literal: string(c), isLit: true})
			i++
			continue
		}
		n := 1
		for i+n < len(p) && p[i+n] == c {
			n++
		}
		out = append(out, run{ch: c, n: n})
		i += n
	}
	return out, nil
}

func isPatternChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// Compile translates a java.time pattern into a Go layout, or returns an
// error naming the exact token it will not approximate.
//
// Supported core: numeric date/time fields, fixed-width literals, and the
// offset forms. Locale text tokens (MMM/MMMM/EEE/EEEE) are supported ONLY as
// Locale.ROOT (English), which is stated rather than discovered: on a JVM
// with a non-English default locale, ofPattern("MMM") yields "juil." where Go
// yields "Jul" and nothing errors. A caller who wants agreement must force
// Locale.ROOT on the JVM side.
func Compile(p string) (Compiled, error) {
	runs, err := lex(p)
	if err != nil {
		return Compiled{}, err
	}
	var b strings.Builder
	for _, r := range runs {
		if r.isLit {
			b.WriteString(r.literal)
			continue
		}
		if why, bad := unsupported[r.ch]; bad {
			return Compiled{}, fmt.Errorf("unsupported pattern token %q in %q: %s",
				strings.Repeat(string(r.ch), r.n), p, why)
		}
		// THE FRACTION IS NOT CONTEXT-FREE, and this is the one case a
		// token-at-a-time translator gets silently wrong. Verified against
		// the JVM oracle (java.time 2026-07-31):
		//
		//	"HH:mm:ss.SSS" => 09:04:05.123     the dot is a LITERAL
		//	"HH:mm:ss,SSS" => 09:04:05,123     so is the comma
		//	"SSS"          => 123              bare digits, no separator
		//
		// Go ties the separator to the fraction — its layout is ".000" or
		// ",000" as ONE token, and it has no spelling at all for a bare
		// fraction. Emitting "000" for S and letting the literal "." through
		// separately yields "..000": a silent mis-format of the single most
		// common pattern anyone writes. So S consumes the separator the
		// literal just emitted, and refuses when there is none.
		if r.ch == 'S' {
			if r.n < 1 || r.n > 9 {
				return Compiled{}, fmt.Errorf("fraction must be 1-9 S's, got %d in pattern %q", r.n, p)
			}
			cur := b.String()
			if !strings.HasSuffix(cur, ".") && !strings.HasSuffix(cur, ",") {
				return Compiled{}, fmt.Errorf(
					"fractional seconds (%s) in %q must follow a literal '.' or ',': "+
						"Go's layout has no bare-fraction form, and java.time's bare SSS "+
						"prints digits with no separator",
					strings.Repeat("S", r.n), p)
			}
			sep := cur[len(cur)-1:]
			b.Reset()
			b.WriteString(cur[:len(cur)-1])
			b.WriteString(sep + strings.Repeat("0", r.n))
			continue
		}
		lay, err := token(r.ch, r.n)
		if err != nil {
			return Compiled{}, fmt.Errorf("%w in pattern %q", err, p)
		}
		b.WriteString(lay)
	}
	return Compiled{Pattern: p, Layout: b.String()}, nil
}

func token(c byte, n int) (string, error) {
	switch c {
	case 'y':
		switch n {
		case 2:
			return "06", nil
		case 4:
			return "2006", nil
		}
		return "", fmt.Errorf("year must be yy or yyyy, got %d y's", n)
	case 'M':
		switch n {
		case 1:
			return "1", nil
		case 2:
			return "01", nil
		case 3:
			return "Jan", nil
		case 4:
			return "January", nil
		}
		return "", fmt.Errorf("month must be M, MM, MMM or MMMM, got %d M's", n)
	case 'd':
		switch n {
		case 1:
			return "2", nil
		case 2:
			return "02", nil
		}
		return "", fmt.Errorf("day must be d or dd, got %d d's", n)
	case 'E':
		if n >= 4 {
			return "Monday", nil
		}
		return "Mon", nil
	case 'H':
		switch n {
		case 1:
			return "15", nil // Go has no single-digit 24h form; 15 is zero-padded
		case 2:
			return "15", nil
		}
		return "", fmt.Errorf("hour must be H or HH, got %d H's", n)
	case 'h':
		switch n {
		case 1:
			return "3", nil
		case 2:
			return "03", nil
		}
		return "", fmt.Errorf("hour must be h or hh, got %d h's", n)
	case 'm':
		switch n {
		case 1:
			return "4", nil
		case 2:
			return "04", nil
		}
		return "", fmt.Errorf("minute must be m or mm, got %d m's", n)
	case 's':
		switch n {
		case 1:
			return "5", nil
		case 2:
			return "05", nil
		}
		return "", fmt.Errorf("second must be s or ss, got %d s's", n)
	case 'a':
		return "PM", nil
	case 'X':
		switch n {
		case 1:
			return "Z07", nil
		case 2:
			return "Z0700", nil
		case 3:
			return "Z07:00", nil
		}
		return "", fmt.Errorf("offset must be X, XX or XXX, got %d X's", n)
	case 'Z':
		return "-0700", nil
	}
	return "", fmt.Errorf("unknown pattern token %q", string(c))
}
