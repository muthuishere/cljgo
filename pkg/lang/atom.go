package lang

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type (
	Atom struct {
		state   atomic.Value
		watches IPersistentMap

		meta IPersistentMap

		// validator + its mutex: SetValidator/Validator and every mutation
		// (Reset/CompareAndSet/Swap) take validatorMtx so a concurrent
		// set-validator! can't race a swap! reading a half-updated fn
		// (ADR 0022 batch E — atom validator arity, design/08 §5).
		validatorMtx sync.RWMutex
		validator    IFn
	}
)

var (
	_ IAtom2 = (*Atom)(nil)
	_ IRef   = (*Atom)(nil)
)

func NewAtom(val any) *Atom {
	a := &Atom{}
	a.state.Store(Box{val})
	a.watches = emptyMap
	return a
}

func NewAtomWithMeta(val any, meta IPersistentMap) *Atom {
	a := NewAtom(val)
	if meta != nil {
		a.meta = meta
	}
	return a
}

// NewAtomWithValidator is like NewAtom but also installs a validator,
// checked against the initial value exactly like SetValidator does
// (Clojure: (atom v :validator vf) throws immediately if vf rejects v).
func NewAtomWithValidator(val any, vf IFn) *Atom {
	a := NewAtom(val)
	a.SetValidator(vf)
	return a
}

func (a *Atom) Deref() interface{} {
	return a.state.Load().(Box).val
}

// validate runs the current validator (if any) against val, panicking
// with Clojure's "Invalid reference state" message on rejection — either
// the validator returning falsey or itself throwing.
func (a *Atom) validate(val any) {
	a.validatorMtx.RLock()
	vf := a.validator
	a.validatorMtx.RUnlock()
	if vf == nil {
		return
	}
	if !BooleanCast(vf.Invoke(val)) {
		panic(fmt.Errorf("Invalid reference state"))
	}
}

func (a *Atom) SetValidator(vf IFn) {
	a.validatorMtx.Lock()
	defer a.validatorMtx.Unlock()
	if vf != nil {
		if !BooleanCast(vf.Invoke(a.Deref())) {
			panic(fmt.Errorf("Invalid reference state"))
		}
	}
	a.validator = vf
}

func (a *Atom) Validator() IFn {
	a.validatorMtx.RLock()
	defer a.validatorMtx.RUnlock()
	return a.validator
}

func (a *Atom) Watches() IPersistentMap {
	return a.watches
}

func (a *Atom) AddWatch(key interface{}, fn IFn) IRef {
	a.watches = a.watches.Assoc(key, fn).(IPersistentMap)
	return a
}

func (a *Atom) RemoveWatch(key interface{}) {
	a.watches = a.watches.Without(key)
}

func (a *Atom) notifyWatches(oldVal, newVal interface{}) {
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

func (a *Atom) Swap(f IFn, args ISeq) interface{} {
	for {
		old := a.state.Load().(Box)
		nw := f.ApplyTo(NewCons(old.val, args))
		if a.CompareAndSet(old.val, nw) {
			return nw
		}
	}
}

func (a *Atom) CompareAndSet(oldv, newv interface{}) bool {
	a.validate(newv)
	swapped := a.state.CompareAndSwap(Box{val: oldv}, Box{val: newv})
	if swapped {
		a.notifyWatches(oldv, newv)
	}
	return swapped
}

func (a *Atom) Reset(newVal interface{}) interface{} {
	a.validate(newVal)
	old := a.state.Load().(Box)

	a.state.Store(Box{newVal})
	a.notifyWatches(old.val, newVal)
	return newVal
}

func (a *Atom) Meta() IPersistentMap {
	if a.meta == nil {
		return nil
	}
	return a.meta
}

// SetMeta installs meta wholesale (used by the `atom` builtin's :meta
// option, ADR 0022 batch E) — unlike Var's SetMeta, no merge/ns-tagging.
func (a *Atom) SetMeta(meta IPersistentMap) {
	a.meta = meta
}
