package s19

// Q1 refinement forced by the oracle: JVM core.async close! does NOT
// reject parked puts — they stay parked and are DELIVERED to takers that
// arrive after the close (oracle/probe3.txt: parked-put-survives-close
// => [:v true]). M4-v0's shape (close the Go chan, recover the
// send-on-closed panic) returns false and LOSES the parked value — a
// real divergence.
//
// GoBacked2 fixes it: close! never closes the data chan. Instead a
// `done` chan signals closure; Take prefers draining data (which still
// rendezvous-es with parked senders) and only reports nil when done has
// fired AND no value is immediately available. Put checks the closed
// flag up front (new puts after close => false) and otherwise blocks in
// a plain send — a parked sender survives close exactly like the JVM.
// The question this file answers: what does that fidelity COST per op?

import (
	"fmt"
	"sync"
)

type GoBacked2 struct {
	ch   chan any
	done chan struct{}

	mu     sync.Mutex
	closed bool
}

func NewGoBacked2(n int) *GoBacked2 {
	return &GoBacked2{ch: make(chan any, n), done: make(chan struct{})}
}

// Put: false immediately if already closed; otherwise a plain (possibly
// parking) send. A put that parked BEFORE close stays parked and remains
// deliverable — JVM parity. (It also stays parked forever if no taker
// ever comes, which is the JVM behavior too: timeout-put-still-parked.)
func (c *GoBacked2) Put(v any) bool {
	if v == nil {
		panic(fmt.Errorf("can't put nil on a channel"))
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()
	// Race window: close! can land here, after the check — that put is
	// "in flight at close time", which the JVM also delivers; fine.
	c.ch <- v
	return true
}

// Take: drain-preferring receive. Fast path: a non-blocking receive
// (covers buffered values AND parked senders). Slow path: block on
// data-or-done; when done fires, one final drain attempt decides
// value vs nil.
func (c *GoBacked2) Take() any {
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

// Close: flip the flag, fire done. The data chan is never closed, so no
// panic, no recover, and parked senders live on.
func (c *GoBacked2) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.done)
	}
}
