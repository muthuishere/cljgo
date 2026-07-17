package eval

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// evalHost evaluates Go interop nodes (OpHostRef / OpHostCall) in the
// interpreter (ADR 0010, design/05 §1–§2). M3-v0: a reflect-backed seed
// registry (strings/strconv/math/fmt), require-go alias resolution, and
// the shared shaping table ([v err] vectors, `!` throw, nil/number
// normalization). Both consumers — this reflect path and the AOT emitter
// via go/types — MUST apply the identical shaping so behavior is
// dual-mode-identical (design/00 §1.4, the unforgivable divergence).
func (e *Evaluator) evalHost(n *ast.Node, s *Scope) (any, error) {
	switch n.Op {
	case ast.OpHostRef:
		r := n.Sub.(*ast.HostRefNode)
		rv, ok := lookupHostMember(r.Pkg, r.Member)
		if !ok {
			if isThirdPartyGoPath(r.Pkg) {
				return nil, nil // AOT-only member (ADR 0021 B2): compile-time no-op
			}
			return nil, fmt.Errorf("unable to resolve Go member: %s.%s", r.Pkg, r.Member)
		}
		if rv.Kind() == reflect.Func {
			// Fn-as-value: wrap as a native IFn cljgo can apply. Used in
			// value position it shapes with Throw=false (no `!` sugar on a
			// bare ref), so a multi-return fn yields the [v err]/[v ok]
			// vector exactly as a direct call would.
			name := r.Pkg + "." + r.Member
			frv := rv
			return corelib.NewNativeFn(name, func(args ...any) any {
				return callHostFn(name, frv, args, false)
			}), nil
		}
		// Const/var value (e.g. math/Pi): number/nil normalized so the
		// printer renders 3.141592653589793, not float64(...).
		return normalizeResult(rv), nil

	case ast.OpHostCall:
		c := n.Sub.(*ast.HostCallNode)
		rv, ok := lookupHostMember(c.Pkg, c.Member)
		argVals := make([]any, len(c.Args))
		for i, an := range c.Args {
			v, err := e.Eval(an, s)
			if err != nil {
				return nil, err
			}
			argVals[i] = v
		}
		if !ok {
			if isThirdPartyGoPath(c.Pkg) {
				// AOT-only member (ADR 0021 B2): args are evaluated for their
				// side effects, but the unlinked call is a compile-time no-op —
				// the emitted binary makes the real, non-reflective call.
				return nil, nil
			}
			return nil, fmt.Errorf("unable to resolve Go member: %s.%s", c.Pkg, c.Member)
		}
		if rv.Kind() != reflect.Func {
			return nil, fmt.Errorf("Go member is not callable: %s.%s", c.Pkg, c.Member)
		}
		// callHostFn panics on coercion failure or a thrown (`!`) error —
		// recovered into an error at the IFn boundary / top level, matching
		// how builtins.go signals runtime failures.
		return callHostFn(c.Pkg+"."+c.Member, rv, argVals, c.Throw), nil

	case ast.OpHostMethod:
		m := n.Sub.(*ast.HostMethodNode)
		recv, err := e.Eval(m.Recv, s)
		if err != nil {
			return nil, err
		}
		argVals := make([]any, len(m.Args))
		for i, an := range m.Args {
			v, aerr := e.Eval(an, s)
			if aerr != nil {
				return nil, aerr
			}
			argVals[i] = v
		}
		// The receiver's type is only known at runtime (v0), so BOTH modes
		// reflect through CallGoMethod — the AOT emitter reaches the very
		// same function via rt.CallMethod, guaranteeing byte-identity.
		return CallGoMethod(recv, m.Method, m.Throw, argVals), nil

	case ast.OpHostField:
		f := n.Sub.(*ast.HostFieldNode)
		recv, err := e.Eval(f.Recv, s)
		if err != nil {
			return nil, err
		}
		// Reflective in both modes (v0); the AOT emitter reaches the same
		// GoFieldGet via rt.FieldGet.
		return GoFieldGet(recv, f.Field), nil

	case ast.OpHostNew:
		nw := n.Sub.(*ast.HostNewNode)
		if nw.Zero {
			return NewGoStruct(nw.Pkg, nw.Type), nil
		}
		var fields any
		if nw.Fields != nil {
			v, err := e.Eval(nw.Fields, s)
			if err != nil {
				return nil, err
			}
			fields = v
		}
		// Reflective in both modes (v0); the AOT emitter reaches the same
		// MakeGoStruct via rt.MakeStruct.
		return MakeGoStruct(nw.Pkg, nw.Type, fields), nil

	default:
		return nil, fmt.Errorf("evalHost: unexpected op %v", n.Op)
	}
}

// resolveHost is the analyzer's Go-interop hook (ResolveHost). It resolves
// a namespaced symbol whose namespace is a `:require-go` alias in the
// current ns to (import-path, member). The precedence principle (CLAUDE.md)
// is non-negotiable: Clojure is first-class, so a namespace that resolves
// as a Clojure namespace OR a Clojure ns-alias in the current ns wins and
// this returns ok=false. Membership is gated on the seed registry so that
// (a) an unknown member falls through to Clojure's resolution error rather
// than a host miss, and (b) the analyzer's `!`-suffix retry works: for
// `sc/Atoi!` the full name misses the registry (ok=false) and the analyzer
// retries the `!`-stripped `sc/Atoi`, which hits.
func (e *Evaluator) resolveHost(sym *lang.Symbol) (pkg, member string, ok bool) {
	if !sym.HasNamespace() || sym.Namespace() == "" {
		return "", "", false
	}
	nsName := sym.Namespace()
	nsSym := lang.NewSymbol(nsName)
	// Precedence: Clojure alias / namespace always wins.
	if e.CurrentNS().LookupAlias(nsSym) != nil {
		return "", "", false
	}
	if lang.FindNamespace(nsSym) != nil {
		return "", "", false
	}
	aliases := e.hostAliases[e.CurrentNS().Name().Name()]
	path, found := aliases[nsName]
	if !found {
		return "", "", false
	}
	if _, inReg := lookupHostMember(path, sym.Name()); inReg {
		return path, sym.Name(), true
	}
	// Third-party modules (a domain-dotted import path, declared via a
	// build.cljgo `go-require`, ADR 0021 B2) are NOT in the reflect seed
	// registry. Resolve any member as host anyway: the AOT emitter validates
	// and links it from go/packages type facts (zero hand-written bindings),
	// and the interpreter's compile-time pass no-ops the unlinked call
	// (evalHost). A trailing `!` yields to the analyzer's bang-retry.
	if isThirdPartyGoPath(path) && !strings.HasSuffix(sym.Name(), "!") {
		return path, sym.Name(), true
	}
	return "", "", false
}

// isThirdPartyGoPath reports whether an import path is a third-party module
// (its first `/`-segment is a domain, i.e. contains a dot) rather than a Go
// stdlib package (strings, net/url, …). Third-party members resolve through
// go/packages type facts, not the reflect seed registry.
func isThirdPartyGoPath(path string) bool {
	first := path
	if i := strings.IndexByte(path, '/'); i >= 0 {
		first = path[:i]
	}
	return strings.Contains(first, ".")
}

// registerRequireGo backs the `require-go` builtin: it records an
// alias→import-path mapping scoped to the current namespace (ADR 0010,
// design/05 §1). Each libspec is a vector: a path (a symbol — one segment,
// no slash — or a string that may contain slashes), then optional
// `:as alias`. The default alias is the last `/`-segment of the path.
func (e *Evaluator) registerRequireGo(specs []any) {
	for _, spec := range specs {
		path, alias := parseRequireGoSpec(spec)
		nsName := e.CurrentNS().Name().Name()
		if e.hostAliases[nsName] == nil {
			e.hostAliases[nsName] = map[string]string{}
		}
		e.hostAliases[nsName][alias] = path
	}
}

func parseRequireGoSpec(spec any) (path, alias string) {
	// A bare symbol/string libspec (no options).
	switch s := spec.(type) {
	case *lang.Symbol:
		path = hostPathOf(s)
		return path, defaultHostAlias(path)
	case string:
		return s, defaultHostAlias(s)
	}
	vec, ok := spec.(lang.IPersistentVector)
	if !ok || vec.Count() == 0 {
		panic(fmt.Errorf("require-go expects a libspec vector, got: %s", lang.PrintString(spec)))
	}
	switch p := vec.Nth(0).(type) {
	case *lang.Symbol:
		path = hostPathOf(p)
	case string:
		path = p
	default:
		panic(fmt.Errorf("require-go path must be a symbol or string, got: %s", lang.PrintString(vec.Nth(0))))
	}
	alias = defaultHostAlias(path)
	kwAs := lang.NewKeyword("as")
	for i := 1; i < vec.Count(); i++ {
		if lang.Equiv(vec.Nth(i), kwAs) {
			if i+1 >= vec.Count() {
				panic(fmt.Errorf("require-go :as requires an alias, in libspec: %s", lang.PrintString(vec)))
			}
			as, ok := vec.Nth(i + 1).(*lang.Symbol)
			if !ok {
				panic(fmt.Errorf("require-go :as alias must be a symbol, got: %s", lang.PrintString(vec.Nth(i+1))))
			}
			alias = as.Name()
			i++
		}
	}
	return path, alias
}

// hostPathOf reconstructs a path from a symbol libspec. A single-segment
// name (strings, strconv) is the common case; a namespaced symbol is
// reassembled (avoid it — slashed paths belong in a string).
func hostPathOf(s *lang.Symbol) string {
	if s.HasNamespace() && s.Namespace() != "" {
		return s.Namespace() + "/" + s.Name()
	}
	return s.Name()
}

func defaultHostAlias(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

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

func lookupHostType(pkg, typeName string) (reflect.Type, bool) {
	if m, ok := hostTypeRegistry[pkg]; ok {
		if t, ok := m[typeName]; ok {
			return t, true
		}
	}
	return nil, false
}

func lookupHostMember(pkg, member string) (reflect.Value, bool) {
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
	rv := reflect.ValueOf(recv)
	mv := rv.MethodByName(method)
	if !mv.IsValid() {
		panic(fmt.Errorf("no method %s on %s", method, rv.Type()))
	}
	return callHostFn("."+method, mv, args, throw)
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
	if v, ok := corelib.InstanceField(recv, field); ok {
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
	return normalizeResult(fv)
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
	return normalizeResult(fv)
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
	t, ok := lookupHostType(pkg, typeName)
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
	t, ok := lookupHostType(pkg, typeName)
	if !ok {
		panic(fmt.Errorf("unable to resolve Go type: %s.%s", pkg, typeName))
	}
	return reflect.New(t).Interface()
}

// resolveHostType is the analyzer's Go-type-resolution hook
// (ResolveHostType). It mirrors resolveHost's precedence exactly — Clojure
// namespaces/aliases win — but resolves the symbol's name against the type
// registry rather than the member registry (ADR 0010, design/05 §1).
func (e *Evaluator) resolveHostType(sym *lang.Symbol) (pkg, typeName string, ok bool) {
	if !sym.HasNamespace() || sym.Namespace() == "" {
		return "", "", false
	}
	nsName := sym.Namespace()
	nsSym := lang.NewSymbol(nsName)
	if e.CurrentNS().LookupAlias(nsSym) != nil {
		return "", "", false
	}
	if lang.FindNamespace(nsSym) != nil {
		return "", "", false
	}
	aliases := e.hostAliases[e.CurrentNS().Name().Name()]
	path, found := aliases[nsName]
	if !found {
		return "", "", false
	}
	if _, inReg := lookupHostType(path, sym.Name()); !inReg {
		return "", "", false
	}
	return path, sym.Name(), true
}

// callHostFn coerces args, reflect-Calls, and shapes the results. It
// panics (not returns) on a coercion error or a thrown (`!`) error — the
// interpreter's IFn boundary recovers panics into errors, mirroring
// builtins.go.
func callHostFn(name string, rv reflect.Value, argVals []any, throw bool) any {
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
		errAny := normalizeResult(last) // nil error → Clojure nil
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
			parts = append(parts, normalizeResult(v))
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
			parts = append(parts, normalizeResult(v))
		}
		parts = append(parts, okv)
		return lang.NewVector(parts...)
	}

	if n == 1 {
		return normalizeResult(results[0])
	}
	parts := make([]any, 0, n)
	for _, v := range results {
		parts = append(parts, normalizeResult(v))
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
		return normalizeResult(vals[0])
	default:
		parts := make([]any, 0, len(vals))
		for _, v := range vals {
			parts = append(parts, normalizeResult(v))
		}
		return lang.NewVector(parts...)
	}
}

func implementsError(t reflect.Type) bool {
	return t.Implements(errType)
}

// normalizeResult applies nil-normalization then number-normalization to a
// single Go result (design/05 §2). Nilable kinds (Ptr/Interface/Map/Slice/
// Chan/Func) that IsNil() become Clojure nil — so a nil error is falsy in
// if/when and a non-nil error stays truthy. Go integer/uint kinds fold to
// int64 and float32/float64 to float64, keeping dual-mode output identical
// (the printer renders 42, not int(42)).
func normalizeResult(rv reflect.Value) any {
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
