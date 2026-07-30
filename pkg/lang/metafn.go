package lang

import "fmt"

// MetaFn is a function value that carries metadata.
//
// On the JVM every fn extends AFunction, which implements IObj, so
// (with-meta (fn [] 1) {:tag :mock}) returns the fn WITH its metadata —
// verified against Clojure 1.12.5:
//
//	$ clojure -M -e '(let [f (with-meta (fn [] 1) {:tag :mock})] [(f) (meta f)])'
//	[1 {:tag :mock}]
//
// On the Go host a fn value is a bare closure — lang.FnFuncN /
// *lang.NamedFnN in a compiled binary, *eval.evalFn in the interpreter —
// with nowhere to put a map. Before this type, with-meta on a fn threw in
// the interpreter ("value of type *eval.evalFn can't have metadata") and
// SILENTLY dropped the map in a compiled binary (the closures' WithMeta was
// a no-op returning the receiver) — a JVM divergence and a leg divergence
// at once (spike s66, ADR 0105).
//
// So with-meta boxes the closure instead: MetaFn IS the fn (it delegates
// Invoke/ApplyTo) and holds the map beside it. Nothing pays for the box
// unless with-meta is actually called — an unwrapped closure is bit-for-bit
// what it was, so the ADR 0064 direct-call and Apply0..4 fast paths are
// untouched.
type MetaFn struct {
	// Fn is the wrapped function. It is never itself a *MetaFn:
	// re-applying with-meta REPLACES the map rather than nesting.
	Fn   IFn
	meta IPersistentMap
}

var (
	_ IFn  = (*MetaFn)(nil)
	_ IObj = (*MetaFn)(nil)
)

// FnWithMeta returns fn carrying meta, JVM AFunction.withMeta style: a NEW
// value every time (so (identical? f (with-meta f m)) is false, as on the
// JVM), with meta replacing — never nesting — any map fn already carries.
func FnWithMeta(fn IFn, meta IPersistentMap) any {
	if mf, ok := fn.(*MetaFn); ok {
		fn = mf.Fn
	}
	return &MetaFn{Fn: fn, meta: meta}
}

// CanCarryFnMeta reports whether v is a genuine function value — the kind
// the JVM's AFunction/IObj pairing lets carry metadata — as opposed to one
// of the invokable data structures (keyword, symbol, var, collection,
// multimethod), none of which extend AFunction on the JVM and so must keep
// rejecting with-meta here too. Mirrors corelib's fn? (isRealFn).
func CanCarryFnMeta(v any) bool {
	switch v.(type) {
	case Keyword, *Symbol, *Var, IPersistentCollection, *MultiFn:
		return false
	}
	_, ok := v.(IFn)
	return ok
}

func (f *MetaFn) Invoke(args ...any) any { return f.Fn.Invoke(args...) }

func (f *MetaFn) ApplyTo(args ISeq) any { return f.Fn.ApplyTo(args) }

func (f *MetaFn) Meta() IPersistentMap { return f.meta }

func (f *MetaFn) WithMeta(meta IPersistentMap) any { return FnWithMeta(f.Fn, meta) }

// String prints as the wrapped fn does, so metadata never changes how a fn
// reads (the interpreter's *eval.evalFn and the compiled *NamedFnN both
// render "#object[name]").
func (f *MetaFn) String() string {
	if s, ok := f.Fn.(fmt.Stringer); ok {
		return s.String()
	}
	return "#object[fn]"
}
