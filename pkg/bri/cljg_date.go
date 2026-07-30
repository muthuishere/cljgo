// cljg_date.go — the Go half of cljg.date (ADR 0101): time primitives —
// monotonic nanoseconds (for measuring elapsed durations) and wall-clock
// epoch millis. The monotonic reading PROMOTES the same source the `time`
// macro's private -nano-time uses (pkg/corelib, time.Since over a fixed
// boot instant riding Go's monotonic clock — the System/nanoTime analog),
// exposed publicly here as cljg.date/nano-time; the private core builtin the
// `time` macro depends on is untouched. Thin shims over stdlib time — pure
// Go, non-OptIn (time is stdlib, no dependency to isolate). The ergonomic API
// (since / since-ms) is portable Clojure (core/cljg/date.cljg). Interned as
// :private vars into cljg.date.
//
// cljg.date rides the same name-generic embedded-namespace registry as bri
// and cljg.io / cljg.os / cljg.system (ADR 0087 §1).
package bri

import (
	"fmt"
	"time"
)

// dateBootInstant anchors -nano-time: time.Since on a fixed instant rides
// Go's monotonic clock, so elapsed measurements survive wall-clock
// adjustments — the same guarantee System/nanoTime gives the JVM (and the
// same technique pkg/corelib's private -nano-time uses for the `time` macro).
var dateBootInstant = time.Now()

// installDateShims interns cljg.date's private Go time primitives.
func installDateShims(def func(name string, fn func(args ...any) any)) {
	// -nano-time -> monotonic nanoseconds since process start. Only
	// DIFFERENCES of two readings are meaningful (the epoch is arbitrary).
	def("-nano-time", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -nano-time", len(args)))
		}
		return time.Since(dateBootInstant).Nanoseconds()
	})

	// -now-millis -> wall-clock time as epoch milliseconds.
	def("-now-millis", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -now-millis", len(args)))
		}
		return time.Now().UnixMilli()
	})

	// --- ISO-8601 / RFC 3339 (ADR 0110 ask 4) --------------------------------
	// -format-iso millis -> the instant as an ISO-8601 UTC string, matching
	// java.time.Instant.toString() at millisecond precision: no fraction when
	// the instant is a whole second, exactly three fractional digits otherwise
	// (oracle clojure 1.12.5 / JDK: (str (Instant/ofEpochMilli 0)) =>
	// "1970-01-01T00:00:00Z"; 1500 => "1970-01-01T00:00:01.500Z").
	def("-format-iso", func(args ...any) any {
		ms := asInt64("cljg.date/format-iso", one("-format-iso", args))
		return formatISO(ms)
	})
	// -parse-iso s -> epoch milliseconds. Accepts any RFC 3339 timestamp,
	// with or without a fractional part and with any offset ("Z" or ±hh:mm) —
	// the offset is honoured, not dropped (oracle: (.toEpochMilli (Instant/parse
	// "2026-07-30T12:00:00+05:30")) => 1785393000000).
	def("-parse-iso", func(args ...any) any {
		s := asString(one("-parse-iso", args))
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			// RFC 3339 §5.6 says the 'T' and 'Z' are case-INSENSITIVE, and the
			// JVM agrees: (.toEpochMilli (Instant/parse "2026-07-30T12:00:00z"))
			// => 1785412800000, same as the uppercase form (oracle 1.12.5,
			// 2026-07-30). Go's RFC3339 layout only matches the uppercase
			// spelling, so retry once on the upcased separators.
			t, err = time.Parse(time.RFC3339, upcaseISOSeparators(s))
		}
		if err != nil {
			panic(fmt.Errorf("cljg.date/parse-iso: not an ISO-8601 instant: %q (expected e.g. 2026-07-30T12:00:00Z)", s))
		}
		return t.UnixMilli()
	})
	// -format-layout (millis layout) -> the UTC instant rendered with a GO
	// reference-time layout ("2006-01-02 15:04:05"). This is a Go host, not a
	// JVM one: there is deliberately no java.time DateTimeFormatter pattern
	// translation behind it (see the docstring in core/cljg/date.cljg).
	def("-format-layout", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -format-layout (expects 2: [millis layout])", len(args)))
		}
		ms := asInt64("cljg.date/format", args[0])
		return time.UnixMilli(ms).UTC().Format(asString(args[1]))
	})
	// -parse-layout (s layout) -> epoch milliseconds, parsing s with a GO
	// reference-time layout. A layout with no zone parses as UTC.
	def("-parse-layout", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -parse-layout (expects 2: [s layout])", len(args)))
		}
		s, layout := asString(args[0]), asString(args[1])
		t, err := time.Parse(layout, s)
		if err != nil {
			panic(fmt.Errorf("cljg.date/parse: cannot parse %q with layout %q (Go reference-time layout, e.g. \"2006-01-02 15:04:05\")", s, layout))
		}
		return t.UnixMilli()
	})
}

// formatISO renders epoch millis the way java.time.Instant.toString() does at
// millisecond precision: always UTC, the fractional part omitted on a whole
// second and exactly three digits otherwise.
func formatISO(ms int64) string {
	t := time.UnixMilli(ms).UTC()
	// The zone suffix is appended literally rather than through a layout
	// element: the instant is already UTC, and a bare "Z" in a Go layout is
	// zone SYNTAX (Z07:00), not a character.
	if ms%1000 == 0 {
		return t.Format("2006-01-02T15:04:05") + "Z"
	}
	return t.Format("2006-01-02T15:04:05.000") + "Z"
}

// asInt64 coerces a Clojure integer (a fixnum int64, or an int from an
// arithmetic path) to int64, naming the PUBLIC fn in the failure message.
func asInt64(name string, v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		panic(fmt.Errorf("%s: expected epoch milliseconds (an integer), got: %v", name, v))
	}
}

// upcaseISOSeparators upcases the date/time separator (position 10) and a
// trailing zone designator, so the lowercase spellings RFC 3339 §5.6 permits —
// "2026-07-30t12:00:00z" — parse like the uppercase ones, matching
// java.time.Instant.parse. Nothing else in the string is touched.
func upcaseISOSeparators(s string) string {
	b := []byte(s)
	if len(b) > 10 && b[10] == 't' {
		b[10] = 'T'
	}
	if n := len(b); n > 0 && b[n-1] == 'z' {
		b[n-1] = 'Z'
	}
	return string(b)
}
