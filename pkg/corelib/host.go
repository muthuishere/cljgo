// host.go — the reflect-backed Go interop substrate (ADR 0010,
// design/05 §1–§2), shared by BOTH execution paths: the interpreter's
// evalHost (pkg/eval) and AOT-emitted code (through pkg/emit/rt) call
// these SAME functions, so `(.Method recv …)` / `(.-Field recv)` /
// `(pkg/Type. {…})` are byte-identical in REPL and binary by
// construction. They live in corelib since ADR 0046 (AOT-core piece 3):
// a compiled binary does interop with no interpreter linked. Only the
// analysis-time half — alias resolution (require-go), which reads
// per-evaluator state — stayed in pkg/eval.
package corelib

import (
	"fmt"
	"io"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// --- reflect registry (seed set) ---------------------------------------

// hostRegistry maps import-path → member → reflect.Value, built once at
// package load. Hand-registered, exactly the M3-v0 seed set of design/05.
var hostRegistry = buildHostRegistry()

func buildHostRegistry() map[string]map[string]reflect.Value {
	return map[string]map[string]reflect.Value{
		"strings": {
			"ToUpper":     reflect.ValueOf(strings.ToUpper),
			"ToLower":     reflect.ValueOf(strings.ToLower),
			"Repeat":      reflect.ValueOf(strings.Repeat),
			"Contains":    reflect.ValueOf(strings.Contains),
			"Split":       reflect.ValueOf(strings.Split),
			"TrimSpace":   reflect.ValueOf(strings.TrimSpace),
			"HasPrefix":   reflect.ValueOf(strings.HasPrefix),
			"NewReplacer": reflect.ValueOf(strings.NewReplacer),
			// NewReader: an in-memory io.Reader/RuneScanner — the natural
			// stream to hand clojure.edn/read (conformance edn-read-stream).
			"NewReader": reflect.ValueOf(strings.NewReader),
		},
		"strconv": {
			"Itoa":       reflect.ValueOf(strconv.Itoa),
			"Atoi":       reflect.ValueOf(strconv.Atoi),
			"ParseFloat": reflect.ValueOf(strconv.ParseFloat),
			"FormatInt":  reflect.ValueOf(strconv.FormatInt),
		},
		"math": {
			"Sqrt": reflect.ValueOf(math.Sqrt),
			"Pow":  reflect.ValueOf(math.Pow),
			"Abs":  reflect.ValueOf(math.Abs),
			"Max":  reflect.ValueOf(math.Max),
			"Min":  reflect.ValueOf(math.Min),
			"Pi":   reflect.ValueOf(math.Pi),
			"E":    reflect.ValueOf(math.E),
		},
		"fmt": {
			"Sprintf": reflect.ValueOf(fmt.Sprintf),
		},
		"net/url": {
			"Parse": reflect.ValueOf(url.Parse),
		},
	}
}

// hostTypeRegistry maps import-path → type-name → reflect.Type, built once
// at package load — the type side of the seed set (ADR 0010, design/05 §1).
// It backs struct constructors (`(url/URL. {...})`) and `(go/new url/URL)`:
// both the interpreter and the AOT-emitted binary reach it through the SAME
// shared MakeGoStruct / NewGoStruct, so reflection resolves the identical
// reflect.Type on both paths and the results are byte-identical.
var hostTypeRegistry = buildHostTypeRegistry()

func buildHostTypeRegistry() map[string]map[string]reflect.Type {
	return map[string]map[string]reflect.Type{
		"net/url": {
			"URL": reflect.TypeOf(url.URL{}),
		},
	}
}

func LookupHostType(pkg, typeName string) (reflect.Type, bool) {
	if m, ok := hostTypeRegistry[pkg]; ok {
		if t, ok := m[typeName]; ok {
			return t, true
		}
	}
	return nil, false
}

func LookupHostMember(pkg, member string) (reflect.Value, bool) {
	if m, ok := hostRegistry[pkg]; ok {
		if rv, ok := m[member]; ok {
			return rv, true
		}
	}
	return reflect.Value{}, false
}

// --- call + shaping -----------------------------------------------------

var errType = reflect.TypeOf((*error)(nil)).Elem()

// CallGoMethod invokes a Go method by name on a receiver via reflection and
// shapes the result exactly as a package fn does (design/05 §1–§2, ADR
// 0010). It is the SINGLE implementation shared by both execution paths: the
// interpreter calls it directly for OpHostMethod, and AOT-emitted code
// reaches it through rt.CallMethod — so `(.Method recv arg...)` is
// byte-identical in REPL and binary by construction (the receiver's static
// type is unknown in v0, so AOT reflects too). Panics on an unknown method,
// a coercion failure, or a thrown (`!`) error — recovered at the IFn/recover
// boundary like every other interop failure.
func CallGoMethod(recv any, method string, throw bool, args []any) any {
	if recv == nil {
		panic(fmt.Errorf("cannot call method .%s on nil", method))
	}
	// A narrow, explicit bridge for ONE cljgo-owned type: #inst values
	// (reader.Inst) stand in for java.util.Date, whose lowercase
	// `.getTime` the clojure-test-suite's epoch-millis helper calls
	// (edn_test/read_string.cljc, :default branch). Go reflection can
	// never resolve a lowercase method name (unexported methods aren't
	// visible via reflect.Value.MethodByName, regardless of package), and
	// design/05-interop-concurrency.md deliberately does NOT auto-
	// capitalize FieldOrMethod for general Go interop — so this does not
	// generalize to arbitrary receivers, only to cljgo's own Inst.
	if inst, ok := recv.(reader.Inst); ok && method == "getTime" && len(args) == 0 {
		return inst.EpochMillis()
	}
	// The same kind of narrow bridge for java.io.Writer's lowercase
	// `.write` (batch A2, printing): a custom print-method /
	// print-dup method's idiomatic body is (.write w (str ...)), and w is a
	// Go io.Writer (os.Stdout, with-out-str's buffer, pr-str's builder).
	// Lowercase names are invisible to reflection, so without this the
	// canonical JVM-portable method body cannot run. Strings and chars
	// only — the two overloads such methods actually use.
	if method == "write" && len(args) == 1 {
		if w, ok := recv.(io.Writer); ok {
			switch s := args[0].(type) {
			case string:
				io.WriteString(w, s)
				return nil
			case lang.Char:
				io.WriteString(w, string(rune(s)))
				return nil
			}
		}
	}
	rv := reflect.ValueOf(recv)
	mv := rv.MethodByName(method)
	if !mv.IsValid() {
		panic(fmt.Errorf("no method %s on %s", method, rv.Type()))
	}
	return CallHostFn("."+method, mv, args, throw)
}

// GoFieldGet reads an exported struct field by name via reflection and
// normalizes the result exactly as a method/fn result (design/05 §1, ADR
// 0010). It is the SINGLE implementation shared by both paths: the
// interpreter calls it for OpHostField, and AOT-emitted code reaches it
// through rt.FieldGet — so `(.-Field recv)` is byte-identical in REPL and
// binary. Pointers are auto-dereferenced (Go's field selection does the
// same). Panics on nil, a non-struct receiver, or an unknown field —
// recovered at the IFn/recover boundary like every other interop failure.
func GoFieldGet(recv any, field string) any {
	if recv == nil {
		panic(fmt.Errorf("cannot read field .-%s on nil", field))
	}
	// deftype / defrecord instances address fields by name, not through Go
	// reflection — `(.-f x)` reads a declared field (ADR: polymorphism v0).
	if v, ok := InstanceField(recv, field); ok {
		return v
	}
	rv := reflect.ValueOf(recv)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			panic(fmt.Errorf("cannot read field .-%s on nil", field))
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		panic(fmt.Errorf("cannot read field .-%s on %s", field, reflect.TypeOf(recv)))
	}
	fv := rv.FieldByName(field)
	if !fv.IsValid() {
		panic(fmt.Errorf("no field %s on %s", field, reflect.TypeOf(recv)))
	}
	return NormalizeResult(fv)
}

// GoFieldSet assigns an exported struct field by name via reflection and
// returns the assigned (normalized) value, matching Clojure's set! (design/05
// §1, ADR 0010). Shared by both paths (interpreter OpSetBang → OpHostField
// target; AOT via rt.FieldSet), so byte-identical. Field assignment needs an
// addressable receiver, so recv MUST be a non-nil pointer to the struct.
// Panics on a value receiver, an unknown/unexported field, or a coercion
// failure.
func GoFieldSet(recv any, field string, val any) any {
	if recv == nil {
		panic(fmt.Errorf("cannot set field .-%s on nil", field))
	}
	rv := reflect.ValueOf(recv)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		panic(fmt.Errorf("cannot set field .-%s: receiver must be a non-nil pointer, got %s", field, reflect.TypeOf(recv)))
	}
	sv := rv.Elem()
	if sv.Kind() != reflect.Struct {
		panic(fmt.Errorf("cannot set field .-%s on %s", field, reflect.TypeOf(recv)))
	}
	fv := sv.FieldByName(field)
	if !fv.IsValid() {
		panic(fmt.Errorf("no field %s on %s", field, sv.Type()))
	}
	if !fv.CanSet() {
		panic(fmt.Errorf("cannot set unexported field %s on %s", field, sv.Type()))
	}
	cv, err := coerceArg(val, fv.Type())
	if err != nil {
		panic(err)
	}
	fv.Set(cv)
	return NormalizeResult(fv)
}

// MakeGoStruct builds a Go struct from a Clojure field map and returns a
// POINTER to it (design/05 §1: `(T. {...})` => `&T{...}`), shared by both
// paths (interpreter OpHostNew; AOT via rt.MakeStruct) so byte-identical.
// v0 populates via reflection — reflect.New + per-field Set from the keyword
// map — deferring direct `&T{...}` emission (which needs go/types field
// typing). fields is a Clojure map (keyword → value) or nil. Panics on an
// unknown type, a non-keyword key, an unknown/unexported field, or a
// coercion failure.
func MakeGoStruct(pkg, typeName string, fields any) any {
	t, ok := LookupHostType(pkg, typeName)
	if !ok {
		panic(fmt.Errorf("unable to resolve Go type: %s.%s", pkg, typeName))
	}
	ptr := reflect.New(t)
	elem := ptr.Elem()
	if fields != nil {
		m, ok := fields.(lang.IPersistentMap)
		if !ok {
			panic(fmt.Errorf("struct constructor %s.%s requires a map of fields", pkg, typeName))
		}
		for s := lang.Seq(m); s != nil; s = s.Next() {
			ent, ok := s.First().(lang.IMapEntry)
			if !ok {
				panic(fmt.Errorf("struct constructor %s.%s: malformed field map", pkg, typeName))
			}
			kw, ok := ent.Key().(lang.Keyword)
			if !ok {
				panic(fmt.Errorf("struct field key must be a keyword, got: %s", lang.PrintString(ent.Key())))
			}
			name := kw.Name()
			fv := elem.FieldByName(name)
			if !fv.IsValid() {
				panic(fmt.Errorf("no field %s on %s", name, t))
			}
			if !fv.CanSet() {
				panic(fmt.Errorf("cannot set unexported field %s on %s", name, t))
			}
			cv, err := coerceArg(ent.Val(), fv.Type())
			if err != nil {
				panic(err)
			}
			fv.Set(cv)
		}
	}
	return ptr.Interface()
}

// NewGoStruct returns a pointer to a zero-valued Go struct (design/05 §1:
// `(go/new T)` => `new(T)`), shared by both paths (interpreter OpHostNew
// Zero; AOT via rt.NewStruct). Panics on an unknown type.
func NewGoStruct(pkg, typeName string) any {
	t, ok := LookupHostType(pkg, typeName)
	if !ok {
		panic(fmt.Errorf("unable to resolve Go type: %s.%s", pkg, typeName))
	}
	return reflect.New(t).Interface()
}

// CallHostFn coerces args, reflect-Calls, and shapes the results. It
// panics (not returns) on a coercion error or a thrown (`!`) error — the
// interpreter's IFn boundary recovers panics into errors, mirroring
// builtins.go.
func CallHostFn(name string, rv reflect.Value, argVals []any, throw bool) any {
	in, err := buildArgs(name, rv.Type(), argVals)
	if err != nil {
		panic(err)
	}
	results := rv.Call(in)
	return shapeResults(name, results, throw)
}

// shapeResults applies the shared shaping table (design/05 §2). THE RULES
// ARE EXACT — the AOT emitter reproduces them byte-for-byte:
//   - 0 results        → nil
//   - trailing error   → plain: only-error returns the error-or-nil
//     directly; otherwise a vector [v… err] with err nil-normalized.
//     Throw: panic a non-nil error, else the value(s) (v or [v…]).
//   - trailing bool (comma-ok, ≥2 results) → plain: [v… ok]; Throw: the
//     value(s) if ok, else panic.
//   - otherwise        → 1 result: normalized; ≥2: a vector [a b …].
func shapeResults(name string, results []reflect.Value, throw bool) any {
	n := len(results)
	if n == 0 {
		return nil
	}
	last := results[n-1]

	if implementsError(last.Type()) {
		vals := results[:n-1]
		errAny := NormalizeResult(last) // nil error → Clojure nil
		if throw {
			if errAny != nil {
				if e, ok := errAny.(error); ok {
					panic(e)
				}
				panic(fmt.Errorf("%v", errAny))
			}
			return valuesToResult(vals)
		}
		if len(vals) == 0 {
			// Only-error result: return the error-or-nil directly, NOT a vector.
			return errAny
		}
		parts := make([]any, 0, len(vals)+1)
		for _, v := range vals {
			parts = append(parts, NormalizeResult(v))
		}
		parts = append(parts, errAny)
		return lang.NewVector(parts...)
	}

	if n >= 2 && last.Kind() == reflect.Bool {
		vals := results[:n-1]
		okv := last.Bool()
		if throw {
			if !okv {
				panic(fmt.Errorf("%s returned false", name))
			}
			return valuesToResult(vals)
		}
		parts := make([]any, 0, len(vals)+1)
		for _, v := range vals {
			parts = append(parts, NormalizeResult(v))
		}
		parts = append(parts, okv)
		return lang.NewVector(parts...)
	}

	if n == 1 {
		return NormalizeResult(results[0])
	}
	parts := make([]any, 0, n)
	for _, v := range results {
		parts = append(parts, NormalizeResult(v))
	}
	return lang.NewVector(parts...)
}

// valuesToResult shapes the non-error/non-ok value portion for the Throw
// path: 0 → nil, 1 → the value, ≥2 → a vector.
func valuesToResult(vals []reflect.Value) any {
	switch len(vals) {
	case 0:
		return nil
	case 1:
		return NormalizeResult(vals[0])
	default:
		parts := make([]any, 0, len(vals))
		for _, v := range vals {
			parts = append(parts, NormalizeResult(v))
		}
		return lang.NewVector(parts...)
	}
}

func implementsError(t reflect.Type) bool {
	return t.Implements(errType)
}

// NormalizeResult applies nil-normalization then number-normalization to a
// single Go result (design/05 §2). Nilable kinds (Ptr/Interface/Map/Slice/
// Chan/Func) that IsNil() become Clojure nil — so a nil error is falsy in
// if/when and a non-nil error stays truthy. Go integer/uint kinds fold to
// int64 and float32/float64 to float64, keeping dual-mode output identical
// (the printer renders 42, not int(42)).
func NormalizeResult(rv reflect.Value) any {
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		if rv.IsNil() {
			return nil
		}
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	}
	return rv.Interface()
}

// --- arg coercion (Clojure → Go), enough for the seed set --------------

func buildArgs(name string, ft reflect.Type, argVals []any) ([]reflect.Value, error) {
	numIn := ft.NumIn()
	variadic := ft.IsVariadic()
	if variadic {
		if len(argVals) < numIn-1 {
			return nil, fmt.Errorf("wrong number of args (%d) passed to: %s", len(argVals), name)
		}
	} else if len(argVals) != numIn {
		return nil, fmt.Errorf("wrong number of args (%d) passed to: %s", len(argVals), name)
	}
	in := make([]reflect.Value, len(argVals))
	for i, av := range argVals {
		var pt reflect.Type
		if variadic && i >= numIn-1 {
			pt = ft.In(numIn - 1).Elem()
		} else {
			pt = ft.In(i)
		}
		cv, err := coerceArg(av, pt)
		if err != nil {
			return nil, fmt.Errorf("%s: arg %d: %w", name, i, err)
		}
		in[i] = cv
	}
	return in, nil
}

func coerceArg(av any, pt reflect.Type) (reflect.Value, error) {
	if av == nil {
		switch pt.Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
			return reflect.Zero(pt), nil
		default:
			return reflect.Value{}, fmt.Errorf("cannot pass nil to Go %s parameter", pt)
		}
	}
	switch pt.Kind() {
	case reflect.String:
		if s, ok := av.(string); ok {
			return reflect.ValueOf(s), nil
		}
	case reflect.Bool:
		if b, ok := av.(bool); ok {
			return reflect.ValueOf(b), nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if i, ok := av.(int64); ok {
			return reflect.ValueOf(i).Convert(pt), nil
		}
	case reflect.Float32, reflect.Float64:
		switch x := av.(type) {
		case float64:
			return reflect.ValueOf(x).Convert(pt), nil
		case int64:
			return reflect.ValueOf(x).Convert(pt), nil
		}
	case reflect.Interface:
		rv := reflect.ValueOf(av)
		if rv.Type().AssignableTo(pt) {
			return rv, nil
		}
		return reflect.Value{}, fmt.Errorf("cannot pass %T to Go %s parameter", av, pt)
	}
	// Guarded same-kind fallback (named types); never cross-kind, which
	// would enable int64→string rune conversions and similar footguns.
	rv := reflect.ValueOf(av)
	if rv.Type().Kind() == pt.Kind() && rv.Type().ConvertibleTo(pt) {
		return rv.Convert(pt), nil
	}
	return reflect.Value{}, fmt.Errorf("cannot coerce %T to Go %s", av, pt)
}
