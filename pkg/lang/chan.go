package lang

import (
	"fmt"
	"sync"
)

// Channels & goroutines (M4 → ADR 0040, design/05 §4). These are cljgo
// extensions, NOT vendored Glojure code, so no EPL header applies.
//
// The thesis (design/05 §4): goroutines ARE the cheap thing core.async's
// CPS `go` macro emulates on the JVM, so there is NO IOC/state-machine
// rewrite here. `(go ...)` runs its body in a real goroutine and `<!`/`>!`
// simply block. A cljgo Channel wraps a Go `chan any`; the SAME helpers
// (ChanSend/ChanRecv/ChanClose/Go/Alts) serve BOTH execution modes — the
// tree-walk interpreter calls them directly, and AOT-emitted binaries
// link pkg/lang and call the identical functions. This is why behaviour
// is identical interpreted vs. compiled with no emitter special-casing.
//
// Close semantics (ADR 0040 #2, spike S19 Q1′ `gobacked2.go`): close!
// NEVER closes the Go data channel. A separate `done` channel signals
// closure; takes prefer draining data (which still rendezvouses with
// parked senders) and only report nil when done has fired AND no value
// is immediately available. This is what makes a put parked BEFORE the
// close survive it and deliver to a later taker, returning true — JVM
// core.async 1.6.681 parity (oracle: parked-put-survives-close =>
// [:v true], spikes/s19-core-async/oracle/probe3.txt). It also deletes
// every send-on-closed panic/recover shim the M4-v0 shape needed.

// NSAsync is the clojure.core.async namespace (ADR 0040 #5: the
// canonical home of every async var; the M4-v0 clojure.core names are
// refers of the same vars). Pinned at package init — like NSCore — so
// the namespace object is stable for the process lifetime: re-runs of
// corelib.RegisterAll re-intern the same Vars, and test harnesses that
// snapshot/remove namespaces never see it as "new".
var NSAsync = FindOrCreateNamespace(NewSymbol("clojure.core.async"))

// Channel is a first-class cljgo value wrapping a Go channel. nil is
// never a legal element (core.async parity), so a closed+drained receive
// is unambiguously Clojure nil.
type Channel struct {
	ch   chan any      // the data channel — NEVER closed (ADR 0040 #2)
	done chan struct{} // closed by close!: the closure signal

	// mu guards closed / promise state and serializes the transducer
	// step on the put side (core.async serializes xf under its channel
	// lock the same way — S19 Q1).
	mu     sync.Mutex
	closed bool

	// policy is the over-capacity put behaviour (design/05 §4 "buffer
	// policies"). PolicyFixed (the zero value) blocks the producer when
	// the buffer is full — plain (chan)/(chan n). PolicyDropping /
	// PolicySliding never block the producer: see bufferAdd.
	policy BufPolicy

	// step is the transducer put step: it feeds transformed elements to
	// bufferAdd and returns false when the transducer said `reduced`
	// (the channel must close). nil when the channel has no transducer.
	step func(v any) (more bool)
	// flush is the transducer completion arity, run once at close! so a
	// stateful transducer (partition-all, …) flushes its pending state
	// into the buffer. nil when the channel has no transducer.
	flush func()

	// promise-chan mode (ADR 0040 / S19 Q6): a latch, not a queue — the
	// first put wins, EVERY take sees that value, later puts are
	// accepted-and-ignored (oracle: promise-chan-put-after-first =>
	// [:a :a]). delivered+pval are written before close(done), so reads
	// after <-done need no lock (happens-before via the channel close).
	promise   bool
	delivered bool
	pval      any
}

// NewChan builds an unbuffered (n == 0) or fixed-buffer (n > 0) channel
// with the default (blocking) buffer policy. Rejecting n == 0 at the
// `chan` builtin (JVM parity: "fixed buffers must have size > 0") is the
// caller's job — internally n == 0 means a rendezvous channel, which is
// what (chan) builds.
func NewChan(n int) *Channel {
	if n < 0 {
		panic(fmt.Errorf("chan buffer size must be non-negative, got %d", n))
	}
	return &Channel{ch: make(chan any, n), done: make(chan struct{})}
}

// NewPromiseChan implements (promise-chan): a latch channel. (T1 scope:
// the optional xform/ex-handler arities of promise-chan are not wired.)
func NewPromiseChan() *Channel {
	return &Channel{done: make(chan struct{}), promise: true}
}

// SetXform installs a transducer + optional ex-handler on c (the
// (chan buf xform ex-handler) arities — ADR 0040 #1). xform is a cljgo
// transducer IFn: (xform rf) => rf'. The reducing fn rf adds one
// already-transformed element to the buffer (policy-aware, so a fixed
// buffer applies backpressure mid-expansion — the S19-documented
// divergence: values identical to the JVM, timing differs). A step
// panic is routed to exh: nil/absent ex-handler drops the poisoned
// value and keeps the channel usable (oracle xform-no-ex-handler-
// throws-where => :put-returned); a non-nil ex-handler return is added
// in the value's place (oracle xform-ex-handler => :handled).
func (c *Channel) SetXform(xform, exh any) {
	rf := NewFnFunc(func(args ...any) any {
		switch len(args) {
		case 0:
			return nil
		case 1:
			return args[0] // completion: nothing to finalize at the buffer
		case 2:
			c.bufferAdd(args[1])
			return args[0]
		default:
			panic(fmt.Errorf("channel reducing step expects 0-2 args, got %d", len(args)))
		}
	})
	rfx := Apply1(xform, rf)
	guarded := func(f func() any) (res any) {
		defer func() {
			if r := recover(); r != nil {
				res = nil
				if exh == nil {
					return // drop the poisoned value; channel stays usable
				}
				err, ok := r.(error)
				if !ok {
					err = fmt.Errorf("%v", r)
				}
				if sub := Apply1(exh, err); sub != nil {
					c.bufferAdd(sub)
				}
			}
		}()
		return f()
	}
	c.step = func(v any) bool {
		return !IsReduced(guarded(func() any { return Apply2(rfx, nil, v) }))
	}
	c.flush = func() {
		guarded(func() any { return Apply1(rfx, nil) })
	}
}

// bufferAdd is the policy-aware put of ONE element into the buffer: the
// reducing step handed to a transducer, and the put path of dropping/
// sliding channels. PolicyFixed may block awaiting a taker; the other
// policies never block the producer (Go channels have no native
// drop/slide, so this is a thin layer over a non-blocking send —
// let-go's chanPolicy).
func (c *Channel) bufferAdd(v any) {
	switch c.policy {
	case PolicyDropping:
		// Never block: if the buffer is full, the new value is dropped.
		select {
		case c.ch <- v:
		default:
		}
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
	default:
		c.ch <- v
	}
}

// ChanSend implements (>! c v) / (>!! c v): blocking put, returning true
// when the value was accepted and false when the channel was already
// closed (core.async contract; oracle closed-put->!! => false). A nil
// put is rejected with the JVM's exact message (oracle: "Can't put nil
// on channel", IllegalArgumentException). A put that parked BEFORE
// close! stays parked and remains deliverable — JVM parity (the data
// chan is never closed, so there is nothing to panic on).
func ChanSend(c *Channel, v any) bool {
	if c == nil {
		panic(NewIllegalArgumentError(">! expects a channel, got nil"))
	}
	if v == nil {
		panic(NewIllegalArgumentError("Can't put nil on channel"))
	}
	if c.promise {
		return c.promisePut(v)
	}
	if c.step == nil && c.policy == PolicyFixed {
		// Fast path: plain channel. Check the flag, then a plain
		// (possibly parking) send. A close! landing between the check
		// and the send makes this put "in flight at close time", which
		// the JVM also delivers (S19 Q1′).
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return false
		}
		c.ch <- v
		return true
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	if c.step == nil {
		// policy put, no xform: bufferAdd never blocks for drop/slide,
		// so holding mu closes the close-vs-put race entirely.
		c.bufferAdd(v)
		c.mu.Unlock()
		return true
	}
	more := c.step(v)
	c.mu.Unlock()
	if !more {
		ChanClose(c) // transducer said reduced → channel closes
	}
	return true
}

// promisePut is the promise-chan put: first put latches the value and
// wakes every waiter; later puts are accepted-and-ignored (true); puts
// after close! return false (oracle: promise-put-after-close => false).
func (c *Channel) promisePut(v any) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	if c.delivered {
		return true // accepted and ignored (oracle [:a :a])
	}
	c.pval = v
	c.delivered = true
	close(c.done)
	return true
}

// ChanRecv implements (<! c) / (<!! c): blocking take. Drain-preferring
// (ADR 0040 #2): a non-blocking probe first (covers buffered values AND
// parked senders), then a blocking select on data|done, and when done
// fires one final drain probe decides value vs nil — so a closed
// channel yields its buffered values and parked puts before nil forever
// (oracle closed-read-drains-buffer => [1 2 nil]).
func ChanRecv(c *Channel) any {
	if c == nil {
		panic(NewIllegalArgumentError("<! expects a channel, got nil"))
	}
	if c.promise {
		<-c.done
		return c.pval // nil when closed without a value
	}
	select {
	case v := <-c.ch:
		return v
	default:
	}
	select {
	case v := <-c.ch:
		return v
	case <-c.done:
		select {
		case v := <-c.ch:
			return v
		default:
			return nil
		}
	}
}

// ChanClose implements (close! c): idempotent, returns nil (oracle
// double-close => nil). Flips the flag, runs the transducer's completion
// arity (a stateful xf flushes pending state into the buffer — a fixed
// buffer with no room applies backpressure until takers drain it), then
// fires done. The data chan is never closed, so no panic, no recover,
// and parked senders live on (ADR 0040 #2).
func ChanClose(c *Channel) any {
	if c == nil {
		panic(NewIllegalArgumentError("close! expects a channel, got nil"))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.promise {
		if !c.delivered {
			close(c.done) // waiters wake with nil; a delivered value stays
		}
		return nil
	}
	if c.flush != nil {
		c.flush()
	}
	close(c.done)
	return nil
}

// ChanOffer implements (offer! c v): non-blocking put — true when the
// value was accepted immediately, nil (NOT false) when it would have
// blocked (oracle offer-poll => [true nil 1 nil], offer-on-unbuffered-
// no-taker => nil), false when the channel is closed (the put contract).
func ChanOffer(c *Channel, v any) any {
	if c == nil {
		panic(NewIllegalArgumentError("offer! expects a channel, got nil"))
	}
	if v == nil {
		panic(NewIllegalArgumentError("Can't put nil on channel"))
	}
	if c.promise {
		return c.promisePut(v) // never blocks (oracle promise-offer-take)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	if c.step != nil {
		// Through the transducer only when the buffer has room, so the
		// step's adds cannot block (offer! must never block).
		if len(c.ch) < cap(c.ch) {
			more := c.step(v)
			c.mu.Unlock()
			if !more {
				ChanClose(c)
			}
			return true
		}
		c.mu.Unlock()
		return nil
	}
	if c.policy != PolicyFixed {
		c.bufferAdd(v) // drop/slide: never blocks
		c.mu.Unlock()
		return true
	}
	c.mu.Unlock()
	// Plain fixed channel: one non-blocking send (rendezvouses with a
	// parked taker on an unbuffered channel).
	select {
	case c.ch <- v:
		return true
	default:
		return nil
	}
}

// ChanPoll implements (poll! c): non-blocking take — the value when one
// is immediately available, nil otherwise (also nil on closed+drained).
// On a promise-chan a delivered value is returned WITHOUT consuming it
// (oracle promise-offer-take => [true :v :v]).
func ChanPoll(c *Channel) any {
	if c == nil {
		panic(NewIllegalArgumentError("poll! expects a channel, got nil"))
	}
	if c.promise {
		select {
		case <-c.done:
			return c.pval
		default:
			return nil
		}
	}
	select {
	case v := <-c.ch:
		return v
	default:
		return nil
	}
}

// ChanPutAsync implements (put! c v) / (put! c v fn1): asynchronous put.
// Returns false (and calls fn1 with false) when the channel is already
// closed; otherwise returns true immediately — the value is delivered by
// a goroutine if it cannot complete without blocking (goroutines are the
// cheap thing here; the JVM queues a pending put for the same reason) —
// and fn1, when given, receives the put's boolean once it completes.
// Oracle: put!-returns-before-taker => true; put!-cb-on-closed =>
// [false false].
func ChanPutAsync(c *Channel, v any, cb any) bool {
	if c == nil {
		panic(NewIllegalArgumentError("put! expects a channel, got nil"))
	}
	if v == nil {
		panic(NewIllegalArgumentError("Can't put nil on channel"))
	}
	done := func(ok bool) {
		if cb != nil {
			Apply1(cb, ok)
		}
	}
	switch res := ChanOffer(c, v); res {
	case true:
		done(true)
		return true
	case false:
		done(false)
		return false
	default: // nil: would block — complete asynchronously
		go func() { done(ChanSend(c, v)) }()
		return true
	}
}

// ChanTakeAsync implements (take! c fn1): asynchronous take — fn1 is
// called with the taken value (nil for closed+drained) from a goroutine.
// Oracle: take!-callback => :v; take!-on-closed => [:got nil].
func ChanTakeAsync(c *Channel, cb any) any {
	if c == nil {
		panic(NewIllegalArgumentError("take! expects a channel, got nil"))
	}
	go func() { Apply1(cb, ChanRecv(c)) }()
	return nil
}

// Raw exposes the backing Go channel, receive-only — the interop edge
// (design/05 §1): AOT code can hand it to Go APIs taking <-chan any,
// and the receive-only view keeps outsiders from bypassing the
// transducer or closing it. nil for a promise-chan (no data channel).
func (c *Channel) Raw() <-chan any { return c.ch }

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
