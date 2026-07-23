package reader

// Generalized tagged literals (design/01-reader.md §Phase 2). The
// built-in tags #uuid and #inst are wired here, alongside the existing
// cljgo Result/Option tags (ADR 0014). Unknown tags fall through to a
// *data-readers*-style extension point (lang.VarDataReaders): if that
// var maps the tag symbol to a callable, it is invoked on the read
// form. This keeps the reader dumb about domain types while letting a
// program register its own readers.

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// UUID is the value of a #uuid "..." literal. A POINTER type (not a bare
// struct): java.util.UUID is a real object on the JVM, so two #uuid
// literals with the SAME string are `=` but never `identical?` (oracle
// 1.12.5: (let [u1 #uuid "..." u2 (edn/read-string "#uuid \"...\"")] (and
// (= u1 u2) (not (identical? u1 u2)))) => true). A value-typed UUID{s...}
// would make Go's `==` (identical?'s substrate, goIdentical) compare TWO
// separately-parsed UUIDs equal by field value — wrong. *UUID gives each
// parse its own address, so identical? (pointer equality) is false across
// instances while Equals/Hash below keep `=` and map/set lookup working by
// VALUE, exactly like java.util.UUID.equals/hashCode.
type UUID struct{ s string }

func (u *UUID) String() string { return `#uuid "` + u.s + `"` }

// Value returns the canonical (lowercase) UUID string.
func (u *UUID) Value() string { return u.s }

// Equals implements lang.Equalser: two UUIDs are `=` iff their canonical
// strings match, regardless of pointer identity.
func (u *UUID) Equals(other any) bool {
	o, ok := other.(*UUID)
	return ok && u.s == o.s
}

// Hash implements lang.Hasher, keeping it consistent with Equals (equal
// UUIDs must hash equal for map/set lookup) instead of falling back to
// pkg/lang's default pointer-address hash for unrecognized struct types.
func (u *UUID) Hash() uint32 {
	h := fnv.New32a()
	h.Write([]byte(u.s))
	return h.Sum32()
}

// NewUUID builds a UUID value from a canonical 8-4-4-4-12 string,
// lowercasing it, and reports whether the string was well-formed. It
// lets pkg/eval's parse-uuid / random-uuid produce the SAME value type
// a #uuid literal reads to (Batch 2, numeric tower). Each call mints a
// fresh *UUID (never interned/cached), matching java.util.UUID's object
// identity.
func NewUUID(s string) (*UUID, bool) {
	if !uuidRe.MatchString(s) {
		return nil, false
	}
	return &UUID{s: strings.ToLower(s)}, true
}

// Inst is the value of an #inst "..." literal: the literal timestamp text
// (round-trips verbatim through print/read) plus the parsed instant as
// epoch milliseconds (millis), computed once at read time by NewInst so
// `.getTime` (clojure-test-suite's epoch-millis helper, :default branch)
// has a real value to return (see pkg/eval/host.go's CallGoMethod
// special-case — Inst is a cljgo-owned type standing in for
// java.util.Date, not a general Go interop receiver, so this is a
// narrow, explicit bridge, not the auto-capitalizing method rewrite
// design/05-interop-concurrency.md deliberately rejects for real Go
// interop).
type Inst struct {
	s      string
	millis int64
}

func (i Inst) String() string { return `#inst "` + i.s + `"` }

// Value returns the instant's literal text.
func (i Inst) Value() string { return i.s }

// EpochMillis returns the instant as milliseconds since the Unix epoch
// (java.util.Date#getTime's substrate).
func (i Inst) EpochMillis() int64 { return i.millis }

// uuidRe matches the canonical 8-4-4-4-12 hex UUID shape.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// instRe is clojure.instant's #inst grammar (org.clojure/clojure
// clojure/instant.clj): a year, with month/day/hour:minute:second[.frac]
// and a Z|+-HH:MM offset all optionally nested — oracle-verified against
// clojure.core's (and clojure.edn's) #inst reader, which both throw on
// anything this doesn't match ("Unrecognized date/time syntax") AND on a
// syntactically-matching but calendrically-invalid value (Feb 29 on a
// non-leap year, hour 24, ...) — see NewInst's validation below.
var instRe = regexp.MustCompile(
	`^(\d{4})(?:-(\d\d)(?:-(\d\d)(?:[T](\d\d)(?::(\d\d)(?::(\d\d)(?:[.](\d+))?)?)?(?:(Z)|([-+])(\d\d):(\d\d))?)?)?)?$`)

// NewInst parses and validates a #inst literal's timestamp text, mirroring
// clojure.instant/validated (oracle 1.12.5: (read-string "#inst
// \"2010-02-29T00:00:00.000Z\"") throws "failed: (<= 1 days
// (days-in-month months (leap-year? years)))"; (read-string "#inst
// \"2010-01-01T24:00:00.000Z\"") throws on the hour range). Returns the
// UTC epoch milliseconds a real java.util.Date#getTime would (local wall
// clock minus the UTC offset), so `.getTime` (epoch-millis in the suite)
// has a real value.
func NewInst(s string) (Inst, error) {
	m := instRe.FindStringSubmatch(s)
	if m == nil {
		return Inst{}, fmt.Errorf("Unrecognized date/time syntax: %s", s)
	}
	atoi := func(s string, dflt int) int {
		if s == "" {
			return dflt
		}
		n, _ := strconv.Atoi(s)
		return n
	}
	year := atoi(m[1], 0)
	month := atoi(m[2], 1)
	day := atoi(m[3], 1)
	hour := atoi(m[4], 0)
	minute := atoi(m[5], 0)
	second := atoi(m[6], 0)
	nanos := 0
	if frac := m[7]; frac != "" {
		if len(frac) > 9 {
			frac = frac[:9]
		} else {
			frac += strings.Repeat("0", 9-len(frac))
		}
		nanos, _ = strconv.Atoi(frac)
	}
	offsetSign := 1
	if m[9] == "-" {
		offsetSign = -1
	}
	offsetHours := atoi(m[10], 0)
	offsetMinutes := atoi(m[11], 0)

	invalid := func(field string) (Inst, error) {
		return Inst{}, fmt.Errorf("Invalid #inst %s: %s", field, s)
	}
	if month < 1 || month > 12 {
		return invalid("month")
	}
	if day < 1 || day > daysInMonth(year, month) {
		return invalid("day")
	}
	if hour < 0 || hour > 23 {
		return invalid("hour")
	}
	if minute < 0 || minute > 59 {
		return invalid("minute")
	}
	maxSecond := 59
	if minute == 59 {
		maxSecond = 60 // leap second, per clojure.instant/validated
	}
	if second < 0 || second > maxSecond {
		return invalid("second")
	}
	if offsetHours < 0 || offsetHours > 23 {
		return invalid("offset-hours")
	}
	if offsetMinutes < 0 || offsetMinutes > 59 {
		return invalid("offset-minutes")
	}

	t := time.Date(year, time.Month(month), day, hour, minute, second, nanos, time.UTC)
	offsetSeconds := offsetSign * (offsetHours*3600 + offsetMinutes*60)
	t = t.Add(-time.Duration(offsetSeconds) * time.Second)
	return Inst{s: s, millis: t.UnixMilli()}, nil
}

// leapYear mirrors clojure.instant's leap-year? predicate.
func leapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// daysInMonth mirrors clojure.instant's days-in-month table (1-indexed
// month, Gregorian).
func daysInMonth(year, month int) int {
	dim := [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if month == 2 && leapYear(year) {
		return 29
	}
	if month < 1 || month > 12 {
		return 0
	}
	return dim[month-1]
}

// defaultDataReaderFn returns the fn bound to *default-data-reader-fn*
// (lang.VarDefaultDataReaderFn), when one is bound (batch A2, reading).
func defaultDataReaderFn() (lang.IFn, bool) {
	v := lang.VarDefaultDataReaderFn
	if v == nil || !v.IsBound() {
		return nil, false
	}
	fn, ok := v.Deref().(lang.IFn)
	return fn, ok
}

// dataReaderFor consults lang.*data-readers* for a registered reader
// function bound to tag, returning it (and true) when present.
func dataReaderFor(tag *lang.Symbol) (lang.IFn, bool) {
	v := lang.VarDataReaders
	if v == nil || !v.IsBound() {
		return nil, false
	}
	m, ok := v.Deref().(lang.IPersistentMap)
	if !ok {
		return nil, false
	}
	fn, ok := m.ValAt(tag).(lang.IFn)
	if !ok {
		return nil, false
	}
	return fn, true
}
