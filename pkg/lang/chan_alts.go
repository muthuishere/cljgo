package lang

import (
	"fmt"
	"reflect"
	"time"
)

// M4+/ADR 0040 concurrency: alts! / timeout / buffer policies (design/05
// §4). These are cljgo extensions layered on the Channel (chan.go), NOT
// vendored Glojure code, so no EPL header applies. Everything here is a
// plain runtime call reachable identically from the tree-walk interpreter
// and from AOT-emitted binaries, so behaviour is byte-identical
// interpreted vs. compiled with no emitter special-casing.

// BufPolicy is a channel's over-capacity put behaviour (chan.go Channel.policy).
type BufPolicy int

const (
	// PolicyFixed is the default (blocking) buffer: a full put blocks the
	// producer. This is plain (chan) / (chan n) / (chan (buffer n)).
	PolicyFixed BufPolicy = iota
	// PolicyDropping rejects new values when full: the producer never blocks
	// and the over-capacity value is discarded (core.async dropping-buffer).
	PolicyDropping
	// PolicySliding evicts the oldest buffered value to admit the newest: the
	// producer never blocks (core.async sliding-buffer).
	PolicySliding
)

// BufferSpec is the value produced by (buffer n) / (dropping-buffer n) /
// (sliding-buffer n) and consumed by (chan buf). It is a first-class cljgo
// value so it can be passed to chan the way core.async passes a buffer
// object.
type BufferSpec struct {
	N      int
	Policy BufPolicy
}

func (b *BufferSpec) String() string {
	switch b.Policy {
	case PolicyDropping:
		return fmt.Sprintf("#object[cljgo.core.DroppingBuffer %d]", b.N)
	case PolicySliding:
		return fmt.Sprintf("#object[cljgo.core.SlidingBuffer %d]", b.N)
	default:
		return fmt.Sprintf("#object[cljgo.core.Buffer %d]", b.N)
	}
}

// FixedBuffer implements (buffer n): a plain blocking buffer of size n.
// The JVM assert message is matched (oracle buffer-zero / chan-zero:
// AssertionError "fixed buffers must have size > 0").
func FixedBuffer(n int) *BufferSpec {
	if n <= 0 {
		panic(fmt.Errorf("Assert failed: fixed buffers must have size > 0"))
	}
	return &BufferSpec{N: n, Policy: PolicyFixed}
}

// DroppingBuffer implements (dropping-buffer n): a buffer of size n whose
// over-capacity puts are dropped.
func DroppingBuffer(n int) *BufferSpec {
	if n <= 0 {
		panic(fmt.Errorf("dropping-buffer size must be positive, got %d", n))
	}
	return &BufferSpec{N: n, Policy: PolicyDropping}
}

// SlidingBuffer implements (sliding-buffer n): a buffer of size n whose
// over-capacity puts evict the oldest buffered value.
func SlidingBuffer(n int) *BufferSpec {
	if n <= 0 {
		panic(fmt.Errorf("sliding-buffer size must be positive, got %d", n))
	}
	return &BufferSpec{N: n, Policy: PolicySliding}
}

// NewChanBuffered builds a channel honouring a BufferSpec's size + policy.
// It backs (chan (buffer n)) / (chan (dropping-buffer n)) / (chan
// (sliding-buffer n)).
func NewChanBuffered(spec *BufferSpec) *Channel {
	c := NewChan(spec.N)
	c.policy = spec.Policy
	return c
}

// NewTimeout implements (timeout ms): a FRESH channel closed by
// time.AfterFunc after ms milliseconds (ADR 0040 #4 — semantics only,
// no JVM-style per-tick channel cache: the docstring promises the close,
// channel identity across calls is an implementation artifact there).
// (<! (timeout ms)) blocks ~ms then yields nil (the closed+drained
// receive); otherwise it is a normal unbuffered channel.
func NewTimeout(ms int64) *Channel {
	c := NewChan(0)
	d := time.Duration(ms) * time.Millisecond
	if d < 0 {
		d = 0
	}
	time.AfterFunc(d, func() { ChanClose(c) })
	return c
}

// kwAltsDefault is the port sentinel returned by (alts! ports :default v):
// the result is [v :default]. Interned once (keywords are values).
var kwAltsDefault = NewKeyword("default")

// altsVec builds the [val port] result with the owning constructor —
// one small-tail vector, no defensive copy (alts is on the T1 perf
// budget, chan_budget_test.go).
func altsVec(v, port any) IPersistentVector {
	return NewVectorOwning([]any{v, port})
}

// altsPort is one parsed alts! port: a read from a cljgo Channel, a
// write [c v] to one, or a read from a FOREIGN Go chan T (the interop
// constraint, design/05 §1 / S19 Q1 — any Go API's channel is a legal
// port; a closed chan T read normalizes to nil, never the element
// type's zero value).
type altsPort struct {
	write bool
	c     *Channel      // nil for a foreign Go channel
	fch   reflect.Value // the foreign channel (read side)
	val   any           // the value to write (write ports)
	id    any           // the port identity in the [val port] result
}

// parseAltsPort turns ONE raw port value of (alts! ports …) into an
// altsPort: *Channel = read, a 2-element vector [chan val] = write, any
// Go channel value = foreign read. Split per-port (and the parsed slice
// passed in by the caller) so the ready fast path below stays
// allocation-light — alts is on the T1 perf budget (chan_budget_test.go).
func parseAltsPort(p any) altsPort {
	switch port := p.(type) {
	case *Channel:
		return altsPort{c: port, id: port}
	case IPersistentVector:
		if Count(port) != 2 {
			panic(fmt.Errorf("alts! write port must be a [channel value] pair, got: %s", PrintString(port)))
		}
		c, ok := port.Nth(0).(*Channel)
		if !ok {
			panic(fmt.Errorf("alts! write port must name a channel, got: %s", PrintString(port.Nth(0))))
		}
		v := port.Nth(1)
		if v == nil {
			panic(NewIllegalArgumentError("Can't put nil on channel"))
		}
		return altsPort{write: true, c: c, val: v, id: c}
	default:
		rv := reflect.ValueOf(p)
		if p == nil || rv.Kind() != reflect.Chan {
			panic(fmt.Errorf("alts! expects channel operations, got: %s", PrintString(p)))
		}
		return altsPort{fch: rv, id: p}
	}
}

// tryReady attempts the port without blocking. ok reports whether the
// operation completed; val is the operation's result value ([val port]).
func (p *altsPort) tryReady() (val any, ok bool) {
	if p.write {
		// A write to a closed channel completes immediately with false
		// (oracle alts-write-closed => false); otherwise attempt a
		// non-blocking put. offer! returns nil when it would block.
		switch res := ChanOffer(p.c, p.val); res {
		case true:
			return true, true
		case false:
			return false, true
		default:
			return nil, false
		}
	}
	if p.c != nil {
		c := p.c
		if c.promise {
			select {
			case <-c.done:
				return c.pval, true
			default:
				return nil, false
			}
		}
		select {
		case v := <-c.ch:
			return v, true
		default:
		}
		select {
		case <-c.done:
			// Closed: one final drain probe decides value vs nil — a
			// closed channel is READY (oracle alts-closed-ready-default
			// => nil, the :default is NOT taken).
			select {
			case v := <-c.ch:
				return v, true
			default:
				return nil, true
			}
		default:
			return nil, false
		}
	}
	// Foreign Go chan: TryRecv distinguishes empty (invalid Value) from
	// closed (valid zero Value) — a closed chan T normalizes to nil.
	v, recvOK := p.fch.TryRecv()
	if recvOK {
		return v.Interface(), true
	}
	if v.IsValid() {
		return nil, true // closed
	}
	return nil, false
}

// Alts implements (alts! ports & {:keys [default priority]}) — ADR 0040
// #3: dynamic port vectors on reflect.Select. Read ports are cljgo
// Channels or foreign Go channels; write ports are [chan val] pairs.
// Returns [val port]; a :default hit returns [defVal :default] without
// blocking; :priority true makes a ready port win in listed order.
//
// Blocking mechanics: each Channel read port contributes TWO select
// cases — data recv and done recv (ADR 0040 #2: the data chan is never
// closed, so closure must be its own case; on fire, a final drain probe
// decides value vs nil). A promise-chan port contributes its done case
// only; a foreign chan one plain recv; a write port one send case (a
// put parked in the select survives a concurrent close!, JVM parity).
func Alts(ports []any, hasDefault bool, defVal any, priority bool) IPersistentVector {
	if len(ports) == 0 {
		panic(fmt.Errorf("Assert failed: alts must have at least one channel operation"))
	}

	// Ordered non-blocking pass: the whole story for :default, the
	// first rung for :priority, and the write-on-closed / promise /
	// already-closed fast paths for the plain blocking form. Ports are
	// parsed one at a time into a stack cell so a ready port costs no
	// slice allocation (the common case and the budgeted one).
	for _, raw := range ports {
		p := parseAltsPort(raw)
		if v, ok := p.tryReady(); ok {
			return altsVec(v, p.id)
		}
	}
	if hasDefault {
		return altsVec(defVal, kwAltsDefault)
	}

	parsed := make([]altsPort, len(ports))
	for i, raw := range ports {
		parsed[i] = parseAltsPort(raw)
	}

	// Blocking phase. caseOf maps a select case index back to its port
	// and the case's meaning.
	type caseRef struct {
		port int
		kind int // 0 = data recv, 1 = done recv, 2 = foreign recv, 3 = send
	}
	var (
		cases []reflect.SelectCase
		refs  []caseRef
		// A write to a transducer channel cannot be expressed as a raw
		// send case (it would bypass the xform), so those ports are
		// polled: a short timer case re-runs the ready pass.
		needPoll bool
	)
	for {
		cases = cases[:0]
		refs = refs[:0]
		for i := range parsed {
			p := &parsed[i]
			switch {
			case p.write:
				if p.c.step != nil || p.c.promise {
					needPoll = true // completes only via tryReady
					continue
				}
				cases = append(cases, reflect.SelectCase{
					Dir: reflect.SelectSend, Chan: reflect.ValueOf(p.c.ch), Send: reflect.ValueOf(p.val)})
				refs = append(refs, caseRef{i, 3})
			case p.c != nil && p.c.promise:
				cases = append(cases, reflect.SelectCase{
					Dir: reflect.SelectRecv, Chan: reflect.ValueOf(p.c.done)})
				refs = append(refs, caseRef{i, 1})
			case p.c != nil:
				cases = append(cases, reflect.SelectCase{
					Dir: reflect.SelectRecv, Chan: reflect.ValueOf(p.c.ch)})
				refs = append(refs, caseRef{i, 0})
				cases = append(cases, reflect.SelectCase{
					Dir: reflect.SelectRecv, Chan: reflect.ValueOf(p.c.done)})
				refs = append(refs, caseRef{i, 1})
			default:
				cases = append(cases, reflect.SelectCase{
					Dir: reflect.SelectRecv, Chan: p.fch})
				refs = append(refs, caseRef{i, 2})
			}
		}
		if needPoll {
			t := time.NewTimer(100 * time.Microsecond)
			cases = append(cases, reflect.SelectCase{
				Dir: reflect.SelectRecv, Chan: reflect.ValueOf(t.C)})
			refs = append(refs, caseRef{-1, -1})
			chosen, recv, recvOK := reflect.Select(cases)
			t.Stop()
			if refs[chosen].port == -1 {
				// Poll tick: re-run the ready pass (covers xform/promise
				// write ports), then rebuild and select again.
				for i := range parsed {
					if v, ok := parsed[i].tryReady(); ok {
						return altsVec(v, parsed[i].id)
					}
				}
				continue
			}
			return altsResult(&parsed[refs[chosen].port], refs[chosen].kind, recv, recvOK)
		}
		chosen, recv, recvOK := reflect.Select(cases)
		return altsResult(&parsed[refs[chosen].port], refs[chosen].kind, recv, recvOK)
	}
}

// altsResult shapes one fired select case into the [val port] vector.
func altsResult(p *altsPort, kind int, recv reflect.Value, recvOK bool) IPersistentVector {
	switch kind {
	case 0: // data recv on a cljgo Channel
		return altsVec(recv.Interface(), p.id)
	case 1: // done fired: promise value, or the final drain probe
		if p.c.promise {
			return altsVec(p.c.pval, p.id)
		}
		select {
		case v := <-p.c.ch:
			return altsVec(v, p.id)
		default:
			return altsVec(nil, p.id)
		}
	case 2: // foreign Go chan recv; closed normalizes to nil
		if !recvOK {
			return altsVec(nil, p.id)
		}
		return altsVec(recv.Interface(), p.id)
	default: // send completed
		return altsVec(true, p.id)
	}
}
