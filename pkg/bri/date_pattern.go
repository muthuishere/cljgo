// date_pattern.go — ADR 0113: java.time pattern formatting and parsing for
// cljg.date, compiled to an OP LIST and never to a Go layout string.
//
// WHY AN OP LIST. The obvious design — translate a java.time pattern into a Go
// reference-time layout and hand it to time.Format — was built, measured, and
// REJECTED in spike s71. It diverged from the JVM on 108 of 4,000 generated
// patterns after every token bug was fixed, and it stopped there, because the
// remaining divergences are not token bugs:
//
//	ref.Format("Mon-at") => "Fri-at"      substitutes
//	ref.Format("Monat")  => "Monat"       SILENTLY DOES NOT
//	ref.Format("Jandu")  => "Jandu"       same for the month name
//
// Go decides whether to substitute a text token based on THE LITERAL THAT
// FOLLOWS IT. So `EEE'at' Z` gave "Monat +0000" where the JVM gives
// "Friat +0000" — a token that vanished because of its neighbour. No
// token-level fix reaches zero, because the translator emits a string that
// something else re-parses under rules the translator cannot see.
//
// The mistake was structural: the pattern was already parsed, and was then
// re-encoded into a SECOND string language so Go could parse it again.
// Deleting that second encoding deletes the whole bug class — and with it the
// special-cased fraction (Go ties the separator to the fraction; we need not)
// and the unpadded-24-hour hole (Go has no layout for it; an integer format
// does). On the same corpus the op list scores 0 divergences and refuses 15
// patterns where the layout design refused 1,128: the correct design is also
// the far more capable one.
//
// THE DESIGN RULE, from the ADR: a pattern language that errors on 20% of
// inputs is fine; one that silently mis-formats on 2% is not. Every token this
// cannot represent EXACTLY is refused by name, at compile time, with a
// registered code — never approximated, never silently dropped.
//
// LOCALE is ENGLISH, and this is not a detail. Text tokens use Go's month and
// weekday names, which are English-only, so a .cljc author who wants cross-host
// agreement must pass Locale.ENGLISH on the JVM side. NOT Locale.ROOT — ROOT's
// CLDR data has no distinct full forms and collapses MMMM to "Jul" and EEEE to
// "Fri" (measured against java.time, 2026-07-31). The ADR's first draft said
// ROOT, which would have caused a silent divergence on exactly the tokens the
// advice existed to protect.
package bri

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// --- the op list -------------------------------------------------------------

type dpKind uint8

const (
	dpLit dpKind = iota
	dpYear2
	dpYear4
	dpMonthNum
	dpMonthShort
	dpMonthLong
	dpDay
	dpWeekShort
	dpWeekLong
	dpHour24
	dpHour12
	dpMinute
	dpSecond
	dpFraction
	dpAmPm
	dpOffsetColon // +01:00 / Z
	dpOffsetBasic // +0100  / Z
	dpOffsetHour  // +01    / Z
	dpOffsetRFC   // +0100  (never Z — java.time's Z token)
)

type dpOp struct {
	kind dpKind
	pad  int    // digits for numeric fields (1 = unpadded)
	lit  string // dpLit only
}

// datePattern is a compiled pattern: a small slice of independent ops. Because
// each op is formatted on its own, no op can be swallowed by the text of its
// neighbour — the property the layout-string design could not offer.
type datePattern struct {
	pattern string
	ops     []dpOp
}

// --- formatting ---------------------------------------------------------------

// dpPad appends v zero-padded to width. Append-based, so it allocates nothing:
// strconv.Itoa would allocate a string per numeric field, which is five garbage
// strings for "yyyy-MM-dd HH:mm:ss".
func dpPad(b []byte, v, width int) []byte {
	var tmp [20]byte
	d := strconv.AppendInt(tmp[:0], int64(v), 10)
	for i := len(d); i < width; i++ {
		b = append(b, '0')
	}
	return append(b, d...)
}

func dpAppendOffset(b []byte, t time.Time, k dpKind) []byte {
	_, off := t.Zone()
	if off == 0 && k != dpOffsetRFC {
		return append(b, 'Z') // java.time: X/XX/XXX render a zero offset as Z
	}
	sign := byte('+')
	if off < 0 {
		sign, off = '-', -off
	}
	h, m := off/3600, (off%3600)/60
	b = dpPad(append(b, sign), h, 2)
	switch k {
	case dpOffsetHour:
		if m == 0 {
			return b
		}
		return dpPad(b, m, 2)
	case dpOffsetColon:
		return dpPad(append(b, ':'), m, 2)
	}
	return dpPad(b, m, 2)
}

// format renders t. One allocation: the returned string. The scratch buffer is
// stack-backed and escape analysis keeps it there for the common case.
func (d *datePattern) format(t time.Time) string {
	var stack [64]byte
	buf := stack[:0]
	for _, o := range d.ops {
		switch o.kind {
		case dpLit:
			buf = append(buf, o.lit...)
		case dpYear2:
			buf = dpPad(buf, t.Year()%100, 2)
		case dpYear4:
			buf = dpPad(buf, t.Year(), 4)
		case dpMonthNum:
			buf = dpPad(buf, int(t.Month()), o.pad)
		case dpMonthShort:
			buf = append(buf, t.Month().String()[:3]...)
		case dpMonthLong:
			buf = append(buf, t.Month().String()...)
		case dpDay:
			buf = dpPad(buf, t.Day(), o.pad)
		case dpWeekShort:
			buf = append(buf, t.Weekday().String()[:3]...)
		case dpWeekLong:
			buf = append(buf, t.Weekday().String()...)
		case dpHour24:
			buf = dpPad(buf, t.Hour(), o.pad)
		case dpHour12:
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			buf = dpPad(buf, h, o.pad)
		case dpMinute:
			buf = dpPad(buf, t.Minute(), o.pad)
		case dpSecond:
			buf = dpPad(buf, t.Second(), o.pad)
		case dpFraction:
			var tmp [10]byte
			f := strconv.AppendInt(tmp[:0], int64(t.Nanosecond()+1000000000), 10)[1:]
			buf = append(buf, f[:o.pad]...)
		case dpAmPm:
			if t.Hour() < 12 {
				buf = append(buf, "AM"...)
			} else {
				buf = append(buf, "PM"...)
			}
		default:
			buf = dpAppendOffset(buf, t, o.kind)
		}
	}
	return string(buf)
}

// --- lexing -------------------------------------------------------------------

// dpRun is one (char, count) run, or a literal. java.time's single-quote
// escaping is honoured: ” is a literal quote, '…' is a literal run.
type dpRun struct {
	ch      byte
	n       int
	literal string
	isLit   bool
}

func dpLex(p string) ([]dpRun, error) {
	var out []dpRun
	for i := 0; i < len(p); {
		c := p[i]
		if c == '\'' {
			if i+1 < len(p) && p[i+1] == '\'' {
				out = append(out, dpRun{literal: "'", isLit: true})
				i += 2
				continue
			}
			j := strings.IndexByte(p[i+1:], '\'')
			if j < 0 {
				return nil, dpErrf(p, "unterminated quoted literal")
			}
			out = append(out, dpRun{literal: p[i+1 : i+1+j], isLit: true})
			i += j + 2
			continue
		}
		if !dpIsPatternChar(c) {
			out = append(out, dpRun{literal: string(c), isLit: true})
			i++
			continue
		}
		n := 1
		for i+n < len(p) && p[i+n] == c {
			n++
		}
		out = append(out, dpRun{ch: c, n: n})
		i += n
	}
	return out, nil
}

func dpIsPatternChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// dpUnsupported is a token cljgo refuses rather than approximates, mapped to
// the reason a user needs to hear. These are limits of Go's CALENDAR, which is
// a strictly smaller set than the limits of Go's layout language — H and a bare
// SSS were only ever limitations of the string this no longer emits.
var dpUnsupported = map[byte]string{
	'G': "era (G) has no cljgo equivalent",
	'Q': "quarter-of-year (Q) has no cljgo equivalent",
	'D': "day-of-year (D) has no cljgo equivalent",
	'w': "week-of-year (w) has no cljgo equivalent",
	'W': "week-of-month (W) has no cljgo equivalent",
	'F': "day-of-week-in-month (F) has no cljgo equivalent",
	'k': "clock-hour-of-day 1-24 (k) has no cljgo equivalent",
	'K': "hour-of-am-pm 0-11 (K) has no cljgo equivalent",
	'Y': "week-based-year (Y) is not the calendar year; use yyyy",
	'u': "uuuu (proleptic year) differs from yyyy only before year 1; use yyyy",
	'z': "zone NAME (z) does not round-trip; use XXX or Z for the offset",
	'V': "zone ID (V) has no cljgo equivalent",
}

// --- compiling ----------------------------------------------------------------

func dpCompile(p string) (*datePattern, error) {
	runs, err := dpLex(p)
	if err != nil {
		return nil, err
	}
	d := &datePattern{pattern: p, ops: make([]dpOp, 0, len(runs))}
	for _, r := range runs {
		if r.isLit {
			d.ops = append(d.ops, dpOp{kind: dpLit, lit: r.literal})
			continue
		}
		if why, bad := dpUnsupported[r.ch]; bad {
			return nil, dpErrf(p, "unsupported pattern token %q: %s",
				strings.Repeat(string(r.ch), r.n), why)
		}
		o, err := dpToken(p, r.ch, r.n)
		if err != nil {
			return nil, err
		}
		d.ops = append(d.ops, o)
	}
	return d, nil
}

func dpToken(p string, c byte, n int) (dpOp, error) {
	num := func(k dpKind, max int) (dpOp, error) {
		if n < 1 || n > max {
			return dpOp{}, dpBadRun(p, c, n)
		}
		return dpOp{kind: k, pad: n}, nil
	}
	switch c {
	case 'y':
		switch n {
		case 2:
			return dpOp{kind: dpYear2}, nil
		case 4:
			return dpOp{kind: dpYear4}, nil
		}
		return dpOp{}, dpBadRun(p, c, n)
	case 'M':
		switch n {
		case 1, 2:
			return dpOp{kind: dpMonthNum, pad: n}, nil
		case 3:
			return dpOp{kind: dpMonthShort}, nil
		case 4:
			return dpOp{kind: dpMonthLong}, nil
		}
		return dpOp{}, dpBadRun(p, c, n)
	case 'd':
		return num(dpDay, 2)
	case 'E':
		switch {
		case n <= 3:
			return dpOp{kind: dpWeekShort}, nil
		case n == 4:
			return dpOp{kind: dpWeekLong}, nil
		}
		return dpOp{}, dpBadRun(p, c, n)
	case 'H':
		return num(dpHour24, 2)
	case 'h':
		return num(dpHour12, 2)
	case 'm':
		return num(dpMinute, 2)
	case 's':
		return num(dpSecond, 2)
	case 'S':
		if n < 1 || n > 9 {
			return dpOp{}, dpBadRun(p, c, n)
		}
		return dpOp{kind: dpFraction, pad: n}, nil
	case 'a':
		if n == 1 {
			return dpOp{kind: dpAmPm}, nil
		}
		return dpOp{}, dpBadRun(p, c, n)
	case 'X':
		switch n {
		case 1:
			return dpOp{kind: dpOffsetHour}, nil
		case 2:
			return dpOp{kind: dpOffsetBasic}, nil
		case 3:
			return dpOp{kind: dpOffsetColon}, nil
		}
		return dpOp{}, dpBadRun(p, c, n)
	case 'Z':
		if n <= 3 {
			return dpOp{kind: dpOffsetRFC}, nil
		}
		return dpOp{}, dpBadRun(p, c, n)
	}
	return dpOp{}, dpBadRun(p, c, n)
}

// --- the memo table -----------------------------------------------------------
//
// Compiling is ~341 ns and ~1,416 B; doing it per call is 6x slower with 60x
// the garbage, and at bri's measured throughput allocation is the term that
// matters. One memo table consulted by one function fixes it and lands at 1.08x
// of the theoretical floor. Failures are memoised too — a bad pattern in a hot
// loop must not re-lex on every call.
//
// This is deliberately NOT compile-time folding of literal patterns: that buys
// a further 7.6% at identical allocation and costs a second path through the
// analyzer, which "simplicity first, then performance" refuses. A memo table is
// not a layer (ADR 0113 decision 3).

type dpEntry struct {
	d   *datePattern
	err error
}

var dpMemo sync.Map // pattern string -> dpEntry

func dpLookup(p string) (*datePattern, error) {
	if v, ok := dpMemo.Load(p); ok {
		e := v.(dpEntry)
		return e.d, e.err
	}
	d, err := dpCompile(p)
	dpMemo.Store(p, dpEntry{d: d, err: err})
	return d, err
}

// --- parsing ------------------------------------------------------------------
//
// Parsing walks the SAME op list, which is why parse and format cannot drift
// apart: there is one description of the pattern, not two.
//
// A pattern that cannot name a whole instant is REFUSED rather than defaulted.
// Silently filling a missing year with 1970, or a missing meridiem with AM, is
// exactly the "silently wrong on 2%" failure this design exists to eliminate.

type dpFields struct {
	year, month, day     int
	hour, minute, second int
	nanos                int
	offset               int
	havePM, pm           bool
	haveOffset, have12   bool
	weekday              time.Weekday
	haveWeekday          bool
}

// dpParseable reports why a compiled pattern cannot be used for parsing, or ""
// when it can. Checked at compile time, before any input is looked at.
func dpParseable(d *datePattern) string {
	var y, mo, dd, h12, meridiem bool
	for _, o := range d.ops {
		switch o.kind {
		case dpYear2, dpYear4:
			y = true
		case dpMonthNum, dpMonthShort, dpMonthLong:
			mo = true
		case dpDay:
			dd = true
		case dpHour12:
			h12 = true
		case dpAmPm:
			meridiem = true
		}
	}
	switch {
	case !y:
		return "it names no year (add yyyy)"
	case !mo:
		return "it names no month (add MM)"
	case !dd:
		return "it names no day (add dd)"
	case h12 && !meridiem:
		return "it uses the 12-hour clock (h) without AM/PM (add a)"
	}
	return ""
}

var dpMonths = [...]string{
	"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

var dpWeekdays = [...]string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
}

// dpDigits consumes between min and max digits from s, returning the value and
// the rest.
func dpDigits(s string, min, max int) (int, string, bool) {
	n := 0
	for n < max && n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	if n < min {
		return 0, s, false
	}
	v, err := strconv.Atoi(s[:n])
	if err != nil {
		return 0, s, false
	}
	return v, s[n:], true
}

// dpParse reads s against the compiled pattern and returns the instant in the
// zone the input named (UTC when it named none).
func (d *datePattern) parse(s string) (time.Time, error) {
	if why := dpParseable(d); why != "" {
		return time.Time{}, dpErrf(d.pattern, "cannot be used for parsing: %s", why)
	}
	f := dpFields{day: 1, month: 1}
	rest := s
	fail := func(what string) (time.Time, error) {
		return time.Time{}, dpParseErrf(s, d.pattern, "expected %s at %q", what, rest)
	}
	for _, o := range d.ops {
		var ok bool
		switch o.kind {
		case dpLit:
			if !strings.HasPrefix(rest, o.lit) {
				return fail(strconv.Quote(o.lit))
			}
			rest = rest[len(o.lit):]
		case dpYear2:
			var v int
			if v, rest, ok = dpDigits(rest, 2, 2); !ok {
				return fail("a 2-digit year")
			}
			f.year = 2000 + v // java.time yy: base 2000
		case dpYear4:
			var v int
			if v, rest, ok = dpDigits(rest, 4, 4); !ok {
				return fail("a 4-digit year")
			}
			f.year = v
		case dpMonthNum:
			var v int
			if v, rest, ok = dpDigits(rest, o.pad, 2); !ok || v < 1 || v > 12 {
				return fail("a month number 1-12")
			}
			f.month = v
		case dpMonthShort, dpMonthLong:
			var v int
			if v, rest, ok = dpName(rest, dpMonths[:], o.kind == dpMonthShort); !ok {
				return fail("a month name")
			}
			f.month = v + 1
		case dpDay:
			var v int
			if v, rest, ok = dpDigits(rest, o.pad, 2); !ok || v < 1 || v > 31 {
				return fail("a day 1-31")
			}
			f.day = v
		case dpWeekShort, dpWeekLong:
			var v int
			if v, rest, ok = dpName(rest, dpWeekdays[:], o.kind == dpWeekShort); !ok {
				return fail("a weekday name")
			}
			f.weekday, f.haveWeekday = time.Weekday(v), true
		case dpHour24:
			if f.hour, rest, ok = dpDigits(rest, o.pad, 2); !ok || f.hour > 23 {
				return fail("an hour 0-23")
			}
		case dpHour12:
			if f.hour, rest, ok = dpDigits(rest, o.pad, 2); !ok || f.hour < 1 || f.hour > 12 {
				return fail("an hour 1-12")
			}
			f.have12 = true
		case dpMinute:
			if f.minute, rest, ok = dpDigits(rest, o.pad, 2); !ok || f.minute > 59 {
				return fail("a minute 0-59")
			}
		case dpSecond:
			if f.second, rest, ok = dpDigits(rest, o.pad, 2); !ok || f.second > 60 {
				return fail("a second 0-60")
			}
			if f.second == 60 {
				f.second = 59 // java.time smart-resolves a leap second
			}
		case dpFraction:
			var v int
			if v, rest, ok = dpDigits(rest, o.pad, o.pad); !ok {
				return fail(fmt.Sprintf("%d fractional digits", o.pad))
			}
			for i := o.pad; i < 9; i++ {
				v *= 10
			}
			f.nanos = v
		case dpAmPm:
			switch {
			case strings.HasPrefix(rest, "AM"):
				f.pm, f.havePM, rest = false, true, rest[2:]
			case strings.HasPrefix(rest, "PM"):
				f.pm, f.havePM, rest = true, true, rest[2:]
			default:
				return fail("AM or PM")
			}
		default:
			if f.offset, rest, ok = dpOffsetOf(rest, o.kind); !ok {
				return fail("a zone offset")
			}
			f.haveOffset = true
		}
	}
	if rest != "" {
		return time.Time{}, dpParseErrf(s, d.pattern, "trailing input %q", rest)
	}
	if f.have12 && f.havePM {
		f.hour %= 12
		if f.pm {
			f.hour += 12
		}
	}
	zone := time.UTC
	if f.haveOffset && f.offset != 0 {
		zone = time.FixedZone("", f.offset)
	}
	t := time.Date(f.year, time.Month(f.month), f.day, f.hour, f.minute, f.second, f.nanos, zone)
	// java.time cross-checks a parsed weekday against the parsed date and
	// throws on a conflict. Matching that keeps a wrong string from becoming a
	// plausible instant.
	if f.haveWeekday && t.Weekday() != f.weekday {
		return time.Time{}, dpParseErrf(s, d.pattern,
			"names %s but that date is a %s", f.weekday, t.Weekday())
	}
	if t.Day() != f.day || int(t.Month()) != f.month {
		return time.Time{}, dpParseErrf(s, d.pattern, "is not a real calendar date")
	}
	return t, nil
}

// dpName matches one of names (short = its first three letters) at the head of
// s, returning the index.
func dpName(s string, names []string, short bool) (int, string, bool) {
	for i, n := range names {
		want := n
		if short {
			want = n[:3]
		}
		if strings.HasPrefix(s, want) {
			return i, s[len(want):], true
		}
	}
	return 0, s, false
}

func dpOffsetOf(s string, k dpKind) (int, string, bool) {
	if k != dpOffsetRFC && strings.HasPrefix(s, "Z") {
		return 0, s[1:], true
	}
	if s == "" || (s[0] != '+' && s[0] != '-') {
		return 0, s, false
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	rest := s[1:]
	h, rest, ok := dpDigits(rest, 2, 2)
	if !ok {
		return 0, s, false
	}
	m := 0
	switch k {
	case dpOffsetColon:
		if !strings.HasPrefix(rest, ":") {
			return 0, s, false
		}
		if m, rest, ok = dpDigits(rest[1:], 2, 2); !ok {
			return 0, s, false
		}
	case dpOffsetHour:
		// +01 or +0130 — the minutes are optional in this form.
		if v, r, got := dpDigits(rest, 2, 2); got {
			m, rest = v, r
		}
	default:
		if m, rest, ok = dpDigits(rest, 2, 2); !ok {
			return 0, s, false
		}
	}
	return sign * (h*3600 + m*60), rest, true
}

// --- diagnostics --------------------------------------------------------------
//
// Every refusal carries G5022 and NAMES THE TOKEN. A pattern is refused at
// compile time, never mid-format, so a program cannot half-render a timestamp.

func dpErrf(pattern, format string, args ...any) error {
	return &lang.CodedError{
		Code: "G5022",
		Msg: fmt.Sprintf("cljg.date: pattern %q %s (java.time pattern; supported tokens: "+
			"y M d E H h m s S a X Z)", pattern, fmt.Sprintf(format, args...)),
	}
}

func dpBadRun(pattern string, c byte, n int) error {
	return dpErrf(pattern, "has token %q repeated %d times, which java.time does not accept",
		string(c), n)
}

func dpParseErrf(input, pattern, format string, args ...any) error {
	return &lang.CodedError{
		Code: "G5022",
		Msg: fmt.Sprintf("cljg.date/parse-pattern: %q does not match pattern %q — %s",
			input, pattern, fmt.Sprintf(format, args...)),
	}
}
