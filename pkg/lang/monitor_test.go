package lang

import (
	"sync"
	"testing"
)

// fnFunc adapts a Go func to IFn for the tests below.
type monitorTestFn struct{ f func() any }

func (m monitorTestFn) Invoke(args ...any) any { return m.f() }
func (m monitorTestFn) ApplyTo(args ISeq) any  { return m.f() }

func TestWithLockReturnsBodyValue(t *testing.T) {
	obj := NewSymbol("lock-me")
	got := WithLock(obj, monitorTestFn{func() any { return int64(42) }})
	if got != int64(42) {
		t.Fatalf("WithLock value = %v, want 42", got)
	}
}

func TestWithLockReentrant(t *testing.T) {
	obj := NewSymbol("reentrant")
	done := make(chan any, 1)
	go func() {
		done <- WithLock(obj, monitorTestFn{func() any {
			return WithLock(obj, monitorTestFn{func() any { return "inner" }})
		}})
	}()
	if got := <-done; got != "inner" {
		t.Fatalf("nested WithLock = %v, want inner", got)
	}
}

func TestWithLockMutualExclusionAndCleanup(t *testing.T) {
	obj := NewKeyword("counter")
	n := 0
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			WithLock(obj, monitorTestFn{func() any {
				n++ // non-atomic on purpose: the monitor is the only guard
				return nil
			}})
		}()
	}
	wg.Wait()
	if n != 100 {
		t.Fatalf("n = %d, want 100 (mutual exclusion violated)", n)
	}
	monitorsMu.Lock()
	left := len(monitors)
	monitorsMu.Unlock()
	if left != 0 {
		t.Fatalf("monitor table has %d leftover entries, want 0 (refcount leak)", left)
	}
}

func TestWithLockReleasesOnPanic(t *testing.T) {
	obj := NewSymbol("panicky")
	func() {
		defer func() { recover() }()
		WithLock(obj, monitorTestFn{func() any { panic("boom") }})
	}()
	// If the panic leaked the lock, this would deadlock.
	got := WithLock(obj, monitorTestFn{func() any { return "after" }})
	if got != "after" {
		t.Fatalf("WithLock after panic = %v, want after", got)
	}
}

func TestWithLockRejectsNilAndNonComparable(t *testing.T) {
	for name, obj := range map[string]any{"nil": nil, "slice": []int{1}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("WithLock(%s) did not panic", name)
				}
			}()
			WithLock(obj, monitorTestFn{func() any { return nil }})
		}()
	}
}
