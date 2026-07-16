package lang

import (
	"sync"
	"time"
)

// Promise is clojure.core's `promise`: a single-value cell that blocks
// deref until delivered, deliverable exactly once (design/08 batch E,
// ADR 0022 — the taps.cljc suite file's await-tap helper needs it
// alongside tap>/add-tap/remove-tap). Distinct from *future*: nothing
// runs it — the caller (or ANY other goroutine holding the value) calls
// Deliver.
type Promise struct {
	mu        sync.Mutex
	done      chan struct{}
	val       any
	delivered bool
}

var (
	_ IBlockingDeref = (*Promise)(nil)
	_ IDeref         = (*Promise)(nil)
	_ IPending       = (*Promise)(nil)
	_ IFn            = (*Promise)(nil)
)

func NewPromise() *Promise {
	return &Promise{done: make(chan struct{})}
}

func (p *Promise) Deref() interface{} {
	<-p.done
	return p.val
}

func (p *Promise) DerefWithTimeout(timeoutMS int64, timeoutVal interface{}) interface{} {
	select {
	case <-p.done:
		return p.val
	case <-time.After(time.Duration(timeoutMS) * time.Millisecond):
		return timeoutVal
	}
}

func (p *Promise) IsRealized() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// Deliver sets the promise's value the FIRST time only, returning true;
// every later call is a no-op returning false (matching real Clojure:
// deliver on an already-delivered promise returns nil, this promise
// unchanged).
func (p *Promise) Deliver(val any) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.delivered {
		return false
	}
	p.delivered = true
	p.val = val
	close(p.done)
	return true
}

func (p *Promise) String() string { return "#object[cljgo.core.Promise]" }

// Invoke makes a Promise itself IFn, matching real Clojure's own
// reify-based promise (design/08 batch E — clojure-test-suite's
// ifn_qmark.cljc asserts (ifn? (promise))): 0-arity returns the promise
// itself (a no-op probe); 1-arity is deliver's OWN mechanism on the JVM
// ((promise-obj val) delivers) — cljgo's `deliver` builtin calls Deliver
// directly instead, so this 1-arity exists for IFn-shape parity, not
// because deliver depends on it.
func (p *Promise) Invoke(args ...any) any {
	switch len(args) {
	case 0:
		return p
	case 1:
		p.Deliver(args[0])
		return p
	default:
		return p
	}
}

func (p *Promise) ApplyTo(args ISeq) any {
	return p.Invoke(seqToSlice(args)...)
}
