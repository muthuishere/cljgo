package eval

import (
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// Class refs (ADR 0036): a fixed, fail-closed table of well-known JVM
// class names resolves to interned, opaque ClassRef values when — and
// only when — normal var resolution fails (user definitions always win).
// A ClassRef is NOT a class: it is a named constant with identity
// equality, usable as a value (hierarchy tag, map/set key, def'd). No
// inheritance is fabricated around it (precedence principle): `(parents
// String)` is nil unless explicitly derived. `class?` answers true for
// ClassRefs and deftype/defrecord TypeMarkers; `descendants` throws on
// both, matching the JVM ("Can't get descendants of classes",
// oracle-verified against clojure 1.12.5 — see the ADR's evidence).
//
// Canonicalization: `String` and `java.lang.String` intern to the SAME
// ClassRef, whose printed name is the fully qualified one (so
// `(pr-str String)` => java.lang.String, as on the JVM).

// ClassRef is the interned value a well-known class-name symbol resolves
// to. Identity equality (interned singletons); lang.Hash pointer-hashes it.
type ClassRef struct{ name string }

func (c *ClassRef) String() string { return c.name }

// Name is the canonical fully qualified class name.
func (c *ClassRef) Name() string { return c.name }

// classRefNames maps every accepted spelling (simple or fully qualified)
// to the canonical fully qualified name. Fail-closed: names outside this
// table do not resolve. The vocabulary is ADR 0026's designator table
// (instance?'s name matching) plus Object/Exception/Throwable/Number.
var classRefNames = func() map[string]string {
	m := map[string]string{}
	// add registers both the simple and the fully qualified spelling —
	// java.lang etc. are auto-imported on the JVM, so `String` is idiomatic.
	add := func(prefix string, simples ...string) {
		for _, s := range simples {
			canonical := prefix + "." + s
			m[s] = canonical
			m[canonical] = canonical
		}
	}
	// addQualified registers only the fully qualified spelling —
	// clojure.lang types are never auto-imported, so a bare `Keyword`
	// must not start resolving (conservative, fail-closed).
	addQualified := func(prefix string, simples ...string) {
		for _, s := range simples {
			canonical := prefix + "." + s
			m[canonical] = canonical
		}
	}
	add("java.lang",
		"Object", "String", "Long", "Integer", "Short", "Byte",
		"Double", "Float", "Boolean", "Character", "Number", "Class",
		"Exception", "Throwable", "RuntimeException", "Comparable",
		"CharSequence")
	add("java.math", "BigInteger", "BigDecimal")
	addQualified("java.util", "UUID")
	addQualified("clojure.lang",
		"Keyword", "Symbol", "Named", "Atom", "Delay", "Var",
		"Namespace", "BigInt", "Ratio", "IFn", "ISeq", "IPending",
		"IDeref", "IObj", "IMeta", "IRecord", "IType", "MapEntry",
		"Sequential", "Associative", "Counted", "Indexed", "Seqable",
		"Reversible", "Sorted", "IPersistentCollection",
		"IPersistentList", "IPersistentMap", "IPersistentSet",
		"IPersistentVector", "PersistentArrayMap", "PersistentHashMap",
		"PersistentHashSet", "PersistentList", "PersistentVector",
		"PersistentTreeMap", "PersistentTreeSet", "LazySeq", "Cons",
		"Range", "LongRange", "Repeat", "ExceptionInfo",
		"ArityException", "Volatile")
	return m
}()

var (
	classRefMu       sync.Mutex
	classRefInterned = map[string]*ClassRef{}
)

// lookupClassRef returns the interned ClassRef for an accepted class-name
// spelling, or nil (fail-closed) for anything outside the table.
func lookupClassRef(name string) *ClassRef {
	canonical, ok := classRefNames[name]
	if !ok {
		return nil
	}
	classRefMu.Lock()
	defer classRefMu.Unlock()
	if c := classRefInterned[canonical]; c != nil {
		return c
	}
	c := &ClassRef{name: canonical}
	classRefInterned[canonical] = c
	return c
}

// nsClasses is the namespace the class-ref vars are lazily interned into
// (kept out of clojure.core so ns-publics/refer stay clean).
var nsClassesSym = lang.NewSymbol("cljgo.classes")

// classRefVar is the resolveVar fallback (ADR 0036): called only after
// normal resolution failed, it returns a var (interned in cljgo.classes
// under the symbol AS WRITTEN, bound to the canonical interned ClassRef)
// for accepted class names, or nil.
func classRefVar(sym *lang.Symbol) *lang.Var {
	c := lookupClassRef(sym.Name())
	if c == nil {
		return nil
	}
	ns := lang.FindOrCreateNamespace(nsClassesSym)
	return lang.InternVar(ns, lang.NewSymbol(sym.Name()), c, false)
}

// classNameMatchesValue is ADR 0026's designator-name matching: does the
// value v "look like an instance of" the class named name? A qualified
// name's LAST dotted segment is what the table matches (clojure.lang.Atom
// ~ Atom, java.util.UUID ~ UUID), mirroring how Clojure programmers read
// them. Shared by -instance-of-name? (the instance? macro's literal-symbol
// fast path) and -instance? with a ClassRef in hand (ADR 0036).
func classNameMatchesValue(name string, v any) bool {
	simple := name
	if i := strings.LastIndex(name, "."); i >= 0 {
		simple = name[i+1:]
	}
	switch simple {
	case "Object":
		return v != nil
	case "String":
		_, ok := v.(string)
		return ok
	case "Long", "Integer", "Short", "Byte":
		switch v.(type) {
		case int64, int, int32, int16, int8:
			return true
		}
		return false
	case "Double", "Float":
		switch v.(type) {
		case float64, float32:
			return true
		}
		return false
	case "Character":
		_, ok := v.(lang.Char)
		return ok
	case "Boolean":
		_, ok := v.(bool)
		return ok
	case "Keyword":
		_, ok := v.(lang.Keyword)
		return ok
	case "Symbol":
		_, ok := v.(*lang.Symbol)
		return ok
	case "Atom":
		_, ok := v.(*lang.Atom)
		return ok
	case "Delay":
		_, ok := v.(*lang.Delay)
		return ok
	case "Var":
		_, ok := v.(*lang.Var)
		return ok
	case "Namespace":
		_, ok := v.(*lang.Namespace)
		return ok
	case "BigInt":
		_, ok := v.(*lang.BigInt)
		return ok
	case "BigDecimal", "BigDec":
		_, ok := v.(*lang.BigDecimal)
		return ok
	case "UUID", "Guid":
		_, ok := v.(*reader.UUID)
		return ok
	case "PersistentVector":
		_, ok := v.(lang.IPersistentVector)
		return ok
	case "PersistentArrayMap", "PersistentHashMap":
		_, ok := v.(lang.IPersistentMap)
		return ok
	case "PersistentHashSet":
		_, ok := v.(lang.IPersistentSet)
		return ok
	case "ISeq":
		_, ok := v.(lang.ISeq)
		return ok
	case "IPending":
		_, ok := v.(lang.IPending)
		return ok
	case "IFn":
		_, ok := v.(lang.IFn)
		return ok
	default:
		return dispatchKey(v) == simple
	}
}
