package eval

import (
	"fmt"
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

// TypeMarker is the value bound to the type-name var a deftype/defrecord
// creates (e.g. `Point`): a handle carrying the fully-qualified type name,
// so `(extend-type Point ...)` and `(instance? Point x)` can recover the
// dispatch key. Built-in types have no marker — their name symbol resolves
// straight to a designator string.
type TypeMarker struct{ name string }

func (t *TypeMarker) String() string { return "#type[" + t.name + "]" }

// dispatchKey is "the type of v" as a protocol dispatch key. deftype /
// defrecord instances use their fully-qualified type name; built-in Go
// values map to stable designator names that extend-type resolves to the
// same string.
func dispatchKey(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
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
func instanceField(recv any, field string) (any, bool) {
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
func (e *Evaluator) internProtocolBuiltins(def func(string, func(...any) any) *lang.Var) {
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

	// (-type-marker qualified-name) -> the value bound to a type's name var.
	defPrivate("-type-marker", func(args ...any) any {
		return &TypeMarker{name: lang.ToString(args[0])}
	})

	// (-qualified-name simple-name) -> "<current-ns>.<simple-name>", the
	// stable type/dispatch name, resolved when the def is evaluated (load
	// time) in the DEFINING namespace.
	defPrivate("-qualified-name", func(args ...any) any {
		return e.CurrentNS().Name().Name() + "." + lang.ToString(args[0])
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
		if v, err := e.resolveVar(sym); err == nil {
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
		p := asProtocol(args[0], "satisfies?")
		return p.satisfies(dispatchKey(args[1]))
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
		p := asProtocol(args[0], "satisfies?")
		return p.satisfies(dispatchKey(args[1]))
	})
}

// seqSlice realizes any seqable into a Go slice.
func seqSlice(v any) []any {
	var out []any
	for s := lang.Seq(v); s != nil; s = s.Next() {
		out = append(out, s.First())
	}
	return out
}
