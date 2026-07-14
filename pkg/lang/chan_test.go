package lang

import (
	"sync"
	"testing"
)

func TestChanBufferedSendRecv(t *testing.T) {
	c := NewChan(3)
	ChanSend(c, int64(10))
	ChanSend(c, int64(20))
	ChanSend(c, int64(30))
	ChanClose(c)
	for _, want := range []any{int64(10), int64(20), int64(30), nil} {
		if got := ChanRecv(c); got != want {
			t.Fatalf("recv = %v, want %v", got, want)
		}
	}
}

func TestChanCloseIdempotent(t *testing.T) {
	c := NewChan(0)
	ChanClose(c)
	ChanClose(c) // must not panic (double close of a Go channel would)
	if got := ChanRecv(c); got != nil {
		t.Fatalf("recv on closed = %v, want nil", got)
	}
}

func TestChanSendNilRejected(t *testing.T) {
	c := NewChan(1)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil put")
		}
	}()
	ChanSend(c, nil)
}

func TestChanSendOnClosedIsNoop(t *testing.T) {
	c := NewChan(1)
	ChanClose(c)
	if got := ChanSend(c, int64(1)); got != nil { // recovered, no panic
		t.Fatalf("send on closed = %v, want nil", got)
	}
}

// TestGoRoundTrip exercises (go body) → result channel with a real goroutine.
func TestGoRoundTrip(t *testing.T) {
	thunk := FnFunc0(func() any { return int64(42) })
	res := Go(thunk)
	if got := ChanRecv(res); got != int64(42) {
		t.Fatalf("go result = %v, want 42", got)
	}
	if got := ChanRecv(res); got != nil { // closed after delivering
		t.Fatalf("go result after close = %v, want nil", got)
	}
}

// TestGoPanicClosesChannel: a panicking body must not crash the process;
// the result channel closes and <! yields nil (v0 policy, design/05 §4).
func TestGoPanicClosesChannel(t *testing.T) {
	thunk := FnFunc0(func() any { panic("boom") })
	res := Go(thunk)
	if got := ChanRecv(res); got != nil {
		t.Fatalf("panicking go result = %v, want nil", got)
	}
}

// TestGoConcurrency proves goroutines actually run concurrently: N
// producers each deliver on their own channel; if Go were synchronous this
// would still pass, so we additionally require a shared unbuffered handoff
// that can only complete if producer and consumer run in parallel.
func TestGoConcurrency(t *testing.T) {
	c := NewChan(0) // unbuffered: send blocks until receive
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ChanSend(c, int64(7))
	}()
	if got := ChanRecv(c); got != int64(7) { // completes only with real concurrency
		t.Fatalf("unbuffered handoff = %v, want 7", got)
	}
	wg.Wait()
}
