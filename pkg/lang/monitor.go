package lang

// monitor.go — per-object reentrant monitors backing clojure.core/locking
// (cljgo original, not vendored from Glojure; fundamentals batch 1). The
// JVM gives `locking` reentrant object monitors for free
// (monitor-enter/monitor-exit); Go has no per-object monitor, so this is
// an explicit table: lock object -> monitor, entries refcounted and
// removed when the last holder/waiter leaves, reentrancy tracked by
// goroutine id (the same goid the dynamic-var frames key on, var.go).
//
// DEVIATION (logged in the locking macro, core/core.clj): the table keys
// by Go map equality, not JVM object identity — two `=`-equal comparable
// values (e.g. equal strings) share one monitor. That over-serializes but
// never under-locks, so mutual exclusion still holds.

import (
	"fmt"
	"reflect"
	"sync"
)

type monitor struct {
	mu    sync.Mutex
	owner int64 // goroutine id of the current holder; 0 = unheld. Guarded by monitorsMu.
	refs  int   // holders + waiters. Guarded by monitorsMu.
}

var (
	monitorsMu sync.Mutex
	monitors   = map[any]*monitor{}
)

// WithLock runs f while holding obj's monitor, backing the `locking`
// macro: acquire, invoke, release on the way out (panic included).
// Reentrant like a JVM monitor — nested locking on the same object from
// the same goroutine just runs the body.
func WithLock(obj any, f IFn) any {
	if IsNil(obj) {
		panic(NewIllegalArgumentError("locking: lock object must not be nil"))
	}
	if !reflect.TypeOf(obj).Comparable() {
		panic(NewIllegalArgumentError(fmt.Sprintf("locking: unsupported lock object type %T", obj)))
	}
	gid := getGoroutineID()

	monitorsMu.Lock()
	m := monitors[obj]
	if m == nil {
		m = &monitor{}
		monitors[obj] = m
	}
	if m.owner == gid {
		// Reentrant hold: the outer locking already owns the monitor.
		monitorsMu.Unlock()
		return f.Invoke()
	}
	m.refs++
	monitorsMu.Unlock()

	m.mu.Lock()
	monitorsMu.Lock()
	m.owner = gid
	monitorsMu.Unlock()

	defer func() {
		monitorsMu.Lock()
		m.owner = 0
		m.refs--
		if m.refs == 0 {
			delete(monitors, obj)
		}
		monitorsMu.Unlock()
		m.mu.Unlock()
	}()
	return f.Invoke()
}
