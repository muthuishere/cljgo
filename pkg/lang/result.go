package lang

// Result/Option primitive tagged values (ADR 0014; representation frozen
// by spike S11 — variant D, "type-per-tag"). These are cljgo extensions,
// NOT vendored Glojure code, so no EPL header applies.
//
// Distinct Go types per tag give three properties for free:
//   - predicates are Go type switches (no tag-byte read);
//   - (just nil) and none are DIFFERENT types, so nil-safety is free
//     (a nil payload never collapses into "absence");
//   - equality is "same concrete type + Equiv on the payload".
//
// The types live in the runtime package so BOTH execution modes share
// them: the tree-walk interpreter references them directly, and
// AOT-emitted binaries link pkg/lang. Constructors/predicates are
// registered into clojure.core as Go builtins (pkg/eval), so rt.Boot()
// makes them available in compiled programs too.
type (
	okT   struct{ v any }
	errT  struct{ v any }
	justT struct{ v any }
	noneT struct{}
)

// None is the single shared Option-absence sentinel; the clojure.core
// var `none` is bound to it. There is exactly one none value.
var None any = noneT{}

// NewOk, NewErr, NewJust box a payload into the corresponding tagged
// value. A nil payload is fine and stays distinct from none.
func NewOk(v any) any   { return okT{v} }
func NewErr(v any) any  { return errT{v} }
func NewJust(v any) any { return justT{v} }

// Predicates — Go type switches (S11's key win).
func IsOk(x any) bool   { _, ok := x.(okT); return ok }
func IsErr(x any) bool  { _, ok := x.(errT); return ok }
func IsJust(x any) bool { _, ok := x.(justT); return ok }
func IsNone(x any) bool { _, ok := x.(noneT); return ok }

// IsResult / IsOption classify by tag family.
func IsResult(x any) bool {
	switch x.(type) {
	case okT, errT:
		return true
	}
	return false
}

func IsOption(x any) bool {
	switch x.(type) {
	case justT, noneT:
		return true
	}
	return false
}

// IsResultOption reports whether x is any of the four tagged values.
// Used by Equiv/Equals to route these types before the `a == b` fast
// path, whose struct comparison would panic on an incomparable payload
// (e.g. a raw Go slice inside an okT).
func IsResultOption(x any) bool {
	switch x.(type) {
	case okT, errT, justT, noneT:
		return true
	}
	return false
}

// ResultPayload returns the wrapped value of an ok/err/just; none (and
// any non-tagged value) yields nil. Non-throwing — the combinators use
// it after a predicate guard; `unwrap` adds the throw-on-failure bridge.
func ResultPayload(x any) any {
	switch t := x.(type) {
	case okT:
		return t.v
	case errT:
		return t.v
	case justT:
		return t.v
	}
	return nil
}

// resultOptionEquiv implements Clojure `=` for the tagged values: same
// concrete type, and payload Equiv (none == none, being the singleton
// empty struct). Different tags are never equal.
func resultOptionEquiv(a, b any) bool {
	switch av := a.(type) {
	case okT:
		bv, ok := b.(okT)
		return ok && Equiv(av.v, bv.v)
	case errT:
		bv, ok := b.(errT)
		return ok && Equiv(av.v, bv.v)
	case justT:
		bv, ok := b.(justT)
		return ok && Equiv(av.v, bv.v)
	case noneT:
		_, ok := b.(noneT)
		return ok
	}
	// a is not a tagged value; b must be (this is only reached via the
	// Equiv/Equals guard) — reflect the comparison.
	return resultOptionEquiv(b, a)
}

// Printing: readable tagged literals (design D4). `pr` reaches these via
// ToString's fmt.Stringer branch, so the String methods ARE the print
// path; the payload uses PrintString (readable) so strings are quoted.
func (x okT) String() string   { return "#cljgo/ok " + PrintString(x.v) }
func (x errT) String() string  { return "#cljgo/err " + PrintString(x.v) }
func (x justT) String() string { return "#cljgo/just " + PrintString(x.v) }
func (x noneT) String() string { return "none" }

// HashEq makes the tagged values valid map/set keys with value
// semantics: tag-seeded so the four families never collide, folded with
// the payload's HashEq.
func (x okT) HashEq() uint32   { return 0x0c0000ab ^ HashEq(x.v) }
func (x errT) HashEq() uint32  { return 0x0e110000 ^ HashEq(x.v) }
func (x justT) HashEq() uint32 { return 0x105700ab ^ HashEq(x.v) }
func (x noneT) HashEq() uint32 { return 0x0e0e0e0e }
