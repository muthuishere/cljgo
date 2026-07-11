package alts

import (
	"testing"
	"testing/synctest"
	"time"
)

// All tests run inside synctest bubbles: virtual time makes the timeout
// tests instant, and synctest's exit check (every goroutine started in the
// bubble must have exited or be durably blocked on nothing) is itself the
// goroutine-leak detector — a leaked goroutine fails/panics the test.

func TestReadReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := make(chan any, 1)
		c <- 42
		v, ch, ok := Alts([]AltOp{{Chan: c}}, AltOpts{})
		if v != 42 || ch != any(c) || !ok {
			t.Fatalf("got (%v, %v, %v), want (42, c, true)", v, ch, ok)
		}
	})
}

func TestMixedReadWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		empty := make(chan any) // never ready for read
		w := make(chan any)     // unbuffered write target
		done := make(chan any, 1)
		go func() { done <- <-w }() // consumer makes the write ready
		synctest.Wait()             // consumer is now durably blocked on w

		v, ch, ok := Alts([]AltOp{
			{Chan: empty},
			{Chan: w, Value: "hello", IsWrite: true},
		}, AltOpts{})
		if v != true || ch != any(w) || !ok {
			t.Fatalf("write op: got (%v, %v, %v), want (true, w, true)", v, ch, ok)
		}
		if got := <-done; got != "hello" {
			t.Fatalf("consumer got %v, want hello", got)
		}
	})
}

// Typed channel (chan int): closed read must normalize the zero value (0)
// to nil per doc 05 — this is exactly the case where recvOK is load-bearing.
func TestClosedReadNilNormalization(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := make(chan int)
		close(c)
		v, ch, ok := Alts([]AltOp{{Chan: c}}, AltOpts{})
		if v != nil || ch != any(c) || ok {
			t.Fatalf("closed read: got (%v, %v, %v), want (nil, c, false)", v, ch, ok)
		}
	})
}

func TestClosedWriteReturnsFalse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := make(chan any)
		close(c)
		v, ch, ok := Alts([]AltOp{{Chan: c, Value: 1, IsWrite: true}}, AltOpts{})
		if v != false || ch != any(c) || ok {
			t.Fatalf("closed write: got (%v, %v, %v), want (false, c, false)", v, ch, ok)
		}
	})
}

// Closed write mixed with a live-but-not-ready read: must still not panic
// and must report the closed port (exercises the recover+probe path).
func TestClosedWriteAmongLiveOps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		closed := make(chan any)
		close(closed)
		idle := make(chan any)
		v, ch, ok := Alts([]AltOp{
			{Chan: idle},
			{Chan: closed, Value: 1, IsWrite: true},
		}, AltOpts{})
		if v != false || ch != any(closed) || ok {
			t.Fatalf("got (%v, %v, %v), want (false, closed, false)", v, ch, ok)
		}
	})
}

func TestDefaultOnlyWhenNothingReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := make(chan any, 1)
		// nothing ready -> default
		v, ch, ok := Alts([]AltOp{{Chan: c}}, AltOpts{HasDefault: true, Default: "dflt"})
		if v != "dflt" || ch != DefaultPort || ok {
			t.Fatalf("empty: got (%v, %v, %v), want (dflt, DefaultPort, false)", v, ch, ok)
		}
		// something ready -> default must NOT be taken
		c <- 7
		v, ch, ok = Alts([]AltOp{{Chan: c}}, AltOpts{HasDefault: true, Default: "dflt"})
		if v != 7 || ch != any(c) || !ok {
			t.Fatalf("ready: got (%v, %v, %v), want (7, c, true)", v, ch, ok)
		}
		// same, with :priority (separate code path)
		c <- 8
		v, ch, ok = Alts([]AltOp{{Chan: c}}, AltOpts{HasDefault: true, Default: "dflt", Priority: true})
		if v != 8 || ch != any(c) || !ok {
			t.Fatalf("priority ready: got (%v, %v, %v), want (8, c, true)", v, ch, ok)
		}
	})
}

func TestTimeoutFiresAtVirtualDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		idle := make(chan any)
		start := time.Now()
		v, ch, ok := Alts([]AltOp{{Chan: idle}}, AltOpts{HasTimeout: true, Timeout: 250 * time.Millisecond})
		if v != nil || ch != TimeoutPort || ok {
			t.Fatalf("got (%v, %v, %v), want (nil, TimeoutPort, false)", v, ch, ok)
		}
		if el := time.Since(start); el != 250*time.Millisecond {
			t.Fatalf("virtual elapsed %v, want exactly 250ms", el)
		}
	})
}

func TestTimeoutLosesToReadyOp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := make(chan any)
		go func() {
			time.Sleep(50 * time.Millisecond)
			c <- "late-but-in-time"
		}()
		v, ch, ok := Alts([]AltOp{{Chan: c}}, AltOpts{HasTimeout: true, Timeout: 500 * time.Millisecond})
		if v != "late-but-in-time" || ch != any(c) || !ok {
			t.Fatalf("got (%v, %v, %v), want value from c", v, ch, ok)
		}
	})
}

// Default fairness: reflect.Select must be pseudo-random across ready cases
// (the doc's RANDOM claim). With 2 always-ready channels over 2000 rounds,
// a uniform choice puts each side well above 25%.
func TestDefaultFairnessIsRandom(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c1 := make(chan any, 1)
		c2 := make(chan any, 1)
		counts := map[any]int{}
		for i := 0; i < 2000; i++ {
			c1 <- 1
			c2 <- 2
			_, ch, _ := Alts([]AltOp{{Chan: c1}, {Chan: c2}}, AltOpts{})
			counts[ch]++
			// drain the loser so both are ready again next round
			select {
			case <-c1:
			default:
			}
			select {
			case <-c2:
			default:
			}
		}
		if counts[any(c1)] < 500 || counts[any(c2)] < 500 {
			t.Fatalf("not random: c1=%d c2=%d of 2000", counts[any(c1)], counts[any(c2)])
		}
	})
}

// :priority — with both ready, the first listed op must ALWAYS win.
func TestPriorityIsDeterministic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c1 := make(chan any, 1)
		c2 := make(chan any, 1)
		for i := 0; i < 500; i++ {
			c1 <- 1
			c2 <- 2
			_, ch, _ := Alts([]AltOp{{Chan: c1}, {Chan: c2}}, AltOpts{Priority: true})
			if ch != any(c1) {
				t.Fatalf("round %d: priority picked %v, want c1", i, ch)
			}
			<-c2 // drain untouched loser
		}
	})
}

// :priority with nothing initially ready must still block (not spin/default)
// and complete when a port becomes ready.
func TestPriorityBlocksWhenNothingReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := make(chan any)
		go func() {
			time.Sleep(10 * time.Millisecond)
			c <- "eventually"
		}()
		v, ch, ok := Alts([]AltOp{{Chan: c}}, AltOpts{Priority: true})
		if v != "eventually" || ch != any(c) || !ok {
			t.Fatalf("got (%v, %v, %v)", v, ch, ok)
		}
	})
}

func TestNilPutPanics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("nil put did not panic")
			}
		}()
		c := make(chan any, 1)
		Alts([]AltOp{{Chan: c, Value: nil, IsWrite: true}}, AltOpts{})
	})
}

// Typed interop channel (chan int) used directly as an alts! port —
// doc 05's "interop and core.async are the same fabric" claim.
func TestTypedInteropChannel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := make(chan int, 1)
		v, _, ok := Alts([]AltOp{{Chan: c, Value: 99, IsWrite: true}}, AltOpts{})
		if v != true || !ok {
			t.Fatalf("typed write failed: (%v, %v)", v, ok)
		}
		v, _, ok = Alts([]AltOp{{Chan: c}}, AltOpts{})
		if v != 99 || !ok {
			t.Fatalf("typed read: got (%v, %v), want (99, true)", v, ok)
		}
	})
}

// Explicit no-leak check: goroutines blocked in Alts must be releasable and
// the bubble must exit clean. (synctest fails the test on any leak.)
func TestNoGoroutineLeaks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := make(chan any)
		done := make(chan any, 1)
		go func() {
			v, _, _ := Alts([]AltOp{{Chan: c}}, AltOpts{})
			done <- v
		}()
		synctest.Wait() // Alts goroutine is durably blocked in reflect.Select
		c <- "release"
		if v := <-done; v != "release" {
			t.Fatalf("got %v", v)
		}
	})
}
