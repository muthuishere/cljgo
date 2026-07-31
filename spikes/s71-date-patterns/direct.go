package pattern

// direct.go — the design the stress test forced.
//
// THE DEFECT. Compiling a java.time pattern into a Go LAYOUT STRING means
// emitting a string in a second language and letting time.Format re-parse it.
// That re-parse is context-sensitive in a way no translator can see:
//
//	ref.Format("Mon-at") => "Fri-at"      substitutes
//	ref.Format("Monat")  => "Monat"       SILENTLY DOES NOT
//	ref.Format("Jandu")  => "Jandu"       same for the month name
//
// A text token followed immediately by a literal starting with a lowercase
// letter is emitted verbatim. So `EEE'at' Z` — a perfectly ordinary pattern —
// produced "Monat +0000" where the JVM gives "Friat +0000". Not a wrong token:
// a token that vanished because of the LITERAL NEXT TO IT.
//
// Patching adjacency rules into the translator would be the wrong repair. The
// mistake is structural: we already parsed the pattern, and then re-encoded it
// into another string language so that Go could parse it AGAIN. Deleting that
// second encoding deletes the entire bug class — along with the special-cased
// fraction (Go ties the separator to the fraction; we do not have to) and the
// unpadded-24-hour hole (Go has no layout for it; an integer format does).
//
// So Compiled holds a small op list and formats directly. Simpler — no
// adjacency rules, no separator stealing, no second grammar — and, because Go
// re-scans its layout string on every Format call and this does not, faster.

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type opKind uint8

const (
	opLit opKind = iota
	opYear2
	opYear4
	opMonthNum
	opMonthShort
	opMonthLong
	opDay
	opWeekShort
	opWeekLong
	opHour24
	opHour12
	opMinute
	opSecond
	opFraction
	opAmPm
	opOffsetColon // +01:00 / Z
	opOffsetBasic // +0100  / Z
	opOffsetHour  // +01    / Z
	opOffsetRFC   // +0100  (never Z — java.time's Z token)
)

type op struct {
	kind opKind
	pad  int    // digits for numeric fields (1 = unpadded)
	lit  string // opLit only
}

// Direct is a compiled pattern that formats without a Go layout string.
type Direct struct {
	Pattern string
	ops     []op
}

// pad appends v zero-padded to width. Append-based, so it allocates nothing:
// strconv.Itoa would allocate a string per numeric field, which is 5 garbage
// strings for "yyyy-MM-dd HH:mm:ss".
func pad(b []byte, v, width int) []byte {
	var tmp [20]byte
	d := strconv.AppendInt(tmp[:0], int64(v), 10)
	for i := len(d); i < width; i++ {
		b = append(b, '0')
	}
	return append(b, d...)
}

func appendOffset(b []byte, t time.Time, k opKind) []byte {
	_, off := t.Zone()
	if off == 0 && k != opOffsetRFC {
		return append(b, 'Z') // java.time: X/XX/XXX render a zero offset as Z
	}
	sign := byte('+')
	if off < 0 {
		sign, off = '-', -off
	}
	h, m := off/3600, (off%3600)/60
	b = pad(append(b, sign), h, 2)
	switch k {
	case opOffsetHour:
		if m == 0 {
			return b
		}
		return pad(b, m, 2)
	case opOffsetColon:
		return pad(append(b, ':'), m, 2)
	}
	return pad(b, m, 2)
}

// Format renders t. Every op is independent, so no op can be swallowed by the
// text of its neighbour — the property the layout-string design could not
// offer.
func (d Direct) Format(t time.Time) string {
	// Stack-backed for the common case; Go's escape analysis keeps the array
	// off the heap, so the only allocation left is the returned string.
	var stack [64]byte
	buf := stack[:0]
	for _, o := range d.ops {
		switch o.kind {
		case opLit:
			buf = append(buf, o.lit...)
		case opYear2:
			buf = pad(buf, t.Year()%100, 2)
		case opYear4:
			buf = pad(buf, t.Year(), 4)
		case opMonthNum:
			buf = pad(buf, int(t.Month()), o.pad)
		case opMonthShort:
			buf = append(buf, t.Month().String()[:3]...)
		case opMonthLong:
			buf = append(buf, t.Month().String()...)
		case opDay:
			buf = pad(buf, t.Day(), o.pad)
		case opWeekShort:
			buf = append(buf, t.Weekday().String()[:3]...)
		case opWeekLong:
			buf = append(buf, t.Weekday().String()...)
		case opHour24:
			buf = pad(buf, t.Hour(), o.pad)
		case opHour12:
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			buf = pad(buf, h, o.pad)
		case opMinute:
			buf = pad(buf, t.Minute(), o.pad)
		case opSecond:
			buf = pad(buf, t.Second(), o.pad)
		case opFraction:
			var tmp [10]byte
			f := strconv.AppendInt(tmp[:0], int64(t.Nanosecond()+1000000000), 10)[1:]
			buf = append(buf, f[:o.pad]...)
		case opAmPm:
			if t.Hour() < 12 {
				buf = append(buf, "AM"...)
			} else {
				buf = append(buf, "PM"...)
			}
		default:
			buf = appendOffset(buf, t, o.kind)
		}
	}
	return string(buf)
}

// CompileDirect translates a java.time pattern into an op list. It refuses
// exactly the tokens Go's CALENDAR cannot express — which is a strictly
// smaller set than the tokens Go's LAYOUT LANGUAGE cannot express: unpadded
// 24-hour (H) and the bare fraction (SSS) are both fine here, because neither
// was ever a calendar limitation. They were limitations of the string we
// stopped emitting.
func CompileDirect(p string) (Direct, error) {
	runs, err := lex(p)
	if err != nil {
		return Direct{}, err
	}
	d := Direct{Pattern: p}
	for _, r := range runs {
		if r.isLit {
			d.ops = append(d.ops, op{kind: opLit, lit: r.literal})
			continue
		}
		if why, bad := unsupported[r.ch]; bad {
			return Direct{}, tokenErr(r.ch, r.n, p, why)
		}
		o, err := directToken(r.ch, r.n)
		if err != nil {
			return Direct{}, wrapPattern(err, p)
		}
		d.ops = append(d.ops, o)
	}
	return d, nil
}

func directToken(c byte, n int) (op, error) {
	num := func(k opKind, max int) (op, error) {
		if n < 1 || n > max {
			return op{}, badRun(c, n)
		}
		return op{kind: k, pad: n}, nil
	}
	switch c {
	case 'y':
		switch n {
		case 2:
			return op{kind: opYear2}, nil
		case 4:
			return op{kind: opYear4}, nil
		}
		return op{}, badRun(c, n)
	case 'M':
		switch n {
		case 1, 2:
			return op{kind: opMonthNum, pad: n}, nil
		case 3:
			return op{kind: opMonthShort}, nil
		case 4:
			return op{kind: opMonthLong}, nil
		}
		return op{}, badRun(c, n)
	case 'd':
		return num(opDay, 2)
	case 'E':
		switch {
		case n <= 3:
			return op{kind: opWeekShort}, nil
		case n == 4:
			return op{kind: opWeekLong}, nil
		}
		return op{}, badRun(c, n)
	case 'H':
		return num(opHour24, 2)
	case 'h':
		return num(opHour12, 2)
	case 'm':
		return num(opMinute, 2)
	case 's':
		return num(opSecond, 2)
	case 'S':
		if n < 1 || n > 9 {
			return op{}, badRun(c, n)
		}
		return op{kind: opFraction, pad: n}, nil
	case 'a':
		if n == 1 {
			return op{kind: opAmPm}, nil
		}
		return op{}, badRun(c, n)
	case 'X':
		switch n {
		case 1:
			return op{kind: opOffsetHour}, nil
		case 2:
			return op{kind: opOffsetBasic}, nil
		case 3:
			return op{kind: opOffsetColon}, nil
		}
		return op{}, badRun(c, n)
	case 'Z':
		if n <= 3 {
			return op{kind: opOffsetRFC}, nil
		}
		return op{}, badRun(c, n)
	}
	return op{}, badRun(c, n)
}

// --- shared diagnostics ----------------------------------------------------

func badRun(c byte, n int) error {
	return fmt.Errorf("token %q is not valid at length %d", string(c), n)
}

func tokenErr(c byte, n int, p, why string) error {
	return fmt.Errorf("unsupported pattern token %q in %q: %s",
		strings.Repeat(string(c), n), p, why)
}

func wrapPattern(err error, p string) error {
	return fmt.Errorf("%w in pattern %q", err, p)
}
