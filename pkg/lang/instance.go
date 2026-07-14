package lang

import (
	"fmt"
	"io"
	"strings"
)

// This file adds the runtime value representations for cljgo's
// polymorphism layer — deftype and defrecord instances (defprotocol lives
// entirely in pkg/eval as a dispatch table, needing no lang value). Both
// representations are plain Go values that flow through the SAME evaluator
// and the SAME AOT-booted runtime (rt.Boot → eval.New), so a deftype /
// defrecord behaves byte-identically interpreted and compiled — the
// registry/shared-dispatch strategy of design/00 §2 (mirrors M3.1 method
// calls). See core/protocols.cljg for the macro surface.
//
//   - *DType is a bare typed value: a type name plus positional fields, no
//     collection behavior. Fields are addressed by name (method bodies bind
//     them as locals; `(.-f x)` reads them). Equality is identity.
//   - *Record is map-backed: it IS an IPersistentMap (get/assoc/keys/vals/=
//     /seq/count all work) but carries a type identity, so two records of
//     different types — or a record and a plain map — are never `=`, and it
//     prints as `#ns.Name{:a 1, :b 2}`.

// DType is a deftype instance: a named tuple of fields with no map
// semantics (design/00 §6 M5, "deftype→struct"). Identity equality.
type DType struct {
	typeName   string
	fieldNames []string
	fields     []any
}

// NewDType builds a deftype instance. len(vals) must equal len(fieldNames).
func NewDType(typeName string, fieldNames []string, vals []any) *DType {
	return &DType{typeName: typeName, fieldNames: fieldNames, fields: vals}
}

// TypeName is the fully-qualified type name (the protocol dispatch key).
func (d *DType) TypeName() string { return d.typeName }

// Field reads a declared field by name; ok is false for an unknown field.
func (d *DType) Field(name string) (any, bool) {
	for i, fn := range d.fieldNames {
		if fn == name {
			return d.fields[i], true
		}
	}
	return nil, false
}

// String prints a deftype instance readably as `#ns.Name[v1 v2]`. Real
// Clojure deftype has no reader form; this is a stable, informative
// rendering (conformance exercises deftype BEHAVIOR, not this literal).
func (d *DType) String() string {
	var b strings.Builder
	b.WriteString("#")
	b.WriteString(d.typeName)
	b.WriteString("[")
	for i, v := range d.fields {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(PrintString(v))
	}
	b.WriteString("]")
	return b.String()
}

// Record is a defrecord instance: an IPersistentMap with a type identity.
// It delegates the whole map protocol to an embedded array/hash map (built
// in declared-field order, so seq/print order is stable) and overrides
// only the methods where record semantics differ from a plain map:
// construction results stay records, and equality/printing are
// type-aware.
type Record struct {
	typeName   string
	fieldNames []string
	m          IPersistentMap
	meta       IPersistentMap
}

// NewRecord builds a record from positional field values (the ->R ctor).
func NewRecord(typeName string, fieldNames []string, vals []any) *Record {
	kvs := make([]any, 0, 2*len(fieldNames))
	for i, fn := range fieldNames {
		kvs = append(kvs, InternKeywordString(fn), vals[i])
	}
	return &Record{typeName: typeName, fieldNames: fieldNames, m: NewMap(kvs...)}
}

// NewRecordFromMap builds a record from a source map (the map->R ctor):
// each declared field takes its value from src (nil when absent), and any
// extra keys in src are carried through as record "extension" entries.
func NewRecordFromMap(typeName string, fieldNames []string, src IPersistentMap) *Record {
	fieldSet := make(map[any]bool, len(fieldNames))
	kvs := make([]any, 0, 2*len(fieldNames))
	for _, fn := range fieldNames {
		k := InternKeywordString(fn)
		fieldSet[k] = true
		kvs = append(kvs, k, Get(src, k))
	}
	m := NewMap(kvs...)
	if src != nil {
		for s := src.Seq(); s != nil; s = s.Next() {
			e := s.First().(IMapEntry)
			if !fieldSet[e.Key()] {
				m = m.Assoc(e.Key(), e.Val()).(IPersistentMap)
			}
		}
	}
	return &Record{typeName: typeName, fieldNames: fieldNames, m: m}
}

// TypeName is the fully-qualified type name (the protocol dispatch key).
func (r *Record) TypeName() string { return r.typeName }

// wrap re-wraps a derived map as a record of the same type.
func (r *Record) wrap(m IPersistentMap) *Record {
	return &Record{typeName: r.typeName, fieldNames: r.fieldNames, m: m, meta: r.meta}
}

func (r *Record) isField(k any) bool {
	for _, fn := range r.fieldNames {
		if Equiv(InternKeywordString(fn), k) {
			return true
		}
	}
	return false
}

// --- ILookup / Associative / IPersistentMap (mostly delegation) ---

func (r *Record) ValAt(k any) any            { return r.m.ValAt(k) }
func (r *Record) ValAtDefault(k, d any) any  { return r.m.ValAtDefault(k, d) }
func (r *Record) ContainsKey(k any) bool     { return r.m.ContainsKey(k) }
func (r *Record) EntryAt(k any) IMapEntry    { return r.m.EntryAt(k) }
func (r *Record) Count() int                 { return r.m.Count() }
func (r *Record) xxx_counted()               {}
func (r *Record) Seq() ISeq                  { return r.m.Seq() }
func (r *Record) Assoc(k, v any) Associative { return r.wrap(r.m.Assoc(k, v).(IPersistentMap)) }
func (r *Record) AssocEx(k, v any) IPersistentMap {
	return r.wrap(r.m.AssocEx(k, v))
}

// Without dissocs a key. Removing a DECLARED field demotes the record to a
// plain map (Clojure semantics: a record must always carry its base
// fields); removing an extension key keeps it a record.
func (r *Record) Without(k any) IPersistentMap {
	if r.isField(k) {
		return r.m.Without(k)
	}
	return r.wrap(r.m.Without(k))
}

func (r *Record) Cons(x any) Conser {
	return r.wrap(r.m.Cons(x).(IPersistentMap))
}

// Empty returns an empty plain map (an "empty record" is ill-defined —
// records require their fields).
func (r *Record) Empty() IPersistentCollection { return r.m.Empty() }

func (r *Record) Meta() IPersistentMap { return r.meta }
func (r *Record) WithMeta(meta IPersistentMap) any {
	return &Record{typeName: r.typeName, fieldNames: r.fieldNames, m: r.m, meta: meta}
}

// Invoke makes a record callable on a key, like a map: (r :k) / (r :k d).
func (r *Record) Invoke(args ...any) any {
	switch len(args) {
	case 1:
		return r.m.ValAt(args[0])
	case 2:
		return r.m.ValAtDefault(args[0], args[1])
	default:
		panic(fmt.Errorf("wrong number of args (%d) passed to record", len(args)))
	}
}

func (r *Record) ApplyTo(args ISeq) any {
	var s []any
	for a := args; a != nil; a = a.Next() {
		s = append(s, a.First())
	}
	return r.Invoke(s...)
}

// Equiv is type-aware `=`: another value equals this record only when it
// is a record of the SAME type with equal contents. A record never equals
// a plain map (the reverse direction is enforced in apersistentmapEquiv).
func (r *Record) Equiv(o any) bool {
	or, ok := o.(*Record)
	if !ok || or.typeName != r.typeName {
		return false
	}
	return r.m.Equiv(or.m)
}

func (r *Record) Equals(o any) bool {
	or, ok := o.(*Record)
	if !ok || or.typeName != r.typeName {
		return false
	}
	return Equals(r.m, or.m)
}

// Hash matches Equiv: type name folded into the map's hash so records of
// different types with equal fields hash apart.
func (r *Record) Hash() uint32 {
	h := Hash(r.typeName)
	if hm, ok := r.m.(interface{ Hash() uint32 }); ok {
		h ^= hm.Hash()
	}
	return h
}

// printRecord renders a record as `#ns.Name{:a 1, :b 2}` (design/00 §6,
// M5). Called from Print (strconv.go) before the generic IPersistentMap
// branch so records don't render as bare maps.
func printRecord(r *Record, w io.Writer) {
	io.WriteString(w, "#")
	io.WriteString(w, r.typeName)
	io.WriteString(w, "{")
	for s := r.m.Seq(); s != nil; s = s.Next() {
		e := s.First().(IMapEntry)
		Print(e.Key(), w)
		io.WriteString(w, " ")
		Print(e.Val(), w)
		if s.Next() != nil {
			io.WriteString(w, ", ")
		}
	}
	io.WriteString(w, "}")
}

// IsRecord reports whether v is a defrecord instance. Used by
// apersistentmapEquiv to keep `(= plain-map record)` false (a record is
// never equal to a plain map, in either direction).
func IsRecord(v any) bool {
	_, ok := v.(*Record)
	return ok
}
