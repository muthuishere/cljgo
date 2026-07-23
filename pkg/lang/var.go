package lang

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/muthuishere/cljgo/pkg/lang/internal/goid"
)

type (
	Var struct {
		ns   *Namespace
		sym  *Symbol
		root atomic.Value

		meta atomic.Value

		// TODO: populate this from meta in the right places
		dynamic      bool
		dynamicBound atomic.Bool

		// isMacroCached: 0=unknown, 1=false, 2=true
		isMacroCached atomic.Int32

		// sealed marks a var whose root mutation must flip CoreArithDirty
		// (spike s43 / ADR 0066). rt.Boot seals the seven core arithmetic
		// vars (+ - * / < > =) after the pristine snapshot; a later
		// BindRoot/AlterRoot on any of them (def redefinition,
		// alter-var-root, with-redefs) trips the global dirty flag so the
		// emitted intrinsics drop their per-call guard until then.
		sealed atomic.Bool

		watches IPersistentMap

		syncLock sync.Mutex
	}

	UnboundVar struct {
		v *Var
	}

	varBindings map[*Var]*Box
	glStorage   struct {
		bindings []varBindings
	}

	// TODO: public rev counter
)

func (uv *UnboundVar) String() string {
	return "Unbound: " + uv.v.String()
}

// CoreArithDirty is the process-global "a sealed core arithmetic var has
// been redefined" flag (spike s43 / ADR 0066). It starts false and is
// tripped — monotonically — the first time BindRoot/AlterRoot mutates a
// sealed var (see Var.Seal). The emitted arithmetic intrinsics in
// pkg/emit/rt read it ONCE per call (a single relaxed atomic.Bool load
// that branch-predicts to false) instead of the per-call var deref +
// interface-compare the ADR 0004 guard used to do. While it is false the
// intrinsic open-codes the int64 op directly; once it is true the
// intrinsic falls back to the guarded (deref-and-compare) path, so a live
// redefinition is still seen — the ADR 0004 liveness escape hatch. It is
// deliberately never reset to false: correctness only requires that a
// currently-redefined var take the guarded path, and a monotonic flag
// avoids any reset race across goroutines for a one-line, worst-case-slow
// but always-correct outcome.
var CoreArithDirty atomic.Bool

var (
	NSCore = FindOrCreateNamespace(SymbolCoreNamespace)

	VarNS   = InternVar(NSCore, NewSymbol("ns"), false, true)
	VarInNS = InternVar(NSCore, NewSymbol("in-ns"), false, true)

	VarLoadFile = InternVar(NSCore, NewSymbol("load-file"), nil, true)

	VarCurrentNS        = InternVarReplaceRoot(NSCore, NewSymbol("*ns*"), NSCore).SetDynamic()
	VarWarnOnReflection = InternVarReplaceRoot(NSCore, NewSymbol("*warn-on-reflection*"), false).SetDynamic()
	VarUncheckedMath    = InternVarReplaceRoot(NSCore, NewSymbol("*unchecked-math*"), false).SetDynamic()
	VarAgent            = InternVarReplaceRoot(NSCore, NewSymbol("*agent*"), nil).SetDynamic()
	VarPrintReadably    = InternVarReplaceRoot(NSCore, NewSymbol("*print-readably*"), true).SetDynamic()
	// VarPrintLength backs *print-length* (root nil = unlimited, exactly
	// clojure.core): when bound to an int, Print emits at most that many
	// elements of a seq/vector/map/set followed by "...". cljgo addition to
	// the vendored printer (PROVENANCE.md): without it, printing an
	// infinite lazy seq (e.g. a failing clojure.test assertion whose actual
	// value is unbounded) never terminates.
	VarPrintLength  = InternVarReplaceRoot(NSCore, NewSymbol("*print-length*"), nil).SetDynamic()
	VarOut          = InternVarReplaceRoot(NSCore, NewSymbol("*out*"), os.Stdout).SetDynamic()
	VarIn           = InternVarReplaceRoot(NSCore, NewSymbol("*in*"), os.Stdin).SetDynamic()
	VarAssert       = InternVarReplaceRoot(NSCore, NewSymbol("*assert*"), false).SetDynamic()
	VarCompileFiles = InternVarReplaceRoot(NSCore, NewSymbol("*compile-files*"), false).SetDynamic()
	VarFile         = InternVarReplaceRoot(NSCore, NewSymbol("*file*"), "NO_SOURCE_FILE").SetDynamic()
	VarDataReaders  = InternVarReplaceRoot(NSCore, NewSymbol("*data-readers*"), emptyMap).SetDynamic()
	// VarMathContext backs *math-context* (ADR 0032 follow-on): root nil =
	// unbound = unlimited-precision BigDecimal arithmetic (today's default:
	// exact add/sub/mul, divide throws on non-termination). `with-precision`
	// binds it to a *MathContext (pkg/lang/bigdecimal.go); the decimal Ops
	// (numberops.go bigDecimalOps) consult it on every +/-/*//.
	VarMathContext = InternVarReplaceRoot(NSCore, NewSymbol("*math-context*"), nil).SetDynamic()

	// TODO: use variant of InternVar that doesn't replace root.
	VarPrintInitialized = InternVarName(NSCore.Name(), NewSymbol("print-initialized"))
	VarPrOn             = InternVarName(NSCore.Name(), NewSymbol("pr-on"))
	VarParents          = InternVarName(NSCore.Name(), NewSymbol("parents"))

	// TODO: use an atomic and CAS
	glsBindings    = make(map[int64]*glStorage)
	glsBindingsMtx sync.RWMutex

	_ IRef = (*Var)(nil)
	_ IFn  = (*Var)(nil)
)

func InternVarReplaceRoot(ns *Namespace, sym *Symbol, root interface{}) *Var {
	return InternVar(ns, sym, root, true)
}

func InternVar(ns *Namespace, sym *Symbol, root interface{}, replaceRoot bool) *Var {
	dvout := ns.Intern(sym)
	if !dvout.HasRoot() || replaceRoot {
		dvout.BindRoot(root)
	}
	return dvout
}

func InternVarName(nsSym, nameSym *Symbol) *Var {
	ns := FindOrCreateNamespace(nsSym)
	return ns.Intern(nameSym)
}

func NewVar(ns *Namespace, sym *Symbol) *Var {
	v := &Var{
		ns:      ns,
		sym:     sym,
		watches: emptyMap,
	}
	v.root.Store(Box{val: &UnboundVar{v: v}})
	v.meta.Store(NewBox(emptyMap))
	return v
}

func NewVarWithRoot(ns *Namespace, sym *Symbol, root interface{}) *Var {
	v := NewVar(ns, sym)
	v.BindRoot(root)
	return v
}

func (v *Var) Namespace() *Namespace {
	return v.ns
}

func (v *Var) Symbol() *Symbol {
	return v.sym
}

func (v *Var) ToSymbol() *Symbol {
	return InternSymbol(v.ns.Name().String(), v.sym.Name())
}

func (v *Var) String() string {
	return "#'" + v.ns.Name().String() + "/" + v.sym.Name()
}

func (v *Var) HasRoot() bool {
	box := v.root.Load().(Box)
	_, ok := box.val.(*UnboundVar)
	return !ok
}

// Seal marks the var so that any future root mutation trips CoreArithDirty
// (spike s43 / ADR 0066). rt.Boot calls it on the seven core arithmetic
// vars after the pristine builtin snapshot, so the boot-time BindRoots that
// install the builtins and load compiled core do NOT trip the flag — only a
// user/core redefinition after boot does. Returns the var for chaining.
func (v *Var) Seal() *Var {
	v.sealed.Store(true)
	return v
}

// tripIfSealed flips the global dirty flag when a sealed var's root moves.
// A plain load-then-maybe-store: the common case (unsealed var) is a single
// predictable bool load, and the rare sealed case only ever stores true, so
// no CAS or lock is needed.
func (v *Var) tripIfSealed() {
	if v.sealed.Load() {
		CoreArithDirty.Store(true)
	}
}

func (v *Var) BindRoot(root interface{}) {
	// TODO: handle metadata correctly
	old := v.root.Swap(Box{val: root})
	v.tripIfSealed()
	v.notifyWatches(old.(Box).val, root)
}

func (v *Var) IsBound() bool {
	return v.HasRoot() || v.dynamicBound.Load() && v.getDynamicBinding() != nil
}

func (v *Var) getRoot() interface{} {
	return v.root.Load().(Box).val
}

func (v *Var) Get() interface{} {
	if !v.dynamicBound.Load() {
		return v.getRoot()
	}
	return v.Deref()
}

func (v *Var) Set(val interface{}) interface{} {
	// TODO: validate
	b := v.getDynamicBinding()
	if b == nil {
		panic(fmt.Sprintf("can't change/establish root binding of: %s", v))
	}
	old := b.val
	b.val = val
	v.notifyWatches(old, val)
	return val
}

func (v *Var) Meta() IPersistentMap {
	return v.meta.Load().(*Box).val.(IPersistentMap)
}

func (v *Var) SetMeta(meta IPersistentMap) {
	// TODO: ResetMeta
	v.isMacroCached.Store(0) // invalidate IsMacro cache
	meta = Assoc(meta, KWNS, v.ns).(IPersistentMap)
	v.meta.Store(NewBox(meta))
}

func (v *Var) AlterMeta(alter IFn, args ISeq) IPersistentMap {
	meta := alter.ApplyTo(NewCons(v.Meta(), args)).(IPersistentMap)
	v.SetMeta(meta)
	return meta
}

func (v *Var) IsMacro() bool {
	if cached := v.isMacroCached.Load(); cached != 0 {
		return cached == 2
	}
	meta := v.Meta()
	isMacro := meta.EntryAt(KWMacro)
	result := isMacro != nil && isMacro.Val() == true
	if result {
		v.isMacroCached.Store(2)
	} else {
		v.isMacroCached.Store(1)
	}
	return result
}

func (v *Var) SetMacro() {
	v.SetMeta(v.Meta().Assoc(KWMacro, true).(IPersistentMap))
}

func (v *Var) IsPublic() bool {
	meta := v.Meta()
	isPrivate := meta.EntryAt(KWPrivate)
	if isPrivate == nil {
		return true
	}
	return !BooleanCast(isPrivate.Val())
}

func (v *Var) isDynamic() bool {
	return v.dynamic
}

func (v *Var) SetDynamic() *Var {
	v.dynamic = true
	return v
}

// SetPrivate marks the var ^:private in its metadata and returns it for
// chaining — pkg/emit's hoistVar replays compile-time :private meta into
// the compiled binary through this, exactly as SetDynamic replays
// :dynamic (fundamentals audit 2026-07: without it a compiled binary's
// ns-publics showed ^:private vars the interpreter hid).
func (v *Var) SetPrivate() *Var {
	if m := v.Meta(); m != nil {
		v.SetMeta(m.Assoc(KWPrivate, true).(IPersistentMap))
	} else {
		v.SetMeta(NewMap(KWPrivate, true))
	}
	return v
}

func (v *Var) Deref() interface{} {
	if b := v.getDynamicBinding(); b != nil {
		return b.val
	}
	return v.getRoot()
}

func (v *Var) getDynamicBinding() *Box {
	if !v.dynamicBound.Load() {
		return nil
	}
	var storage *glStorage
	gid := getGoroutineID()

	glsBindingsMtx.RLock()
	storage, ok := glsBindings[gid]
	glsBindingsMtx.RUnlock()

	if !ok {
		return nil
	}
	return storage.get(v)
}

func (v *Var) AlterRoot(alter IFn, args ISeq) interface{} {
	v.syncLock.Lock()
	defer v.syncLock.Unlock()

	oldRoot := v.Get()
	newRoot := alter.ApplyTo(NewCons(oldRoot, args))
	// TODO: validate, ++rev
	v.root.Store(Box{val: newRoot})
	v.tripIfSealed()
	v.notifyWatches(oldRoot, newRoot)
	return newRoot
}

func (v *Var) SetValidator(vf IFn) {
	panic("not implemented")
}

func (v *Var) Validator() IFn {
	panic("not implemented")
}

func (v *Var) Watches() IPersistentMap {
	return v.watches
}

func (v *Var) AddWatch(key interface{}, fn IFn) IRef {
	v.watches = v.watches.Assoc(key, fn).(IPersistentMap)
	return v
}

func (v *Var) RemoveWatch(key interface{}) {
	v.watches = v.watches.Without(key)
}

func (v *Var) notifyWatches(oldVal, newVal interface{}) {
	watches := v.watches
	if watches == nil || watches.Count() == 0 {
		return
	}

	for seq := watches.Seq(); seq != nil; seq = seq.Next() {
		entry := seq.First().(IMapEntry)
		key := entry.Key()
		fn := entry.Val().(IFn)
		// Call watch function with key, ref, old-state, new-state
		fn.Invoke(key, v, oldVal, newVal)
	}
}

func (v *Var) Hash() uint32 {
	return hashPtr(uintptr(unsafe.Pointer(v)))
}

func (v *Var) fn() IFn {
	val := v.Deref()
	if _, ok := val.(*UnboundVar); ok {
		panic(fmt.Errorf("cannot call unbound var: %s/%s", v.ns.Name(), v.sym.Name()))
	}
	if val == nil {
		panic(fmt.Errorf("var %s/%s is bound to nil", v.ns.Name(), v.sym.Name()))
	}
	return val.(IFn)
}

func (v *Var) Invoke(args ...interface{}) interface{} {
	if !v.IsBound() {
		panic(fmt.Errorf("cannot call unbound var: %s/%s", v.ns.Name(), v.sym.Name()))
	}
	fn := v.fn()
	if fn == nil {
		panic(fmt.Errorf("var %s/%s is bound to nil", v.ns.Name(), v.sym.Name()))
	}
	return fn.Invoke(args...)
}

func (v *Var) ApplyTo(args ISeq) interface{} {
	if !v.IsBound() {
		panic(fmt.Errorf("cannot call unbound var: %s/%s", v.ns.Name(), v.sym.Name()))
	}
	fn := v.fn()
	if fn == nil {
		panic(fmt.Errorf("var %s/%s is bound to nil", v.ns.Name(), v.sym.Name()))
	}
	return fn.ApplyTo(args)
}

////////////////////////////////////////////////////////////////////////////////
// Dynamic binding

func (s *glStorage) get(v *Var) *Box {
	for i := len(s.bindings) - 1; i >= 0; i-- {
		if b, ok := s.bindings[i][v]; ok {
			return b
		}
	}
	return nil
}

func getGoroutineID() int64 {
	return goid.Get()
}

func PushThreadBindings(bindings IPersistentMap) {
	gid := getGoroutineID()

	glsBindingsMtx.RLock()
	storage, ok := glsBindings[gid]
	glsBindingsMtx.RUnlock()
	if !ok {
		glsBindingsMtx.Lock()
		storage = &glStorage{}
		glsBindings[gid] = storage
		glsBindingsMtx.Unlock()
	}

	store := make(varBindings)
	storage.bindings = append(storage.bindings, store)

	for seq := Seq(bindings); seq != nil; seq = seq.Next() {
		entry := seq.First().(IMapEntry)
		vr := entry.Key().(*Var)
		val := entry.Val()

		if !vr.isDynamic() {
			panic("cannot dynamically bind non-dynamic var: " + vr.String())
		}
		// TODO: validate
		vr.dynamicBound.Store(true)
		store[vr] = &Box{val: val}
	}
}

func PopThreadBindings() {
	gid := getGoroutineID()
	glsBindingsMtx.RLock()
	storage := glsBindings[gid]
	glsBindingsMtx.RUnlock()

	if len(storage.bindings) > 1 {
		storage.bindings = storage.bindings[:len(storage.bindings)-1]
		return
	}

	glsBindingsMtx.Lock()
	delete(glsBindings, gid)
	glsBindingsMtx.Unlock()
}

func GetThreadBindings() IPersistentMap {
	gid := getGoroutineID()
	glsBindingsMtx.RLock()
	storage := glsBindings[gid]
	glsBindingsMtx.RUnlock()

	var ret IPersistentMap = emptyMap
	if storage == nil {
		return ret
	}
	for i := len(storage.bindings) - 1; i >= 0; i-- {
		for v, b := range storage.bindings[i] {
			// most recent binding wins
			if ret.EntryAt(v) == nil {
				ret = ret.Assoc(v, b.val).(IPersistentMap)
			}
		}
	}
	return ret
}

func CloneThreadBindingFrame() any {
	gid := getGoroutineID()
	glsBindingsMtx.RLock()
	defer glsBindingsMtx.RUnlock()
	bindings := glsBindings[gid]
	if bindings == nil {
		return nil
	}
	// DEEP-copy the .bindings slice, not just the *glStorage struct: a
	// shallow `derefBindings := *bindings` shares the underlying array,
	// so a later PushThreadBindings on THIS (the calling) goroutine can
	// append in place (when spare capacity exists) and silently mutate
	// the frame a future/bound-fn already handed to another goroutine —
	// an intermittent cross-goroutine data race (design/08 batch E: hit
	// as clojure-test-suite binding.cljc's future-preserves-bindings
	// cases flaked under `go test -race`-style timing). A fresh slice of
	// exact length has its own backing array, so no append can alias it.
	cp := make([]varBindings, len(bindings.bindings))
	copy(cp, bindings.bindings)
	return &glStorage{bindings: cp}
}

func ResetThreadBindingFrame(frame any) {
	gid := getGoroutineID()
	glsBindingsMtx.Lock()
	defer glsBindingsMtx.Unlock()
	if frame == nil {
		// The goroutine CloneThreadBindingFrame captured from had no
		// bindings at all (e.g. future/bound-fn called with nothing
		// dynamically bound yet) — mirror that as "no storage" rather
		// than panicking on the nil->*glStorage assertion (design/08
		// batch E: AgentSubmit's binding conveyance hits this on the
		// very first future/bound-fn call in a process).
		delete(glsBindings, gid)
		return
	}
	glsBindings[gid] = frame.(*glStorage)
}
