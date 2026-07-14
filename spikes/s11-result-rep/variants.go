// Package s11 benchmarks Result/Option representations for cljgo
// (ADR 0014, openspec result-option-primitives D1).
//
// Values flow as `any` (ADR 0004 calling convention); every candidate
// therefore exposes constructors returning `any` and combinators shaped
// `func(any, func(any) any) any` — the shape emitted code / the
// interpreter would actually use.
//
// Candidates:
//
//	A — small tagged struct, *pointer* boxed in any: {tag uint8; val any}
//	B — 2-element persistent vector [::ok v] (keyword tag)
//	C — same tagged struct as A but boxed *by value* in any
//	D — two distinct struct types per tag: OkD{v any} / ErrD{v any},
//	    tag carried by the Go type; none is a zero-size singleton type
package s11

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Tag hash masks, one per constructor, so (ok 1), (err 1) and (just 1)
// hash apart while two (ok 1) collide (Clojure hashEq contract).
const (
	okHashMask   = 0x5f2b1a01
	errHashMask  = 0x6c3d2b17
	justHashMask = 0x7a4e3c2d
	noneHashSeed = 0x0b5f6e4d
)

// ---------------------------------------------------------------------------
// Candidate A — tagged struct, pointer-boxed: &TaggedA{tag, val} as any.
// One representation covers Result and Option (4 tags).
// ---------------------------------------------------------------------------

const (
	tagOk uint8 = iota
	tagErr
	tagJust
	tagNone
)

var tagMasks = [4]uint32{okHashMask, errHashMask, justHashMask, noneHashSeed}
var tagNames = [4]string{"#cljgo/ok ", "#cljgo/err ", "#cljgo/just ", "#cljgo/none nil"}

type TaggedA struct {
	tag uint8
	val any
}

// Interned singletons: the zero-payload values every program churns
// through. Constructing (ok nil) or none never allocates.
var (
	NoneA  any = &TaggedA{tag: tagNone}
	okNilA any = &TaggedA{tag: tagOk}
)

func OkA(v any) any {
	if v == nil {
		return okNilA
	}
	return &TaggedA{tag: tagOk, val: v}
}

func ErrA(v any) any { return &TaggedA{tag: tagErr, val: v} }

func IsOkA(r any) bool {
	t, ok := r.(*TaggedA)
	return ok && t.tag == tagOk
}

func IsErrA(r any) bool {
	t, ok := r.(*TaggedA)
	return ok && t.tag == tagErr
}

func UnwrapA(r any) any { return r.(*TaggedA).val }

// AndThenA: railway bind. err/none pass through untouched; ok/just feed
// the unwrapped payload to f (which returns a new Result as any).
func AndThenA(r any, f func(any) any) any {
	t := r.(*TaggedA)
	if t.tag == tagErr || t.tag == tagNone {
		return r
	}
	return f(t.val)
}

// Equiv: Clojure `=`. Only Equiver is implemented (not Equalser) so
// lang.Equiv dispatch lands here and payloads compare with Equiv
// semantics ((= (ok 1) (ok 1N)) follows (= 1 1N)).
func (t *TaggedA) Equiv(o any) bool {
	o2, ok := o.(*TaggedA)
	return ok && t.tag == o2.tag && lang.Equiv(t.val, o2.val)
}

func (t *TaggedA) HashEq() uint32 { return lang.HashEq(t.val) ^ tagMasks[t.tag] }
func (t *TaggedA) Hash() uint32   { return t.HashEq() }

// String prints the D4 tagged-literal form; lang.Print falls back to
// ToString → fmt.Stringer, so pr-str produces #cljgo/ok 5 for free.
func (t *TaggedA) String() string {
	if t.tag == tagNone {
		return tagNames[tagNone]
	}
	return tagNames[t.tag] + lang.PrintString(t.val)
}

var (
	_ lang.Equiver = (*TaggedA)(nil)
	_ lang.IHashEq = (*TaggedA)(nil)
	_ lang.Hasher  = (*TaggedA)(nil)
	_ fmt.Stringer = (*TaggedA)(nil)
)

// ---------------------------------------------------------------------------
// Candidate B — 2-element persistent vector [:cljgo.result/ok v].
// Equiv / HashEq / map-key behavior come from *Vector for free.
// ---------------------------------------------------------------------------

var (
	kwOk   = lang.NewKeyword("cljgo.result/ok")
	kwErr  = lang.NewKeyword("cljgo.result/err")
	kwNone = lang.NewKeyword("cljgo.option/none")

	NoneB any = lang.NewVector(kwNone, nil)
)

func OkB(v any) any  { return lang.NewVector(kwOk, v) }
func ErrB(v any) any { return lang.NewVector(kwErr, v) }

func tagOfB(r any) (lang.Keyword, *lang.Vector, bool) {
	vec, ok := r.(*lang.Vector)
	if !ok || vec.Count() != 2 {
		return lang.Keyword{}, nil, false
	}
	kw, ok := vec.Nth(0).(lang.Keyword)
	return kw, vec, ok
}

func IsOkB(r any) bool {
	kw, _, ok := tagOfB(r)
	return ok && kw == kwOk
}

func IsErrB(r any) bool {
	kw, _, ok := tagOfB(r)
	return ok && kw == kwErr
}

func UnwrapB(r any) any { return r.(*lang.Vector).Nth(1) }

func AndThenB(r any, f func(any) any) any {
	kw, vec, ok := tagOfB(r)
	if !ok {
		panic("not a result")
	}
	if kw == kwErr || kw == kwNone {
		return r
	}
	return f(vec.Nth(1))
}

// ---------------------------------------------------------------------------
// Candidate C — same struct as A, boxed BY VALUE in any.
// ---------------------------------------------------------------------------

type TaggedC struct {
	tag uint8
	val any
}

var NoneC any = TaggedC{tag: tagNone}

func OkC(v any) any  { return TaggedC{tag: tagOk, val: v} }
func ErrC(v any) any { return TaggedC{tag: tagErr, val: v} }

func IsOkC(r any) bool {
	t, ok := r.(TaggedC)
	return ok && t.tag == tagOk
}

func IsErrC(r any) bool {
	t, ok := r.(TaggedC)
	return ok && t.tag == tagErr
}

func UnwrapC(r any) any { return r.(TaggedC).val }

func AndThenC(r any, f func(any) any) any {
	t := r.(TaggedC)
	if t.tag == tagErr || t.tag == tagNone {
		return r
	}
	return f(t.val)
}

func (t TaggedC) Equiv(o any) bool {
	o2, ok := o.(TaggedC)
	return ok && t.tag == o2.tag && lang.Equiv(t.val, o2.val)
}

func (t TaggedC) HashEq() uint32 { return lang.HashEq(t.val) ^ tagMasks[t.tag] }
func (t TaggedC) Hash() uint32   { return t.HashEq() }

func (t TaggedC) String() string {
	if t.tag == tagNone {
		return tagNames[tagNone]
	}
	return tagNames[t.tag] + lang.PrintString(t.val)
}

// ---------------------------------------------------------------------------
// Candidate D — distinct struct type per tag, boxed by value in any.
// The Go type IS the tag: result? / ok? are single type switches.
// noneD is zero-size, so boxing the none singleton never allocates.
// ---------------------------------------------------------------------------

type OkD struct{ v any }
type ErrD struct{ v any }
type JustD struct{ v any }
type noneD struct{}

var NoneD any = noneD{}

func MkOkD(v any) any  { return OkD{v} }
func MkErrD(v any) any { return ErrD{v} }

func IsOkD(r any) bool  { _, ok := r.(OkD); return ok }
func IsErrD(r any) bool { _, ok := r.(ErrD); return ok }

func UnwrapD(r any) any {
	switch t := r.(type) {
	case OkD:
		return t.v
	case JustD:
		return t.v
	}
	panic("unwrap on err/none")
}

func AndThenD(r any, f func(any) any) any {
	switch t := r.(type) {
	case OkD:
		return f(t.v)
	case JustD:
		return f(t.v)
	case ErrD, noneD:
		return r
	}
	panic("not a result")
}

func (t OkD) Equiv(o any) bool {
	o2, ok := o.(OkD)
	return ok && lang.Equiv(t.v, o2.v)
}
func (t OkD) HashEq() uint32 { return lang.HashEq(t.v) ^ okHashMask }
func (t OkD) Hash() uint32   { return t.HashEq() }
func (t OkD) String() string { return "#cljgo/ok " + lang.PrintString(t.v) }

func (t ErrD) Equiv(o any) bool {
	o2, ok := o.(ErrD)
	return ok && lang.Equiv(t.v, o2.v)
}
func (t ErrD) HashEq() uint32 { return lang.HashEq(t.v) ^ errHashMask }
func (t ErrD) Hash() uint32   { return t.HashEq() }
func (t ErrD) String() string { return "#cljgo/err " + lang.PrintString(t.v) }

func (t JustD) Equiv(o any) bool {
	o2, ok := o.(JustD)
	return ok && lang.Equiv(t.v, o2.v)
}
func (t JustD) HashEq() uint32 { return lang.HashEq(t.v) ^ justHashMask }
func (t JustD) Hash() uint32   { return t.HashEq() }
func (t JustD) String() string { return "#cljgo/just " + lang.PrintString(t.v) }

func (noneD) Equiv(o any) bool { _, ok := o.(noneD); return ok }
func (noneD) HashEq() uint32   { return noneHashSeed }
func (noneD) Hash() uint32     { return noneHashSeed }
func (noneD) String() string   { return "#cljgo/none nil" }
