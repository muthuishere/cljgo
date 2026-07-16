package s19

// Semantics of the GoBacked prototype, frozen against the JVM core.async
// oracle transcript (oracle/transcript.txt, core.async 1.6.681).

import (
	"testing"
	"time"
)

func TestXformMap(t *testing.T) {
	// oracle xform-map: (>!! c 1) then (<!! c) => 2
	c := NewGoBacked(3, PolicyFixed, XfMap(func(v any) any { return v.(int) + 1 }))
	c.Put(1)
	if got := c.Take(); got != 2 {
		t.Fatalf("want 2, got %v", got)
	}
}

func TestXformFilterDrops(t *testing.T) {
	// oracle xform-filter-drops: put 2 (dropped), put 3, take => 3
	c := NewGoBacked(3, PolicyFixed, XfFilter(func(v any) bool { return v.(int)%2 == 1 }))
	c.Put(2)
	c.Put(3)
	if got := c.Take(); got != 3 {
		t.Fatalf("want 3, got %v", got)
	}
}

func TestXformMapcatExpansion(t *testing.T) {
	// oracle xform-mapcat-expansion: buffer 2, expansion of 3.
	// JVM: the put COMPLETES into a temporarily over-full buffer.
	// GoBacked divergence: element 3 blocks until a taker drains one —
	// the put's total effect is identical, only backpressure timing
	// differs. This test freezes OUR semantics: all 3 values arrive.
	c := NewGoBacked(2, PolicyFixed, XfMapcat(func(v any) []any {
		return []any{v, v, v}
	}))
	done := make(chan bool)
	go func() { c.Put(1); done <- true }()
	got := []any{c.Take(), c.Take(), c.Take()}
	<-done
	for _, v := range got {
		if v != 1 {
			t.Fatalf("want [1 1 1], got %v", got)
		}
	}
}

func TestXformReducedCloses(t *testing.T) {
	// oracle xform-reduced-closes: (chan 5 (take 2)); after 2 puts the
	// channel is closed; third take => nil, subsequent put => false.
	c := NewGoBacked(5, PolicyFixed, XfTake(2))
	c.Put(1)
	c.Put(2)
	if v := c.Take(); v != 1 {
		t.Fatalf("want 1, got %v", v)
	}
	if v := c.Take(); v != 2 {
		t.Fatalf("want 2, got %v", v)
	}
	if v := c.Take(); v != nil {
		t.Fatalf("want nil (closed), got %v", v)
	}
	if ok := c.Put(3); ok {
		t.Fatal("put after reduced-close should report false")
	}
}

func TestDroppingWithXform(t *testing.T) {
	// oracle dropping-with-xform: (chan (dropping-buffer 2) (map inc)),
	// 5 puts of 0..4, close, drain => [1 2 nil]
	c := NewGoBacked(2, PolicyDropping, XfMap(func(v any) any { return v.(int) + 1 }))
	for i := 0; i < 5; i++ {
		c.Put(i)
	}
	c.Close()
	if a, b, z := c.Take(), c.Take(), c.Take(); a != 1 || b != 2 || z != nil {
		t.Fatalf("want [1 2 nil], got [%v %v %v]", a, b, z)
	}
}

func TestSlidingWithXform(t *testing.T) {
	// oracle sliding-with-xform: => [4 5 nil] (keeps newest)
	c := NewGoBacked(2, PolicySliding, XfMap(func(v any) any { return v.(int) + 1 }))
	for i := 0; i < 5; i++ {
		c.Put(i)
	}
	c.Close()
	if a, b, z := c.Take(), c.Take(), c.Take(); a != 4 || b != 5 || z != nil {
		t.Fatalf("want [4 5 nil], got [%v %v %v]", a, b, z)
	}
}

func TestClosedSemantics(t *testing.T) {
	// oracle closed-read / closed-put / closed-read-drains-buffer
	c := NewGoBacked(2, PolicyFixed, nil)
	c.Put(1)
	c.Put(2)
	c.Close()
	if a, b, z := c.Take(), c.Take(), c.Take(); a != 1 || b != 2 || z != nil {
		t.Fatalf("want [1 2 nil], got [%v %v %v]", a, b, z)
	}
	if ok := c.Put(9); ok {
		t.Fatal("put on closed must report false")
	}
}

func TestNilPutPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil put must panic")
		}
	}()
	NewGoBacked(1, PolicyFixed, nil).Put(nil)
}

// --- AsyncChan parity: the same contract on the handler representation ---

func TestAsyncChanBasics(t *testing.T) {
	c := NewAsyncChan(2)
	c.Put(1)
	c.Put(2)
	c.Close()
	if a, b, z := c.Take(), c.Take(), c.Take(); a != 1 || b != 2 || z != nil {
		t.Fatalf("want [1 2 nil], got [%v %v %v]", a, b, z)
	}
}

func TestAsyncChanRendezvous(t *testing.T) {
	c := NewAsyncChan(0)
	go func() { c.Put(42) }()
	if v := c.Take(); v != 42 {
		t.Fatalf("want 42, got %v", v)
	}
}

func TestAltsHandlerBasic(t *testing.T) {
	ready, idle := NewAsyncChan(1), NewAsyncChan(1)
	ready.Put(7)
	v, ch, ok := AltsH([]any{idle, ready}, false, nil)
	if v != 7 || ch != ready || !ok {
		t.Fatalf("want (7 ready true), got (%v %v %v)", v, ch, ok)
	}
	// idle's parked handler is inert: a later put must NOT vanish into it.
	idle.Put(8)
	if v := idle.Take(); v != 8 {
		t.Fatalf("parked dead handler swallowed the value: got %v", v)
	}
}

func TestAltsHandlerDefault(t *testing.T) {
	c := NewAsyncChan(1)
	v, port, ok := AltsH([]any{c}, true, "none")
	if v != "none" || port != "default" || ok {
		t.Fatalf("want default branch, got (%v %v %v)", v, port, ok)
	}
}

func TestAltsHandlerParkedWakeup(t *testing.T) {
	c := NewAsyncChan(0)
	go func() { time.Sleep(5 * time.Millisecond); c.Put(9) }()
	v, port, ok := AltsH([]any{c}, false, nil)
	if v != 9 || port != c || !ok {
		t.Fatalf("want (9 c true), got (%v %v %v)", v, port, ok)
	}
}

func TestAltsReflectForeignChan(t *testing.T) {
	// interop constraint: a raw Go chan works as an alts port unchanged.
	foreign := make(chan int, 1)
	foreign <- 5
	v, port, ok := AltsR([]any{NewGoBacked(1, PolicyFixed, nil), foreign}, false, nil)
	if v != 5 || !ok {
		t.Fatalf("want 5 from foreign chan, got (%v %v %v)", v, port, ok)
	}
}

func TestAltsReflectClosedForeignChan(t *testing.T) {
	// closed chan int must normalize to nil, not int(0) (S10 finding).
	foreign := make(chan int)
	close(foreign)
	v, _, ok := AltsR([]any{foreign}, false, nil)
	if v != nil || ok {
		t.Fatalf("want (nil false), got (%v _ %v)", v, ok)
	}
}

// --- GoBacked2: close-fidelity variant (oracle probe3) ---

func TestGoBacked2ParkedPutSurvivesClose(t *testing.T) {
	// oracle parked-put-survives-close => [:v true]
	c := NewGoBacked2(0)
	ret := make(chan bool, 1)
	go func() { ret <- c.Put("v") }()
	time.Sleep(20 * time.Millisecond) // let the put park
	c.Close()
	if v := c.Take(); v != "v" {
		t.Fatalf("taker after close must receive the parked put, got %v", v)
	}
	if ok := <-ret; !ok {
		t.Fatal("the delivered parked put must report true")
	}
}

func TestGoBacked2ClosedSemantics(t *testing.T) {
	c := NewGoBacked2(2)
	c.Put(1)
	c.Put(2)
	c.Close()
	if a, b, z := c.Take(), c.Take(), c.Take(); a != 1 || b != 2 || z != nil {
		t.Fatalf("want [1 2 nil], got [%v %v %v]", a, b, z)
	}
	if ok := c.Put(9); ok {
		t.Fatal("put after close must report false")
	}
}

func TestGoBacked2CloseWakesBlockedTakers(t *testing.T) {
	c := NewGoBacked2(0)
	got := make(chan any, 3)
	for i := 0; i < 3; i++ {
		go func() {
			got <- func() any {
				v := c.Take()
				if v == nil {
					return "nil"
				}
				return v
			}()
		}()
	}
	time.Sleep(20 * time.Millisecond)
	c.Close()
	for i := 0; i < 3; i++ {
		if v := <-got; v != "nil" {
			t.Fatalf("blocked taker must wake with nil on close, got %v", v)
		}
	}
}
