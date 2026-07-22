package lang

// agent.go — queue-backed agents + goroutine futures (ADR 0038 completed
// the vendored Glojure stub: agents got a state cell and a serialized
// action queue, futures got cooperative cancellation; surgery logged in
// PROVENANCE.md).

import (
	"sync"
	"time"
)

type (
	// Agent is a value cell whose mutations (send/send-off actions) are
	// serialized by one dedicated goroutine draining its queue — actions
	// for one agent run in send order, never concurrently. send and
	// send-off are the same operation: goroutines have no bounded-pool vs
	// new-thread distinction (the go/thread collapse, design/05 §4).
	Agent struct {
		meta IPersistentMap

		mtx     sync.Mutex
		state   any
		watches IPersistentMap

		// err is the stored throwable once an action (or one of its watch
		// notifications) has panicked in :fail mode — nil while the agent is
		// :ready or when the mode is :continue.
		err error

		// errorMode is :fail or :continue (fundamentals audit 2026-07,
		// oracle 1.12.5). Default :fail, or :continue when an error-handler
		// is supplied at construction. In :continue mode a throwing action
		// does NOT fail the agent — the error is reported to the handler (if
		// any) and the queue keeps draining. errorHandler, when non-nil, is
		// called (agent, throwable) on every action/watch error regardless
		// of mode; its own panics are swallowed.
		errorMode    Keyword
		errorHandler IFn

		queue chan func()
	}

	future struct {
		done chan struct{}
		res  interface{}

		// settle guards the ONE completion: normal body completion and
		// future-cancel race for it; the loser is a no-op (ADR 0038).
		settle    sync.Once
		cancelled bool
	}
)

var (
	_ IRef = (*Agent)(nil)

	_ IBlockingDeref = (*future)(nil)
	_ IDeref         = (*future)(nil)
	_ IPending       = (*future)(nil)
	_ Future         = (*future)(nil)
)

func (f *future) Deref() interface{} {
	<-f.done
	if p, ok := f.res.(*futurePanic); ok {
		panic(p.recovered)
	}
	return f.res
}

func (f *future) DerefWithTimeout(timeoutMS int64, timeoutVal interface{}) interface{} {
	select {
	case <-f.done:
		return f.res
	case <-time.After(time.Duration(timeoutMS) * time.Millisecond):
		return timeoutVal
	}
}

func (f *future) Get() interface{} {
	return f.Deref()
}

func (f *future) GetWithTimeout(timeout int64, timeUnit time.Duration) interface{} {
	select {
	case <-f.done:
		return f.res
	case <-time.After(time.Duration(timeout) * time.Millisecond):
		panic(NewTimeoutError("future timeout"))
	}
}

func (f *future) IsRealized() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

// settleWith completes the future with res exactly once; it reports
// whether THIS call won the settle.
func (f *future) settleWith(res interface{}) bool {
	won := false
	f.settle.Do(func() {
		f.res = res
		close(f.done)
		won = true
	})
	return won
}

// Cancel backs future-cancel (ADR 0038; JVM oracle Clojure 1.12.5:
// cancelling a completed future => false, a pending one => true, after
// which realized?/future-cancelled? are true and deref throws
// CancellationException). Cancellation is cooperative-only: the body
// goroutine is NOT interrupted — it runs to completion and its result is
// discarded by the already-settled sync.Once.
func (f *future) Cancel() bool {
	won := false
	f.settle.Do(func() {
		f.cancelled = true // before close(done): visible to post-realized readers
		f.res = &futurePanic{NewIllegalStateError("future-cancel: the future was cancelled")}
		close(f.done)
		won = true
	})
	return won
}

// IsCancelled backs future-cancelled?.
func (f *future) IsCancelled() bool {
	return f.cancelled
}

////////////////////////////////////////////////////////////////////////////////
// Agent

// kwFail / kwContinue are the two error-mode values.
var (
	kwFail     = NewKeyword("fail")
	kwContinue = NewKeyword("continue")
)

// NewAgent builds an agent holding val and starts its queue-draining
// goroutine (never shut down — ShutdownAgents remains a no-op). Default
// error-mode :fail, no error-handler; use SetErrorMode/SetErrorHandler
// (the agent builtin's :error-mode/:error-handler options) to change them.
func NewAgent(val any) *Agent {
	a := &Agent{state: val, watches: emptyMap, errorMode: kwFail, queue: make(chan func(), 32)}
	go func() {
		for act := range a.queue {
			act()
		}
	}()
	return a
}

// ErrorMode backs error-mode: :fail or :continue.
func (a *Agent) ErrorMode() Keyword {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.errorMode
}

// SetErrorMode backs set-error-mode! — validated to :fail/:continue by the
// caller (the builtin), matching clojure.core's {:pre} on the mode.
func (a *Agent) SetErrorMode(mode Keyword) {
	a.mtx.Lock()
	a.errorMode = mode
	a.mtx.Unlock()
}

// ErrorHandler backs error-handler: the fn, or nil.
func (a *Agent) ErrorHandler() IFn {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.errorHandler
}

// SetErrorHandler backs set-error-handler!.
func (a *Agent) SetErrorHandler(fn IFn) {
	a.mtx.Lock()
	a.errorHandler = fn
	a.mtx.Unlock()
}

func (a *Agent) Deref() any {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.state
}

// Send backs send/send-off: enqueue (apply f state args); the new value is
// installed and watches fire when the action runs, in send order. A
// FAILED agent rejects the send synchronously (oracle: clojure 1.12.5,
// 2026-07-17 — "Agent is failed, needs restart"), matching the JVM's
// default :fail error-mode.
func (a *Agent) Send(f IFn, args ISeq) *Agent {
	a.mtx.Lock()
	failed := a.err != nil
	a.mtx.Unlock()
	if failed {
		panic(NewIllegalStateError("Agent is failed, needs restart"))
	}
	a.queue <- func() { a.runAction(f, args) }
	return a
}

// runAction runs one queued action. If it panics, the OLD state is kept
// (the action never installed a new one) and the panic is stored as the
// agent's error. Otherwise the new state installs first, then watches
// fire; a panicking watch ALSO fails the agent, but the state stays
// installed (oracle: clojure 1.12.5, 2026-07-17 — the state write and the
// watch notification are two separate steps on the JVM too). An action
// queued before a prior one failed is dropped rather than run — the JVM
// holds pending actions for a restart-agent with :clear-actions false to
// resume, which cljgo does not model (documented gap; unreached by the
// suite).
func (a *Agent) runAction(f IFn, args ISeq) {
	a.mtx.Lock()
	if a.err != nil {
		a.mtx.Unlock()
		return
	}
	old := a.state
	a.mtx.Unlock()

	var nw any
	failed := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				a.recordError(r)
				failed = true
			}
		}()
		nw = f.ApplyTo(NewCons(old, args))
	}()
	if failed {
		// action threw: the new state was never produced, old state stays.
		// In :continue mode the agent is still :ready and keeps draining.
		return
	}

	a.mtx.Lock()
	a.state = nw
	a.mtx.Unlock()

	func() {
		defer func() {
			if r := recover(); r != nil {
				a.recordError(r)
			}
		}()
		a.notifyWatches(old, nw)
	}()
}

// recordError handles an action/watch panic per the agent's error-mode
// (oracle 1.12.5, fundamentals audit 2026-07): the error-handler, if any,
// is ALWAYS called (agent, throwable) — in both modes — and its own panic
// is swallowed; then in :fail mode the throwable is stored (agent enters
// :failed, rejecting further sends), while in :continue mode it is dropped
// and the queue keeps draining. The agent's default mode is :continue when
// an error-handler is supplied at construction, else :fail — the builtin
// resolves that; here the stored mode is authoritative.
func (a *Agent) recordError(r any) {
	e := asAgentError(r)
	a.mtx.Lock()
	handler := a.errorHandler
	mode := a.errorMode
	a.mtx.Unlock()
	if handler != nil {
		func() {
			defer func() { _ = recover() }()
			handler.Invoke(a, e)
		}()
	}
	if mode == kwContinue {
		return
	}
	a.mtx.Lock()
	a.err = e
	a.mtx.Unlock()
}

// asAgentError normalizes a recovered panic into the throwable stored as
// the agent's error (agent-error). cljgo panics already carry an `error`
// (eval.Throw wraps any thrown non-error Clojure value before panicking),
// so this is a defensive fallback for a stray non-error panic.
func asAgentError(r any) error {
	if err, ok := r.(error); ok {
		return err
	}
	return NewError(ToString(r))
}

// Await backs await: block until every action sent to this agent BEFORE
// the call has run (a latch action through the same queue, like the JVM's
// CountDownLatch send). A FAILED agent rejects await, same as Send
// (oracle-verified) — checked both before enqueueing the latch (already
// failed) and after it fires (one of the actions ahead of the latch
// failed while this call was waiting).
func (a *Agent) Await() {
	a.mtx.Lock()
	failed := a.err != nil
	a.mtx.Unlock()
	if failed {
		panic(NewIllegalStateError("Agent is failed, needs restart"))
	}
	done := make(chan struct{})
	a.queue <- func() { close(done) }
	<-done

	a.mtx.Lock()
	failed = a.err != nil
	a.mtx.Unlock()
	if failed {
		panic(NewIllegalStateError("Agent is failed, needs restart"))
	}
}

// AwaitForOne backs await-for for a single agent: block until every action
// sent before the call has run, but no later than the shared absolute
// deadline. Reports whether the agent drained in time (true) or the
// deadline hit first (false). A failed agent short-circuits to false —
// like Await it cannot make progress, but await-for returns false rather
// than throwing (oracle 1.12.5, 2026-07-21).
func (a *Agent) AwaitForOne(deadline time.Time) bool {
	a.mtx.Lock()
	failed := a.err != nil
	a.mtx.Unlock()
	if failed {
		return false
	}
	done := make(chan struct{})
	a.queue <- func() { close(done) }
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-done:
		a.mtx.Lock()
		failed = a.err != nil
		a.mtx.Unlock()
		return !failed
	case <-timer.C:
		return false
	}
}

// AgentError backs agent-error: the stored throwable, or nil while ready.
func (a *Agent) AgentError() error {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.err
}

// Restart backs restart-agent: install newState and clear the error,
// returning newState. Throws on a non-failed agent (oracle: clojure
// 1.12.5, 2026-07-17 — "Agent does not need a restart").
func (a *Agent) Restart(newState any) any {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if a.err == nil {
		panic(NewIllegalStateError("Agent does not need a restart"))
	}
	a.state = newState
	a.err = nil
	return newState
}

func (a *Agent) SetValidator(vf IFn) {
	panic(NewIllegalStateError("agent validators are not implemented (ADR 0038)"))
}

func (a *Agent) Validator() IFn {
	return nil
}

// Meta / SetMeta back the agent's IObj metadata (the :meta constructor
// option); stored on the cell, unused by the runtime otherwise.
func (a *Agent) Meta() IPersistentMap { return a.meta }
func (a *Agent) SetMeta(m IPersistentMap) {
	a.mtx.Lock()
	a.meta = m
	a.mtx.Unlock()
}

func (a *Agent) Watches() IPersistentMap {
	return a.watches
}

func (a *Agent) AddWatch(key any, fn IFn) IRef {
	a.watches = a.watches.Assoc(key, fn).(IPersistentMap)
	return a
}

func (a *Agent) RemoveWatch(key interface{}) {
	a.watches = a.watches.Without(key)
}

func (a *Agent) notifyWatches(oldVal, newVal interface{}) {
	watches := a.watches
	if watches == nil || watches.Count() == 0 {
		return
	}

	for seq := watches.Seq(); seq != nil; seq = seq.Next() {
		entry := seq.First().(IMapEntry)
		key := entry.Key()
		fn := entry.Val().(IFn)
		// Call watch function with key, ref, old-state, new-state
		fn.Invoke(key, a, oldVal, newVal)
	}
}

////////////////////////////////////////////////////////////////////////////////

func ShutdownAgents() {
	// TODO
}

// AgentSubmit runs fn in a new goroutine and returns a future satisfying
// deref (blocking) and realized? (IPending) — the `future` builtin's
// substrate (design/08 batch E, ADR 0022). It CONVEYS the calling
// goroutine's dynamic-var bindings into the spawned one via
// CloneThreadBindingFrame/ResetThreadBindingFrame, matching real
// Clojure's future (and bound-fn's own use of the same primitive):
// (binding [*x* :v] @(future *x*)) sees :v even though the goroutine
// running the body is a different one. A panic in fn is recovered and
// re-raised on Deref, exactly like a real ExecutionException would be
// unwrapped and rethrown by @fut.
func AgentSubmit(fn IFn) IBlockingDeref {
	frame := CloneThreadBindingFrame()
	fut := &future{
		done: make(chan struct{}),
	}
	go func() {
		ResetThreadBindingFrame(frame)
		var res interface{}
		func() {
			defer func() {
				if r := recover(); r != nil {
					res = &futurePanic{r}
				}
			}()
			res = fn.Invoke()
		}()
		fut.settleWith(res)
	}()
	return fut
}

// futurePanic wraps a recovered panic so Deref can re-raise it in the
// DEREFING goroutine instead of silently returning the panic value as
// data (matching @future rethrowing the worker's exception).
type futurePanic struct{ recovered any }
