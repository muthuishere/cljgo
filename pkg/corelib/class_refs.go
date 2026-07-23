package corelib

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
// classRefIsInterface marks which canonical names denote INTERFACES —
// only concrete classes get the flattened Object super (ADR 0039): on
// the JVM every concrete class has Object among its ancestors, while an
// interface never does.
var classRefNames, classRefIsInterface = func() (map[string]string, map[string]bool) {
	m := map[string]string{}
	iface := map[string]bool{}
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
	markIface := func(prefix string, simples ...string) {
		for _, s := range simples {
			iface[prefix+"."+s] = true
		}
	}
	add("java.lang",
		"Object", "String", "Long", "Integer", "Short", "Byte",
		"Double", "Float", "Boolean", "Character", "Number", "Class",
		"Exception", "Throwable", "RuntimeException", "Comparable",
		"CharSequence",
		// The standard typed exceptions (ADR 0039 addendum): resolvable as
		// values so catch clauses and instance? checks name them the way
		// ported JVM code does. Ancestry lives in throwableMatches.
		"ArithmeticException", "ClassCastException",
		"NullPointerException", "IndexOutOfBoundsException",
		"StringIndexOutOfBoundsException", "IllegalArgumentException",
		"NumberFormatException", "IllegalStateException",
		"UnsupportedOperationException")
	markIface("java.lang", "Comparable", "CharSequence")
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
	markIface("clojure.lang",
		"Named", "IFn", "ISeq", "IPending", "IDeref", "IObj", "IMeta",
		"IRecord", "IType", "Sequential", "Associative", "Counted",
		"Indexed", "Seqable", "Reversible", "Sorted",
		"IPersistentCollection", "IPersistentList", "IPersistentMap",
		"IPersistentSet", "IPersistentVector")
	return m, iface
}()

// classRefSupers is the real, flattened ancestry cljgo can vouch for on a
// well-known class ref (ADR 0039): instances of every concrete class ARE
// Objects (exactly the claim classNameMatchesValue already makes for
// instance?), so Object is reported as its sole base/super; Object itself
// and interface names report none. No intermediate JVM superclasses
// (Number, Throwable chains, APersistentSet, ...) are encoded — that
// graph stays un-fabricated per ADR 0036.
func classRefSupers(c *ClassRef) []any {
	if c.name == "java.lang.Object" || classRefIsInterface[c.name] {
		return nil
	}
	return []any{lookupClassRef("java.lang.Object")}
}

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

// InternClassRefVar is classRefVar for an accepted class-name spelling,
// exported for pkg/emit/rt: a compiled binary hoists every reference to
// a cljgo.classes var through it so the var is interned BOUND to the
// same canonical ClassRef the interpreter's resolveVar fallback binds —
// a plain lang.InternVarName there leaves the var unbound and the
// binary diverges from the REPL (the ADR 0002/0007 blocker). Returns
// nil for names outside the ADR 0036 table.
func InternClassRefVar(name string) *lang.Var {
	return classRefVar(lang.NewSymbol(name))
}

// typeClassVar resolves a qualified GENERATED class name for one of OUR
// types (ADR 0039): on the JVM, (defprotocol P) / (defrecord R ...) /
// (deftype T ...) in namespace my.name-space generate classes named
// my.name_space.P (namespace dashes munged to underscores), and suite
// code references them as plain dotted symbols. cljgo resolves such a
// symbol — only after every normal lookup AND the class-ref table missed
// — to the very var the defprotocol/defrecord/deftype interned, whose
// value (*Protocol / *TypeMarker) is the same value the ancestry
// machinery uses. Fail-closed: the dotted prefix must name a LOADED
// namespace (as written, or demunged _→-) and the var must exist there;
// a var bound to anything other than a protocol or type marker does not
// resolve. An interned-but-unbound var is accepted because resolution
// runs at analysis time: a def earlier in the same top-level form is
// interned but not yet evaluated.
func typeClassVar(sym *lang.Symbol) *lang.Var {
	name := sym.Name()
	i := strings.LastIndex(name, ".")
	if i <= 0 || i == len(name)-1 {
		return nil
	}
	nsPart, simple := name[:i], name[i+1:]
	ns := lang.FindNamespace(lang.NewSymbol(nsPart))
	if ns == nil {
		ns = lang.FindNamespace(lang.NewSymbol(strings.ReplaceAll(nsPart, "_", "-")))
	}
	if ns == nil {
		return nil
	}
	v := ns.FindInternedVar(lang.NewSymbol(simple))
	if v == nil {
		return nil
	}
	if !v.IsBound() {
		return v
	}
	switch v.Get().(type) {
	case *Protocol, *TypeMarker:
		return v
	}
	return nil
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
		// Exception-class names (ADR 0039 addendum): an error VALUE is an
		// instance of the JVM exception classes throwableMatches maps it
		// to, ancestry included — (instance? RuntimeException e) is true
		// for a caught arithmetic error, as on the JVM.
		if err, ok := v.(error); ok {
			if matched, known := throwableMatches(simple, err); known {
				return matched
			}
		}
		return dispatchKey(v) == simple
	}
}
