package s19

// Q1 candidate (a) + Q2 candidate 2: a Go port of core.async's
// ManyToManyChannel — mutex-guarded ring buffer + queues of pending take
// and put HANDLERS, where a handler carries a commit flag shared across
// all ports of one alts! call (first committed wins). This is the
// representation that makes the handler-protocol alts possible; blocking
// ops are a handler whose callback signals a 1-slot Go chan.

import (
	"fmt"
	"sync"
)

// Handler is core.async's Lock+Handler collapsed: Commit atomically
// claims the operation (false if another port of the same alts already
// won). A blocking op uses a nil flag = always commits.
type Handler struct {
	flag *sync.Mutex // shared across an alts call's ports; nil = solo op
	won  *bool       // guarded by flag
}

func soloHandler() *Handler { return &Handler{} }

func altsFlag() (*sync.Mutex, *bool) { return &sync.Mutex{}, new(bool) }

// Active reports whether the handler can still commit (peek without
// claiming). Commit claims it.
func (h *Handler) Active() bool {
	if h.flag == nil {
		return true
	}
	h.flag.Lock()
	defer h.flag.Unlock()
	return !*h.won
}

func (h *Handler) Commit() bool {
	if h.flag == nil {
		return true
	}
	h.flag.Lock()
	defer h.flag.Unlock()
	if *h.won {
		return false
	}
	*h.won = true
	return true
}

type takeWaiter struct {
	h  *Handler
	cb func(v any, ok bool)
}

type putWaiter struct {
	h  *Handler
	v  any
	cb func(ok bool)
}

// AsyncChan is the ManyToManyChannel port (fixed buffer only — policies
// and xform would slot into bufAdd exactly as in GoBacked).
type AsyncChan struct {
	mu      sync.Mutex
	buf     []any
	head    int
	count   int
	takers  []takeWaiter
	putters []putWaiter
	closed  bool
}

func NewAsyncChan(n int) *AsyncChan {
	return &AsyncChan{buf: make([]any, max(n, 0))}
}

func (c *AsyncChan) bufPush(v any) {
	c.buf[(c.head+c.count)%len(c.buf)] = v
	c.count++
}

func (c *AsyncChan) bufPop() any {
	v := c.buf[c.head]
	c.buf[c.head] = nil
	c.head = (c.head + 1) % len(c.buf)
	c.count--
	return v
}

// PutH is the handler-protocol put: returns (completed, ok). completed
// false means the op was parked (callback fires later).
func (c *AsyncChan) PutH(v any, h *Handler, cb func(ok bool)) (bool, bool) {
	if v == nil {
		panic(fmt.Errorf("can't put nil on a channel"))
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		if !h.Commit() {
			return false, false
		}
		return true, false
	}
	// 1. A waiting taker? hand off directly (rendezvous).
	for len(c.takers) > 0 {
		t := c.takers[0]
		c.takers = c.takers[1:]
		if !t.h.Active() {
			continue
		}
		// two-phase commit: claim ourselves first, then the taker.
		if !h.Commit() {
			c.takers = append([]takeWaiter{t}, c.takers...)
			c.mu.Unlock()
			return false, false
		}
		if !t.h.Commit() {
			continue // taker lost its own alts race; keep scanning
		}
		c.mu.Unlock()
		t.cb(v, true)
		return true, true
	}
	// 2. Buffer room?
	if c.count < len(c.buf) {
		if !h.Commit() {
			c.mu.Unlock()
			return false, false
		}
		c.bufPush(v)
		c.mu.Unlock()
		return true, true
	}
	// 3. Park.
	c.putters = append(c.putters, putWaiter{h: h, v: v, cb: cb})
	c.mu.Unlock()
	return false, false
}

// TakeH is the handler-protocol take.
func (c *AsyncChan) TakeH(h *Handler, cb func(v any, ok bool)) (bool, any, bool) {
	c.mu.Lock()
	// 1. Buffered value?
	if c.count > 0 {
		if !h.Commit() {
			c.mu.Unlock()
			return false, nil, false
		}
		v := c.bufPop()
		// refill from a parked putter, if any.
		for len(c.putters) > 0 {
			p := c.putters[0]
			c.putters = c.putters[1:]
			if p.h.Commit() {
				c.bufPush(p.v)
				c.mu.Unlock()
				p.cb(true)
				return true, v, true
			}
		}
		c.mu.Unlock()
		return true, v, true
	}
	// 2. A parked putter? (unbuffered rendezvous)
	for len(c.putters) > 0 {
		p := c.putters[0]
		c.putters = c.putters[1:]
		if !p.h.Active() {
			continue
		}
		if !h.Commit() {
			c.putters = append([]putWaiter{p}, c.putters...)
			c.mu.Unlock()
			return false, nil, false
		}
		if !p.h.Commit() {
			continue
		}
		c.mu.Unlock()
		p.cb(true)
		return true, p.v, true
	}
	// 3. Closed?
	if c.closed {
		if !h.Commit() {
			c.mu.Unlock()
			return false, nil, false
		}
		c.mu.Unlock()
		return true, nil, false
	}
	// 4. Park.
	c.takers = append(c.takers, takeWaiter{h: h, cb: cb})
	c.mu.Unlock()
	return false, nil, false
}

// Close wakes parked takers with (nil,false).
func (c *AsyncChan) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	takers := c.takers
	c.takers = nil
	c.mu.Unlock()
	for _, t := range takers {
		if t.h.Commit() {
			t.cb(nil, false)
		}
	}
}

// Blocking facade (what >!/<! would compile to on this representation):
// a solo handler whose callback signals a 1-slot Go chan.

func (c *AsyncChan) Put(v any) bool {
	done := make(chan bool, 1)
	completed, ok := c.PutH(v, soloHandler(), func(ok bool) { done <- ok })
	if completed {
		return ok
	}
	return <-done
}

type takeResult struct {
	v  any
	ok bool
}

func (c *AsyncChan) Take() any {
	done := make(chan takeResult, 1)
	completed, v, ok := c.TakeH(soloHandler(), func(v any, ok bool) {
		done <- takeResult{v, ok}
	})
	if completed {
		if !ok {
			return nil
		}
		return v
	}
	r := <-done
	if !r.ok {
		return nil
	}
	return r.v
}

// AltsH is the handler-protocol alts!: try each port; first commit wins.
// Read ports are *AsyncChan, write ports are [2]any{*AsyncChan, val}.
func AltsH(ports []any, hasDefault bool, defVal any) (any, any, bool) {
	flag, won := altsFlag()
	done := make(chan [3]any, 1)
	for _, p := range ports {
		switch port := p.(type) {
		case *AsyncChan:
			h := &Handler{flag: flag, won: won}
			ch := port
			completed, v, ok := ch.TakeH(h, func(v any, ok bool) {
				done <- [3]any{v, ch, ok}
			})
			if completed {
				return v, ch, ok
			}
		case [2]any:
			ch := port[0].(*AsyncChan)
			h := &Handler{flag: flag, won: won}
			completed, ok := ch.PutH(port[1], h, func(ok bool) {
				done <- [3]any{ok, ch, ok}
			})
			if completed {
				return ok, ch, ok
			}
		default:
			panic(fmt.Errorf("alts: bad port %T", p))
		}
	}
	if hasDefault {
		// claim the flag; if a parked op already won meanwhile it beat us.
		h := &Handler{flag: flag, won: won}
		if h.Commit() {
			return defVal, "default", false
		}
	}
	r := <-done
	return r[0], r[1], r[2].(bool)
}
