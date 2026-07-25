// os_cron_test.go — white-box tests for cljg.os's cron next-fire math (ADR
// 0088). All times are constructed in time.Local (as cronNext uses), so the
// assertions are timezone-independent.
package bri

import (
	"testing"
	"time"
)

func TestCronNext(t *testing.T) {
	at := func(y int, mo time.Month, d, h, mi int) int64 {
		return time.Date(y, mo, d, h, mi, 0, 0, time.Local).UnixMilli()
	}
	// 2026-01-01 is a Thursday; Jan 2 Fri, Jan 4 Sun, Jan 5 Mon.
	cases := []struct {
		expr       string
		from, want int64
	}{
		{"* * * * *", at(2026, 1, 1, 10, 0), at(2026, 1, 1, 10, 1)},    // strictly next minute
		{"*/5 * * * *", at(2026, 1, 1, 10, 2), at(2026, 1, 1, 10, 5)},  // step
		{"0 * * * *", at(2026, 1, 1, 10, 30), at(2026, 1, 1, 11, 0)},   // top of next hour
		{"0 9 * * *", at(2026, 1, 1, 10, 0), at(2026, 1, 2, 9, 0)},     // daily 09:00 → tomorrow
		{"0 0 15 * *", at(2026, 1, 1, 0, 0), at(2026, 1, 15, 0, 0)},    // day-of-month
		{"0 0 * * 1", at(2026, 1, 1, 0, 0), at(2026, 1, 5, 0, 0)},      // next Monday
		{"0 0 * * 7", at(2026, 1, 1, 0, 0), at(2026, 1, 4, 0, 0)},      // 7 == Sunday
		{"0 0 1 6 *", at(2026, 1, 1, 0, 0), at(2026, 6, 1, 0, 0)},      // month only June
		{"0 0 13 * 5", at(2026, 1, 1, 0, 0), at(2026, 1, 2, 0, 0)},     // dom OR dow: Friday Jan 2 beats the 13th
		{"30 8-9 * * *", at(2026, 1, 1, 9, 40), at(2026, 1, 2, 8, 30)}, // hour range
	}
	for _, c := range cases {
		got, err := cronNext(c.expr, c.from)
		if err != nil {
			t.Errorf("cronNext(%q): %v", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("cronNext(%q, %s) = %s, want %s", c.expr,
				time.UnixMilli(c.from), time.UnixMilli(got), time.UnixMilli(c.want))
		}
	}

	// malformed / never-firing expressions error rather than loop
	for _, bad := range []string{"nope", "* * * *", "* * * * * *", "60 * * * *", "*/0 * * * *"} {
		if _, err := cronNext(bad, 0); err == nil {
			t.Errorf("cronNext(%q) should error", bad)
		}
	}
	if _, err := cronNext("0 0 30 2 *", at(2026, 1, 1, 0, 0)); err == nil {
		t.Error("Feb 30 never fires — cronNext should error, not loop forever")
	}
}
