package lang

import (
	"fmt"
	"sync"
)

// Channels & goroutines (M4, design/05 §4). These are cljgo extensions,
// NOT vendored Glojure code, so no EPL header applies.
//
// The thesis (design/05 §4): goroutines ARE the cheap thing core.async's
// CPS `go` macro emulates on the JVM, so there is NO IOC/state-machine
// rewrite here. `(go ...)` runs its body in a real goroutine and `<!`/`>!`
// simply block. A cljgo Channel wraps a Go `chan any`; the SAME helpers
// (ChanSend/ChanRecv/ChanClose/Go) serve BOTH execution modes — the
// tree-walk interpreter calls them directly, and AOT-emitted binaries
// link pkg/lang and call the identical functions (rt.Boot registers the
// clojure.core builtins that wrap them). This is why behaviour is
// identical interpreted vs. compiled with no emitter special-casing.

// Channel is a first-class cljgo value wrapping a Go channel. nil is
// never a legal element (core.async parity), so a closed+drained receive
// is unambiguously Clojure nil. `closed` + `mu` make close! idempotent
// (double-close of a Go channel panics; here it is a no-op).
type Channel struct {
	ch     chan any
	mu     sync.Mutex
	closed bool
	// policy is the over-capacity put behaviour (design/05 §4 "buffer
	// policies"). PolicyFixed (the zero value) blocks the producer when the
	// buffer is full — plain (chan)/(chan n). PolicyDropping / PolicySliding
	// never block the producer: see ChanSend. Go channels have no native
	// drop/slide, so the policy is a thin layer over a non-blocking send
	// (let-go's chanPolicy).
	policy BufPolicy
}

// NewChan builds an unbuffered (n == 0) or fixed-buffer (n > 0) channel with
// the default (blocking) buffer policy.
func NewChan(n int) *Channel {
	if n < 0 {
		panic(fmt.Errorf("chan buffer size must be non-negative, got %d", n))
	}
	return &Channel{ch: make(chan any, n)}
}

// ChanSend implements (>! c v) / (>!! c v): blocking put, returns nil.
// A nil put is rejected (core.async parity — it would be indistinguishable
// from a closed+drained receive). A put on a closed channel would panic in
// Go; v0 recovers it into a no-op (design/05 §4 "closed put returns
// false/nil, not panic"), returning nil.
func ChanSend(c *Channel, v any) (res any) {
	if c == nil {
		panic(fmt.Errorf(">! expects a channel, got nil"))
	}
	if v == nil {
		panic(fmt.Errorf("can't put nil on a channel"))
	}
	defer func() {
		// send-on-closed-channel panic → no-op put, nil result.
		if r := recover(); r != nil {
			res = nil
		}
	}()
	switch c.policy {
	case PolicyDropping:
		// Never block: if the buffer is full, the new value is dropped.
		select {
		case c.ch <- v:
		default:
		}
		return nil
	case PolicySliding:
		// Never block: if the buffer is full, evict the oldest buffered
		// value to make room, then put. The evict+put pair makes progress
		// under the single-producer discipline these buffers are for.
		select {
		case c.ch <- v:
		default:
			select {
			case <-c.ch: // drop oldest
			default:
			}
			select {
			case c.ch <- v:
			default: // a racing consumer refilled it; drop rather than block
			}
		}
		return nil
	default:
		c.ch <- v
		return nil
	}
}

// ChanRecv implements (<! c) / (<!! c): blocking take. A closed+drained
// channel yields Clojure nil via the comma-ok receive.
func ChanRecv(c *Channel) any {
	if c == nil {
		panic(fmt.Errorf("<! expects a channel, got nil"))
	}
	v, ok := <-c.ch
	if !ok {
		return nil
	}
	return v
}

// ChanClose implements (close! c): idempotent close, returns nil.
func ChanClose(c *Channel) any {
	if c == nil {
		panic(fmt.Errorf("close! expects a channel, got nil"))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.ch)
	}
	return nil
}

// Go runs thunk (a 0-arg cljgo fn) in a REAL goroutine and returns a
// result channel (buffer 1) that receives the thunk's value then closes —
// so (<! (go ...)) composes (let-go's contract, design/05 §4). A nil body
// value sends nothing (nil puts are rejected); the receiver then reads the
// closed+drained channel as nil, which is the same value — consistent.
//
// Panic policy (v0): a panic in the body is recovered and the result
// channel is simply closed, so (<! (go ...)) yields nil rather than
// crashing the process. (A richer error hook is design/05 §4's later work.)
func Go(thunk any) *Channel {
	result := NewChan(1)
	go func() {
		defer func() {
			_ = recover() // body panic → close result, <! yields nil
			ChanClose(result)
		}()
		v := Apply0(thunk)
		if v != nil {
			ChanSend(result, v)
		}
	}()
	return result
}

// String gives a channel a stable printed form (channels are rarely
// pr-str'd, but PrintString must not choke on one).
func (c *Channel) String() string { return "#object[cljgo.core.Channel]" }
