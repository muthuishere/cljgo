// os_cron.go — the Go half of cljg.os's cron scheduler (ADR 0088). A standard
// 5-field cron parser (min hour day-of-month month day-of-week; *, lists,
// ranges, steps) + a next-fire search over stdlib time — deterministic and
// directly unit-tested. The scheduler LOOP is portable Clojure (core/cljg/
// os.cljg) over these host primitives; time is stdlib so cljg.os is pure-Go and
// non-OptIn. Interned as :private vars into cljg.os.
package bri

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// installOSShims interns cljg.os's private Go primitives.
func installOSShims(def func(name string, fn func(args ...any) any)) {
	def("-now-millis", func(args ...any) any { return time.Now().UnixMilli() })
	def("-sleep-millis", func(args ...any) any {
		ms := int64(asInt(one("-sleep-millis", args)))
		if ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		return nil
	})
	// -cron-next (expr fromMillis) -> the next fire strictly after fromMillis
	// (epoch millis, local time), or a panic on a malformed expression.
	def("-cron-next", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-cron-next expects 2 args (expr from-millis), got %d", len(args)))
		}
		next, err := cronNext(asString(args[0]), int64(asInt(args[1])))
		if err != nil {
			panic(err)
		}
		return next
	})
	installServiceShims(def) // cljg.os also owns service management (os_service.go)
}

// cronField is the set of allowed values for one cron position.
type cronField map[int]bool

func (f cronField) has(v int) bool { return f[v] }

// parseField parses one cron field into the allowed set over [min,max].
// Supports *, a, a-b, a,b, */n, a-b/n, a/n. dowNormalize maps 7 -> 0 (Sunday).
func parseField(spec string, min, max int, dowNormalize bool) (cronField, error) {
	f := cronField{}
	for _, part := range strings.Split(spec, ",") {
		step := 1
		rng := part
		if i := strings.IndexByte(part, '/'); i >= 0 {
			rng = part[:i]
			s, err := strconv.Atoi(part[i+1:])
			if err != nil || s < 1 {
				return nil, fmt.Errorf("cljg.os: bad step in cron field %q", part)
			}
			step = s
		}
		lo, hi := min, max
		if rng != "*" {
			if i := strings.IndexByte(rng, '-'); i >= 0 {
				a, err1 := strconv.Atoi(rng[:i])
				b, err2 := strconv.Atoi(rng[i+1:])
				if err1 != nil || err2 != nil {
					return nil, fmt.Errorf("cljg.os: bad range in cron field %q", part)
				}
				lo, hi = a, b
			} else {
				v, err := strconv.Atoi(rng)
				if err != nil {
					return nil, fmt.Errorf("cljg.os: bad value in cron field %q", part)
				}
				lo, hi = v, v
			}
		}
		for v := lo; v <= hi; v += step {
			nv := v
			if dowNormalize && nv == 7 {
				nv = 0
			}
			if nv < min || nv > max {
				return nil, fmt.Errorf("cljg.os: cron value %d out of range [%d,%d]", nv, min, max)
			}
			f[nv] = true
		}
	}
	if len(f) == 0 {
		return nil, fmt.Errorf("cljg.os: empty cron field %q", spec)
	}
	return f, nil
}

type cronSpec struct {
	min, hour, dom, month, dow cronField
	domStar, dowStar           bool
}

func parseCron(expr string) (*cronSpec, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return nil, fmt.Errorf("cljg.os: cron needs 5 fields (min hour dom month dow), got %d in %q", len(parts), expr)
	}
	var err error
	c := &cronSpec{domStar: parts[2] == "*", dowStar: parts[4] == "*"}
	if c.min, err = parseField(parts[0], 0, 59, false); err != nil {
		return nil, err
	}
	if c.hour, err = parseField(parts[1], 0, 23, false); err != nil {
		return nil, err
	}
	if c.dom, err = parseField(parts[2], 1, 31, false); err != nil {
		return nil, err
	}
	if c.month, err = parseField(parts[3], 1, 12, false); err != nil {
		return nil, err
	}
	if c.dow, err = parseField(parts[4], 0, 6, true); err != nil {
		return nil, err
	}
	return c, nil
}

// matches reports whether time t satisfies the spec. Vixie day rule: when BOTH
// dom and dow are restricted, a day matches if EITHER matches; otherwise both
// (the *-ed one being trivially true) must hold.
func (c *cronSpec) matches(t time.Time) bool {
	if !c.min.has(t.Minute()) || !c.hour.has(t.Hour()) || !c.month.has(int(t.Month())) {
		return false
	}
	domOK := c.dom.has(t.Day())
	dowOK := c.dow.has(int(t.Weekday()))
	if !c.domStar && !c.dowStar {
		return domOK || dowOK
	}
	return domOK && dowOK
}

// cronNext returns the next fire STRICTLY after fromMillis (local time), by
// stepping minute-by-minute (capped at 5 years — a malformed-but-parseable
// expression that never matches, e.g. Feb 30, errors rather than looping).
func cronNext(expr string, fromMillis int64) (int64, error) {
	c, err := parseCron(expr)
	if err != nil {
		return 0, err
	}
	// start at the next whole minute after `from`
	t := time.UnixMilli(fromMillis).Truncate(time.Minute).Add(time.Minute)
	const maxMinutes = 5 * 366 * 24 * 60
	for i := 0; i < maxMinutes; i++ {
		if c.matches(t) {
			return t.UnixMilli(), nil
		}
		t = t.Add(time.Minute)
	}
	return 0, fmt.Errorf("cljg.os: cron %q has no next fire within 5 years", expr)
}
