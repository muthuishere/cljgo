package lang

// ref.go — STM-lite refs (ADR 0038). Rewritten from the vendored Glojure
// stub (which had a lock-free, transaction-count-only sketch); surgery
// logged in PROVENANCE.md.
//
// A Ref is a mutex-guarded cell with watches. Transactions are one global
// lock: dosync (via RunInTransaction) holds it for the body and marks the
// goroutine in-transaction through a dynamic var, so the mark conveys into
// futures/bound-fns exactly like any other binding and nested dosync joins
// the outer transaction. alter/ref-set/commute demand the mark — outside a
// transaction they throw "No transaction running" (JVM oracle:
// IllegalStateException, Clojure 1.12.5). No MVCC, no retries; watches
// fire per mutation (deviations logged in the ADR).

import (
	"sync"
)

type Ref struct {
	mtx       sync.Mutex
	state     any
	watches   IPersistentMap
	validator IFn
}

var (
	_ IRef   = (*Ref)(nil)
	_ IDeref = (*Ref)(nil)

	// stmMtx is THE transaction lock: dosync bodies serialize on it.
	stmMtx sync.Mutex

	// varInTx marks "a transaction is running on this goroutine" via the
	// thread-binding frame. Interned private-style (`-` prefix) in
	// clojure.core like the other runtime-substrate vars.
	varInTx = InternVarReplaceRoot(NSCore, NewSymbol("-in-transaction"), false).SetDynamic()
)

func NewRef(val any) *Ref {
	return &Ref{state: val, watches: emptyMap}
}

// RunInTransaction backs dosync: nested transactions join the outer one;
// an outermost transaction takes the global lock and binds the
// in-transaction mark for the body.
func RunInTransaction(f IFn) any {
	if InTransaction() {
		return f.Invoke()
	}
	stmMtx.Lock()
	defer stmMtx.Unlock()
	PushThreadBindings(NewMap(varInTx, true))
	defer PopThreadBindings()
	return f.Invoke()
}

// InTransaction reports whether the calling goroutine is inside a dosync.
func InTransaction() bool {
	return BooleanCast(varInTx.Deref())
}

func (r *Ref) Deref() any {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.state
}

// TxAlter backs alter/commute: (apply f current-value args), in-transaction
// only.
func (r *Ref) TxAlter(f IFn, args ISeq) any {
	return r.txSwap(func(old any) any { return f.ApplyTo(NewCons(old, args)) })
}

// TxSet backs ref-set: replace the value, in-transaction only.
func (r *Ref) TxSet(val any) any {
	return r.txSwap(func(any) any { return val })
}

func (r *Ref) txSwap(f func(old any) any) any {
	if !InTransaction() {
		panic(NewIllegalStateError("No transaction running"))
	}
	r.mtx.Lock()
	old := r.state
	nw := f(old)
	if vf := r.validator; vf != nil && !BooleanCast(vf.Invoke(nw)) {
		r.mtx.Unlock()
		panic(NewIllegalStateError("Invalid reference state"))
	}
	r.state = nw
	r.mtx.Unlock()
	r.notifyWatches(old, nw)
	return nw
}

func (r *Ref) SetValidator(vf IFn) {
	if vf != nil && !BooleanCast(vf.Invoke(r.Deref())) {
		panic(NewIllegalStateError("Invalid reference state"))
	}
	r.validator = vf
}

func (r *Ref) Validator() IFn {
	return r.validator
}

func (r *Ref) Watches() IPersistentMap {
	return r.watches
}

func (r *Ref) AddWatch(key any, fn IFn) IRef {
	r.watches = r.watches.Assoc(key, fn).(IPersistentMap)
	return r
}

func (r *Ref) RemoveWatch(key any) {
	r.watches = r.watches.Without(key)
}

func (r *Ref) notifyWatches(oldVal, newVal any) {
	watches := r.watches
	if watches == nil || watches.Count() == 0 {
		return
	}
	for seq := watches.Seq(); seq != nil; seq = seq.Next() {
		entry := seq.First().(IMapEntry)
		entry.Val().(IFn).Invoke(entry.Key(), r, oldVal, newVal)
	}
}
