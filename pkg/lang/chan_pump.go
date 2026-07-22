package lang

import "sync"

// Channel pumps — the T2 distribution/composition surface of
// clojure.core.async (ADR 0040 #9 tier T2, openspec core-async-first-class
// §2). These are cljgo extensions, NOT vendored Glojure code, so no EPL
// header applies.
//
// Every pump is a real goroutine (or a small set of them) over the T1
// primitives (ChanRecv/ChanSend/ChanClose/Alts) — the thesis (design/05
// §4) is that goroutines ARE the cheap thing core.async's go-macro
// emulates on the JVM, so a "pump" here is just a `go func(){}` reading one
// channel and writing others. The SAME helpers serve the interpreter and
// AOT binaries, so behaviour is identical interpreted vs. compiled.
//
// Close propagation and every state rule below is frozen against REAL JVM
// core.async 1.6.681 (Clojure 1.12.5) — see conformance/tests/chan-*.clj.
// nil is never a legal channel value (ChanSend rejects it), so a nil from
// ChanRecv unambiguously means "closed and drained" and drives every
// pump's termination.

// OntoChan implements (onto-chan! ch coll) / (onto-chan! ch coll close?):
// a goroutine puts every element of coll onto ch in order, then closes ch
// when close? (default true). Returns a channel that closes once the pump
// finishes (oracle onto-chan!-closes => [10 20 30], onto-chan!-noclose
// keeps ch open).
func OntoChan(ch *Channel, coll any, closeWhenDone bool) *Channel {
	done := NewChan(1)
	go func() {
		for s := Seq(coll); !IsNil(s); s = s.Next() {
			if !ChanSend(ch, s.First()) {
				break // ch closed under us — stop pumping
			}
		}
		if closeWhenDone {
			ChanClose(ch)
		}
		ChanClose(done)
	}()
	return done
}

// ToChan implements (to-chan! coll): a fresh channel carrying coll's
// elements that closes when they are exhausted. Buffer is the element
// count capped at 100 — JVM parity ((chan (bounded-count 100 coll))),
// oracle to-chan! => [1 2 3].
func ToChan(coll any) *Channel {
	var vals []any
	for s := Seq(coll); !IsNil(s); s = s.Next() {
		vals = append(vals, s.First())
	}
	n := len(vals)
	if n > 100 {
		n = 100
	}
	ch := NewChan(n)
	go func() {
		for _, v := range vals {
			if !ChanSend(ch, v) {
				break
			}
		}
		ChanClose(ch)
	}()
	return ch
}

// Pipe implements (pipe from to) / (pipe from to close?): a goroutine
// copies every value from `from` to `to`; when `from` closes it closes
// `to` unless close? is false, and if `to` closes first it closes `from`
// (JVM parity). Returns `to` (oracle pipe => [1 2 3], pipe-returns-to
// => true).
func Pipe(from, to *Channel, closeWhenDone bool) *Channel {
	go func() {
		for {
			v := ChanRecv(from)
			if v == nil {
				if closeWhenDone {
					ChanClose(to)
				}
				return
			}
			if !ChanSend(to, v) {
				ChanClose(from) // to closed → stop reading from
				return
			}
		}
	}()
	return to
}

// Merge implements (merge chans) / (merge chans buf-or-n): a channel that
// carries every value from every input channel and closes once ALL inputs
// have closed. One pump goroutine per input, a WaitGroup coordinator for
// the close (oracle merge => sorted [1 2 3 4]).
func MergeChans(chans []*Channel, buf int) *Channel {
	out := NewChan(buf)
	var wg sync.WaitGroup
	for _, c := range chans {
		wg.Add(1)
		go func(c *Channel) {
			defer wg.Done()
			for {
				v := ChanRecv(c)
				if v == nil {
					return
				}
				if !ChanSend(out, v) {
					return // out closed
				}
			}
		}(c)
	}
	go func() {
		wg.Wait()
		ChanClose(out)
	}()
	return out
}

// MapChans implements (map f chs) / (map f chs buf-or-n): a fresh channel
// that, each round, takes ONE value from every input channel (in order) and
// delivers (apply f values). It closes as soon as ANY input closes — the
// value already taken from earlier inputs that round is discarded (JVM
// parity: oracle map-sum => 11 22 33 nil, map-uneven => [1 10] [2 20] nil).
// An empty chs vector closes the output immediately (oracle map-empty =>
// nil). buf-or-n sizes the output channel (nil/0 = unbuffered).
func MapChans(f any, chans []*Channel, buf int) *Channel {
	out := NewChan(buf)
	if len(chans) == 0 {
		ChanClose(out)
		return out
	}
	go func() {
		vals := make([]any, len(chans))
		for {
			for i, c := range chans {
				v := ChanRecv(c)
				if v == nil {
					ChanClose(out) // any input closed → close out (JVM parity)
					return
				}
				vals[i] = v
			}
			args := make([]any, len(vals))
			copy(args, vals)
			if !ChanSend(out, Apply(f, args)) {
				return // out closed under us
			}
		}
	}()
	return out
}

// Split implements (split p ch) / (split p ch t-buf f-buf): returns two
// channels [tc fc]; each value goes to tc when (p v) is truthy, else fc.
// Both close when ch closes (oracle split => [2 4 6] / [1 3 5]).
func Split(pred any, ch *Channel, tbuf, fbuf int) (*Channel, *Channel) {
	tc := NewChan(tbuf)
	fc := NewChan(fbuf)
	go func() {
		for {
			v := ChanRecv(ch)
			if v == nil {
				ChanClose(tc)
				ChanClose(fc)
				return
			}
			dst := fc
			if IsTruthy(Apply1(pred, v)) {
				dst = tc
			}
			if !ChanSend(dst, v) {
				return // destination closed — JVM parity: stop, leave the other
			}
		}
	}()
	return tc, fc
}

// ChanInto implements (into coll ch): a channel delivering ONE value — coll
// with every value from ch conj'd on, after ch closes. Reduce over conj,
// so order/collection semantics match clojure.core `into` (oracle
// into => [1 2 3], into-list => (3 2 1), into-set => #{1 2 3}).
func ChanInto(coll any, ch *Channel) *Channel {
	out := NewChan(1)
	conj := NSCore.FindInternedVar(NewSymbol("conj")).Deref()
	go func() {
		acc := coll
		for {
			v := ChanRecv(ch)
			if v == nil {
				break
			}
			acc = Apply2(conj, acc, v)
		}
		if acc != nil {
			ChanSend(out, acc)
		}
		ChanClose(out)
	}()
	return out
}

// ChanReduce implements (reduce f init ch): a channel delivering the single
// fold result after ch closes; the `reduced` box short-circuits (oracle
// reduce => 10, reduce-reduced => 3, reduce-empty => the init).
func ChanReduce(f, init any, ch *Channel) *Channel {
	out := NewChan(1)
	go func() {
		acc := init
		for {
			v := ChanRecv(ch)
			if v == nil {
				break
			}
			acc = Apply2(f, acc, v)
			if r, ok := acc.(*Reduced); ok {
				acc = r.Deref()
				break
			}
		}
		if acc != nil {
			ChanSend(out, acc)
		}
		ChanClose(out)
	}()
	return out
}

// ChanTransduce implements (transduce xform f init ch): reduce with a
// transducer, calling the completion (1-arity) step at the end — oracle
// transduce => 9 ((map inc) over [1 2 3] summed).
func ChanTransduce(xform, f, init any, ch *Channel) *Channel {
	out := NewChan(1)
	rf := Apply1(xform, f)
	go func() {
		acc := init
		for {
			v := ChanRecv(ch)
			if v == nil {
				break
			}
			acc = Apply2(rf, acc, v)
			if r, ok := acc.(*Reduced); ok {
				acc = r.Deref()
				break
			}
		}
		acc = Apply1(rf, acc) // completion arity
		if acc != nil {
			ChanSend(out, acc)
		}
		ChanClose(out)
	}()
	return out
}

// ChanTake implements (take n ch) / (take n ch buf-or-n): a channel that
// delivers at most n values from ch then closes (oracle take => first 3,
// take-more-than => all when fewer, take-0 => []).
func ChanTake(n int, ch *Channel, buf int) *Channel {
	out := NewChan(buf)
	go func() {
		for i := 0; i < n; i++ {
			v := ChanRecv(ch)
			if v == nil {
				break
			}
			if !ChanSend(out, v) {
				break
			}
		}
		ChanClose(out)
	}()
	return out
}

// ---------------------------------------------------------------------
// pipeline / pipeline-blocking / pipeline-async  (ADR 0040 §2 tier T3)
// ---------------------------------------------------------------------
//
// A pipeline reads values from `from`, transforms each with parallelism
// `n`, and writes the results to `to` IN INPUT ORDER, closing `to` when
// `from` drains (unless close?=false). It returns a completion channel
// that closes once every result has been flushed to `to` — JVM parity
// (core.async's pipeline* returns the go-loop's result channel; oracle
// done-yields => nil then nil).
//
// Order preservation with n>1 is the contract: a transform that sleeps
// longer on earlier inputs still emits in input order (oracle
// order => [0 10 … 70] for sleeps inversely proportional to input). The
// mechanism mirrors core.async: the dispatcher, reading `from` serially,
// creates ONE result channel per input and enqueues it on the bounded
// `results` channel in input order; n workers fill those result channels
// concurrently; a single writer drains `results` — hence each input's
// result channel — strictly in order into `to`.
//
// pipeline vs pipeline-blocking: on the JVM they differ ONLY by executor
// (a bounded compute pool vs the unbounded blocking pool). On the Go host
// every worker is a goroutine, so the distinction collapses (ADR 0040 #9)
// — both call pipelineImpl and are observably identical here. The xform
// is applied PER INPUT on a fresh (chan 1 xf ex-handler) transducer
// channel, so a stateful/aggregating transducer does NOT accumulate across
// inputs (oracle statefulxf: (partition-all 2) => [[1] [2] [3] [4]]).

type pipelineJob struct {
	v   any
	res *Channel
}

// pipelineImpl is the shared engine. newRes creates a per-input result
// channel; fill (run on a worker goroutine) populates it with the input's
// 0+ output values and closes it.
func pipelineImpl(n int, to, from *Channel, closeWhenDone bool, newRes func() *Channel, fill func(any, *Channel)) *Channel {
	jobs := make(chan pipelineJob, n)
	results := NewChan(n)
	done := NewChan(1)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				fill(j.v, j.res)
			}
		}()
	}
	// results closes once every worker has drained its jobs — every result
	// channel has already been enqueued on results by then (the dispatcher
	// sends each res before it closes jobs), so nothing is lost.
	go func() {
		wg.Wait()
		ChanClose(results)
	}()

	// dispatcher: read from serially, enqueue a result channel per input in
	// input order (onto both jobs and results).
	go func() {
		for {
			v := ChanRecv(from)
			if v == nil {
				close(jobs)
				return
			}
			res := newRes()
			jobs <- pipelineJob{v, res}
			ChanSend(results, res)
		}
	}()

	// writer: drain each result channel fully, in order, into to.
	go func() {
		for {
			r := ChanRecv(results)
			if r == nil {
				if closeWhenDone {
					ChanClose(to)
				}
				ChanClose(done)
				return
			}
			res := r.(*Channel)
			for {
				v := ChanRecv(res)
				if v == nil {
					break
				}
				if !ChanSend(to, v) {
					break // to closed downstream — stop forwarding this result
				}
			}
		}
	}()
	return done
}

// Pipeline implements (pipeline n to xf from) / (+ close?) / (+ ex-handler)
// and (pipeline-blocking …) — the same engine on Go. Each input is
// transduced on a fresh (chan 1 xf ex-handler) channel: an ex-handler
// (nil = drop-and-continue, JVM logs+drops) receives a thrown exception and
// its return replaces the value (oracle exh => [10 :handled 30], exhdrop
// and exdefault => [10 30]).
func Pipeline(n int, to *Channel, xf any, from *Channel, closeWhenDone bool, exh any) *Channel {
	return pipelineImpl(n, to, from, closeWhenDone,
		func() *Channel {
			res := NewChan(1)
			res.SetXform(xf, exh)
			return res
		},
		func(v any, res *Channel) {
			ChanSend(res, v)
			ChanClose(res)
		})
}

// PipelineAsync implements (pipeline-async n to af from) / (+ close?): the
// async fn af is (fn [val result-ch]) and delivers 0+ results to result-ch
// and closes it (oracle async => [100 101 200 201 300 301], async-zero =>
// [10 30] when af emits nothing for even inputs). af is expected to be
// non-blocking (it spawns its own go block), so the worker returns as soon
// as af does.
func PipelineAsync(n int, to *Channel, af any, from *Channel, closeWhenDone bool) *Channel {
	return pipelineImpl(n, to, from, closeWhenDone,
		func() *Channel { return NewChan(1) },
		func(v any, res *Channel) { Apply2(af, v, res) })
}

// ---------------------------------------------------------------------
// mult / tap  (ADR 0040 §2.3)
// ---------------------------------------------------------------------

// Mult is the fan-out distributor of (mult ch): every tapped channel
// receives every value. Puts to taps are blocking, so a slow tap applies
// backpressure to the whole mult (JVM parity: slow-tap-blocks-all). A tap
// whose put returns false (the tap was closed) is auto-removed. When ch
// closes, taps registered with close?=true are closed (oracle
// mult-close-propagates, tap-noclose).
type Mult struct {
	ch   *Channel
	mu   sync.Mutex
	taps map[*Channel]bool // tap channel -> close-when-source-closes?
}

func NewMult(ch *Channel) *Mult {
	m := &Mult{ch: ch, taps: map[*Channel]bool{}}
	go m.pump()
	return m
}

func (m *Mult) pump() {
	for {
		v := ChanRecv(m.ch)
		if v == nil {
			m.mu.Lock()
			for t, closeIt := range m.taps {
				if closeIt {
					ChanClose(t)
				}
			}
			m.mu.Unlock()
			return
		}
		m.mu.Lock()
		taps := make([]*Channel, 0, len(m.taps))
		for t := range m.taps {
			taps = append(taps, t)
		}
		m.mu.Unlock()
		for _, t := range taps {
			if !ChanSend(t, v) {
				m.Untap(t) // tap closed — drop it
			}
		}
	}
}

func (m *Mult) Tap(ch *Channel, closeWhenDone bool) {
	m.mu.Lock()
	m.taps[ch] = closeWhenDone
	m.mu.Unlock()
}

func (m *Mult) Untap(ch *Channel) {
	m.mu.Lock()
	delete(m.taps, ch)
	m.mu.Unlock()
}

func (m *Mult) UntapAll() {
	m.mu.Lock()
	m.taps = map[*Channel]bool{}
	m.mu.Unlock()
}

// ---------------------------------------------------------------------
// pub / sub  (ADR 0040 §2.4)
// ---------------------------------------------------------------------

type pubTopic struct {
	topic any
	ch    *Channel
	mult  *Mult
}

// Pub is (pub ch topic-fn) / (pub ch topic-fn buf-fn): a per-topic Mult
// fed by a pump that routes each source value to (topic-fn v)'s topic. A
// value whose topic has no subscribers is dropped. Subscribing a channel
// taps that topic's mult; unsubscribing untaps it. When the source closes,
// every topic channel is closed, propagating through the mults to the
// subscribers (oracle pub-a/pub-b, pub-drop-unsubbed, unsub-drops,
// unsub-all).
type Pub struct {
	ch      *Channel
	topicFn any
	bufFn   any // topic -> buffer size, or nil for unbuffered topics
	mu      sync.Mutex
	topics  []*pubTopic
}

func NewPub(ch *Channel, topicFn, bufFn any) *Pub {
	p := &Pub{ch: ch, topicFn: topicFn, bufFn: bufFn, topics: nil}
	go p.pump()
	return p
}

// findTopic must be called with p.mu held.
func (p *Pub) findTopic(topic any) *pubTopic {
	for _, t := range p.topics {
		if Equals(t.topic, topic) {
			return t
		}
	}
	return nil
}

func (p *Pub) pump() {
	for {
		v := ChanRecv(p.ch)
		if v == nil {
			p.mu.Lock()
			for _, t := range p.topics {
				ChanClose(t.ch)
			}
			p.mu.Unlock()
			return
		}
		topic := Apply1(p.topicFn, v)
		p.mu.Lock()
		t := p.findTopic(topic)
		p.mu.Unlock()
		if t != nil {
			ChanSend(t.ch, v) // fans out via the topic's mult
		}
		// no subscribers for this topic → dropped
	}
}

func (p *Pub) Sub(topic any, ch *Channel, closeWhenDone bool) {
	p.mu.Lock()
	t := p.findTopic(topic)
	if t == nil {
		buf := 0
		if p.bufFn != nil {
			if n, ok := Apply1(p.bufFn, topic).(int64); ok {
				buf = int(n)
			}
		}
		tch := NewChan(buf)
		t = &pubTopic{topic: topic, ch: tch, mult: NewMult(tch)}
		p.topics = append(p.topics, t)
	}
	p.mu.Unlock()
	t.mult.Tap(ch, closeWhenDone)
}

func (p *Pub) Unsub(topic any, ch *Channel) {
	p.mu.Lock()
	t := p.findTopic(topic)
	p.mu.Unlock()
	if t != nil {
		t.mult.Untap(ch)
	}
}

// UnsubAll implements (unsub-all p) and (unsub-all p topic): untap every
// subscriber of the given topic, or of all topics when hasTopic is false.
func (p *Pub) UnsubAll(hasTopic bool, topic any) {
	p.mu.Lock()
	targets := make([]*Mult, 0, len(p.topics))
	for _, t := range p.topics {
		if !hasTopic || Equals(t.topic, topic) {
			targets = append(targets, t.mult)
		}
	}
	p.mu.Unlock()
	for _, m := range targets {
		m.UntapAll()
	}
}

// ---------------------------------------------------------------------
// mix  (ADR 0040 §2.5)
// ---------------------------------------------------------------------

type mixInputState struct{ mute, pause, solo bool }

// Mix is the stateful fan-in of (mix out): admixed input channels are
// merged into out, with per-input mute (consumed, not forwarded), pause
// (not consumed), and solo (when any input is soloed, non-solo inputs are
// muted or paused per solo-mode). The pump re-reads its snapshot whenever
// state changes (a signal on the `change` channel wakes the alts), so a
// channel added via toggle in a target state is never read in any other
// state — JVM parity, and what makes the mute/pause/solo behaviours
// deterministic (oracle mix-mute, mix-pause, mix-solo-mute,
// mix-solo-pause, unmix, unmix-all).
type Mix struct {
	out      *Channel
	mu       sync.Mutex
	inputs   map[*Channel]*mixInputState
	soloMode Keyword
	change   *Channel
}

var mixSoloModeMute = NewKeyword("mute")
var mixSoloModePause = NewKeyword("pause")
var mixKwMute = NewKeyword("mute")
var mixKwPause = NewKeyword("pause")
var mixKwSolo = NewKeyword("solo")

func NewMix(out *Channel) *Mix {
	m := &Mix{
		out:      out,
		inputs:   map[*Channel]*mixInputState{},
		soloMode: mixSoloModeMute,
		change:   NewChan(1),
	}
	go m.pump()
	return m
}

// signal wakes the pump to re-snapshot; the change channel is buffered 1
// so coalesced signals never block a caller and never get lost.
func (m *Mix) signal() { ChanOffer(m.change, true) }

func (m *Mix) pump() {
	for {
		m.mu.Lock()
		anySolo := false
		for _, s := range m.inputs {
			if s.solo {
				anySolo = true
				break
			}
		}
		soloModeMute := m.soloMode != mixSoloModePause
		ports := []any{m.change}
		forward := map[*Channel]bool{}
		for ch, s := range m.inputs {
			var readIt, fwd bool
			if anySolo {
				if s.solo {
					readIt, fwd = true, true
				} else if soloModeMute {
					readIt, fwd = true, false // muted
				} else {
					readIt = false // paused
				}
			} else if s.pause {
				readIt = false
			} else if s.mute {
				readIt, fwd = true, false
			} else {
				readIt, fwd = true, true
			}
			if readIt {
				ports = append(ports, ch)
				forward[ch] = fwd
			}
		}
		m.mu.Unlock()

		res := Alts(ports, false, nil, false)
		port := res.Nth(1)
		if port == any(m.change) {
			continue // state changed → re-snapshot
		}
		pch, _ := port.(*Channel)
		val := res.Nth(0)
		if val == nil {
			// input closed and drained → drop it from the mix
			m.mu.Lock()
			delete(m.inputs, pch)
			m.mu.Unlock()
			continue
		}
		if forward[pch] {
			if !ChanSend(m.out, val) {
				return // out closed → mix stops (JVM parity)
			}
		}
	}
}

func (m *Mix) Admix(ch *Channel) {
	m.mu.Lock()
	if _, ok := m.inputs[ch]; !ok {
		m.inputs[ch] = &mixInputState{}
	}
	m.mu.Unlock()
	m.signal()
}

func (m *Mix) Unmix(ch *Channel) {
	m.mu.Lock()
	delete(m.inputs, ch)
	m.mu.Unlock()
	m.signal()
}

func (m *Mix) UnmixAll() {
	m.mu.Lock()
	m.inputs = map[*Channel]*mixInputState{}
	m.mu.Unlock()
	m.signal()
}

func (m *Mix) SoloMode(mode Keyword) {
	m.mu.Lock()
	m.soloMode = mode
	m.mu.Unlock()
	m.signal()
}

// Toggle merges a {channel -> {:mute :pause :solo}} state map into the
// mix, adding channels not yet present — the atomic add-in-a-state that
// makes state changes race-free (JVM parity).
func (m *Mix) Toggle(stateMap any) {
	m.mu.Lock()
	for s := Seq(stateMap); !IsNil(s); s = s.Next() {
		entry := s.First().(IMapEntry)
		ch, ok := entry.Key().(*Channel)
		if !ok {
			continue
		}
		st := entry.Val()
		cur := m.inputs[ch]
		if cur == nil {
			cur = &mixInputState{}
			m.inputs[ch] = cur
		}
		if v := Get(st, mixKwMute); v != nil {
			cur.mute = IsTruthy(v)
		}
		if v := Get(st, mixKwPause); v != nil {
			cur.pause = IsTruthy(v)
		}
		if v := Get(st, mixKwSolo); v != nil {
			cur.solo = IsTruthy(v)
		}
	}
	m.mu.Unlock()
	m.signal()
}
