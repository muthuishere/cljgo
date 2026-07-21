package lang

// Close-rework + T1 semantics tests (ADR 0040, openspec
// core-async-first-class 1.1/1.2/1.4/1.5). Every expectation here is a
// JVM core.async 1.6.681 oracle line (spikes/s19-core-async/oracle/
// transcript*.txt, probe3.txt, or the fresh 2026-07-21 T1 run recorded
// in conformance/tests/chan-*.clj comments). Run with -race: these are
// also the channel runtime's data-race canaries.

import (
	"sync"
	"testing"
	"time"
)

// TestParkedPutSurvivesClose is THE close-rework proof (oracle probe3:
// parked-put-survives-close => [:v true]): a put parked before close!
// stays parked, is delivered to a taker arriving AFTER the close, and
// returns true. The M4-v0 shape (close the data chan + recover) lost
// the value and returned false.
func TestParkedPutSurvivesClose(t *testing.T) {
	c := NewChan(0)
	res := make(chan bool, 1)
	go func() { res <- ChanSend(c, NewKeyword("v")) }()
	time.Sleep(20 * time.Millisecond) // let the put park
	ChanClose(c)
	if got := ChanRecv(c); got != NewKeyword("v") {
		t.Fatalf("take after close = %v, want :v (the parked put's value)", got)
	}
	select {
	case ok := <-res:
		if !ok {
			t.Fatal("parked put returned false, want true (JVM parity)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("parked put never completed after its value was taken")
	}
	if got := ChanRecv(c); got != nil {
		t.Fatalf("second take on closed = %v, want nil", got)
	}
}

// TestCloseDrainsBufferThenNil: oracle closed-read-drains-buffer =>
// [1 2 nil]; closed-put->!! => false; double-close => nil (no-op).
func TestCloseDrainsBufferThenNil(t *testing.T) {
	c := NewChan(2)
	ChanSend(c, int64(1))
	ChanSend(c, int64(2))
	ChanClose(c)
	ChanClose(c) // double close: no-op
	if got := ChanSend(c, int64(9)); got != false {
		t.Fatalf("put after close = %v, want false", got)
	}
	for _, want := range []any{int64(1), int64(2), nil, nil} {
		if got := ChanRecv(c); got != want {
			t.Fatalf("drain = %v, want %v", got, want)
		}
	}
}

// TestCloseWakesBlockedTakers: takers parked at close! wake with nil.
func TestCloseWakesBlockedTakers(t *testing.T) {
	c := NewChan(0)
	var wg sync.WaitGroup
	got := make([]any, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); got[i] = ChanRecv(c) }(i)
	}
	time.Sleep(20 * time.Millisecond)
	ChanClose(c)
	wg.Wait()
	for i, v := range got {
		if v != nil {
			t.Fatalf("taker %d woke with %v, want nil", i, v)
		}
	}
}

// TestChanConcurrentPutTakeClose is a pure race canary: many producers,
// many consumers, a mid-stream close. No assertion beyond termination
// and no lost non-nil values after close+drain — -race does the work.
func TestChanConcurrentPutTakeClose(t *testing.T) {
	c := NewChan(4)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if !ChanSend(c, int64(j+1)) {
					return
				}
			}
		}()
	}
	var taken sync.WaitGroup
	for i := 0; i < 8; i++ {
		taken.Add(1)
		go func() {
			defer taken.Done()
			for ChanRecv(c) != nil {
			}
		}()
	}
	time.Sleep(10 * time.Millisecond)
	ChanClose(c)
	wg.Wait()
	taken.Wait()
}

// mapIncXform is a Go-side stand-in for (map inc) with the cljgo
// transducer calling convention: (xform rf) => rf'; rf' 2-arity steps,
// 1-arity completes.
var mapIncXform = NewFnFunc(func(args ...any) any {
	rf := args[0]
	return NewFnFunc(func(inner ...any) any {
		switch len(inner) {
		case 1:
			return Apply1(rf, inner[0])
		case 2:
			return Apply2(rf, inner[0], inner[1].(int64)+1)
		}
		return nil
	})
})

// takeNXform stands in for (take n): after n steps it returns a Reduced,
// which must close the channel (oracle xform-reduced-closes =>
// [1 2 nil false]).
func takeNXform(n int) IFn {
	return NewFnFunc(func(args ...any) any {
		rf := args[0]
		left := n
		return NewFnFunc(func(inner ...any) any {
			switch len(inner) {
			case 1:
				return Apply1(rf, inner[0])
			case 2:
				res := Apply2(rf, inner[0], inner[1])
				left--
				if left <= 0 {
					return NewReduced(res)
				}
				return res
			}
			return nil
		})
	})
}

func TestChanXformMap(t *testing.T) {
	c := NewChan(3)
	c.SetXform(mapIncXform, nil)
	ChanSend(c, int64(1))
	if got := ChanRecv(c); got != int64(2) {
		t.Fatalf("xform map take = %v, want 2 (oracle xform-map)", got)
	}
}

func TestChanXformReducedCloses(t *testing.T) {
	c := NewChan(5)
	c.SetXform(takeNXform(2), nil)
	ChanSend(c, int64(1))
	ChanSend(c, int64(2))
	for _, want := range []any{int64(1), int64(2), nil} {
		if got := ChanRecv(c); got != want {
			t.Fatalf("drain = %v, want %v", got, want)
		}
	}
	if got := ChanSend(c, int64(3)); got != false {
		t.Fatalf("put after reduced-close = %v, want false", got)
	}
}

// TestChanXformExHandler: a step panic routed to the ex-handler; its
// non-nil return is put instead (oracle xform-ex-handler => :handled);
// with no handler the value is dropped and the channel stays usable
// (oracle xform-no-ex-handler-throws-where => :put-returned).
func TestChanXformExHandler(t *testing.T) {
	boom := NewFnFunc(func(args ...any) any {
		rf := args[0]
		return NewFnFunc(func(inner ...any) any {
			switch len(inner) {
			case 1:
				return Apply1(rf, inner[0])
			case 2:
				if inner[1] == int64(0) {
					panic(NewArithmeticError("Divide by zero"))
				}
				return Apply2(rf, inner[0], inner[1])
			}
			return nil
		})
	})
	c := NewChan(1)
	c.SetXform(boom, NewFnFunc(func(args ...any) any { return NewKeyword("handled") }))
	if got := ChanSend(c, int64(0)); got != true {
		t.Fatalf("poisoned put = %v, want true", got)
	}
	if got := ChanRecv(c); got != NewKeyword("handled") {
		t.Fatalf("ex-handler substitute = %v, want :handled", got)
	}

	c2 := NewChan(1)
	c2.SetXform(boom, nil)
	if got := ChanSend(c2, int64(0)); got != true { // put completes, value dropped
		t.Fatalf("no-handler poisoned put = %v, want true", got)
	}
	if got := ChanSend(c2, int64(2)); got != true {
		t.Fatalf("channel unusable after dropped poison: put = %v, want true", got)
	}
	if got := ChanRecv(c2); got != int64(2) {
		t.Fatalf("value after poison = %v, want 2", got)
	}
}

// TestPromiseChan: first put wins, every take sees it, later puts are
// accepted-and-ignored; close!-without-value wakes takers with nil
// (oracle promise-chan-put-after-first => [:a :a],
// promise-close-no-value => nil, promise-put-after-close => false).
func TestPromiseChan(t *testing.T) {
	p := NewPromiseChan()
	if got := ChanSend(p, NewKeyword("a")); got != true {
		t.Fatalf("first put = %v, want true", got)
	}
	if got := ChanSend(p, NewKeyword("b")); got != true {
		t.Fatalf("second put = %v, want true (accepted-and-ignored)", got)
	}
	for i := 0; i < 2; i++ {
		if got := ChanRecv(p); got != NewKeyword("a") {
			t.Fatalf("take %d = %v, want :a", i, got)
		}
	}
	if got := ChanPoll(p); got != NewKeyword("a") { // poll does not consume
		t.Fatalf("poll = %v, want :a", got)
	}

	empty := NewPromiseChan()
	ChanClose(empty)
	if got := ChanRecv(empty); got != nil {
		t.Fatalf("take on closed empty promise-chan = %v, want nil", got)
	}
	if got := ChanSend(empty, int64(1)); got != false {
		t.Fatalf("put after close = %v, want false", got)
	}
}

// TestPromiseChanConcurrentWaiters: takers parked before delivery all
// wake with the delivered value (race canary).
func TestPromiseChanConcurrentWaiters(t *testing.T) {
	p := NewPromiseChan()
	var wg sync.WaitGroup
	got := make([]any, 4)
	for i := range got {
		wg.Add(1)
		go func(i int) { defer wg.Done(); got[i] = ChanRecv(p) }(i)
	}
	time.Sleep(10 * time.Millisecond)
	ChanSend(p, int64(7))
	wg.Wait()
	for i, v := range got {
		if v != int64(7) {
			t.Fatalf("waiter %d = %v, want 7", i, v)
		}
	}
}

// TestOfferPoll: oracle offer-poll => [true nil 1 nil];
// offer-on-unbuffered-no-taker => nil.
func TestOfferPoll(t *testing.T) {
	c := NewChan(1)
	if got := ChanOffer(c, int64(1)); got != true {
		t.Fatalf("offer into empty buffer = %v, want true", got)
	}
	if got := ChanOffer(c, int64(2)); got != nil {
		t.Fatalf("offer into full buffer = %v, want nil", got)
	}
	if got := ChanPoll(c); got != int64(1) {
		t.Fatalf("poll = %v, want 1", got)
	}
	if got := ChanPoll(c); got != nil {
		t.Fatalf("poll empty = %v, want nil", got)
	}
	if got := ChanOffer(NewChan(0), int64(1)); got != nil {
		t.Fatalf("offer unbuffered no taker = %v, want nil", got)
	}
	closed := NewChan(1)
	ChanClose(closed)
	if got := ChanOffer(closed, int64(1)); got != false {
		t.Fatalf("offer on closed = %v, want false", got)
	}
}

// TestPutTakeAsync: oracle put!-returns-before-taker => true;
// put!-cb-on-closed => [false false]; take!-callback => :v.
func TestPutTakeAsync(t *testing.T) {
	c := NewChan(1)
	cbres := make(chan any, 1)
	cb := NewFnFunc(func(args ...any) any { cbres <- args[0]; return nil })
	if got := ChanPutAsync(c, NewKeyword("v"), cb); got != true {
		t.Fatalf("put! = %v, want true", got)
	}
	if got := <-cbres; got != true {
		t.Fatalf("put! callback = %v, want true", got)
	}
	tkres := make(chan any, 1)
	ChanTakeAsync(c, NewFnFunc(func(args ...any) any { tkres <- args[0]; return nil }))
	if got := <-tkres; got != NewKeyword("v") {
		t.Fatalf("take! callback = %v, want :v", got)
	}

	closed := NewChan(1)
	ChanClose(closed)
	if got := ChanPutAsync(closed, int64(1), cb); got != false {
		t.Fatalf("put! on closed = %v, want false", got)
	}
	if got := <-cbres; got != false {
		t.Fatalf("put! on closed callback = %v, want false", got)
	}

	// a put! that must park still returns true immediately and its
	// callback fires once a taker arrives.
	unbuf := NewChan(0)
	if got := ChanPutAsync(unbuf, int64(9), cb); got != true {
		t.Fatalf("parked put! = %v, want true", got)
	}
	if got := ChanRecv(unbuf); got != int64(9) {
		t.Fatalf("take of parked put! = %v, want 9", got)
	}
	if got := <-cbres; got != true {
		t.Fatalf("parked put! callback = %v, want true", got)
	}
}

// TestAltsDoneIntegration: alts read ports carry a done case — a port
// closed DURING the wait yields [nil port] (fresh oracle 2026-07-21:
// alts-closed-ready => [nil ch]); a write port to a closed channel
// completes immediately with false (alts-write-closed => false); a
// write port parked in the select survives a concurrent close (JVM:
// timeout-put-still-parked-after-close).
func TestAltsDoneIntegration(t *testing.T) {
	c := NewChan(0)
	go func() {
		time.Sleep(20 * time.Millisecond)
		ChanClose(c)
	}()
	res := Alts([]any{c}, false, nil, false)
	if res.Nth(0) != nil || res.Nth(1) != c {
		t.Fatalf("alts on closing chan = %v, want [nil c]", res)
	}

	closed := NewChan(1)
	ChanClose(closed)
	w := Alts([]any{NewVector(closed, int64(1))}, false, nil, false)
	if w.Nth(0) != false || w.Nth(1) != closed {
		t.Fatalf("alts write on closed = %v, want [false c]", w)
	}
}

// TestAltsDefaultAndPriority: oracle alts-default => :none (port
// :default), alts-priority-first-wins => :a, alts-priority-and-default
// => :dflt, and a CLOSED port counts as ready so :default is NOT taken
// (fresh oracle: alts-closed-ready-default => nil).
func TestAltsDefaultAndPriority(t *testing.T) {
	idle := NewChan(0)
	res := Alts([]any{idle}, true, NewKeyword("none"), false)
	if res.Nth(0) != NewKeyword("none") || res.Nth(1) != kwAltsDefault {
		t.Fatalf("alts default = %v, want [:none :default]", res)
	}

	c1, c2 := NewChan(1), NewChan(1)
	ChanSend(c1, NewKeyword("a"))
	ChanSend(c2, NewKeyword("b"))
	res = Alts([]any{c1, c2}, false, nil, true)
	if res.Nth(0) != NewKeyword("a") || res.Nth(1) != c1 {
		t.Fatalf("alts priority = %v, want [:a c1]", res)
	}

	closed := NewChan(0)
	ChanClose(closed)
	res = Alts([]any{closed}, true, NewKeyword("none"), false)
	if res.Nth(0) != nil || res.Nth(1) != closed {
		t.Fatalf("alts closed-ready with default = %v, want [nil c]", res)
	}
}

// TestAltsForeignGoChan: the interop constraint (design/05 §1, S19 Q1) —
// a FOREIGN Go chan T is a legal alts port, and a closed chan int
// normalizes to nil, never 0.
func TestAltsForeignGoChan(t *testing.T) {
	f := make(chan int, 1)
	f <- 5
	res := Alts([]any{f}, false, nil, false)
	if res.Nth(0) != 5 || res.Nth(1) != any(f) {
		t.Fatalf("alts foreign = %v, want [5 f]", res)
	}
	close(f)
	res = Alts([]any{f}, false, nil, false)
	if res.Nth(0) != nil {
		t.Fatalf("alts closed foreign chan int = %v, want nil (normalized)", res.Nth(0))
	}
}

// TestAltsWriteXformChannel: a write port to a transducer channel goes
// through the xform (never a raw send that would bypass it) — polled in
// the blocking phase.
func TestAltsWriteXformChannel(t *testing.T) {
	c := NewChan(1)
	c.SetXform(mapIncXform, nil)
	res := Alts([]any{NewVector(c, int64(1))}, false, nil, false)
	if res.Nth(0) != true {
		t.Fatalf("alts write xform = %v, want [true c]", res)
	}
	if got := ChanRecv(c); got != int64(2) {
		t.Fatalf("xform write value = %v, want 2 (through the transducer)", got)
	}

	// full buffer: the write must park (poll loop) until a taker drains.
	got := make(chan any, 1)
	go func() {
		time.Sleep(15 * time.Millisecond)
		got <- ChanRecv(c)
	}()
	ChanSend(c, int64(10)) // fill the buffer (stored as 11)
	res = Alts([]any{NewVector(c, int64(20))}, false, nil, false)
	if res.Nth(0) != true {
		t.Fatalf("parked alts write xform = %v, want [true c]", res)
	}
	if v := <-got; v != int64(11) {
		t.Fatalf("drained value = %v, want 11", v)
	}
	if v := ChanRecv(c); v != int64(21) {
		t.Fatalf("second xform write value = %v, want 21", v)
	}
}

// TestTimeoutCloses: (timeout ms) closes after ~ms (semantics-only,
// ADR 0040 #4 — no channel cache, so two calls are distinct channels).
func TestTimeoutCloses(t *testing.T) {
	c := NewTimeout(10)
	start := time.Now()
	if got := ChanRecv(c); got != nil {
		t.Fatalf("timeout yielded %v, want nil", got)
	}
	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("timeout closed too early")
	}
	if NewTimeout(50) == NewTimeout(50) {
		t.Fatal("timeout channels must be fresh per call (documented divergence)")
	}
}
