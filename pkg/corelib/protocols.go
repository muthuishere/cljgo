package corelib

import (
	"fmt"
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// This file implements cljgo's polymorphism runtime — the dispatch table
// and instance/registry builtins that the core/protocols.cljg macros
// (defprotocol / deftype / defrecord / extend-type / extend-protocol)
// expand onto. The whole layer is macros-over-builtins with NO new AST op:
// both the interpreter and AOT-emitted code call these clojure.core
// builtins, and the compiled binary boots this SAME evaluator via
// rt.Boot() → eval.New(), so a protocol dispatches byte-identically in the
// REPL and in a native binary (design/00 §2; the shared-dispatch strategy
// of M3.1 method calls). The private `-`-prefixed builtins are the macro
// substrate; the public spellings (satisfies?) are user-facing.
//
// A Protocol is a plain mutable value stored in a Var (like an atom): its
// impl table lives inside it, so no global Go state exists — every fresh
// evaluator (each compiled-binary boot) rebuilds it from the loaded forms.
// Dispatch keys are strings: a deftype/defrecord instance carries its
// fully-qualified type name; built-in Go values map to stable designator
// names (String, Long, …) that extend-type/extend-protocol resolve to the
// same string, so extending a protocol to a built-in type works.

// Protocol is a runtime protocol: its name (for error messages), the
// method names it declares, and the impl registry keyed
// typeKey → methodName → fn.
type Protocol struct {
	name    string
	methods []string
	mu      sync.RWMutex
	impls   map[string]map[string]lang.IFn
}

func (p *Protocol) String() string { return "#protocol[" + p.name + "]" }

// typeKeys lists every dispatch type with an impl registered (extenders).
// Registry-map order — unspecified, like the JVM's.
func (p *Protocol) typeKeys() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, 0, len(p.impls))
	for k := range p.impls {
		out = append(out, k)
	}
	return out
}

// implsFor snapshots the method-name -> fn table for one dispatch type
// (find-protocol-impl), nil when the type has no impls.
func (p *Protocol) implsFor(typeKey string) map[string]lang.IFn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	byType, ok := p.impls[typeKey]
	if !ok {
		return nil
	}
	out := make(map[string]lang.IFn, len(byType))
	for m, fn := range byType {
		out[m] = fn
	}
	return out
}

// register installs an impl fn for one method of one dispatch type.
func (p *Protocol) register(typeKey, method string, fn lang.IFn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	byType, ok := p.impls[typeKey]
	if !ok {
		byType = map[string]lang.IFn{}
		p.impls[typeKey] = byType
	}
	byType[method] = fn
}

// lookup finds the impl fn for a method + dispatch type, if any.
func (p *Protocol) lookup(typeKey, method string) (lang.IFn, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if byType, ok := p.impls[typeKey]; ok {
		fn, ok := byType[method]
		return fn, ok
	}
	return nil, false
}

// satisfies reports whether typeKey has ANY impl registered — a value
// satisfies a protocol when its type implements the protocol.
func (p *Protocol) satisfies(typeKey string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.impls[typeKey]
	return ok
}

// declare records that typeKey implements the protocol even when no
// method impl follows — a deftype/defrecord that lists a METHOD-LESS
// protocol still satisfies it (oracle: clojure 1.12.5, (defprotocol P)
// (defrecord R [] P) (satisfies? P (->R)) => true; ADR 0039).
func (p *Protocol) declare(typeKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.impls[typeKey]; !ok {
		p.impls[typeKey] = map[string]lang.IFn{}
	}
}

// TypeMarker is the value bound to the type-name var a deftype/defrecord
// creates (e.g. `Point`): a handle carrying the fully-qualified type name,
// so `(extend-type Point ...)` and `(instance? Point x)` can recover the
// dispatch key. Built-in types have no marker — their name symbol resolves
// straight to a designator string. Since ADR 0039 it also carries the
// type's REAL ancestry inputs: its kind ("record"/"type") and the
// protocol values DECLARED in the defining form (extend-type additions
// deliberately excluded — on the JVM `extend` never alters the class).
type TypeMarker struct {
	name   string
	kind   string // "record" | "type" | "" (unknown)
	supers []any  // declared *Protocol values, declaration order
}

func (t *TypeMarker) String() string { return "#type[" + t.name + "]" }

// ReifyInstance is an anonymous protocol-satisfying value produced by
// `(reify P (m [this] ..) ..)` (ADR 0049). Unlike a deftype/defrecord it has
// NO named type and NO fields — its method bodies are plain `fn` closures that
// capture the enclosing lexical environment, and its impl table is
// PER-INSTANCE (keyed by protocol identity → method → fn), not per-type. So it
// never touches the type-keyed registry: dispatch (-invoke-method) and
// satisfies? check for a *ReifyInstance receiver first and consult its own
// table. The method's first param is `this`, bound to the instance itself.
type ReifyInstance struct {
	protos []*Protocol // declared protocols, declaration order (printing)
	impls  map[*Protocol]map[string]lang.IFn
}

func (r *ReifyInstance) String() string {
	var b strings.Builder
	b.WriteString("#reify[")
	for i, p := range r.protos {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(p.name)
	}
	b.WriteString("]")
	return b.String()
}

// lookup finds this instance's impl fn for a protocol method, if any.
func (r *ReifyInstance) lookup(p *Protocol, method string) (lang.IFn, bool) {
	if byM, ok := r.impls[p]; ok {
		fn, ok := byM[method]
		return fn, ok
	}
	return nil, false
}

// satisfies reports whether this instance was declared with the protocol.
func (r *ReifyInstance) satisfies(p *Protocol) bool {
	_, ok := r.impls[p]
	return ok
}

// interned interface ClassRefs shared by every record/type marker's
// ancestry (ADR 0039). Each entry is real: the named Go interface is
// genuinely implemented by pkg/lang's *Record (instance.go) — see the
// compile-time assertions in protocols_ancestry_test.go — and each is a
// member of the JVM record class's bases/ancestors (oracle 1.12.5).
func classRefsOf(names ...string) []any {
	out := make([]any, 0, len(names))
	for _, n := range names {
		out = append(out, lookupClassRef(n))
	}
	return out
}

// typeBases is the marker's DIRECT supers (JVM `bases`: superclass +
// directly implemented interfaces), typeSupers the transitive set (JVM
// `supers`). Oracle (clojure 1.12.5, 2026-07-17): for (defrecord R [] P),
// (parents R) ∩ our table = #{P Object IPersistentMap IRecord IObj} and
// (ancestors R) adds #{Associative Counted Seqable IMeta
// IPersistentCollection}; for (deftype T [] P) both are #{P IType Object}.
func (t *TypeMarker) typeBases() []any {
	base := append([]any{}, t.supers...)
	switch t.kind {
	case "record":
		return append(base, classRefsOf(
			"java.lang.Object", "clojure.lang.IRecord",
			"clojure.lang.IPersistentMap", "clojure.lang.IObj")...)
	case "type":
		return append(base, classRefsOf(
			"clojure.lang.IType", "java.lang.Object")...)
	}
	return append(base, classRefsOf("java.lang.Object")...)
}

func (t *TypeMarker) typeSupers() []any {
	s := t.typeBases()
	if t.kind == "record" {
		s = append(s, classRefsOf(
			"clojure.lang.Associative", "clojure.lang.Counted",
			"clojure.lang.Seqable", "clojure.lang.IMeta",
			"clojure.lang.IPersistentCollection")...)
	}
	return s
}

// typeBasesOf / typeSupersOf answer "the real ancestry of this class
// value" for the two class kinds cljgo has (ADR 0039): our type markers
// (protocols + genuinely implemented interfaces + Object) and well-known
// class refs (flattened Object for concretes, nothing for interfaces).
// nil for anything else. hierarchies.cljg consumes these via the
// -type-bases/-type-supers builtins.
func typeBasesOf(v any) []any {
	switch x := v.(type) {
	case *TypeMarker:
		return x.typeBases()
	case *ClassRef:
		return classRefSupers(x)
	}
	return nil
}

func typeSupersOf(v any) []any {
	switch x := v.(type) {
	case *TypeMarker:
		return x.typeSupers()
	case *ClassRef:
		return classRefSupers(x)
	}
	return nil
}

// dispatchKey is "the type of v" as a protocol dispatch key. deftype /
// defrecord instances use their fully-qualified type name; built-in Go
// values map to stable designator names that extend-type resolves to the
// same string.
func dispatchKey(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case *ReifyInstance:
		// A reify never uses the type-keyed registry; this synthetic key
		// keeps it out of that map when dispatch falls through to it.
		return "reify"
	case *lang.Record:
		return x.TypeName()
	case *lang.DType:
		return x.TypeName()
	case bool:
		return "Boolean"
	case string:
		return "String"
	case lang.Char:
		return "Character"
	case int64, int, int32, int16, int8:
		return "Long"
	case float64, float32:
		return "Double"
	case lang.Keyword:
		return "Keyword"
	case *lang.Symbol:
		return "Symbol"
	case lang.IPersistentVector:
		return "PersistentVector"
	case lang.IPersistentSet:
		return "PersistentHashSet"
	case lang.IPersistentMap:
		return "PersistentArrayMap"
	case lang.ISeq:
		return "ISeq"
	case lang.IFn:
		return "Fn"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// instanceField reads a named field off a deftype/defrecord instance, for
// GoFieldGet (`.-f`) and the `-field` builtin (method-body field locals).
func instanceField(recv any, field string) (any, bool) { return InstanceField(recv, field) }

// InstanceField reads a deftype/defrecord declared field by name —
// exported for pkg/eval's host dot-form path (GoFieldGet).
func InstanceField(recv any, field string) (any, bool) {
	switch x := recv.(type) {
	case *lang.DType:
		return x.Field(field)
	case *lang.Record:
		k := lang.InternKeywordString(field)
		if x.ContainsKey(k) {
			return x.ValAt(k), true
		}
		return nil, false
	}
	return nil, false
}

func asProtocol(v any, ctx string) *Protocol {
	p, ok := v.(*Protocol)
	if !ok {
		panic(fmt.Errorf("%s: not a protocol: %s", ctx, lang.PrintString(v)))
	}
	return p
}

func stringVals(v lang.IPersistentVector) []string {
	out := make([]string, 0, v.Count())
	for i := 0; i < v.Count(); i++ {
		out = append(out, lang.ToString(v.Nth(i)))
	}
	return out
}

// internProtocolBuiltins registers the polymorphism substrate into
// clojure.core (design/00 §6 M5). The `-`-prefixed helpers are private
// (invisible to unqualified user code); the macros in protocols.cljg emit
// calls to them.
func internProtocolBuiltins(def func(string, func(...any) any) *lang.Var) {
	// These `-`-prefixed builtins are the substrate the protocols.cljg
	// macros expand onto. They are PUBLIC (referred into user like the rest
	// of clojure.core) because macro expansions reference them by
	// unqualified name in the user namespace; the `-` prefix keeps them out
	// of the way of ordinary user code.
	defPrivate := def

	// (-protocol name-string methods-vector) -> a fresh Protocol.
	defPrivate("-protocol", func(args ...any) any {
		name := lang.ToString(args[0])
		methods := stringVals(args[1].(lang.IPersistentVector))
		return &Protocol{name: name, methods: methods, impls: map[string]map[string]lang.IFn{}}
	})

	// (-type-marker qualified-name kind declared-protocols-vector) -> the
	// value bound to a type's name var. kind is "record"/"type"; the
	// declared protocol values become the marker's real ancestry (ADR
	// 0039) and each is told the type implements it — so a METHOD-LESS
	// protocol in a deftype/defrecord still satisfies (JVM-faithful).
	defPrivate("-type-marker", func(args ...any) any {
		m := &TypeMarker{name: lang.ToString(args[0])}
		if len(args) > 1 {
			m.kind = lang.ToString(args[1])
		}
		if len(args) > 2 {
			for _, pv := range seqSlice(args[2]) {
				p, ok := pv.(*Protocol)
				if !ok {
					continue // e.g. a class name in the spec slot — not a protocol
				}
				m.supers = append(m.supers, p)
				p.declare(m.name)
			}
		}
		return m
	})

	// (-type-bases class) / (-type-supers class) -> a set of the class's
	// real direct / transitive supers (ADR 0039), nil when it has none or
	// the value is not a class. The hierarchy fns (hierarchies.cljg) union
	// these with derive-established relationships, mirroring clojure.core's
	// parents/ancestors bases/supers branches.
	defPrivate("-type-bases", func(args ...any) any {
		if s := typeBasesOf(args[0]); s != nil {
			return lang.NewSet(s...)
		}
		return nil
	})
	defPrivate("-type-supers", func(args ...any) any {
		if s := typeSupersOf(args[0]); s != nil {
			return lang.NewSet(s...)
		}
		return nil
	})

	// (-qualified-name simple-name) -> "<current-ns>.<simple-name>", the
	// stable type/dispatch name, resolved when the def is evaluated (load
	// time) in the DEFINING namespace.
	defPrivate("-qualified-name", func(args ...any) any {
		return currentNS().Name().Name() + "." + lang.ToString(args[0])
	})

	// (-extend-key protocol type-key-string method-name-string fn) -> nil.
	defPrivate("-extend-key", func(args ...any) any {
		p := asProtocol(args[0], "extend")
		fn, ok := args[3].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("extend: method impl is not a function: %s", lang.PrintString(args[3])))
		}
		p.register(lang.ToString(args[1]), lang.ToString(args[2]), fn)
		return nil
	})

	// (-type-key type-designator-symbol) -> the dispatch-key string for a
	// type named in extend-type/extend-protocol: a user deftype/defrecord
	// resolves (via its name var → TypeMarker) to its qualified name; a
	// built-in type name (String, Long, …) that resolves to no var is used
	// as-is, matching dispatchKey for built-in values.
	defPrivate("-type-key", func(args ...any) any {
		sym, ok := args[0].(*lang.Symbol)
		if !ok {
			return lang.ToString(args[0])
		}
		if v, err := ResolveVar(sym); err == nil {
			if m, ok := v.Deref().(*TypeMarker); ok {
				return m.name
			}
		}
		return sym.Name()
	})

	// (-type-name value) -> its dispatch key (a user-visible `type`-ish
	// hook; mainly for tests/messages).
	defPrivate("-type-name", func(args ...any) any {
		return dispatchKey(args[0])
	})

	// (-invoke-method protocol method-name-string args-seq): dispatch on
	// the FIRST arg's type and apply the impl, else a Clojure-shaped
	// "No implementation of method" error.
	defPrivate("-invoke-method", func(args ...any) any {
		p := asProtocol(args[0], "dispatch")
		method := lang.ToString(args[1])
		callArgs := seqSlice(args[2])
		if len(callArgs) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to protocol method: %s", method))
		}
		// A reify carries its OWN per-instance impls; its table wins over the
		// type-keyed registry (ADR 0049). A miss falls through to the standard
		// "No implementation" error via the synthetic "reify" dispatch key.
		if ri, ok := callArgs[0].(*ReifyInstance); ok {
			if fn, ok := ri.lookup(p, method); ok {
				return lang.Apply(fn, callArgs)
			}
		}
		key := dispatchKey(callArgs[0])
		fn, ok := p.lookup(key, method)
		if !ok {
			panic(fmt.Errorf("No implementation of method: %s of protocol: %s found for: %s",
				method, p.name, key))
		}
		return lang.Apply(fn, callArgs)
	})

	// (-satisfies? protocol value) -> bool.
	defPrivate("-satisfies?", func(args ...any) any {
		return protocolSatisfiedBy(asProtocol(args[0], "satisfies?"), args[1])
	})

	// (-reify [P Q] P method-name-string fn ...) -> a *ReifyInstance. args[0]
	// is the declared-protocol vector (for satisfies?); the rest are
	// (protocol method-name fn) triples. The fns are `fn` closures that
	// captured the enclosing locals in whichever mode built them (ADR 0049).
	defPrivate("-reify", func(args ...any) any {
		ri := &ReifyInstance{impls: map[*Protocol]map[string]lang.IFn{}}
		for _, pv := range seqSlice(args[0]) {
			if p, ok := pv.(*Protocol); ok {
				ri.protos = append(ri.protos, p)
				if _, seen := ri.impls[p]; !seen {
					ri.impls[p] = map[string]lang.IFn{}
				}
			}
		}
		rest := args[1:]
		for i := 0; i+2 < len(rest); i += 3 {
			p := asProtocol(rest[i], "reify")
			method := lang.ToString(rest[i+1])
			fn, ok := rest[i+2].(lang.IFn)
			if !ok {
				panic(fmt.Errorf("reify: method impl is not a function: %s", lang.PrintString(rest[i+2])))
			}
			byM, seen := ri.impls[p]
			if !seen {
				byM = map[string]lang.IFn{}
				ri.impls[p] = byM
			}
			byM[method] = fn
		}
		return ri
	})

	// (-instance? type-marker value) -> bool: value's type IS the marked
	// type (deftype/defrecord identity). A ClassRef in hand (ADR 0036 —
	// the class name resolved to a value, e.g. via (def c String)) matches
	// through the same designator-name table the instance? macro's
	// literal-symbol fast path uses.
	defPrivate("-instance?", func(args ...any) any {
		switch m := args[0].(type) {
		case *TypeMarker:
			return dispatchKey(args[1]) == m.name
		case *ClassRef:
			return classNameMatchesValue(m.name, args[1])
		}
		return false
	})

	// (-new-type type-name field-names-vector & vals) -> a deftype instance.
	defPrivate("-new-type", func(args ...any) any {
		typeName := lang.ToString(args[0])
		fields := stringVals(args[1].(lang.IPersistentVector))
		return lang.NewDType(typeName, fields, append([]any(nil), args[2:]...))
	})

	// (-new-record type-name field-names-vector & vals) -> a defrecord
	// instance (positional ->R ctor).
	defPrivate("-new-record", func(args ...any) any {
		typeName := lang.ToString(args[0])
		fields := stringVals(args[1].(lang.IPersistentVector))
		return lang.NewRecord(typeName, fields, append([]any(nil), args[2:]...))
	})

	// (-map->record type-name field-names-vector map) -> a defrecord
	// instance (map->R ctor: declared fields from the map, extra keys kept).
	defPrivate("-map->record", func(args ...any) any {
		typeName := lang.ToString(args[0])
		fields := stringVals(args[1].(lang.IPersistentVector))
		var src lang.IPersistentMap
		if args[2] != nil {
			src = args[2].(lang.IPersistentMap)
		}
		return lang.NewRecordFromMap(typeName, fields, src)
	})

	// (-field instance field-name-string) -> the field value (method-body
	// field locals; also the deftype/record field reader).
	defPrivate("-field", func(args ...any) any {
		v, ok := instanceField(args[0], lang.ToString(args[1]))
		if !ok {
			panic(fmt.Errorf("no field %s on %s", lang.ToString(args[1]), dispatchKey(args[0])))
		}
		return v
	})

	// Public: (satisfies? protocol x).
	def("satisfies?", func(args ...any) any {
		return protocolSatisfiedBy(asProtocol(args[0], "satisfies?"), args[1])
	})

	// --- the functional protocol surface (tail wave, 2026-07-23):
	// extend / extends? / extenders / find-protocol-impl /
	// find-protocol-method, over the SAME registry the extend-type /
	// extend-protocol macros feed (-extend-key), so a type extended
	// either way dispatches, satisfies and reports identically.
	// Oracle (JVM 1.12.5, scratch tailwave/o1.clj): (extend TT PP {:pm
	// (fn [x] :pm-tt)}) then (pm (TT.)) => :pm-tt, (extends? PP TT) =>
	// true, (satisfies? PP (TT.)) => true, ((find-protocol-method PP :pm
	// x) x) => :pm-tt, (keys (find-protocol-impl PP x)) => (:pm :pn).

	// (extend atype proto+mmap-pairs...): atype is a class VALUE — a
	// deftype/defrecord's *TypeMarker or a well-known ClassRef (String,
	// Long, ...); each method map is keyword -> fn.
	def("extend", func(args ...any) any {
		if len(args) < 3 || len(args)%2 == 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: extend", len(args)))
		}
		key := classDispatchKey(args[0], "extend")
		for i := 1; i+1 < len(args); i += 2 {
			p := asProtocol(args[i], "extend")
			mmap, ok := args[i+1].(lang.IPersistentMap)
			if !ok {
				panic(fmt.Errorf("extend: method map is not a map: %s", lang.PrintString(args[i+1])))
			}
			for s := lang.Seq(mmap); s != nil; s = s.Next() {
				e := s.First().(lang.IMapEntry)
				k, ok := e.Key().(lang.Keyword)
				if !ok {
					panic(fmt.Errorf("extend: method key is not a keyword: %s", lang.PrintString(e.Key())))
				}
				fn, ok := e.Val().(lang.IFn)
				if !ok {
					panic(fmt.Errorf("extend: method impl is not a function: %s", lang.PrintString(e.Val())))
				}
				p.register(key, k.Name(), fn)
			}
		}
		return nil
	})

	// (extends? protocol atype) -> does the class have any impl registered.
	def("extends?", func(args ...any) any {
		p := asProtocol(args[0], "extends?")
		return p.satisfies(classDispatchKey(args[1], "extends?"))
	})

	// (extenders protocol) -> the extended types. DEVIATION (documented):
	// the JVM returns Class objects; cljgo returns the dispatch-key
	// SYMBOLS ('user.TT, 'String, ...) — the registry has no reverse map
	// from a key back to its marker. Order unspecified, as on the JVM.
	def("extenders", func(args ...any) any {
		p := asProtocol(oneArg("extenders", args), "extenders")
		keys := p.typeKeys()
		if len(keys) == 0 {
			return nil
		}
		out := make([]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, lang.NewSymbol(k))
		}
		return lang.NewList(out...).Seq()
	})

	// (find-protocol-impl protocol x) -> x's method map for the protocol
	// ({:method fn ...}), nil when its type has none. A reify answers
	// from its per-instance table (ADR 0049).
	def("find-protocol-impl", func(args ...any) any {
		p := asProtocol(args[0], "find-protocol-impl")
		x := args[1]
		if ri, ok := x.(*ReifyInstance); ok {
			byM, ok := ri.impls[p]
			if !ok {
				return nil
			}
			return methodMapValue(byM)
		}
		m := p.implsFor(dispatchKey(x))
		if m == nil {
			return nil
		}
		return methodMapValue(m)
	})

	// (find-protocol-method protocol methodk x) -> the impl fn, nil when
	// unimplemented.
	def("find-protocol-method", func(args ...any) any {
		p := asProtocol(args[0], "find-protocol-method")
		k, ok := args[1].(lang.Keyword)
		if !ok {
			panic(fmt.Errorf("find-protocol-method: method key is not a keyword: %s", lang.PrintString(args[1])))
		}
		x := args[2]
		if ri, ok := x.(*ReifyInstance); ok {
			if fn, ok := ri.lookup(p, k.Name()); ok {
				return fn
			}
			return nil
		}
		if fn, ok := p.lookup(dispatchKey(x), k.Name()); ok {
			return fn
		}
		return nil
	})
}

// classDispatchKey is "the dispatch key of this class VALUE" — the
// extend/extends? counterpart of -type-key's symbol resolution: a
// *TypeMarker names itself; a well-known ClassRef maps to its simple
// name, which IS the built-in dispatch key ("java.lang.String" ->
// "String", exactly what dispatchKey yields for a string value).
func classDispatchKey(c any, ctx string) string {
	switch x := c.(type) {
	case *TypeMarker:
		return x.name
	case *ClassRef:
		name := x.Name()
		if i := strings.LastIndex(name, "."); i >= 0 {
			return name[i+1:]
		}
		return name
	}
	panic(fmt.Errorf("%s: not a class: %s", ctx, lang.PrintString(c)))
}

// methodMapValue turns a method-name -> fn table into the persistent map
// find-protocol-impl returns ({:method fn ...}).
func methodMapValue(m map[string]lang.IFn) any {
	kvs := make([]any, 0, 2*len(m))
	for name, fn := range m {
		kvs = append(kvs, lang.InternKeywordString(name), fn)
	}
	return lang.NewMap(kvs...)
}

// protocolSatisfiedBy reports whether v satisfies p — an anonymous reify
// answers from its own declared-protocol set (ADR 0049), everything else from
// the type-keyed registry.
func protocolSatisfiedBy(p *Protocol, v any) bool {
	if ri, ok := v.(*ReifyInstance); ok {
		return ri.satisfies(p)
	}
	return p.satisfies(dispatchKey(v))
}

// seqSlice realizes any seqable into a Go slice.
func seqSlice(v any) []any {
	var out []any
	for s := lang.Seq(v); s != nil; s = s.Next() {
		out = append(out, s.First())
	}
	return out
}
