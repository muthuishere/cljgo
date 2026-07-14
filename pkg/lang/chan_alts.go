package lang

import (
	"fmt"
	"reflect"
	"time"
)

// M4+ concurrency: alts! / timeout / buffer policies (design/05 §4). These
// are cljgo extensions layered on the M4 v0 Channel (chan.go), NOT vendored
// Glojure code, so no EPL header applies. Everything here is a plain runtime
// call reachable identically from the tree-walk interpreter and from
// AOT-emitted binaries (rt.Boot registers the wrapping builtins), so behaviour
// is byte-identical interpreted vs. compiled with no emitter special-casing.

// BufPolicy is a channel's over-capacity put behaviour (chan.go Channel.policy).
type BufPolicy int

const (
	// PolicyFixed is the default (blocking) buffer: a full put blocks the
	// producer. This is plain (chan) / (chan n).
	PolicyFixed BufPolicy = iota
	// PolicyDropping rejects new values when full: the producer never blocks
	// and the over-capacity value is discarded (core.async dropping-buffer).
	PolicyDropping
	// PolicySliding evicts the oldest buffered value to admit the newest: the
	// producer never blocks (core.async sliding-buffer).
	PolicySliding
)

// BufferSpec is the value produced by (dropping-buffer n) / (sliding-buffer n)
// and consumed by (chan buf). It is a first-class cljgo value so it can be
// passed to chan the way core.async passes a buffer object.
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
// It backs (chan (dropping-buffer n)) / (chan (sliding-buffer n)).
func NewChanBuffered(spec *BufferSpec) *Channel {
	c := NewChan(spec.N)
	c.policy = spec.Policy
	return c
}

// NewTimeout implements (timeout ms): a channel that auto-closes after ms
// milliseconds. (<! (timeout ms)) blocks ~ms then yields nil (the
// closed+drained receive). Deterministic result value (always nil) despite the
// wall-clock delay.
func NewTimeout(ms int64) *Channel {
	c := NewChan(0)
	go func() {
		if ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		ChanClose(c)
	}()
	return c
}

// kwAltsDefault is the port sentinel returned by (alts! ports :default v):
// the result is [v :default]. Interned once (keywords are values).
var kwAltsDefault = NewKeyword("default")

// Alts implements (alts! ports & {:keys [default]}): wait on several take-ports
// simultaneously and return [val port] for the first ready one (a closed port
// yields [nil port]). Built on reflect.Select over the ports' underlying Go
// channels (design/05 §4 — let-go's model carries over). When hasDefault is
// set and no port is immediately ready, returns [defVal :default] without
// blocking (core.async's :default). v0 supports take-ports only (bare
// Channels).
func Alts(ports []*Channel, hasDefault bool, defVal any) IPersistentVector {
	if len(ports) == 0 {
		panic(fmt.Errorf("alts! expects at least one port"))
	}
	cases := make([]reflect.SelectCase, 0, len(ports)+1)
	for _, p := range ports {
		if p == nil {
			panic(fmt.Errorf("alts! expects channels, got nil"))
		}
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(p.ch),
		})
	}
	if hasDefault {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectDefault})
	}
	chosen, recv, recvOK := reflect.Select(cases)
	if hasDefault && chosen == len(ports) {
		return NewVector(defVal, kwAltsDefault)
	}
	port := ports[chosen]
	if !recvOK {
		// closed+drained port → [nil port]
		return NewVector(nil, port)
	}
	return NewVector(recv.Interface(), port)
}
