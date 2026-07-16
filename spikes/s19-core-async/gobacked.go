package s19

// Q1 candidate (c): ONE wrapper struct holding a real Go chan + buffer
// policy + transducer logic — the M4-v0 `*lang.Channel` shape extended
// with xform-on-put. The Go runtime keeps doing rendezvous, parking,
// wakeup and select; we only add the core.async feature layer on the put
// side, serialized by a put-side mutex exactly like core.async serializes
// the xf step inside the channel lock.

import (
	"fmt"
	"sync"
)

type BufPolicy int

const (
	PolicyFixed BufPolicy = iota
	PolicyDropping
	PolicySliding
)

// Xform models a transducer for the spike: given the buffer-add step it
// returns the xf step; the step returns false when the transducer says
// `reduced` (channel must close). Real cljgo transducers are IFns with
// the same shape — this is the minimal Go skeleton of (xform rf).
type Xform func(step func(any)) func(v any) (more bool)

// GoBacked is the extended M4-v0 channel.
type GoBacked struct {
	ch     chan any
	policy BufPolicy

	mu     sync.Mutex // guards closed + serializes the xf step
	closed bool
	xf     func(v any) (more bool) // nil when no transducer
}

func NewGoBacked(n int, policy BufPolicy, xform Xform) *GoBacked {
	if xform != nil && n <= 0 && policy == PolicyFixed {
		// core.async parity: a transducer requires a buffered channel.
		panic(fmt.Errorf("chan: transducer requires a buffer"))
	}
	c := &GoBacked{ch: make(chan any, n), policy: policy}
	if xform != nil {
		c.xf = xform(c.bufferAdd)
	}
	return c
}

// bufferAdd is the reducing step handed to the transducer: policy-aware
// put of ONE already-transformed element. For PolicyFixed it may block
// (mid-expansion backpressure — the documented divergence from JVM
// core.async, which completes an expansion into a temporarily over-full
// buffer; see VERDICT Q1).
func (c *GoBacked) bufferAdd(v any) {
	switch c.policy {
	case PolicyDropping:
		select {
		case c.ch <- v:
		default: // full: drop the new value
		}
	case PolicySliding:
		select {
		case c.ch <- v:
		default:
			select {
			case <-c.ch: // evict oldest
			default:
			}
			select {
			case c.ch <- v:
			default:
			}
		}
	default:
		c.ch <- v
	}
}

// Put implements >! / >!!. Returns false iff the channel is closed
// (core.async contract).
func (c *GoBacked) Put(v any) (ok bool) {
	if v == nil {
		panic(fmt.Errorf("can't put nil on a channel"))
	}
	if c.xf == nil && c.policy == PolicyFixed {
		// Fast path: plain channel, no put-side lock at all. Close is
		// handled by recovering the send-on-closed panic (M4-v0 shape).
		defer func() {
			if r := recover(); r != nil {
				ok = false
			}
		}()
		c.ch <- v
		return true
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	if c.xf == nil {
		// policy put, no xform: bufferAdd never blocks for drop/slide, so
		// holding mu is fine and closes the close-vs-put race entirely.
		c.bufferAdd(v)
		c.mu.Unlock()
		return true
	}
	more := c.xf(v)
	c.mu.Unlock()
	if !more {
		c.Close() // transducer said reduced → channel closes
	}
	return true
}

// Take implements <! / <!!: closed+drained → nil (Go's comma-ok receive).
func (c *GoBacked) Take() any {
	v, ok := <-c.ch
	if !ok {
		return nil
	}
	return v
}

// Close is idempotent close! .
func (c *GoBacked) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.ch)
	}
}

// Raw exposes the backing channel — the interop edge (receive-only view
// keeps outsiders from closing it or bypassing the xform on put).
func (c *GoBacked) Raw() <-chan any { return c.ch }

// --- spike transducers (Go skeletons of (map f), (filter p), (mapcat f),
// (take n)) — enough to prove expansion, filtering and reduced. ---

func XfMap(f func(any) any) Xform {
	return func(step func(any)) func(any) bool {
		return func(v any) bool { step(f(v)); return true }
	}
}

func XfFilter(pred func(any) bool) Xform {
	return func(step func(any)) func(any) bool {
		return func(v any) bool {
			if pred(v) {
				step(v)
			}
			return true
		}
	}
}

func XfMapcat(f func(any) []any) Xform {
	return func(step func(any)) func(any) bool {
		return func(v any) bool {
			for _, x := range f(v) {
				step(x)
			}
			return true
		}
	}
}

func XfTake(n int) Xform {
	return func(step func(any)) func(any) bool {
		left := n
		return func(v any) bool {
			if left > 0 {
				left--
				step(v)
			}
			return left > 0 // false => reduced → close
		}
	}
}
