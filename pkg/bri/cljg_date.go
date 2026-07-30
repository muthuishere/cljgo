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
}
